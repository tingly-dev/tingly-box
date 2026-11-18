package client

import (
	"bytes"
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

// GeminiClient wraps GoogleClient with Gemini CLI OAuth-specific behaviors.
// It embeds *GoogleClient to inherit standard genai SDK functionality, while
// swapping in a transport that speaks the Google Code Assist envelope.
//
// Gemini CLI (Google Code Assist OAuth) requirements:
//   - Authorization: Bearer <token>     (not X-Goog-Api-Key)
//   - Rewrite /v1beta/models/<m>:<op> → /v1internal:<op>
//   - Wrap body with {model, project, user_prompt_id, request}
//   - Unwrap the "response" envelope on the way back
type GeminiClient struct {
	*GoogleClient
}

// NewGeminiClient builds a GeminiClient. The project_id stored on the OAuth
// detail by GeminiHook.AfterToken (loadCodeAssist/onboardUser) is required for
// every generateContent call — without it the Code Assist API rejects the
// request, so we still construct the client but log a warning.
func NewGeminiClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*GeminiClient, error) {
	if provider.OAuthDetail == nil || provider.OAuthDetail.GetIssuer() != ai.IssuerGemini {
		return nil, fmt.Errorf("gemini client requires a gemini OAuth provider")
	}

	project := provider.OAuthDetail.GetExtraFieldString("project_id")
	if project == "" {
		logrus.Warnf("[Gemini] provider %s has no project_id in OAuth metadata; Code Assist calls will fail until loadCodeAssist/onboardUser populates it", provider.Name)
	}

	transport := &geminiRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
		project:      project,
		proxyURL:     provider.ProxyURL,
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
	return &GeminiClient{GoogleClient: base}, nil
}

// geminiRoundTripper applies the Gemini CLI Code Assist envelope around an
// upstream Google genai request and unwraps the response envelope on the way
// back. The body shape mirrors gemini-cli/packages/core/src/code_assist:
//
//	{
//	  "model":          "<model>",
//	  "project":        "<project_id>",
//	  "user_prompt_id": "<uuid>",
//	  "request":        { ...original google genai body, minus "model"... }
//	}
type geminiRoundTripper struct {
	http.RoundTripper
	project  string
	proxyURL string
}

func (t *geminiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
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
			newPath = fmt.Sprintf("/v1internal:%s", parts[1])
		}
	}

	if newPath != originalPath {
		logrus.WithContext(req.Context()).Debugf("[Gemini] Rewriting URL path: %s -> %s", originalPath, newPath)
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
				"model":          model,
				"project":        t.project,
				"user_prompt_id": uuid.New().String(),
				"request":        cleanBody,
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
	req.Header.Set("User-Agent", "GeminiCLI/0.1.0 (linux; amd64)")
	req.Header.Set("Content-Type", "application/json")
	if req.ContentLength > 0 {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", req.ContentLength))
	}
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	logrus.WithContext(req.Context()).Debugf("[Gemini] Sending request to %s, Content-Length=%d, isStreaming=%v", req.URL.Path, req.ContentLength, isStreaming)

	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.WithContext(req.Context()).Errorf("[Gemini] Request failed: %v", err)
		return nil, err
	}

	logrus.WithContext(req.Context()).Debugf("[Gemini] Response received, status=%d", resp.StatusCode)

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
