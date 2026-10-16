package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// Config wires a Service.
type Config struct {
	Store *db.GitHubStore
	// DataDir holds the empty gh config dir handed to agents.
	DataDir string
	// BaseURL is this server's own address as an agent on the same machine
	// reaches it (for the MCP endpoint). Required for the write tools.
	BaseURL func() string
	// Enabled reports the `github` feature flag. nil means off.
	Enabled func() bool

	// Test seams; zero values mean github.com.
	APIBase    string
	WebBase    string
	HTTPClient *http.Client
	// RemoteURL builds the git URL tb_push pushes to.
	RemoteURL func(repo string) string
	Now       func() time.Time
}

// Service is the host-side GitHub integration: connection state, token
// minting, the per-session launch options, and the write tools.
type Service struct {
	cfg     Config
	api     *apiClient
	webBase string
	now     func() time.Time

	pending pendingFlows
	reads   tokenCache
	writes  tokenCache

	mu       sync.Mutex
	sessions map[string]*mcpSession // by bearer
	bearers  map[string]string      // session id → bearer
}

// New builds a Service. cfg.Store is required.
func New(cfg Config) *Service {
	s := &Service{cfg: cfg, sessions: map[string]*mcpSession{}, bearers: map[string]string{}}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	base := cfg.APIBase
	if base == "" {
		base = defaultAPIBase
	}
	s.api = &apiClient{base: strings.TrimRight(base, "/"), http: hc}
	s.webBase = strings.TrimRight(cfg.WebBase, "/")
	if s.webBase == "" {
		s.webBase = "https://github.com"
	}
	s.now = cfg.Now
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

func (s *Service) enabled() bool { return s.cfg.Enabled != nil && s.cfg.Enabled() }

// ---------- connection state ----------

// InstallationInfo is one account the App is installed on.
type InstallationInfo struct {
	ID                  int64  `json:"id"`
	Account             string `json:"account"`
	AccountType         string `json:"account_type"`
	RepositorySelection string `json:"repository_selection"`
	// Repositories is the live repository count, -1 when not queried.
	Repositories int `json:"repositories"`
}

// ConnectionStatus is what the settings page shows.
type ConnectionStatus struct {
	Connected     bool               `json:"connected"`
	AppSlug       string             `json:"app_slug,omitempty"`
	AppOwner      string             `json:"app_owner,omitempty"`
	AppURL        string             `json:"app_url,omitempty"`
	InstallURL    string             `json:"install_url,omitempty"`
	Installations []InstallationInfo `json:"installations"`
	HasPAT        bool               `json:"has_pat"`
}

// Status reports the connection. live also asks GitHub for each
// installation's repository count.
func (s *Service) Status(ctx context.Context, live bool) (ConnectionStatus, error) {
	st := ConnectionStatus{Installations: []InstallationInfo{}}
	pat, err := s.cfg.Store.PAT()
	if err != nil {
		return st, err
	}
	st.HasPAT = pat != ""
	app, err := s.cfg.Store.App()
	if err != nil || app == nil {
		return st, err
	}
	st.Connected = true
	st.AppSlug, st.AppOwner = app.Slug, app.Owner
	st.AppURL = s.webBase + "/apps/" + app.Slug
	st.InstallURL = st.AppURL + "/installations/new"
	ins, err := s.cfg.Store.Installations()
	if err != nil {
		return st, err
	}
	for _, in := range ins {
		info := InstallationInfo{ID: in.InstallationID, Account: in.Account, AccountType: in.AccountType, RepositorySelection: in.RepositorySelection, Repositories: -1}
		if live {
			if n, err := s.repositoryCount(ctx, app, in.InstallationID); err == nil {
				info.Repositories = n
			} else {
				logrus.WithError(err).Debug("github: repository count")
			}
		}
		st.Installations = append(st.Installations, info)
	}
	return st, nil
}

func (s *Service) repositoryCount(ctx context.Context, app *db.GitHubAppRecord, installationID int64) (int, error) {
	tok, err := s.installationToken(ctx, app, installationID, "", readPermissions, &s.reads, 5*time.Minute)
	if err != nil {
		return 0, err
	}
	var out struct {
		TotalCount int `json:"total_count"`
	}
	if err := s.api.do(ctx, http.MethodGet, "/installation/repositories?per_page=1", "Bearer "+tok, nil, &out); err != nil {
		return 0, err
	}
	return out.TotalCount, nil
}

// Disconnect forgets the App, its installations and the PAT. The App itself
// stays on GitHub; the user deletes it there.
func (s *Service) Disconnect() error {
	s.reads.clear()
	s.writes.clear()
	if err := s.cfg.Store.DeleteApp(); err != nil {
		return err
	}
	return s.cfg.Store.SetPAT("")
}

// SetPAT stores the fallback token.
func (s *Service) SetPAT(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token is empty")
	}
	return s.cfg.Store.SetPAT(token)
}

