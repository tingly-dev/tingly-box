package llmclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/sirupsen/logrus"
)

// DebugRoundTripper is an http.RoundTripper that logs headers and body in indented JSON format
type DebugRoundTripper struct {
	transport http.RoundTripper
}

// NewDebugRoundTripper creates a new debug round tripper wrapping the given transport
func NewDebugRoundTripper(transport http.RoundTripper) *DebugRoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &DebugRoundTripper{transport: transport}
}

// RoundTrip executes a single HTTP transaction and logs headers and body
func (d *DebugRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log request headers as indented JSON
	reqHeaders := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			reqHeaders[k] = v[0]
		}
	}
	reqHeadersJSON, _ := json.MarshalIndent(reqHeaders, "", "  ")
	logrus.Infof("Request Headers:\n%s", string(reqHeadersJSON))

	// Log request body if present
	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body.Close()
		if len(bodyBytes) > 0 {
			// Try to format as JSON if possible
			var jsonObj interface{}
			if err := json.Unmarshal(bodyBytes, &jsonObj); err == nil {
				formattedJSON, _ := json.MarshalIndent(jsonObj, "", "  ")
				logrus.Infof("Request Body:\n%s", string(formattedJSON))
			} else {
				logrus.Infof("Request Body:\n%s", string(bodyBytes))
			}
			// Restore the body for the actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	// Execute the request
	resp, err := d.transport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Log response headers as indented JSON
	respHeaders := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[k] = v[0]
		}
	}
	respHeadersJSON, _ := json.MarshalIndent(respHeaders, "", "  ")
	logrus.Infof("Response Headers:\n%s", string(respHeadersJSON))

	// Log response body if present
	if resp.Body != nil && resp.Body != http.NoBody {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if len(bodyBytes) > 0 {
			// Try to format as JSON if possible
			var jsonObj interface{}
			if err := json.Unmarshal(bodyBytes, &jsonObj); err == nil {
				formattedJSON, _ := json.MarshalIndent(jsonObj, "", "  ")
				logrus.Infof("Response Body:\n%s", string(formattedJSON))
			} else {
				logrus.Infof("Response Body:\n%s", string(bodyBytes))
			}
			// Restore the body for the actual response
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	return resp, nil
}
