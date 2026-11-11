package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

type qoderTransport func(*http.Request) (*http.Response, error)

func (f qoderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func qoderResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestQoderDeviceFlowAndRefresh(t *testing.T) {
	polls := 0
	client := &http.Client{Transport: qoderTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/deviceToken/poll":
			polls++
			if r.URL.Query().Get("nonce") == "" || r.URL.Query().Get("verifier") == "" || r.URL.Query().Get("challenge_method") != "S256" {
				t.Error("missing poll parameters")
			}
			if polls == 1 {
				return qoderResponse(404, ""), nil
			}
			return qoderResponse(200, `{"token":"device","refresh_token":"refresh","expires_in":7200}`), nil
		case "/api/v1/deviceToken/refresh":
			var body map[string]string
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&body) != nil || body["refresh_token"] != "refresh" || body["machine_id"] == "" {
				t.Error("invalid refresh request")
			}
			return qoderResponse(200, `{"token":"rotated","refresh_token":"next"}`), nil
		default:
			return qoderResponse(404, ""), nil
		}
	})}
	c := &QoderClient{Site: QoderSite{BrowserURL: "https://qoder.com", APIURL: "https://example.test"}, HTTPClient: client, PollInterval: time.Millisecond}
	flow, err := c.Start()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(flow.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(flow.Verifier))
	if u.Query().Get("challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) || u.Query().Get("machine_id") != flow.MachineID || u.Query().Get("client_id") != qoderClientID {
		t.Fatal("incorrect authorization URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	token, err := c.Poll(ctx, flow)
	if err != nil || token.token() != "device" || polls != 2 {
		t.Fatalf("poll: %#v, %v", token, err)
	}
	token, err = c.Refresh(ctx, token.RefreshToken, flow.MachineID)
	if err != nil || token.token() != "rotated" {
		t.Fatalf("refresh: %#v, %v", token, err)
	}
}

func TestQoderPollCancellation(t *testing.T) {
	client := &http.Client{Transport: qoderTransport(func(*http.Request) (*http.Response, error) { return qoderResponse(404, ""), nil })}
	c := &QoderClient{Site: QoderSite{APIURL: "https://example.test"}, HTTPClient: client, PollInterval: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Poll(ctx, &QoderFlow{Nonce: "n", Verifier: "v"}); err != context.DeadlineExceeded {
		t.Fatalf("got %v", err)
	}
}

func TestQoderManagerFlow(t *testing.T) {
	client := &http.Client{Transport: qoderTransport(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/deviceToken/poll":
			return qoderResponse(200, `{"token":"access","refresh_token":"refresh","expires_in":3600,"user_id":"user-1"}`), nil
		case "/api/v1/userinfo":
			if r.Header.Get("Authorization") != "Bearer access" {
				t.Error("missing profile authorization")
			}
			return qoderResponse(200, `{"uid":"user-1","name":"Alice","organization_id":"org","organization_tags":["tag"],"data_policy_agreed":true}`), nil
		case "/api/v1/deviceToken/refresh":
			return qoderResponse(200, `{"token":"new-access","refresh_token":"new-refresh","expires_in":3600}`), nil
		default:
			return qoderResponse(404, ""), nil
		}
	})}
	m := NewManager()
	flow, err := m.InitiateServerPollFlow(context.Background(), "local", ai.IssuerQoderCN, "", "", WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.CompleteServerPollFlow(context.Background(), flow, WithHTTPClient(client))
	if err != nil {
		t.Fatal(err)
	}
	if token.Metadata["account_id"] != "user-1" || token.Metadata[QoderMachineID] == "" || token.Name != "Alice" {
		t.Fatalf("incorrect token metadata: %#v", token.Metadata)
	}
	refreshed, err := m.refreshToken(context.Background(), ai.IssuerQoderCN, token.RefreshToken, applyOptions(WithHTTPClient(client), WithQoderMachineID(token.Metadata[QoderMachineID].(string))))
	if err != nil || refreshed.AccessToken != "new-access" {
		t.Fatalf("refresh: %v", err)
	}
}

func TestQoderRegistry(t *testing.T) {
	m := NewManager()
	for _, issuer := range []ai.Issuer{ai.IssuerQoder, ai.IssuerQoderCN} {
		cfg, ok := m.GetRegistry().Get(issuer)
		if !ok || cfg.OAuthMethod != OAuthMethodServerPoll {
			t.Fatalf("missing server-poll provider %s", issuer)
		}
		if _, err := ParseIssuer(issuer); err != nil {
			t.Fatal(err)
		}
	}
}
