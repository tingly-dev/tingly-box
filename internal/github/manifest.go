package github

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Manifest is the App Manifest posted to GitHub (design §3).
type Manifest struct {
	Name               string            `json:"name"`
	URL                string            `json:"url"`
	HookAttributes     hookAttributes    `json:"hook_attributes"`
	RedirectURL        string            `json:"redirect_url"`
	SetupURL           string            `json:"setup_url"`
	Description        string            `json:"description"`
	Public             bool              `json:"public"`
	DefaultPermissions map[string]string `json:"default_permissions"`
}

type hookAttributes struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

var orgName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// ConnectPlan is what the browser needs to start the Manifest flow: POST
// Manifest (as the form field "manifest") to ActionURL.
type ConnectPlan struct {
	ActionURL string `json:"action_url"`
	Manifest  string `json:"manifest"`
	State     string `json:"state"`
}

// appName is the default App name: unique per host, within GitHub's 34
// character limit. The user can still change it on GitHub's page.
func appName() string {
	host, _ := os.Hostname()
	host = regexp.MustCompile(`[^A-Za-z0-9-]+`).ReplaceAllString(host, "-")
	name := "tingly-box-" + strings.Trim(host, "-")
	if len(name) > 34 {
		name = name[:34]
	}
	return strings.TrimRight(name, "-")
}

// buildPlan assembles the manifest and the GitHub page to post it to.
// ownerType is "user" or "org"; org is required for "org".
func buildPlan(webBase, baseURL, ownerType, org, state string) (ConnectPlan, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ConnectPlan{}, fmt.Errorf("invalid base_url %q", baseURL)
	}
	base := strings.TrimRight(baseURL, "/")
	m := Manifest{
		Name:        appName(),
		URL:         base,
		RedirectURL: base + "/github/manifest/callback",
		SetupURL:    base + "/github/install/callback",
		Description: "Lets your Tingly Box coding agents push branches and open pull requests, with credentials that stay on this machine.",
		// Webhooks stay off in the first version (design §7); GitHub still
		// wants a URL on the manifest.
		HookAttributes: hookAttributes{URL: base + "/api/v1/github/webhook", Active: false},
		DefaultPermissions: map[string]string{
			"contents": "write", "pull_requests": "write", "issues": "write",
			"checks": "read", "actions": "read", "metadata": "read",
		},
	}
	raw, _ := json.Marshal(m)
	plan := ConnectPlan{Manifest: string(raw), State: state}
	switch ownerType {
	case "user", "":
		plan.ActionURL = webBase + "/settings/apps/new?state=" + url.QueryEscape(state)
	case "org":
		if !orgName.MatchString(org) {
			return ConnectPlan{}, fmt.Errorf("invalid organization name %q", org)
		}
		plan.ActionURL = webBase + "/organizations/" + org + "/settings/apps/new?state=" + url.QueryEscape(state)
	default:
		return ConnectPlan{}, fmt.Errorf("owner_type must be user or org, got %q", ownerType)
	}
	return plan, nil
}

// pendingFlow is one in-progress browser round trip, keyed by its one-time
// state nonce (CSRF protection for the unauthenticated callbacks).
type pendingFlow struct {
	kind    string // "manifest" | "install"
	baseURL string
	expires time.Time
}

type pendingFlows struct {
	mu sync.Mutex
	m  map[string]pendingFlow
}

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (p *pendingFlows) add(kind, baseURL string, now time.Time) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]pendingFlow{}
	}
	for k, v := range p.m { // drop stale entries
		if now.After(v.expires) {
			delete(p.m, k)
		}
	}
	n := newNonce()
	p.m[n] = pendingFlow{kind: kind, baseURL: baseURL, expires: now.Add(time.Hour)}
	return n
}

// take consumes a nonce; it is valid once, for its kind, before it expires.
func (p *pendingFlows) take(kind, nonce string, now time.Time) (pendingFlow, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f, ok := p.m[nonce]
	if !ok || f.kind != kind || now.After(f.expires) {
		return pendingFlow{}, false
	}
	delete(p.m, nonce)
	return f, true
}
