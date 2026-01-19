package oauth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
)

// buildRequestBody builds an HTTP request body from OAuth parameters.
//
// It supports both JSON and form-encoded formats, which are commonly used
// by different OAuth providers for token requests.
//
// Parameters:
//   - params: Key-value pairs of OAuth parameters (grant_type, client_id, etc.)
//   - format: TokenRequestFormatForm or TokenRequestFormatJSON
//
// Returns:
//   - io.Reader: The request body as a reader
//   - string: The Content-Type header value ("application/json" or "application/x-www-form-urlencoded")
//   - error: Any encoding error
//
// Examples:
//
//	// JSON format (e.g., for Anthropic)
//	body, contentType, err := buildRequestBody(map[string]string{
//	    "grant_type": "authorization_code",
//	    "client_id": "xxx",
//	    "code": "yyy",
//	}, TokenRequestFormatJSON)
//	// contentType == "application/json"
//
//	// Form format (standard OAuth)
//	body, contentType, err := buildRequestBody(params, TokenRequestFormatForm)
//	// contentType == "application/x-www-form-urlencoded"
func buildRequestBody(params map[string]string, format TokenRequestFormat) (io.Reader, string, error) {
	switch format {
	case TokenRequestFormatJSON:
		// Convert to map[string]any for JSON marshaling
		jsonData := make(map[string]any, len(params))
		for k, v := range params {
			jsonData[k] = v
		}
		bodyBytes, err := json.Marshal(jsonData)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(bodyBytes), "application/json", nil
	default:
		// Convert to url.Values for form encoding
		data := url.Values{}
		for k, v := range params {
			data.Set(k, v)
		}
		return strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", nil
	}
}