// ClearPAT removes the fallback token.
func (s *Service) ClearPAT() error { return s.cfg.Store.SetPAT("") }

// ---------- Manifest flow ----------

// StartConnect begins the Manifest flow for an account type ("user"/"org").
// baseURL is the address the user's browser reaches this server at.
func (s *Service) StartConnect(ownerType, org, baseURL string) (ConnectPlan, error) {
	state := s.pending.add("manifest", baseURL, s.now())
	plan, err := buildPlan(s.webBase, baseURL, ownerType, org, state)
	if err != nil {
		s.pending.take("manifest", state, s.now())
		return ConnectPlan{}, err
	}
	return plan, nil
}

// FinishManifest handles GitHub's redirect after the user confirmed the App:
// it trades the code for the App's credentials, stores them and returns the
// URL of GitHub's install page.
func (s *Service) FinishManifest(ctx context.Context, code, state string) (installURL string, err error) {
	flow, ok := s.pending.take("manifest", state, s.now())
	if !ok {
		return "", errors.New("unknown or expired state; start the connection again")
	}
	if code == "" {
		return "", errors.New("missing code")
	}
	var conv struct {
		ID            int64  `json:"id"`
		Slug          string `json:"slug"`
		ClientID      string `json:"client_id"`
		ClientSecret  string `json:"client_secret"`
		WebhookSecret string `json:"webhook_secret"`
		PEM           string `json:"pem"`
		Owner         struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"owner"`
	}
	if err := s.api.do(ctx, http.MethodPost, "/app-manifests/"+code+"/conversions", "", nil, &conv); err != nil {
		return "", fmt.Errorf("convert manifest: %w", err)
	}
	if err := s.cfg.Store.SaveApp(db.GitHubAppRecord{
		AppID: conv.ID, Slug: conv.Slug, Owner: conv.Owner.Login, OwnerType: conv.Owner.Type,
		ClientID: conv.ClientID, ClientSecret: conv.ClientSecret, WebhookSecret: conv.WebhookSecret, PrivateKey: conv.PEM,
	}); err != nil {
		return "", err
	}
	s.reads.clear()
	s.writes.clear()
	installState := s.pending.add("install", flow.baseURL, s.now())
	return s.webBase + "/apps/" + conv.Slug + "/installations/new?state=" + installState, nil
}

// FinishInstall handles GitHub's redirect after the user installed the App:
// it refreshes the installations and returns the address to send the browser
// back to.
func (s *Service) FinishInstall(ctx context.Context, installationID int64, state string) (returnURL string, err error) {
	flow, ok := s.pending.take("install", state, s.now())
	if !ok {
		return "", errors.New("unknown or expired state; start the connection again")
	}
	if err := s.Sync(ctx); err != nil {
		return flow.baseURL, err
	}
	return flow.baseURL, nil
}

// Sync refreshes the stored installations from GitHub.
func (s *Service) Sync(ctx context.Context) error {
	app, err := s.cfg.Store.App()
	if err != nil {
		return err
	}
	if app == nil {
		return errors.New("no GitHub App connected")
	}
	jwtTok, err := appJWT(app, s.now())
	if err != nil {
		return err
	}
	var list []struct {
		ID                  int64  `json:"id"`
		RepositorySelection string `json:"repository_selection"`
		Account             struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	}
	if err := s.api.do(ctx, http.MethodGet, "/app/installations?per_page=100", "Bearer "+jwtTok, nil, &list); err != nil {
		return fmt.Errorf("list installations: %w", err)
	}
	recs := make([]db.GitHubInstallationRecord, 0, len(list))
	for _, in := range list {
		recs = append(recs, db.GitHubInstallationRecord{
			InstallationID: in.ID, AppID: app.AppID, Account: in.Account.Login,
			AccountType: in.Account.Type, RepositorySelection: in.RepositorySelection,
		})
	}
	return s.cfg.Store.ReplaceInstallations(recs)
}

// ---------- tokens ----------

// installationToken mints (or reuses) a token for an installation, limited to
// repo ("owner/name") when non-empty.
func (s *Service) installationToken(ctx context.Context, app *db.GitHubAppRecord, installationID int64, repo string, perms map[string]string, cache *tokenCache, minRemaining time.Duration) (string, error) {
	key := fmt.Sprintf("%d|%s", installationID, repo)
	if tok, ok := cache.get(key, s.now(), minRemaining); ok {
		return tok, nil
	}
	jwtTok, err := appJWT(app, s.now())
	if err != nil {
		return "", err
	}
	body := map[string]any{"permissions": perms}
	if repo != "" {
		_, name, _ := strings.Cut(repo, "/")
		body["repositories"] = []string{name}
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := s.api.do(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", installationID), "Bearer "+jwtTok, body, &out); err != nil {
		return "", err
	}
	cache.put(key, out.Token, out.ExpiresAt)
	return out.Token, nil
}

// installationFor finds the installation covering repo.
func (s *Service) installationFor(ctx context.Context, app *db.GitHubAppRecord, repo string) (int64, error) {
	jwtTok, err := appJWT(app, s.now())
	if err != nil {
		return 0, err
	}
	var out struct {
		ID int64 `json:"id"`
	}
	err = s.api.do(ctx, http.MethodGet, "/repos/"+repo+"/installation", "Bearer "+jwtTok, nil, &out)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return 0, fmt.Errorf("the GitHub App %q is not installed on %s; add the repository in Settings → GitHub", app.Slug, repo)
	}
	return out.ID, err
}

// ReadToken returns a read-only token for repo, or "" when none applies
// (no App, or PAT mode where the agent's gh gets no token).
func (s *Service) ReadToken(ctx context.Context, repo string) (string, error) {
	app, err := s.cfg.Store.App()
	if err != nil || app == nil {
		return "", err
	}
	id, err := s.installationFor(ctx, app, repo)
	if err != nil {
		return "", err
	}
	// Reuse only with 15 minutes left, so a long-lived agent process is
	// relaunched with a fresh token before the old one dies (design §6).
	return s.installationToken(ctx, app, id, repo, readPermissions, &s.reads, 15*time.Minute)
}

// WriteToken returns the token tb itself uses to write to repo: an
// installation token, or the PAT. It never reaches an agent.
func (s *Service) WriteToken(ctx context.Context, repo string) (string, error) {
	app, err := s.cfg.Store.App()
	if err != nil {
		return "", err
	}
	if app != nil {
		id, err := s.installationFor(ctx, app, repo)
		if err != nil {
			return "", err
		}
		return s.installationToken(ctx, app, id, repo, writePermissions, &s.writes, 5*time.Minute)
	}
	pat, err := s.cfg.Store.PAT()
	if err != nil {
		return "", err
	}
	if pat == "" {
		return "", errors.New("GitHub is not connected; connect it in Settings → GitHub")
	}
	return pat, nil
}

// connected reports whether any write credential exists.
func (s *Service) connected() bool {
	if app, err := s.cfg.Store.App(); err == nil && app != nil {
		return true
	}
	pat, _ := s.cfg.Store.PAT()
	return pat != ""
}
