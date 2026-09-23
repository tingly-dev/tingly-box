package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// ZCode (Z.ai / BigModel GLM Coding Plan) sign-in.
//
// ZCode is not an OAuth 2.0 client: the zcode.z.ai server owns the callback, so
// the desktop client never listens on localhost. It POSTs `oauth/cli/init`,
// opens the returned authorize URL, and polls `oauth/cli/poll/{flow_id}` until
// the server reports the authorization. Because nothing comes back to this
// machine, the same flow works headless and across devices — the authorize URL
// can be opened on a phone.
//
// The token the poll returns is *not* usable against the model endpoints. It is
// an account token for the business API, which is where the coding plan's real
// credential lives: a long-lived `api_key.secret` pair that the plan endpoints
// accept as a plain API key. ResolveCredential performs that exchange, so what
// tingly-box persists as the provider's access token is that static key.
//
// Protocol verified against the ZCode desktop client (3.12.3 / 3.14.0) and
// cross-checked against two independent implementations:
//   - TriDefender/zcode-api (src/auth/oauth.ts, src/auth/resolver.ts)
//   - router-for-me/CLIProxyAPI PR #5993 (internal/auth/zcode)
//
// See .design/zcode-oauth.md.

const (
	// ZCodeAPIBase is the zcode.z.ai control plane that mediates the login.
	// Both account platforms (Z.ai global, BigModel China) use this one host —
	// only the biz host used to resolve the credential differs.
	ZCodeAPIBase = "https://zcode.z.ai/api/v1"

	// ZCodeAppVersion is the desktop client version this flow presents. The
	// authorize interstitial takes it as a query parameter.
	ZCodeAppVersion = "3.14.0"

	// ZCodeLoginTimeout bounds how long the poller waits for the user to
	// authorize in the browser.
	ZCodeLoginTimeout = 5 * time.Minute

	// zcodeMaxPollInterval caps a server-supplied poll interval so a hostile or
	// mistaken value cannot park the polling goroutine indefinitely.
	zcodeMaxPollInterval = 30 * time.Second

	// zcodeAPIKeyName is the name the desktop client gives the API key it
	// provisions. Reusing it means repeated logins adopt the same key instead of
	// littering the account with one key per login.
	zcodeAPIKeyName = "zcode-api-key"
)

// ZCode account platforms. The issuer picks one; there is no runtime toggle.
const (
	ZCodeVariantZai      = "zai"
	ZCodeVariantBigModel = "bigmodel"
)

// ZCodeVariant maps an issuer to its ZCode account platform. The empty string
// means the issuer is not a ZCode issuer.
func ZCodeVariant(issuer ai.Issuer) string {
	switch issuer {
	case ai.IssuerZCode:
		return ZCodeVariantZai
	case ai.IssuerZCodeCN:
		return ZCodeVariantBigModel
	default:
		return ""
	}
}

// IsZCodeIssuer reports whether issuer signs in through the ZCode flow.
func IsZCodeIssuer(issuer ai.Issuer) bool {
	return ZCodeVariant(issuer) != ""
}

// zcodeEnvelope is the {code,data,msg} wrapper every zcode.z.ai endpoint
// returns. code 0 means success; the HTTP status alone is not authoritative.
type zcodeEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Msg  string          `json:"msg"`
}

// ZCodeFlow is an in-progress ZCode login. It carries the bookkeeping the
// poller needs plus the session metadata the terminal provider step consumes.
type ZCodeFlow struct {
	// FlowID identifies the flow server-side; it is the poll path segment.
	FlowID string

	// AuthorizeURL is the URL the user must open, with the desktop
	// interstitial already applied.
	AuthorizeURL string

	// PollToken is the client-generated bearer sent on both init and poll. It
	// is what ties this process to the flow it started.
	PollToken string

	// Issuer is the tingly-box issuer that started the flow.
	Issuer ai.Issuer

	// Variant is the ZCode account platform ("zai" / "bigmodel").
	Variant string

	// ExpiresAt is the server's deadline for the flow (zero when unreported).
	ExpiresAt time.Time

	// PollInterval is the server-requested delay between polls, clamped.
	PollInterval time.Duration

	// UserID, RedirectTo and Name mirror DeviceCodeData: they ride the flow so
	// the terminal step can name the provider and redirect the browser.
	UserID     string
	RedirectTo string
	Name       string
}

