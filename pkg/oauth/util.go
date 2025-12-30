package oauth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
)

// buildRequestBody builds an HTTP request body from parameters.
// It handles both JSON and form-encoded formats based on the useJSON flag.
// Returns the body reader and content type.
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
