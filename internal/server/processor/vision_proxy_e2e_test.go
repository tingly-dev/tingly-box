//go:build e2e
// +build e2e

package processor

// E2E test for the vision-proxy smart-routing bypass against a real
// tingly-box deployment.
//
// Quickest path (local debug — only the API key is required):
//
//   TINGLY_API_KEY='sk-…' \
//     go test -tags=e2e -v -run TestVisionProxy_E2E ./internal/server/processor/...
//
// Defaults used when the env var is absent:
//   TINGLY_BASE_URL   = http://localhost:12580/anthropic
//   TINGLY_MODEL      = claude-3-5-sonnet-latest
//   TINGLY_IMAGE_PATH = embedded 1x1 black PNG
//
// Override any of them to point at a remote deployment, a different model,
// or a real image. The API key has no default — an unset key skips the test.
//
// Expected wiring on the server side:
//   - A SmartRouting rule with op {Position: proxy_vision, Operation: enabled}
//   - That rule's Services point at a vision-capable Anthropic upstream
//   - The user-facing rule (matched by TINGLY_MODEL) is configured with a
//     text-only downstream model in its main Services list
//
// What the test does:
//   - Sends a v1 Messages.New request (and a Beta variant) with one user
//     message containing a text prompt + an image block.
//   - Asserts: 200 status, non-empty response text.
//   - Logs the response so an operator can eyeball whether the description
//     made it through.

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"
)

// Defaults for local debugging — used when the corresponding TINGLY_*
// env var is unset. Override any of them by exporting the env var. The
// API key is intentionally NOT defaulted: an unset key skips the test
// rather than spamming the upstream with bogus auth.
const (
	defaultBaseURL = "http://localhost:12580/anthropic"
	defaultModel   = "claude-3-5-sonnet-latest"
	defaultPrompt  = "There's an image attached. Tell me what you see in it, in one short sentence."
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// 1x1 black PNG fallback — same blob the unit harness uses. Vision models
// can still describe it ("a tiny black square") so the round-trip is
// observable even without a real image file.
const e2eFallbackPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

func e2eImageSource(t *testing.T) (mediaType, b64 string) {
	t.Helper()
	path := os.Getenv("TINGLY_IMAGE_PATH")
	if path == "" {
		return "image/png", e2eFallbackPNGBase64
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "TINGLY_IMAGE_PATH %q is unreadable", path)
	mt := "image/png"
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".jpg"),
		strings.HasSuffix(strings.ToLower(path), ".jpeg"):
		mt = "image/jpeg"
	case strings.HasSuffix(strings.ToLower(path), ".webp"):
		mt = "image/webp"
	case strings.HasSuffix(strings.ToLower(path), ".gif"):
		mt = "image/gif"
	}
	return mt, base64.StdEncoding.EncodeToString(raw)
}

// TestVisionProxy_E2E_V1Messages sends a v1 Messages.New request with text
// + image against a real tingly-box deployment. The deployment is expected
// to have a `proxy_vision.enabled` smart-routing rule that handles the
// image describe step before routing to the (text-only) downstream model.
func TestVisionProxy_E2E_V1Messages(t *testing.T) {
	apiKey := os.Getenv("TINGLY_API_KEY")
	if apiKey == "" {
		t.Skip("TINGLY_API_KEY not set — export it (and optionally TINGLY_BASE_URL / TINGLY_MODEL / TINGLY_IMAGE_PATH) to run")
	}
	baseURL := envOr("TINGLY_BASE_URL", defaultBaseURL)
	model := envOr("TINGLY_MODEL", defaultModel)

	mediaType, b64 := e2eImageSource(t)
	t.Logf("e2e config: baseURL=%s model=%s mediaType=%s image_bytes_b64=%d",
		baseURL, model, mediaType, len(b64))

	client := anthropic.NewClient(
		anthropicOption.WithAPIKey(apiKey),
		anthropicOption.WithBaseURL(baseURL),
		anthropicOption.WithHTTPClient(&http.Client{Timeout: 90 * time.Second}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 512,
		Messages: []anthropic.MessageParam{
			{
				Role: anthropic.MessageParamRoleUser,
				Content: []anthropic.ContentBlockParamUnion{
					{OfText: &anthropic.TextBlockParam{Text: defaultPrompt}},
					anthropic.NewImageBlock(anthropic.Base64ImageSourceParam{
						Data:      b64,
						MediaType: anthropic.Base64ImageSourceMediaType(mediaType),
					}),
				},
			},
		},
	})
	require.NoError(t, err, "Messages.New must succeed against the vision-proxy rule")
	require.NotNil(t, resp, "response must be non-nil")

	var text strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	body := strings.TrimSpace(text.String())
	t.Logf("response (id=%s stop_reason=%s usage=in:%d/out:%d):\n%s",
		resp.ID, resp.StopReason, resp.Usage.InputTokens, resp.Usage.OutputTokens, body)

	require.NotEmpty(t, body, "downstream model must produce a non-empty text response — empty response usually means smart-routing matched no rule, or the downstream model itself rejected the request")

	// Soft-warn (not fail) when the response carries the fail-strip marker:
	// vision proxy ran but the upstream describe call failed. Routing
	// still worked; the user just needs to fix the vision-proxy upstream.
	if strings.Contains(body, "(description unavailable)") {
		t.Logf("WARNING: response references the vision-proxy fail-strip marker — describe call likely failed on the server side. Check the vision-proxy upstream provider / model.")
	}
}

// TestVisionProxy_E2E_BetaMessages exercises the Beta endpoint path. Same
// wiring expectations as the v1 test; some clients send via Beta so we want
// to confirm both shapes flow through the bypass.
func TestVisionProxy_E2E_BetaMessages(t *testing.T) {
	apiKey := os.Getenv("TINGLY_API_KEY")
	if apiKey == "" {
		t.Skip("TINGLY_API_KEY not set — export it (and optionally TINGLY_BASE_URL / TINGLY_MODEL / TINGLY_IMAGE_PATH) to run")
	}
	baseURL := envOr("TINGLY_BASE_URL", defaultBaseURL)
	model := envOr("TINGLY_MODEL", defaultModel)

	mediaType, b64 := e2eImageSource(t)

	client := anthropic.NewClient(
		anthropicOption.WithAPIKey(apiKey),
		anthropicOption.WithBaseURL(baseURL),
		anthropicOption.WithHTTPClient(&http.Client{Timeout: 90 * time.Second}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	resp, err := client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 512,
		Messages: []anthropic.BetaMessageParam{
			{
				Role: anthropic.BetaMessageParamRoleUser,
				Content: []anthropic.BetaContentBlockParamUnion{
					{OfText: &anthropic.BetaTextBlockParam{Text: defaultPrompt}},
					anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
						Data:      b64,
						MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType),
					}),
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)

	var text strings.Builder
	for _, b := range resp.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	body := strings.TrimSpace(text.String())
	t.Logf("beta response (id=%s stop_reason=%s):\n%s", resp.ID, resp.StopReason, body)
	require.NotEmpty(t, body)
	if strings.Contains(body, "(description unavailable)") {
		t.Logf("WARNING: vision-proxy describe call failed upstream (response carries fail-strip marker)")
	}
}