// ZCodeTokens is what a completed authorization hands back: an account token
// for the business API, not a credential the model endpoints accept. It is the
// input to ResolveCredential.
type ZCodeTokens struct {
	// AccountToken is the platform access token (data.zai/data.bigmodel).
	AccountToken string

	// JWT is the zcode.z.ai plan token returned alongside it.
	JWT string

	// UserID is the upstream account identifier.
	UserID string
}

// ZCodeCredential is the static coding-plan credential a completed login
// resolves to.
type ZCodeCredential struct {
	// APIKey is the plan's API key id.
	APIKey string

	// Secret is the key's secret half. The plan endpoints want the joined
	// `api_key.secret` form; a key without a secret is rejected upstream.
	Secret string

	// JWT is the zcode.z.ai plan token returned alongside the account token.
	// It is not used by the coding-plan endpoints but is kept so a future
	// start-plan path does not need a fresh login.
	JWT string

	// UserID is the upstream account identifier.
	UserID string
}

// FullKey returns the credential in the form the plan endpoints accept.
func (c *ZCodeCredential) FullKey() string {
	if c == nil {
		return ""
	}
	if c.Secret == "" {
		return c.APIKey
	}
	return c.APIKey + "." + c.Secret
}

// ZCodeClient talks to the zcode.z.ai control plane and the vendor business
// API. Hosts are fields rather than constants so tests can point the whole
// flow at an httptest server.
type ZCodeClient struct {
	// HTTPClient is used for every request. Required.
	HTTPClient *http.Client

	// Variant is the account platform ("zai" / "bigmodel").
	Variant string

	// APIBase is the zcode.z.ai control-plane base (no trailing slash).
	APIBase string

	// BizHost is the origin of the business API that owns the plan's API keys.
	BizHost string

	// AppVersion is presented to the authorize interstitial.
	AppVersion string
}

// NewZCodeClient returns a client for variant with production endpoints.
func NewZCodeClient(httpClient *http.Client, variant string) *ZCodeClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	c := &ZCodeClient{
		HTTPClient: httpClient,
		Variant:    variant,
		APIBase:    ZCodeAPIBase,
		AppVersion: ZCodeAppVersion,
	}
	if variant == ZCodeVariantBigModel {
		c.BizHost = "https://bigmodel.cn"
	} else {
		c.BizHost = "https://api.z.ai"
	}
	return c
}

// Start opens a login flow: POST oauth/cli/init, then apply the desktop
// interstitial to the authorize URL the server hands back.
func (c *ZCodeClient) Start(ctx context.Context) (*ZCodeFlow, error) {
	pollToken, err := zcodePollToken()
	if err != nil {
		return nil, err
	}

	var data struct {
		FlowID          string `json:"flow_id"`
		AuthorizeURL    string `json:"authorize_url"`
		ExpiresAt       int64  `json:"expires_at"`
		PollIntervalSec int    `json:"poll_interval_sec"`
	}
	body := fmt.Sprintf(`{"provider":%q}`, c.Variant)
	if err := c.envelope(ctx, http.MethodPost, c.APIBase+"/oauth/cli/init", strings.NewReader(body), pollToken, &data); err != nil {
		return nil, fmt.Errorf("zcode login init: %w", err)
	}
	if data.FlowID == "" || data.AuthorizeURL == "" {
		return nil, fmt.Errorf("zcode login init: response missing flow_id or authorize_url")
	}

	flow := &ZCodeFlow{
		FlowID:       data.FlowID,
		AuthorizeURL: c.applyInterstitial(data.AuthorizeURL),
		PollToken:    pollToken,
		Variant:      c.Variant,
		PollInterval: clampZCodePollInterval(data.PollIntervalSec),
	}
	// expires_at is optional. A zero value must not become "expired in 1970",
	// which would collapse the poll deadline before the browser even opens.
	if data.ExpiresAt > 0 {
		flow.ExpiresAt = time.Unix(data.ExpiresAt, 0)
	}
	return flow, nil
}

