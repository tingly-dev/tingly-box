package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
)

// WorkBuddy (Tencent WorkBuddy / CodeBuddy) sign-in.
//
// Like ZCode this is a server-mediated login, not OAuth 2.0: the client asks
// `/v2/plugin/auth/state` for a login state and URL, the user signs in on that
// URL in any browser, and the client polls `/v2/plugin/auth/token` until the
// server hands over the tokens. Nothing returns to localhost, so the URL works
// from a phone and the flow works headless.
//
// Unlike ZCode the result is an ordinary bearer access token with a refresh
// token, refreshed through `/v2/plugin/auth/token/refresh` with the refresh
// token in X-Refresh-Token. That plugs straight into the background refresher.
//
// WorkBuddy publishes no API for this. Routes, codes and headers follow
// third-party integrations that agree with each other (elavita/
// dsh-codearts-auth, pilciw/codebuddy2api, OmniRoute's CodeBuddy CN gateway);
// response fields are parsed tolerantly because none of them pin a schema.
// See .design/workbuddy-oauth.md.

const (
	// WorkBuddyLoginTimeout bounds how long the poller waits for the user to
	// sign in in the browser.
	WorkBuddyLoginTimeout = 5 * time.Minute

	// workbuddyPollInterval is the delay between token polls.
	workbuddyPollInterval = time.Second

	// workbuddyAccountTimeout bounds the account-info poll that follows the
	// token. Account info only names the provider and fills X-User-Id, so a
	// slow or missing answer must not fail a login that already has tokens.
	workbuddyAccountTimeout = 15 * time.Second

	// Envelope codes meaning "not ready yet, keep polling".
	workbuddyCodeTokenPending   = 11217
	workbuddyCodeAccountPending = 12151
)

// Token metadata keys a WorkBuddy login sets. They land in
// OAuthDetail.ExtraFields; the inference round tripper reads UserID back.
const (
	WorkBuddyMetaUserID   = "workbuddy_uid"
	WorkBuddyMetaNickname = "workbuddy_nickname"
	WorkBuddyMetaDomain   = "workbuddy_enterprise_domain"
)

// WorkBuddyFlow is an in-progress WorkBuddy login.
type WorkBuddyFlow struct {
	// State identifies the login server-side; it is the poll key.
	State string

	// AuthorizeURL is the URL the user must open.
	AuthorizeURL string

	// PollInterval is the delay between token polls.
	PollInterval time.Duration

	// Issuer is the tingly-box issuer that started the flow.
	Issuer ai.Issuer

	// UserID, RedirectTo and Name ride the flow to the terminal step, as for
	// ZCode.
	UserID     string
	RedirectTo string
	Name       string
}

// WorkBuddyAccount is the account the login signed in to.
type WorkBuddyAccount struct {
	UserID   string
	Nickname string
	// EnterpriseDomain is set for enterprise (dedicated / self-hosted)
	// accounts, whose models live on their own host.
	EnterpriseDomain string
}

// WorkBuddyClient talks to a WorkBuddy site's plugin auth API. The origin is a
// field so tests can point the whole flow at an httptest server.
type WorkBuddyClient struct {
	HTTPClient *http.Client
	Site       ai.WorkBuddySite
	// PollInterval overrides workbuddyPollInterval (tests).
	PollInterval time.Duration
}

// NewWorkBuddyClient returns a client for site.
func NewWorkBuddyClient(httpClient *http.Client, site ai.WorkBuddySite) *WorkBuddyClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &WorkBuddyClient{HTTPClient: httpClient, Site: site, PollInterval: workbuddyPollInterval}
}

// Start requests a login state and the URL the user signs in on.
func (c *WorkBuddyClient) Start(ctx context.Context) (*WorkBuddyFlow, error) {
	var data struct {
		State   string `json:"state"`
		AuthURL string `json:"authUrl"`
		URL     string `json:"url"`
	}
	endpoint := c.Site.Origin + "/v2/plugin/auth/state?platform=" + url.QueryEscape(c.Site.Platform)
	if err := c.envelope(ctx, http.MethodPost, endpoint, strings.NewReader("{}"), nil, &data); err != nil {
		return nil, fmt.Errorf("workbuddy login state: %w", err)
	}
	authURL := firstNonEmpty(data.AuthURL, data.URL)
	if data.State == "" || authURL == "" {
		return nil, fmt.Errorf("workbuddy login state: response missing state or authUrl")
	}
	return &WorkBuddyFlow{State: data.State, AuthorizeURL: authURL, PollInterval: c.PollInterval}, nil
}

