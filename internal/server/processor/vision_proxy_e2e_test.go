//go:build e2e
// +build e2e

package processor

// E2E test for the vision-proxy smart-routing bypass against a real
// tingly-box deployment. Run with:
//
//   TINGLY_BASE_URL='https://your-tingly.example/tingly/anthropic' \
//   TINGLY_API_KEY='sk-…' \
//   TINGLY_MODEL='your-downstream-model-name' \
//   TINGLY_IMAGE_PATH='/abs/path/to/picture.png'   # optional
//   go test -tags=e2e -run TestVisionProxy_E2E ./internal/server/processor/...
//
// Expected wiring on the server side:
//   - A SmartRouting rule with op {Position: proxy_vision, Operation: enabled}
//   - That rule's Services point at a vision-capable Anthropic upstream
//   - The user-facing rule (matched by TINGLY_MODEL) is configured with a
//     text-only downstream model in its main Services list
//
// What the test does:
//   - Sends a v1 Messages.New request with one user message containing a
//     text prompt + an image block (base64 if reading from file, or the
//     embedded 1x1 PNG fallback).
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
	baseURL := os.Getenv("TINGLY_BASE_URL")
	apiKey := os.Getenv("TINGLY_API_KEY")
	model := os.Getenv("TINGLY_MODEL")
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("TINGLY_BASE_URL / TINGLY_API_KEY / TINGLY_MODEL must be set")
	}

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
					{OfText: &anthropic.TextBlockParam{
						Text: "There's an image attached. Tell me what you see in it, in one short sentence.",
					}},
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
	baseURL := os.Getenv("TINGLY_BASE_URL")
	apiKey := os.Getenv("TINGLY_API_KEY")
	model := os.Getenv("TINGLY_MODEL")
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("TINGLY_BASE_URL / TINGLY_API_KEY / TINGLY_MODEL must be set")
	}

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
					{OfText: &anthropic.BetaTextBlockParam{
						Text: "There's an image attached. Tell me what you see in it, in one short sentence.",
					}},
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