// applyInterstitial appends the zcode.z.ai interstitial the desktop client
// appends to the server-provided authorize URL. The interstitial records the
// authorization server-side — which is what flips the poll to "ready" — before
// bouncing the browser to the client's custom scheme. The parameter name
// differs per platform: redirect_uri for Z.ai, redirect for BigModel.
func (c *ZCodeClient) applyInterstitial(authorizeURL string) string {
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		// A URL we cannot parse is still worth handing to the browser: the
		// server built it, and the interstitial is fidelity, not a requirement.
		return authorizeURL
	}

	origin := strings.TrimSuffix(strings.TrimSuffix(c.APIBase, "/"), "/api/v1")
	interstitial, err := url.Parse(origin + "/app/oauth/login")
	if err != nil {
		return authorizeURL
	}
	q := interstitial.Query()
	q.Set("redirect", "zcode://oauth/callback")
	q.Set("app_version", c.AppVersion)
	interstitial.RawQuery = q.Encode()

	param := "redirect_uri"
	if c.Variant == ZCodeVariantBigModel {
		param = "redirect"
	}
	values := parsed.Query()
	values.Set(param, interstitial.String())
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

// Poll waits for the user to authorize, then returns the account token.
//
// Retry semantics mirror the desktop client: a 4xx other than 408/429 and an
// envelope error are fatal, while a transport error or 5xx is treated as one
// more "pending" round — a flaky network must not throw away a flow the user is
// in the middle of completing.
func (c *ZCodeClient) Poll(ctx context.Context, flow *ZCodeFlow, timeout time.Duration) (*ZCodeTokens, error) {
	if flow == nil || flow.FlowID == "" {
		return nil, fmt.Errorf("zcode login poll: flow not started")
	}
	deadline := time.Now().Add(timeout)
	if !flow.ExpiresAt.IsZero() && flow.ExpiresAt.Before(deadline) {
		deadline = flow.ExpiresAt
	}
	interval := clampZCodePollInterval(int(flow.PollInterval / time.Second))
	pollURL := fmt.Sprintf("%s/oauth/cli/poll/%s", c.APIBase, url.PathEscape(flow.FlowID))

	for {
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("zcode login poll: authorization timed out, please retry the login")
		}

		var data struct {
			Status string `json:"status"`
			Token  string `json:"token"`
			User   struct {
				UserID string `json:"user_id"`
			} `json:"user"`
			Zai struct {
				AccessToken string `json:"access_token"`
			} `json:"zai"`
			BigModel struct {
				AccessToken string `json:"access_token"`
			} `json:"bigmodel"`
		}

		err := c.envelope(ctx, http.MethodGet, pollURL, nil, flow.PollToken, &data)
		switch {
		case err == nil:
			// handled below
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case isRetryableZCodePollError(err):
			if waitErr := sleepCtx(ctx, interval); waitErr != nil {
				return nil, waitErr
			}
			continue
		default:
			return nil, fmt.Errorf("zcode login poll: %w", err)
		}

		switch data.Status {
		case "ready":
			accessToken := strings.TrimSpace(data.Zai.AccessToken)
			if flow.Variant == ZCodeVariantBigModel {
				accessToken = strings.TrimSpace(data.BigModel.AccessToken)
			}
			// Reading the wrong platform's field would persist a credential
			// pointed at the other vendor, so an empty one is an error rather
			// than a fallback to whichever field is populated.
			if accessToken == "" {
				return nil, fmt.Errorf("zcode login poll: authorized response carries no %s access_token", flow.Variant)
			}
			return &ZCodeTokens{
				AccountToken: accessToken,
				JWT:          strings.TrimSpace(data.Token),
				UserID:       strings.TrimSpace(data.User.UserID),
			}, nil
		case "failed":
			return nil, fmt.Errorf("zcode login poll: authorization failed, please retry the login")
		case "pending":
			if waitErr := sleepCtx(ctx, interval); waitErr != nil {
				return nil, waitErr
			}
		default:
			return nil, fmt.Errorf("zcode login poll: unexpected status %q", data.Status)
		}
	}
}