// workbuddyTokenData is the token payload of auth/token and the refresh
// endpoint. Both camelCase and snake_case spellings are accepted, and the
// expiry may come as a lifetime or as an absolute time.
type workbuddyTokenData struct {
	AccessToken       string          `json:"accessToken"`
	AccessTokenSnake  string          `json:"access_token"`
	RefreshToken      string          `json:"refreshToken"`
	RefreshTokenSnake string          `json:"refresh_token"`
	ExpiresIn         json.RawMessage `json:"expiresIn"`
	ExpiresInSnake    json.RawMessage `json:"expires_in"`
	ExpiresAt         json.RawMessage `json:"expiresAt"`
	ExpiresAtSnake    json.RawMessage `json:"expires_at"`
	Domain            string          `json:"domain"`
}

func (d *workbuddyTokenData) token() *Token {
	t := &Token{
		AccessToken:  firstNonEmpty(d.AccessToken, d.AccessTokenSnake),
		RefreshToken: firstNonEmpty(d.RefreshToken, d.RefreshTokenSnake),
		TokenType:    "Bearer",
	}
	t.Expiry = workbuddyExpiry(t.AccessToken, []json.RawMessage{d.ExpiresIn, d.ExpiresInSnake}, []json.RawMessage{d.ExpiresAt, d.ExpiresAtSnake})
	return t
}

// PollToken waits for the user to sign in and returns the tokens.
//
// Retry semantics follow ZCode: "not ready" and transient failures (transport
// errors, 5xx, 408/429, a body that is not the envelope) are one more round;
// any other error is the flow being rejected and ends the login with the
// server's message.
func (c *WorkBuddyClient) PollToken(ctx context.Context, flow *WorkBuddyFlow, timeout time.Duration) (*Token, error) {
	if flow == nil || flow.State == "" {
		return nil, fmt.Errorf("workbuddy login poll: flow not started")
	}
	deadline := time.Now().Add(timeout)
	endpoint := c.Site.Origin + "/v2/plugin/auth/token?state=" + url.QueryEscape(flow.State)
	for {
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("workbuddy login poll: sign-in timed out, please retry the login")
		}
		var data workbuddyTokenData
		err := c.envelope(ctx, http.MethodGet, endpoint, nil, nil, &data)
		switch {
		case err == nil:
			token := data.token()
			if token.AccessToken != "" {
				return token, nil
			}
			// A success envelope without a token is the server still waiting.
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case isWorkBuddyPending(err, workbuddyCodeTokenPending):
		default:
			return nil, fmt.Errorf("workbuddy login poll: %w", err)
		}
		if err := sleepCtx(ctx, c.PollInterval); err != nil {
			return nil, err
		}
	}
}

// PollAccount fetches the signed-in account. It is best-effort: callers use it
// to name the provider and to fill X-User-Id, never to decide the login.
func (c *WorkBuddyClient) PollAccount(ctx context.Context, flow *WorkBuddyFlow, accessToken string, timeout time.Duration) (*WorkBuddyAccount, error) {
	deadline := time.Now().Add(timeout)
	endpoint := c.Site.Origin + "/v2/plugin/login/account?state=" + url.QueryEscape(flow.State)
	headers := http.Header{"Authorization": {"Bearer " + accessToken}}
	for {
		var data json.RawMessage
		err := c.envelope(ctx, http.MethodGet, endpoint, nil, headers, &data)
		if err == nil {
			// UseNumber keeps a numeric uid exact; a float64 would round
			// ids past 2^53 and send the wrong X-User-Id.
			var raw map[string]any
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.UseNumber()
			_ = dec.Decode(&raw)
			if account := parseWorkBuddyAccount(raw); account.UserID != "" || account.Nickname != "" {
				return account, nil
			}
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		} else if !isWorkBuddyPending(err, workbuddyCodeAccountPending) {
			return nil, err
		}
		if !time.Now().Add(c.PollInterval).Before(deadline) {
			return nil, fmt.Errorf("workbuddy account info not ready")
		}
		if err := sleepCtx(ctx, c.PollInterval); err != nil {
			return nil, err
		}
	}
}

