package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxErrorBodyBytes bounds how much of an upstream error body is quoted back.
const maxErrorBodyBytes = 2048

// postJSON sends a bearer-authenticated JSON request and decodes a 200 reply
// into out. A non-200 reply becomes an error carrying the upstream body, which
// is where every vendor puts the reason (bad size, content policy, quota) the
// user actually needs to see.
func postJSON(ctx context.Context, httpClient *http.Client, url, token string, headers map[string]string, body, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if len(raw) > maxErrorBodyBytes {
			raw = raw[:maxErrorBodyBytes]
		}
		return fmt.Errorf("upstream returned %d: %s", resp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}