// ResolveCredential exchanges the account token for the plan's static API key.
//
// Z.ai first trades the account token for a business-API token; BigModel sends
// the account token to its business API directly. From there both walk the same
// path: default organization → default project → the named API key (created on
// first login) → the key's secret half.
func (c *ZCodeClient) ResolveCredential(ctx context.Context, accountToken string) (*ZCodeCredential, error) {
	accountToken = strings.TrimSpace(accountToken)
	if accountToken == "" {
		return nil, fmt.Errorf("zcode credential: empty account token")
	}

	authorization := accountToken
	if c.Variant != ZCodeVariantBigModel {
		bizToken, err := c.resolveBizToken(ctx, accountToken)
		if err != nil {
			return nil, err
		}
		authorization = "Bearer " + bizToken
	}

	orgID, projectID, err := c.resolveDefaultProject(ctx, authorization)
	if err != nil {
		return nil, err
	}
	apiKey, err := c.findOrCreateAPIKey(ctx, authorization, orgID, projectID)
	if err != nil {
		return nil, err
	}
	secret, err := c.copyAPIKeySecret(ctx, authorization, orgID, projectID, apiKey)
	if c.Variant == ZCodeVariantBigModel {
		// BigModel: the desktop client tolerates a key whose secret cannot be
		// read and sends the bare key. Mirror that — an account the copy
		// endpoint refuses should still complete the login, and a bad key
		// shows up immediately in the model list / quota fetch rather than
		// blocking the sign-in.
		if err != nil {
			secret = ""
		}
		return &ZCodeCredential{APIKey: apiKey, Secret: secret}, nil
	}
	if err != nil {
		return nil, err
	}
	// Z.ai rejects the bare key (the desktop client requires the secret here
	// too), so a missing secret is a login that would only fail later as an
	// opaque 401.
	if secret == "" {
		return nil, fmt.Errorf("zcode credential: API key has no secret; the plan endpoints would reject it")
	}
	return &ZCodeCredential{APIKey: apiKey, Secret: secret}, nil
}

// resolveBizToken trades a Z.ai account token for a business-API token. The
// endpoint is unauthenticated and takes the account token in the body.
func (c *ZCodeClient) resolveBizToken(ctx context.Context, accountToken string) (string, error) {
	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.bizJSON(ctx, http.MethodPost, c.BizHost+"/api/auth/z/login", "",
		map[string]string{"token": accountToken}, &data); err != nil {
		return "", fmt.Errorf("zcode credential: z/login: %w", err)
	}
	if data.AccessToken == "" {
		return "", fmt.Errorf("zcode credential: z/login returned no access_token")
	}
	return data.AccessToken, nil
}

// resolveDefaultProject picks the organization and project the API key belongs
// to, preferring the account's default pair the way the desktop client does and
// falling back to the first of each.
func (c *ZCodeClient) resolveDefaultProject(ctx context.Context, authorization string) (orgID, projectID string, err error) {
	type project struct {
		ProjectID   string `json:"projectId"`
		ID          string `json:"id"`
		ProjectName string `json:"projectName"`
		Name        string `json:"name"`
	}
	type organization struct {
		OrganizationID   string    `json:"organizationId"`
		OrgID            string    `json:"orgId"`
		ID               string    `json:"id"`
		OrganizationName string    `json:"organizationName"`
		Name             string    `json:"name"`
		Projects         []project `json:"projects"`
	}
	var data struct {
		Organizations []organization `json:"organizations"`
		Orgs          []organization `json:"orgs"`
	}
	if err := c.bizJSON(ctx, http.MethodGet, c.BizHost+"/api/biz/customer/getCustomerInfo",
		authorization, nil, &data); err != nil {
		return "", "", fmt.Errorf("zcode credential: getCustomerInfo: %w", err)
	}

	orgs := data.Organizations
	if len(orgs) == 0 {
		orgs = data.Orgs
	}
	if len(orgs) == 0 {
		return "", "", fmt.Errorf("zcode credential: account has no organizations")
	}
	org := orgs[0]
	for _, candidate := range orgs {
		if isZCodeDefaultName(candidate.OrganizationName, candidate.Name) {
			org = candidate
			break
		}
	}
	if len(org.Projects) == 0 {
		return "", "", fmt.Errorf("zcode credential: organization has no projects")
	}
	proj := org.Projects[0]
	for _, candidate := range org.Projects {
		if isZCodeDefaultName(candidate.ProjectName, candidate.Name) {
			proj = candidate
			break
		}
	}

	orgID = firstNonEmpty(org.OrganizationID, org.OrgID, org.ID)
	projectID = firstNonEmpty(proj.ProjectID, proj.ID)
	if orgID == "" || projectID == "" {
		return "", "", fmt.Errorf("zcode credential: getCustomerInfo returned no organization/project id")
	}
	return orgID, projectID, nil
}

