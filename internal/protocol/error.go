package protocol

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// AnthropicErrorType enumerates Anthropic's official error.type values
// (https://docs.anthropic.com/en/api/errors). Gateway-authored errors must
// use one of these; upstream Anthropic errors are passed through verbatim
// when they already validate against this set.
type AnthropicErrorType string

const (
	AnthropicErrInvalidRequest AnthropicErrorType = "invalid_request_error"
	AnthropicErrAuthentication AnthropicErrorType = "authentication_error"
	AnthropicErrPermission     AnthropicErrorType = "permission_error"
	AnthropicErrNotFound       AnthropicErrorType = "not_found_error"
	AnthropicErrRateLimit      AnthropicErrorType = "rate_limit_error"
	AnthropicErrTimeout        AnthropicErrorType = "timeout_error"
	AnthropicErrOverloaded     AnthropicErrorType = "overloaded_error"
	AnthropicErrAPI            AnthropicErrorType = "api_error"
	AnthropicErrBilling        AnthropicErrorType = "billing_error"
)

var validAnthropicErrorTypes = map[AnthropicErrorType]bool{
	AnthropicErrInvalidRequest: true,
	AnthropicErrAuthentication: true,
	AnthropicErrPermission:     true,
	AnthropicErrNotFound:       true,
	AnthropicErrRateLimit:      true,
	AnthropicErrTimeout:        true,
	AnthropicErrOverloaded:     true,
	AnthropicErrAPI:            true,
	AnthropicErrBilling:        true,
}

// AnthropicErrorField is the nested "error" object of an Anthropic error body.
type AnthropicErrorField struct {
	Type    AnthropicErrorType `json:"type"`
	Message string             `json:"message"`
}

// AnthropicErrorBody is the exact wire shape of an Anthropic API error
// response: a top-level "type":"error" plus the nested error object.
type AnthropicErrorBody struct {
	Type  string              `json:"type"`
	Error AnthropicErrorField `json:"error"`
}

// OpenAIErrorField is the nested "error" object of an OpenAI error body.
// Param/Code are pointers so they serialize as JSON null (matching OpenAI's
// own shape) rather than being omitted, when the gateway has no value for them.
type OpenAIErrorField struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

// OpenAIErrorBody is the exact wire shape of an OpenAI API error response.
type OpenAIErrorBody struct {
	Error OpenAIErrorField `json:"error"`
}

// anthropicTypeFromStatus maps an HTTP status code to the closest Anthropic
// error.type, used when the upstream (or the gateway itself) did not supply
// one of Anthropic's own enum values.
func anthropicTypeFromStatus(status int) AnthropicErrorType {
	switch status {
	case http.StatusUnauthorized:
		return AnthropicErrAuthentication
	case http.StatusForbidden:
		return AnthropicErrPermission
	case http.StatusNotFound:
		return AnthropicErrNotFound
	case http.StatusTooManyRequests:
		return AnthropicErrRateLimit
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return AnthropicErrTimeout
	case 529, http.StatusServiceUnavailable:
		return AnthropicErrOverloaded
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return AnthropicErrInvalidRequest
	default:
		return AnthropicErrAPI
	}
}

// openaiTypeFromStatus mirrors anthropicTypeFromStatus for OpenAI, whose SDK
// treats error.type as a free-form string rather than an enum.
func openaiTypeFromStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	default:
		return "api_error"
	}
}

// buildAnthropicErrorField prefers the upstream's real error.type/message
// (only reusing the type when it validates against Anthropic's enum) and
// falls back to fallbackType + err.Error() otherwise. err.Error() is only
// evaluated when actually needed: vendor SDK errors can panic formatting
// themselves when constructed outside a real HTTP round trip (e.g. in tests),
// so this must not call it unconditionally.
func buildAnthropicErrorField(err error, fallbackType AnthropicErrorType) AnthropicErrorField {
	field := AnthropicErrorField{Type: fallbackType}
	info, ok := ExtractUpstreamError(err)
	if ok && info.Message != "" {
		field.Message = info.Message
	} else {
		field.Message = err.Error()
	}
	if ok {
		if t := AnthropicErrorType(info.Type); validAnthropicErrorTypes[t] {
			field.Type = t
		}
	}
	return field
}

