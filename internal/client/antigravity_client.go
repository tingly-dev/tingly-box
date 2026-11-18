package client

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/constant"
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

	project := provider.OAuthDetail.GetExtraFieldString("project_id")
	if project == "" {
		logrus.Warnf("[Antigravity] provider %s has no project_id in OAuth metadata; Code Assist calls will fail until loadCodeAssist populates it", provider.Name)
	}

	transport := &antigravityRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
		project:      project,
		proxyURL:     provider.ProxyURL,
		conversation: antigravityConversationID(provider.UUID, sessionID),
	}

	// MENTION: must set timeout, otherwise operations may fail unexpectedly
	timeout := time.Duration(provider.Timeout) * time.Second
	if provider.Timeout <= 0 {
		timeout = time.Duration(constant.DefaultRequestTimeout) * time.Second
	}

	httpClient := &http.Client{
		Transport: wrapWithLogging(transport, provider),
		Timeout:   timeout,
	}

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
//	  "requestId":   "agent/<conversation>/<unix ms>/<uuid>/<message count>",
//	  "request":     { ...original google genai body, minus "model"... },
//	  "model":       "<model>",
//	  "userAgent":   "antigravity",
//	  "requestType": "agent"
//	}
//
// requestType is omitted for plain text and set to image_gen for images.
// Like the Gemini path, it rewrites the URL to /v1internal:<op>, swaps the
// API-key header for Authorization: Bearer, and unwraps the "response"
// envelope from the upstream reply (streaming and non-streaming).
type antigravityRoundTripper struct {
	http.RoundTripper
	project      string
	proxyURL     string
	conversation string
}

func (t *antigravityRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Preserve the caller's URL and headers for SDK retries.
	req = req.Clone(req.Context())
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	key := req.Header.Get("X-Goog-Api-Key")
	isStreaming := isStreamingRequest(req)
	path, operation, _ := strings.Cut(req.URL.Path, ":")
	if operation == "generateContent" || operation == "streamGenerateContent" {
		model := path[strings.LastIndex(path, "/")+1:]
		if req.Body == nil {
			return nil, fmt.Errorf("antigravity generateContent requires a JSON request body")
		}
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read antigravity request: %w", err)
		}
		if strings.TrimSpace(t.project) == "" {
			return nil, fmt.Errorf("antigravity project_id is missing; reauthenticate the provider to discover its Code Assist project")
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(body, &inner); err != nil || inner == nil {
			return nil, fmt.Errorf("antigravity request body must be a JSON object")
		}
		delete(inner, "model")
		var contents []json.RawMessage
		_ = json.Unmarshal(inner["contents"], &contents)
		conversation := t.conversation
		if conversation == "" {
			conversation = uuid.NewString()
		}
		wrapped := map[string]any{
			"project":   t.project,
			"requestId": fmt.Sprintf("agent/%s/%d/%s/%d", conversation, time.Now().UnixMilli(), uuid.NewString(), len(contents)),
			"request":   inner,
			"model":     model,
			"userAgent": "antigravity",
		}
		if requestType := antigravityRequestType(inner); requestType != "" {
			wrapped["requestType"] = requestType
		}
		wrappedBody, err := json.Marshal(wrapped)
		if err != nil {
			return nil, fmt.Errorf("marshal antigravity request: %w", err)
		}
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(wrappedBody)), nil
		}
		req.Body = io.NopCloser(bytes.NewReader(wrappedBody))
		req.ContentLength = int64(len(wrappedBody))
		req.URL.Path = "/v1internal:" + operation
		req.URL.RawPath = ""
	}

	// Keep the vendor handshake while preserving business headers such as
	// Accept and anthropic-beta. Do not send SDK API keys or quota projects.
	req.Header.Del("X-Goog-Api-Key")
	req.Header.Del("X-Goog-Api-Client")
	req.Header.Del("X-Goog-User-Project")
	req.Header.Del("Content-Length")
	req.Header.Set("User-Agent", "antigravity/1.11.9 windows/amd64")
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	// Errors on streaming endpoints are ordinary Google JSON, not SSE.
	if resp.Body != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if isStreaming {
			resp.Body = &streamingUnwrapReader{reader: resp.Body}
			resp.ContentLength = -1
			resp.Header.Del("Content-Length")
		} else {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("read antigravity response: %w", err)
			}
			unwrapped := unwrapCodeAssistJSON(body)
			resp.Body = io.NopCloser(bytes.NewReader(unwrapped))
			resp.ContentLength = int64(len(unwrapped))
			if resp.Header == nil {
				resp.Header = make(http.Header)
			}
			resp.Header.Set("Content-Length", fmt.Sprint(len(unwrapped)))
		}
	}
	return resp, nil
}

// A real session keeps its conversation identity across client instances.
// Scope it to the provider and avoid exposing user IDs or client IPs upstream.
func antigravityConversationID(providerID string, session typ.SessionID) string {
	if session.IsEmpty() || session.IsIPFallback() {
		return uuid.NewString()
	}
	digest := sha256.Sum256([]byte(providerID + "\x00" + string(session.Source) + "\x00" + session.Value))
	return fmt.Sprintf("%x", digest[:16])
}

// Only tool requests use the agent resource pool. Image generation has its
// own request type; ordinary text requests leave the upstream default intact.
func antigravityRequestType(body map[string]json.RawMessage) string {
	var config struct {
		ResponseModalities []string `json:"responseModalities"`
	}
	_ = json.Unmarshal(body["generationConfig"], &config)
	for _, modality := range config.ResponseModalities {
		if strings.EqualFold(modality, "IMAGE") {
			return "image_gen"
		}
	}
	var tools []json.RawMessage
	_ = json.Unmarshal(body["tools"], &tools)
	if len(tools) > 0 {
		return "agent"
	}
	var contents []struct {
		Parts []map[string]json.RawMessage `json:"parts"`
	}
	_ = json.Unmarshal(body["contents"], &contents)
	for _, content := range contents {
		for _, part := range content.Parts {
			if part["functionCall"] != nil || part["functionResponse"] != nil {
				return "agent"
			}
		}
	}
	return ""
}