// findOrCreateAPIKey returns the plan's API key, creating it on first login.
// A listing failure is not fatal: the create path covers the account that has
// no key yet, which is the same outcome.
func (c *ZCodeClient) findOrCreateAPIKey(ctx context.Context, authorization, orgID, projectID string) (string, error) {
	listURL := fmt.Sprintf("%s/api/biz/v1/organization/%s/projects/%s/api_keys",
		c.BizHost, url.PathEscape(orgID), url.PathEscape(projectID))

	var existing []struct {
		Name   string `json:"name"`
		APIKey string `json:"apiKey"`
	}
	if err := c.bizJSON(ctx, http.MethodGet, listURL, authorization, nil, &existing); err == nil {
		for _, key := range existing {
			if key.Name == zcodeAPIKeyName && key.APIKey != "" {
				return key.APIKey, nil
			}
		}
	}

	var created struct {
		APIKey string `json:"apiKey"`
	}
	if err := c.bizJSON(ctx, http.MethodPost, listURL, authorization,
		map[string]string{"name": zcodeAPIKeyName}, &created); err != nil {
		return "", fmt.Errorf("zcode credential: create api key: %w", err)
	}
	// A renamed or nested field upstream must fail the login loudly instead of
	// persisting an empty credential that 401s on every request.
	if created.APIKey == "" {
		return "", fmt.Errorf("zcode credential: api key creation returned no apiKey")
	}
	return created.APIKey, nil
}

// copyAPIKeySecret reads the secret half of an API key.
func (c *ZCodeClient) copyAPIKeySecret(ctx context.Context, authorization, orgID, projectID, apiKey string) (string, error) {
	copyURL := fmt.Sprintf("%s/api/biz/v1/organization/%s/projects/%s/api_keys/copy/%s",
		c.BizHost, url.PathEscape(orgID), url.PathEscape(projectID), url.PathEscape(apiKey))

	var data struct {
		SecretKey string `json:"secretKey"`
		Secret    string `json:"secret_key"`
	}
	if err := c.bizJSON(ctx, http.MethodGet, copyURL, authorization, nil, &data); err != nil {
		return "", fmt.Errorf("zcode credential: copy api key: %w", err)
	}
	return firstNonEmpty(data.SecretKey, data.Secret), nil
}