// BuildAnthropicError builds an Anthropic-shaped error body for err. When err
// came from an Anthropic upstream, its real error.type/message is reused
// verbatim; otherwise (a different vendor, or a local gateway error) the type
// is derived from statusCode and the message falls back to err.Error() (or
// the differently-shaped upstream message, if one is available).
func BuildAnthropicError(err error, statusCode int) AnthropicErrorBody {
	return AnthropicErrorBody{
		Type:  "error",
		Error: buildAnthropicErrorField(err, anthropicTypeFromStatus(statusCode)),
	}
}

// BuildAnthropicStreamErrorEvent builds the payload for an Anthropic SSE
// "error" event (sent mid-stream, after message_start, when the HTTP status
// line has already committed). fallbackType is used when err carries no
// recognizable upstream error.type.
func BuildAnthropicStreamErrorEvent(err error, fallbackType AnthropicErrorType) map[string]interface{} {
	field := buildAnthropicErrorField(err, fallbackType)
	return map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    string(field.Type),
			"message": field.Message,
		},
	}
}

// BuildOpenAIError builds an OpenAI-shaped error body for err. When err came
// from an OpenAI upstream, its real type/message/param/code are reused
// verbatim; otherwise the type is derived from statusCode. err.Error() is
// only evaluated when actually needed (see buildAnthropicErrorField).
func BuildOpenAIError(err error, statusCode int) OpenAIErrorBody {
	field := OpenAIErrorField{Type: openaiTypeFromStatus(statusCode)}
	info, ok := ExtractUpstreamError(err)
	if ok && info.Message != "" {
		field.Message = info.Message
	} else {
		field.Message = err.Error()
	}
	if ok {
		if info.Type != "" {
			field.Type = info.Type
		}
		if info.Param != "" {
			field.Param = &info.Param
		}
		if info.Code != "" {
			field.Code = &info.Code
		}
	}
	return OpenAIErrorBody{Error: field}
}

// prefixMessage prepends desc to an already-built error message when desc is
// non-empty, so callers can keep attaching local debugging context (e.g.
// "Failed to forward request") without losing the upstream's real message.
func prefixMessage(message, desc string) string {
	if desc == "" {
		return message
	}
	return desc + ": " + message
}

// SendAnthropicError sends an Anthropic-shaped JSON error response, deriving
// the HTTP status from err (propagating the upstream's real status when
// available) and registering err with gin's error log.
func SendAnthropicError(c *gin.Context, err error, desc string) {
	status := UpstreamStatus(err, http.StatusInternalServerError)
	body := BuildAnthropicError(err, status)
	body.Error.Message = prefixMessage(body.Error.Message, desc)
	c.Error(err).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(status, body)
}

// SendOpenAIError sends an OpenAI-shaped JSON error response, deriving the
// HTTP status from err (propagating the upstream's real status when
// available) and registering err with gin's error log.
func SendOpenAIError(c *gin.Context, err error, desc string) {
	status := UpstreamStatus(err, http.StatusInternalServerError)
	body := BuildOpenAIError(err, status)
	body.Error.Message = prefixMessage(body.Error.Message, desc)
	c.Error(err).SetType(gin.ErrorTypePublic) //nolint:errcheck
	c.JSON(status, body)
}

// IsAnthropicAPIType reports whether apiType is one of the Anthropic-family
// API types (v1 or beta), i.e. the client speaks Anthropic's wire protocol.
// Used at the handful of shared choke points that need to pick between
// SendAnthropicError and SendOpenAIError.
func IsAnthropicAPIType(apiType APIType) bool {
	return apiType == TypeAnthropicV1 || apiType == TypeAnthropicBeta
}
