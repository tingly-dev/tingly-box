package quota

import (
	"context"
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
		RawResponse:  rawResponse,
	}

	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := store.Get(context.Background(), want.ProviderUUID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got.RawResponse != rawResponse {
		t.Errorf("RawResponse changed during database round trip:\ngot:  %q\nwant: %q", got.RawResponse, rawResponse)
	}
}
