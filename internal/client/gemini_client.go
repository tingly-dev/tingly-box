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

// Gemini CLI Code Assist routing constants. Requests authenticated with a
// Gemini CLI OAuth credential are always sent to cloudcode-pa.googleapis.com,
// even when the provider's api_base is the canonical Gemini endpoint — the
// round tripper rewrites the host transparently.
const (
	geminiCodeAssistHost     = "cloudcode-pa.googleapis.com"
	geminiCLIUserAgent       = "GeminiCLI/0.1.0 (linux; amd64)"
	geminiCLIApiClientHeader = "gl-node/22.0.0 google-api-nodejs-client/9.0.0"
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

	project := ""
	if provider.OAuthDetail.ExtraFields != nil {
		if p, ok := provider.OAuthDetail.ExtraFields["project_id"].(string); ok {
			project = p
		}
	}
	if project == "" {
		logrus.Warnf("[Gemini] provider %s has no project_id in OAuth metadata; Code Assist calls will fail until loadCodeAssist/onboardUser populates it", provider.Name)
	}

	transport := &geminiRoundTripper{
		RoundTripper: createSessionBoundTransport(provider, sessionID),
		project:      project,
		proxyURL:     provider.ProxyURL,
	}
	httpClient := &http.Client{Transport: transport}

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
	operation := ""

	if strings.Contains(newPath, ":generateContent") ||
		strings.Contains(newPath, ":streamGenerateContent") ||
		strings.Contains(newPath, ":countTokens") {
		parts := strings.Split(newPath, ":")
		if len(parts) >= 2 {
			subparts := strings.Split(parts[0], "/")
			model = subparts[len(subparts)-1]
			operation = parts[1]
			newPath = fmt.Sprintf("/v1internal:%s", operation)
		}
	}

	if newPath != originalPath {
		logrus.Debugf("[Gemini] Rewriting URL path: %s -> %s", originalPath, newPath)
		req.URL.Path = newPath
	}

	// Code Assist requests always go to cloudcode-pa.googleapis.com regardless
	// of the provider's user-facing api_base (e.g. generativelanguage.googleapis.com).
	if req.URL.Host != geminiCodeAssistHost {
		logrus.Debugf("[Gemini] Rewriting host: %s -> %s", req.URL.Host, geminiCodeAssistHost)
		req.URL.Host = geminiCodeAssistHost
		req.Host = geminiCodeAssistHost
	}
	req.URL.Scheme = "https"

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

			// countTokens does not accept safetySettings on the Code Assist
			// envelope — strip it to avoid INVALID_ARGUMENT.
			if operation == "countTokens" {
				delete(cleanBody, "safetySettings")
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
	req.Header.Set("User-Agent", geminiCLIUserAgent)
	req.Header.Set("X-Goog-Api-Client", geminiCLIApiClientHeader)
	req.Header.Set("Content-Type", "application/json")
	if req.ContentLength > 0 {
		req.Header.Set("Content-Length", fmt.Sprintf("%d", req.ContentLength))
	}
	if key != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
	}

	logrus.Debugf("[Gemini] Sending request to %s, Content-Length=%d, isStreaming=%v", req.URL.Path, req.ContentLength, isStreaming)

	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		logrus.Errorf("[Gemini] Request failed: %v", err)
		return nil, err
	}

	logrus.Debugf("[Gemini] Response received, status=%d", resp.StatusCode)

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