// envelope sends a zcode.z.ai request and unwraps its {code,data,msg} body.
// Errors carry the server message and status, both of which reach the login
// dialog — "status=403 msg=flow expired" is diagnosable, "login failed" is not.
func (c *ZCodeClient) envelope(ctx context.Context, method, endpoint string, body io.Reader, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &zcodeTransientError{err: err}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return &zcodeTransientError{err: err}
	}

	// Retry classification goes by status first: a 4xx is the flow being
	// rejected (expired, unknown flow id, bad bearer) whatever the body looks
	// like — retrying it would hang the login until the deadline instead of
	// reporting why. 408/429 and 5xx are blips; so is a body that is not the
	// envelope at all (an edge's HTML error page), which is worth one more
	// round rather than a failed login.
	retryableStatus := resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests
	var env zcodeEnvelope
	if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
		err := fmt.Errorf("invalid response envelope (status=%d)", resp.StatusCode)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && !retryableStatus {
			return err
		}
		return &zcodeTransientError{err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || env.Code != 0 {
		err := fmt.Errorf("status=%d code=%d msg=%s", resp.StatusCode, env.Code, zcodeMsgOrNone(env.Msg))
		if retryableStatus {
			return &zcodeTransientError{err: err}
		}
		return err
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}

// bizJSON sends a business-API request. authorization is the complete header
// value, because Z.ai wants `Bearer <biz token>` while BigModel passes the
// account token bare. Responses come either wrapped in {data:…} or as the
// object itself, so both shapes are accepted.
func (c *ZCodeClient) bizJSON(ctx context.Context, method, endpoint, authorization string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(raw))
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var wrap struct {
		Code json.RawMessage `json:"code"`
		Data json.RawMessage `json:"data"`
		Msg  string          `json:"msg"`
	}
	_ = json.Unmarshal(raw, &wrap)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if wrap.Msg != "" {
			return fmt.Errorf("status=%d msg=%s", resp.StatusCode, wrap.Msg)
		}
		return fmt.Errorf("status=%d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if len(wrap.Data) > 0 {
		if err := json.Unmarshal(wrap.Data, out); err == nil {
			return nil
		}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// zcodeTransientError marks a poll failure worth retrying.
type zcodeTransientError struct{ err error }

func (e *zcodeTransientError) Error() string { return e.err.Error() }
func (e *zcodeTransientError) Unwrap() error { return e.err }

func isRetryableZCodePollError(err error) bool {
	var transient *zcodeTransientError
	return errors.As(err, &transient)
}

// zcodePollToken mints the bearer that ties this process to the flow.
func zcodePollToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("zcode login: generate poll token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func clampZCodePollInterval(seconds int) time.Duration {
	interval := time.Duration(seconds) * time.Second
	if interval < time.Second {
		return time.Second
	}
	if interval > zcodeMaxPollInterval {
		return zcodeMaxPollInterval
	}
	return interval
}

// isZCodeDefaultName reports whether any of names marks the account's default
// organization/project. The API answers in the account's locale, so both the
// Chinese marker and the English word are accepted.
func isZCodeDefaultName(names ...string) bool {
	for _, name := range names {
		if name == "" {
			continue
		}
		if strings.Contains(name, "默认") || strings.Contains(strings.ToLower(name), "default") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func zcodeMsgOrNone(msg string) string {
	if msg == "" {
		return "(none)"
	}
	return msg
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// =============================================
// Manager entry points
// =============================================

// ZCodeTokenMetadataKeys are the OAuth token metadata keys a ZCode login sets.
// They land in OAuthDetail.ExtraFields, where the refresh path reads them back.
const (
	// ZCodeMetaPlatform records which account platform issued the credential.
	ZCodeMetaPlatform = "zcode_platform"

	// ZCodeMetaAPIKey is the plan API key id without its secret. It is kept so
	// a re-resolve can tell "same key, new secret" from "a different key", and
	// so the UI can show which key the plan is using without printing the
	// secret half.
	ZCodeMetaAPIKey = "zcode_api_key"

	// ZCodeMetaUserID is the upstream account id.
	ZCodeMetaUserID = "zcode_user_id"

	// ZCodeMetaPlanJWT is the zcode.z.ai plan token.
	ZCodeMetaPlanJWT = "zcode_plan_jwt"
)

// zcodeClient builds a ZCode client on the options' HTTP client (so a
// per-session proxy applies), honoring test host overrides.
func (m *Manager) zcodeClient(options *Options, variant string) *ZCodeClient {
	client := NewZCodeClient(m.getHTTPClient(options), variant)
	if options.ZCodeAPIBase != "" {
		client.APIBase = strings.TrimSuffix(options.ZCodeAPIBase, "/")
	}
	if options.ZCodeBizHost != "" {
		client.BizHost = strings.TrimSuffix(options.ZCodeBizHost, "/")
	}
	return client
}

// InitiateZCodeFlow starts a ZCode login for issuer and returns the flow whose
// AuthorizeURL the user must open. Nothing is persisted yet: the caller polls
// with CompleteZCodeFlow, which is the step that produces a credential.
func (m *Manager) InitiateZCodeFlow(ctx context.Context, userID string, issuer ai.Issuer, redirectTo, name string, opts ...Option) (*ZCodeFlow, error) {
	variant := ZCodeVariant(issuer)
	if variant == "" {
		return nil, fmt.Errorf("%w: %s is not a ZCode issuer", ErrInvalidProvider, issuer)
	}

	options := applyOptions(opts...)
	client := m.zcodeClient(options, variant)

	flow, err := client.Start(ctx)
	if err != nil {
		return nil, err
	}
	flow.Issuer = issuer
	flow.UserID = userID
	flow.RedirectTo = redirectTo
	flow.Name = name
	return flow, nil
}

// CompleteZCodeFlow waits for the authorization, exchanges the account token for
// the plan's static credential, and returns it shaped as a *Token so the shared
// provider-creation path needs no ZCode-specific branch.
//
// AccessToken is the `api_key.secret` the plan endpoints accept. RefreshToken is
// the account token: it cannot mint access tokens (ZCode has no refresh grant),
// but it is what re-resolves the credential without sending the user back
// through the browser, which is what a manual refresh does.
func (m *Manager) CompleteZCodeFlow(ctx context.Context, flow *ZCodeFlow, opts ...Option) (*Token, error) {
	if flow == nil {
		return nil, fmt.Errorf("zcode login: flow not started")
	}
	options := applyOptions(opts...)
	client := m.zcodeClient(options, flow.Variant)

	tokens, err := client.Poll(ctx, flow, ZCodeLoginTimeout)
	if err != nil {
		return nil, err
	}

	cred, err := client.ResolveCredential(ctx, tokens.AccountToken)
	if err != nil {
		return nil, err
	}
	cred.JWT = tokens.JWT
	cred.UserID = tokens.UserID

	return &Token{
		AccessToken:  cred.FullKey(),
		RefreshToken: tokens.AccountToken,
		TokenType:    "Bearer",
		// No Expiry: the plan credential is static. Leaving it zero keeps the
		// background refresher out of the way, which is correct — there is
		// nothing for it to refresh.
		Issuer:     flow.Issuer,
		RedirectTo: flow.RedirectTo,
		Name:       flow.Name,
		Metadata: map[string]any{
			ZCodeMetaPlatform: flow.Variant,
			ZCodeMetaAPIKey:   cred.APIKey,
			ZCodeMetaUserID:   cred.UserID,
			ZCodeMetaPlanJWT:  cred.JWT,
		},
	}, nil
}

// ReResolveZCodeCredential re-runs the credential exchange from a stored account
// token. This is what "refresh" means for ZCode: the plan credential can be
// rotated or revoked upstream, and re-resolving picks up the current one without
// an interactive login. It fails when the account token itself has expired — the
// caller then offers re-authentication.
func (m *Manager) ReResolveZCodeCredential(ctx context.Context, issuer ai.Issuer, accountToken string, opts ...Option) (*Token, error) {
	variant := ZCodeVariant(issuer)
	if variant == "" {
		return nil, fmt.Errorf("%w: %s is not a ZCode issuer", ErrInvalidProvider, issuer)
	}
	if strings.TrimSpace(accountToken) == "" {
		return nil, fmt.Errorf("zcode refresh: no stored account token; re-authentication required")
	}

	options := applyOptions(opts...)
	client := m.zcodeClient(options, variant)

	cred, err := client.ResolveCredential(ctx, accountToken)
	if err != nil {
		return nil, err
	}

	return &Token{
		AccessToken:  cred.FullKey(),
		RefreshToken: accountToken,
		TokenType:    "Bearer",
		Issuer:       issuer,
		Metadata: map[string]any{
			ZCodeMetaPlatform: variant,
			ZCodeMetaAPIKey:   cred.APIKey,
		},
	}, nil
}
