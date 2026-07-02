package protocol

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tidwall/gjson"
	"google.golang.org/genai"
)

// UpstreamStatus extracts the HTTP status code that an upstream provider
// returned, so the gateway can propagate it to the client instead of flattening
// every forwarding failure into a 500. It understands the error types returned
// by each vendor SDK (OpenAI / Anthropic share apierror.Error; google-genai
// uses genai.APIError). When the error does not carry a usable upstream status
// (e.g. a transport-level failure with no HTTP response), it returns fallback.
func UpstreamStatus(err error, fallback int) int {
	if err == nil {
		return fallback
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) && oaiErr.StatusCode >= 400 {
		return oaiErr.StatusCode
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) && anthropicErr.StatusCode >= 400 {
		return anthropicErr.StatusCode
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) && genaiErr.Code >= 400 {
		return genaiErr.Code
	}

	return fallback
}

// UpstreamErrorInfo is the structured info recovered from a vendor SDK error,
// used to preserve the upstream's real error classification (type/message/
// param/code) instead of collapsing every forwarding failure into a generic
// gateway message.
type UpstreamErrorInfo struct {
	StatusCode int
	Type       string
	Message    string
	Param      string
	Code       string
}

// ExtractUpstreamError pulls structured type/message/param/code out of a
// vendor SDK error (openai.Error / anthropic.Error / genai.APIError). Returns
// ok=false when err is not a recognized vendor error (e.g. a local gateway
// failure such as a JSON marshal error or a transport-level timeout), in
// which case callers should fall back to a status-derived error type and
// err.Error() as the message.
func ExtractUpstreamError(err error) (UpstreamErrorInfo, bool) {
	if err == nil {
		return UpstreamErrorInfo{}, false
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) {
		return UpstreamErrorInfo{
			StatusCode: oaiErr.StatusCode,
			Type:       oaiErr.Type,
			Message:    oaiErr.Message,
			Param:      oaiErr.Param,
			Code:       oaiErr.Code,
		}, true
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) {
		// apierror.Error exposes no public Message field, only Type() and the
		// raw response body, so pull "error.message" out of the raw JSON.
		raw := anthropicErr.RawJSON()
		return UpstreamErrorInfo{
			StatusCode: anthropicErr.StatusCode,
			Type:       string(anthropicErr.Type()),
			Message:    gjson.Get(raw, "error.message").String(),
		}, true
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) {
		// genai.APIError has no Anthropic/OpenAI-style type enum, only a
		// Google-specific Status string (e.g. "RESOURCE_EXHAUSTED"); leave
		// Type empty so callers fall back to their status-derived mapping.
		return UpstreamErrorInfo{
			StatusCode: genaiErr.Code,
			Message:    genaiErr.Message,
		}, true
	}

	return UpstreamErrorInfo{}, false
}
