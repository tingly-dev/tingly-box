package quota

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestGormStorePreservesSuccessfulRawResponse(t *testing.T) {
	t.Parallel()

	store, err := NewGormStore(t.TempDir(), logrus.New())
	if err != nil {
		t.Fatalf("NewGormStore() error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	const rawResponse = `{
  "usage": {"limit": "100", "used": "6", "remaining": "94"},
  "unknownFutureField": {"nested": [1, 2, 3]}
}`
	now := time.Now().UTC().Truncate(time.Millisecond)
	want := &ProviderUsage{
		ProviderUUID: "kimi-code-uuid",
		ProviderName: "Kimi Code",
		ProviderType: ProviderTypeKimiCode,
		FetchedAt:    now,
		ExpiresAt:    now.Add(5 * time.Minute),
		RawResponse:  json.RawMessage(rawResponse),
	}

	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := store.Get(context.Background(), want.ProviderUUID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if string(got.RawResponse) != rawResponse {
		t.Errorf("RawResponse changed during database round trip:\ngot:  %q\nwant: %q", got.RawResponse, rawResponse)
	}
}

func TestProviderUsageMarshalsRawResponseAsJSON(t *testing.T) {
	t.Parallel()

	usage := ProviderUsage{
		ProviderUUID: "kimi-code-uuid",
		RawResponse:  json.RawMessage(`{"usage":{"limit":"100"},"limits":[]}`),
	}
	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if len(response["raw_response"]) == 0 || response["raw_response"][0] != '{' {
		t.Fatalf("raw_response = %s, want a JSON object", response["raw_response"])
	}
}
