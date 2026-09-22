package quota

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestGormStorePreservesSuccessfulRawResponse(t *testing.T) {
	t.Parallel()

	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
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

func TestGormStoreSaveAppendsHistoryWhileUpdatingLatest(t *testing.T) {
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	for i, used := range []float64{20, 35} {
		u := &ProviderUsage{
			ProviderUUID: "provider-1",
			ProviderName: "Provider",
			FetchedAt:    base.Add(time.Duration(i) * time.Hour),
			ExpiresAt:    base.Add(2 * time.Hour),
			RawResponse:  json.RawMessage(`{"source":"upstream"}`),
		}
		u.AddWindow("session", &UsageWindow{Used: used, Limit: 100, Label: "Session"})
		if err := store.Save(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := store.Get(context.Background(), "provider-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := latest.Windows[0].Used; got != 35 {
		t.Fatalf("latest used = %v, want 35", got)
	}
	history, err := store.History(context.Background(), HistoryQuery{ProviderUUID: "provider-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2", len(history))
	}
	if history[0].Windows[0].Used != 35 || history[1].Windows[0].Used != 20 {
		t.Fatalf("history not newest first: %#v", history)
	}
	if got := string(history[0].RawResponse); got != `{"source":"upstream"}` {
		t.Fatalf("history raw response = %s", got)
	}
}

func TestGormStoreHistoryUsesCurrentQuotaColumnMapping(t *testing.T) {
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	currentColumns, err := store.db.Migrator().ColumnTypes(&ProviderUsageRecord{})
	if err != nil {
		t.Fatal(err)
	}
	historyColumns, err := store.db.Migrator().ColumnTypes(&ProviderUsageHistoryRecord{})
	if err != nil {
		t.Fatal(err)
	}
	historyNames := make(map[string]bool, len(historyColumns))
	historyTypes := make(map[string]string, len(historyColumns))
	for _, column := range historyColumns {
		historyNames[column.Name()] = true
		historyTypes[column.Name()] = column.DatabaseTypeName()
	}
	for _, column := range currentColumns {
		if !historyNames[column.Name()] {
			t.Errorf("history table missing current quota column %q", column.Name())
			continue
		}
		if historyTypes[column.Name()] != column.DatabaseTypeName() {
			t.Errorf("history column %q type = %q, current type = %q", column.Name(), historyTypes[column.Name()], column.DatabaseTypeName())
		}
	}
	if historyNames["snapshot"] {
		t.Error("history table should map quota columns directly, not store a snapshot JSON blob")
	}
}

func TestGormStoreHistoryFiltersTimeAndProvider(t *testing.T) {
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	base := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	for i, provider := range []string{"one", "two", "one"} {
		u := &ProviderUsage{ProviderUUID: provider, FetchedAt: base.Add(time.Duration(i) * time.Hour), ExpiresAt: base.Add(3 * time.Hour)}
		if err := store.Save(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	start, end := base.Add(30*time.Minute), base.Add(3*time.Hour)
	got, err := store.History(context.Background(), HistoryQuery{ProviderUUID: "one", StartTime: &start, EndTime: &end, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].FetchedAt.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("filtered history = %#v", got)
	}
}
