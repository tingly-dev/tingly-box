package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tingly-dev/tingly-box/internal/db"
)

const defaultAPIBase = "https://api.github.com"

// APIError is a non-2xx answer from GitHub.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("github: %d %s", e.Status, e.Message) }

type apiClient struct {
	base string
	http *http.Client
}

// do sends one request. auth is the full Authorization value ("Bearer …"),
// empty for the unauthenticated manifest conversion.
func (c *apiClient) do(ctx context.Context, method, path, auth string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "tingly-box")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		_ = json.Unmarshal(data, &e)
		msg := e.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		for _, x := range e.Errors {
			if x.Message != "" {
				msg += ": " + x.Message
			}
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// appJWT signs the short-lived JWT that authenticates as the App itself.
func appJWT(app *db.GitHubAppRecord, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(app.PrivateKey))
	if block == nil {
		return "", errors.New("github app private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("github app private key is not RSA")
		}
		key = rk
	} else {
		return "", fmt.Errorf("parse github app private key: %w", err)
	}
	iss := app.ClientID
	if iss == "" {
		iss = strconv.FormatInt(app.AppID, 10)
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Issuer:    iss,
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // clock skew allowance
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}).SignedString(key)
}

// Permission sets for the two kinds of installation token (design §4).
var (
	readPermissions = map[string]string{
		"contents": "read", "pull_requests": "read", "issues": "read",
		"checks": "read", "actions": "read", "metadata": "read",
	}
	writePermissions = map[string]string{
		"contents": "write", "pull_requests": "write", "issues": "write",
		"checks": "read", "actions": "read", "metadata": "read",
	}
)

type cachedToken struct {
	token   string
	expires time.Time
}

// tokenCache keeps installation tokens until shortly before they expire.
type tokenCache struct {
	mu sync.Mutex
	m  map[string]cachedToken
}

// get returns a cached token with at least minRemaining left.
func (c *tokenCache) get(key string, now time.Time, minRemaining time.Duration) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[key]
	if !ok || t.expires.Sub(now) < minRemaining {
		return "", false
	}
	return t.token, true
}

func (c *tokenCache) put(key, token string, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]cachedToken{}
	}
	c.m[key] = cachedToken{token, expires}
}

func (c *tokenCache) clear() {
	c.mu.Lock()
	c.m = nil
	c.mu.Unlock()
}
