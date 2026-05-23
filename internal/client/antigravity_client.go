package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AntigravityClient wraps GoogleClient with Antigravity OAuth-specific
// behaviors. Antigravity also speaks the Google Code Assist envelope (same
// host as Gemini CLI) but with its own User-Agent, requestType, and the
// "request"/"requestId" shape from the desktop Antigravity client.
type AntigravityClient struct {
	*GoogleClient
}

// NewAntigravityClient builds an AntigravityClient. The project_id must already
// be present in OAuth metadata (populated by AntigravityHook.AfterToken via
// loadCodeAssist).
func NewAntigravityClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AntigravityClient, error) {
	if provider.OAuthDetail == nil || provider.OAuthDetail.GetIssuer() != ai.IssuerAntigravity {
		return nil, fmt.Errorf("antigravity client requires an antigravity OAuth provider")
	}

	project := ""
	if provider.OAuthDetail.ExtraFields != nil {
		if p, ok := provider.OAuthDetail.ExtraFields["project_id"].(string); ok {
			project = p
		}
	}
	if project == "" {
		logrus.Warnf("[Antigravity] provider %s has no project_id in OAuth metadata; Code Assist calls will fail until loadCodeAssist populates it", provider.Name)
	}

	transport := &antigravityRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
		project:      project,
		proxyURL:     provider.ProxyURL,
	}
	httpClient := &http.Client{Transport: transport}

	base, err := newGoogleClientFromHTTPClient(provider, httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create base Google client: %w", err)
	}
	return &AntigravityClient{GoogleClient: base}, nil
}

// antigravityRoundTripper applies the Antigravity desktop Code Assist envelope:
//
//	{
//	  "project":     "<project_id>",
//	  "requestId":   "agent-<uuid>",
//	  "request":     { ...original google genai body, minus "model"... },
//	  "model":       "<model>",
//	  "userAgent":   "antigravity",
//	  "requestType": "agent"
//	}
//
// Like the Gemini path, it rewrites the URL to /v1internal:<op>, swaps the
// API-key header for Authorization: Bearer, and unwraps the "response"
// envelope from the upstream reply (streaming and non-streaming).
type antigravityRoundTripper struct {
	http.RoundTripper
	project, model string
	proxyURL       string
}

func (t *antigravityRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	key := req.Header.Get("X-Goog-Api-Key")
	isStreaming := isStreamingRequest(req)

	originalPath := req.URL.Path
	newPath := originalPath
	model := ""

	if strings.Contains(newPath, ":generateContent") || strings.Contains(newPath, ":streamGenerateContent") {
		parts := strings.Split(newPath, ":")
		if len(parts) >= 2 {
			subparts := strings.Split(parts[0], "/")
			model = subparts[len(subparts)-1]
			operation := parts[1]
			newPath = fmt.Sprintf("/v1internal:%s", operation)
		}
	}

	if newPath != originalPath {
		logrus.WithContext(req.Context()).Debugf("[Antigravity] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	if req.Body != nil && t.project != "" && model != "" {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()

		var originalBody map[string]any
		if err := json.Unmarshal(body, &originalBody); err == nil {
			cleanBody := make(map[string]any)
			for k, v := range originalBody {
				if k != "model" {
					cleanBody[k] = v
				}
			}

			wrapped := map[string]any{
				"project":     t.project,
				"requestId":   fmt.Sprintf("agent-%s", uuid.New().String()),
				"request":     cleanBody,
				"model":       model,
				"userAgent":   "antigravity",
				"requestType": "agent",
			}

			wrappedBody, err := json.Marshal(wrapped)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal wrapped body: %w", err)
			}
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(wrappedBody)), nil
			}
			req.Body = io.NopCloser(bytes.NewReader(wrappedBody))
			req.ContentLength = int64(len(wrappedBody))
		}
	}

	req.Header = http.Header{}
	req.Header.Set("User-Agent", "antigravity/1.11.5 windows/amd64")
	req.Header.Set("Content-Type", "application/json")
	if req.ContentLength > 0 {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", req.ContentLength))
	}
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	logrus.WithContext(req.Context()).Debugf("[Antigravity] Sending request to %s, Content-Length=%d, isStreaming=%v", req.URL.Path, req.ContentLength, isStreaming)

	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.WithContext(req.Context()).Errorf("[Antigravity] Request failed: %v", err)
		return nil, err
	}

	logrus.WithContext(req.Context()).Debugf("[Antigravity] Response received, status=%d", resp.StatusCode)

	if resp.Body != nil {
		if isStreaming {
			resp.Body = &streamingUnwrapReader{reader: resp.Body}
		} else {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}
			unwrapped := unwrapCodeAssistJSON(body)
			resp.Body = io.NopCloser(bytes.NewReader(unwrapped))
			resp.ContentLength = int64(len(unwrapped))
		}
	}

	return resp, nil
}
