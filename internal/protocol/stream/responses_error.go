package stream

import (
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// Preserve machine-readable diagnostics rather than replacing them with a dump
// of the entire SDK event (or incorrectly treating param as the error code).
func responsesStreamFailure(event *responses.ResponseStreamEventUnion) *openai.Error {
	code, message := event.Code, event.Message
	if event.Type == "response.failed" {
		code, message = string(event.Response.Error.Code), event.Response.Error.Message
	}
	if message == "" {
		message = "Responses stream failed"
	}
	return &openai.Error{Code: code, Message: message, Type: "api_error", Param: event.Param}
}
