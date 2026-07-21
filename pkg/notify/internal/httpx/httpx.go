// Package httpx provides shared HTTP helpers for notify providers.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// drainLimit bounds how much of an unread response body is drained before
// closing, so the underlying connection can be reused without buffering an
// arbitrarily large response.
const drainLimit = 1 << 20 // 1 MiB

// DoJSON marshals payload as JSON, sends it to url with the given method and
// headers, and returns the HTTP status code plus up to maxBody bytes of the
// response body. The remainder of the body is drained so the connection can
// be reused by the client's transport.
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, payload any, maxBody int64) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, drainLimit))
	if readErr != nil {
		return resp.StatusCode, respBody, fmt.Errorf("failed to read response: %w", readErr)
	}

	return resp.StatusCode, respBody, nil
}
