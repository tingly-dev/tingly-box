package oauth

// Qoder's device login protocol is documented by the qodercli2api project:
// https://github.com/Liki4/qodercli2api/blob/master/docs/oauth.md
// This client is used by the server-poll provider flow.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/ai"
)

const qoderClientID = "e883ade2-e6e3-4d6d-adf7-f92ceff5fdcb"

// QoderSite holds the browser and API origins for a Qoder region.
type QoderSite struct{ BrowserURL, APIURL string }

var QoderGlobal = QoderSite{"https://qoder.com", "https://openapi.qoder.sh"}
var QoderCN = QoderSite{"https://qoder.cn", "https://openapi.qoder.com.cn"}

// QoderFlow contains secrets for one login attempt. Do not log or expose it.
type QoderFlow struct{ AuthorizeURL, Nonce, Verifier, MachineID string }

// QoderCredential is the device credential returned by Qoder.
type QoderCredential struct {
	Token        string `json:"token"`
	DeviceToken  string `json:"device_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	ExpiresAt    string `json:"expires_at"`
	UserName     string `json:"user_name"`
	UserID       string `json:"user_id"`
}

func (c QoderCredential) token() string {
	if c.Token != "" {
		return c.Token
	}
	if c.DeviceToken != "" {
		return c.DeviceToken
	}
	return c.AccessToken
}

// QoderClient implements browser device authorization and refresh.
type QoderClient struct {
	Site         QoderSite
	HTTPClient   *http.Client
	PollInterval time.Duration
}

// Start creates a new independent login attempt and browser URL.
func (c *QoderClient) Start() (*QoderFlow, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	nonce, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	machine, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(c.Site.BrowserURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("qoder: invalid browser origin")
	}
	u.Path = "/device/selectAccounts"
	q := u.Query()
	q.Set("challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
	q.Set("challenge_method", "S256")
	q.Set("nonce", nonce.String())
	q.Set("machine_id", machine.String())
	q.Set("client_id", qoderClientID)
	u.RawQuery = q.Encode()
	return &QoderFlow{AuthorizeURL: u.String(), Nonce: nonce.String(), Verifier: verifier, MachineID: machine.String()}, nil
}

func (c *QoderClient) client() *http.Client {
	base := c.HTTPClient
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	client := *base
	previous := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || (len(via) > 0 && (req.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(req.URL.Host, via[0].URL.Host))) {
			return http.ErrUseLastResponse
		}
		if previous != nil {
			return previous(req, via)
		}
		return nil
	}
	return &client
}

// Poll waits for authorization until ctx is cancelled. A 404 means pending.
func (c *QoderClient) Poll(ctx context.Context, flow *QoderFlow) (*QoderCredential, error) {
	if flow == nil || flow.Nonce == "" || flow.Verifier == "" {
		return nil, fmt.Errorf("qoder: invalid flow")
	}
	u, err := url.Parse(strings.TrimRight(c.Site.APIURL, "/") + "/api/v1/deviceToken/poll")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("nonce", flow.Nonce)
	q.Set("verifier", flow.Verifier)
	q.Set("challenge_method", "S256")
	u.RawQuery = q.Encode()
	interval := c.PollInterval
	if interval <= 0 {
		interval = time.Second
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.client().Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("qoder poll: request failed")
		}
		if resp.StatusCode != http.StatusNotFound {
			return qoderDecodeCredential(resp)
		}
		resp.Body.Close()
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Refresh exchanges a device refresh token using the same machine identity.
func (c *QoderClient) Refresh(ctx context.Context, refreshToken, machineID string) (*QoderCredential, error) {
	if refreshToken == "" || machineID == "" {
		return nil, fmt.Errorf("qoder: refresh token and machine ID required")
	}
	body, err := json.Marshal(map[string]string{"refresh_token": refreshToken, "machine_id": machineID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Site.APIURL, "/")+"/api/v1/deviceToken/refresh", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("qoder refresh: %w", err)
	}
	return qoderDecodeCredential(resp)
}

func qoderDecodeCredential(resp *http.Response) (*QoderCredential, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qoder: unexpected HTTP status %d", resp.StatusCode)
	}
	var credential QoderCredential
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&credential); err != nil {
		return nil, fmt.Errorf("qoder: invalid credential response: %w", err)
	}
	if credential.token() == "" {
		return nil, fmt.Errorf("qoder: credential response missing token")
	}
	return &credential, nil
}

// QoderMachineID binds a credential to the machine identity used at login.
const QoderMachineID = "qoder_machine_id"

func (m *Manager) qoderClient(opts *Options, issuer ai.Issuer) *QoderClient {
	site := QoderGlobal
	if issuer == ai.IssuerQoderCN {
		site = QoderCN
	}
	return &QoderClient{Site: site, HTTPClient: m.getHTTPClient(opts)}
}
func (c *QoderCredential) asToken(issuer ai.Issuer) *Token {
	t := &Token{AccessToken: c.token(), RefreshToken: c.RefreshToken, ExpiresIn: c.ExpiresIn, TokenType: "Bearer", Issuer: issuer}
	t.setExpiryFromExpiresIn()
	if c.ExpiresAt != "" {
		if expiry, err := time.Parse(time.RFC3339, c.ExpiresAt); err == nil {
			t.Expiry = expiry
		}
	}
	return t
}
func WithQoderMachineID(id string) Option { return func(o *Options) { o.QoderMachineID = id } }

// QoderProfile contains the identity fields required by the inference protocol.
type QoderProfile struct {
	UID              string   `json:"uid"`
	Name             string   `json:"name"`
	OrganizationID   string   `json:"organization_id"`
	OrganizationTags []string `json:"organization_tags"`
	DataPolicyAgreed bool     `json:"data_policy_agreed"`
}

func (c *QoderClient) Profile(ctx context.Context, token string) (*QoderProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.Site.APIURL, "/")+"/api/v1/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "qoder/1.1.74")
	resp, err := c.client().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("qoder profile request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qoder profile: HTTP status %d", resp.StatusCode)
	}
	var p QoderProfile
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p); err != nil {
		return nil, fmt.Errorf("qoder: invalid profile")
	}
	return &p, nil
}