// Refresh exchanges a refresh token for a new access token.
//
// extra carries what the caller knows about the credential being refreshed
// (the current bearer, X-User-Id — see WithWorkBuddyCredential); the official
// client sends them alongside X-Refresh-Token.
func (c *WorkBuddyClient) Refresh(ctx context.Context, refreshToken string, extra http.Header) (*Token, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("workbuddy refresh: no refresh token; re-authentication required")
	}
	var data workbuddyTokenData
	headers := extra.Clone()
	if headers == nil {
		headers = http.Header{}
	}
	headers.Set("X-Refresh-Token", refreshToken)
	if err := c.envelope(ctx, http.MethodPost, c.Site.Origin+"/v2/plugin/auth/token/refresh", strings.NewReader("{}"), headers, &data); err != nil {
		return nil, fmt.Errorf("workbuddy refresh: %w", err)
	}
	token := data.token()
	if token.AccessToken == "" {
		return nil, fmt.Errorf("workbuddy refresh: %w: response carried no access token", ErrTokenExchangeFailed)
	}
	return token, nil
}

// workbuddyEnvelope is the {code,msg,data} wrapper of the plugin API. code 0
// is success; some deployments spell the message "message".
type workbuddyEnvelope struct {
	Code    *int            `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// workbuddyError is an envelope or HTTP error. Transient marks the ones worth
// another poll round.
type workbuddyError struct {
	Status    int
	Code      int
	Msg       string
	Transient bool
}

func (e *workbuddyError) Error() string {
	return fmt.Sprintf("status=%d code=%d msg=%s", e.Status, e.Code, zcodeMsgOrNone(e.Msg))
}

func isWorkBuddyPending(err error, pendingCode int) bool {
	var we *workbuddyError
	if !errors.As(err, &we) {
		return false
	}
	return we.Transient || we.Code == pendingCode
}

func (c *WorkBuddyClient) envelope(ctx context.Context, method, endpoint string, body io.Reader, headers http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", ai.WorkBuddyUserAgent)
	req.Header.Set("X-Domain", c.Site.Domain)
	req.Header.Set("X-Product", ai.WorkBuddyProduct)
	req.Header.Set("X-Product-Code", c.Site.ProductCode)
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &workbuddyError{Msg: err.Error(), Transient: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return &workbuddyError{Status: resp.StatusCode, Msg: err.Error(), Transient: true}
	}

	retryableStatus := resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests
	var env workbuddyEnvelope
	if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
		clientErr := resp.StatusCode >= 400 && resp.StatusCode < 500 && !retryableStatus
		return &workbuddyError{Status: resp.StatusCode, Msg: "invalid response envelope: " + truncateForError(raw), Transient: !clientErr}
	}
	code := 0
	if env.Code != nil {
		code = *env.Code
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || code != 0 {
		return &workbuddyError{Status: resp.StatusCode, Code: code, Msg: firstNonEmpty(env.Msg, env.Message), Transient: retryableStatus}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("decode response data: %w", err)
		}
	}
	return nil
}

// parseWorkBuddyAccount reads the account fields from login/account. The
// payload is not documented, so the common spellings are tried in turn.
func parseWorkBuddyAccount(raw map[string]any) *WorkBuddyAccount {
	account := &WorkBuddyAccount{}
	if raw == nil {
		return account
	}
	// Some responses nest the account one level down.
	for _, key := range []string{"account", "user", "userInfo"} {
		if nested, ok := raw[key].(map[string]any); ok {
			raw = nested
			break
		}
	}
	account.UserID = stringField(raw, "uid", "userId", "user_id", "id")
	account.Nickname = stringField(raw, "nickname", "nickName", "name", "userName", "email")
	account.EnterpriseDomain = stringField(raw, "enterpriseDomain", "enterprise_domain", "endpoint")
	return account
}

func stringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		switch v := raw[key].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case json.Number:
			return v.String()
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return ""
}

// workbuddyExpiry resolves a token's expiry from whichever form the server
// sent: a lifetime in seconds, an absolute time (unix seconds, unix
// milliseconds or RFC 3339), or failing both the JWT's own exp claim. Zero
// means unknown, which the refresher treats as "nothing to schedule".
func workbuddyExpiry(accessToken string, expiresIn, expiresAt []json.RawMessage) time.Time {
	for _, raw := range expiresIn {
		if secs := parseWorkBuddyInt(raw); secs > 0 {
			return time.Now().Add(time.Duration(secs) * time.Second)
		}
	}
	for _, raw := range expiresAt {
		if t := parseWorkBuddyTime(raw); !t.IsZero() {
			return t
		}
	}
	return jwtExpiry(accessToken)
}

func parseWorkBuddyTime(raw json.RawMessage) time.Time {
	if v := parseWorkBuddyInt(raw); v > 0 {
		// Anything past year ~5000 in seconds is milliseconds.
		if v > 1e11 {
			return time.UnixMilli(v)
		}
		return time.Unix(v, 0)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseWorkBuddyInt reads an integer sent either as a JSON number or as a
// numeric string. Zero when absent or not an integer.
func parseWorkBuddyInt(raw json.RawMessage) int64 {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	v, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		if f, ferr := strconv.ParseFloat(text, 64); ferr == nil {
			return int64(f)
		}
		return 0
	}
	return v
}

// jwtExpiry reads the exp claim of a JWT without verifying it. Zero when the
// token is not a JWT or carries no exp.
func jwtExpiry(token string) time.Time {
	claims := parseIDToken(token)
	if claims == nil || claims.ExpiresAt == nil {
		return time.Time{}
	}
	return claims.ExpiresAt.Time
}

// =============================================
// Manager entry points
// =============================================

func (m *Manager) workbuddyClient(options *Options, issuer ai.Issuer) (*WorkBuddyClient, error) {
	site, ok := ai.WorkBuddySiteFor(issuer)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not a WorkBuddy issuer", ErrInvalidProvider, issuer)
	}
	if options.WorkBuddyOrigin != "" {
		site.Origin = strings.TrimSuffix(options.WorkBuddyOrigin, "/")
	}
	client := NewWorkBuddyClient(m.getHTTPClient(options), site)
	if options.WorkBuddyPollInterval > 0 {
		client.PollInterval = options.WorkBuddyPollInterval
	}
	return client, nil
}

// InitiateWorkBuddyFlow starts a WorkBuddy login and returns the flow whose
// AuthorizeURL the user must open. Nothing is persisted yet.
func (m *Manager) InitiateWorkBuddyFlow(ctx context.Context, userID string, issuer ai.Issuer, redirectTo, name string, opts ...Option) (*WorkBuddyFlow, error) {
	client, err := m.workbuddyClient(applyOptions(opts...), issuer)
	if err != nil {
		return nil, err
	}
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

// CompleteWorkBuddyFlow waits for the sign-in and returns the tokens with the
// account's id and name as metadata. The account id becomes the provider's
// UserID (via the "account_id" metadata key) and its X-User-Id header.
func (m *Manager) CompleteWorkBuddyFlow(ctx context.Context, flow *WorkBuddyFlow, opts ...Option) (*Token, error) {
	if flow == nil {
		return nil, fmt.Errorf("workbuddy login: flow not started")
	}
	client, err := m.workbuddyClient(applyOptions(opts...), flow.Issuer)
	if err != nil {
		return nil, err
	}
	token, err := client.PollToken(ctx, flow, WorkBuddyLoginTimeout)
	if err != nil {
		return nil, err
	}
	token.Issuer = flow.Issuer
	token.RedirectTo = flow.RedirectTo
	token.Name = flow.Name

	account, err := client.PollAccount(ctx, flow, token.AccessToken, workbuddyAccountTimeout)
	if err != nil {
		// The tokens are good; only the account's name and id are missing.
		// Say so: without the uid no X-User-Id is sent, and if the upstream
		// needs it this line is what ties the failure back to the login.
		logrus.Warnf("[OAuth] WorkBuddy account info unavailable for %s, provider saved without user id: %v", flow.Issuer, err)
		account = &WorkBuddyAccount{}
	}
	if account.EnterpriseDomain != "" {
		logrus.Warnf("[OAuth] WorkBuddy account reports enterprise domain %q; enterprise endpoints are not supported yet, requests go to the public site", account.EnterpriseDomain)
	}
	token.putMetadata(WorkBuddyMetaUserID, account.UserID)
	token.putMetadata("account_id", account.UserID)
	token.putMetadata(WorkBuddyMetaNickname, account.Nickname)
	token.putMetadata(WorkBuddyMetaDomain, account.EnterpriseDomain)
	// Name the provider after the account; WorkBuddy returns no email, so
	// without a nickname fall back to the issuer's display name rather than
	// an issuer-plus-timestamp name.
	if token.Name == "" {
		token.Name = account.Nickname
	}
	if token.Name == "" {
		if config, ok := m.registry.Get(flow.Issuer); ok {
			token.Name = config.DisplayName
		}
	}
	return token, nil
}

// refreshWorkBuddyToken is refreshToken for the WorkBuddy issuers, which have
// no OAuth refresh grant of their own.
func (m *Manager) refreshWorkBuddyToken(ctx context.Context, issuer ai.Issuer, refreshToken string, opts *Options) (*Token, error) {
	client, err := m.workbuddyClient(opts, issuer)
	if err != nil {
		return nil, err
	}
	return client.Refresh(ctx, refreshToken, opts.ExtraHeaders)
}
