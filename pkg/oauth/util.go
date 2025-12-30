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
//   - useJSON: If true, encode as JSON; otherwise, use form encoding
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
//	}, true)
//	// contentType == "application/json"
//
//	// Form format (standard OAuth)
//	body, contentType, err := buildRequestBody(params, false)
//	// contentType == "application/x-www-form-urlencoded"
func buildRequestBody(params map[string]string, useJSON bool) (io.Reader, string, error) {
	if useJSON {
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
	}

	// Convert to url.Values for form encoding
	data := url.Values{}
	for k, v := range params {
		data.Set(k, v)
	}
	return strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", nil
}
