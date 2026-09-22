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

func TestGormStoreCompactsCompletedDaysToQuotaExtremes(t *testing.T) {
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	zone := time.FixedZone("UTC+8", 8*60*60)
	now := time.Date(2026, 9, 22, 12, 30, 0, 0, zone)
	at := func(day, hour int) time.Time {
		return time.Date(2026, 9, day, hour, 0, 0, 0, zone)
	}
	save := func(provider string, when time.Time, short, weekly float64) {
		t.Helper()
		usage := &ProviderUsage{ProviderUUID: provider, FetchedAt: when, ExpiresAt: when.Add(time.Hour)}
		usage.AddWindow("short", &UsageWindow{Used: short, Limit: 100, Label: "5h"})
		usage.AddWindow("weekly", &UsageWindow{Used: weekly, Limit: 100, Label: "7d"})
		if err := store.Save(context.Background(), usage); err != nil {
			t.Fatal(err)
		}
	}
	save("one", now.Add(-31*24*time.Hour), 1, 1)
	save("one", at(21, 9), 10, 80)  // short minimum, weekly maximum
	save("one", at(21, 10), 90, 60) // short maximum
	save("one", at(21, 11), 50, 10) // weekly minimum
	save("one", at(21, 12), 55, 55) // redundant
	save("two", at(21, 9), 15, 15)
	save("two", at(21, 10), 85, 85)
	save("one", at(22, 9), 25, 25)
	save("one", at(22, 10), 30, 30)

	for range 2 {
		if err := store.CompactHistory(context.Background(), now); err != nil {
			t.Fatal(err)
		}
	}
	one, err := store.History(context.Background(), HistoryQuery{ProviderUUID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 5 {
		t.Fatalf("provider one has %d history records, want 3 daily extremes + 2 today", len(one))
	}
	for _, sample := range one {
		if sample.FetchedAt.Equal(at(21, 12)) || sample.FetchedAt.Before(now.Add(-30*24*time.Hour)) {
			t.Fatalf("redundant or expired sample retained at %s", sample.FetchedAt)
		}
	}
	two, err := store.History(context.Background(), HistoryQuery{ProviderUUID: "two"})
	if err != nil || len(two) != 2 {
		t.Fatalf("provider two history = %d records, error %v", len(two), err)
	}
	latest, err := store.Get(context.Background(), "one")
	if err != nil || latest.Windows[0].Used != 30 {
		t.Fatalf("latest quota changed by history compaction: usage=%#v error=%v", latest, err)
	}
}

func TestGormStoreCompactsUsingReportedAvailable(t *testing.T) {
	store, err := NewGormStore(filepath.Join(t.TempDir(), "test.db"), logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	day := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	for hour, sample := range []struct{ used, available float64 }{
		{10, 40}, {50, 90}, {90, 20}, {95, -1},
	} {
		var available *float64
		if sample.available >= 0 {
			value := sample.available
			available = &value
		}
		usage := &ProviderUsage{ProviderUUID: "one", FetchedAt: day.Add(time.Duration(hour) * time.Hour)}
		usage.AddWindow("session", &UsageWindow{Used: sample.used, Limit: 100, Available: available, Unit: UsageUnitRequests})
		if err := store.Save(context.Background(), usage); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CompactHistory(context.Background(), day.Add(36*time.Hour)); err != nil {
		t.Fatal(err)
	}
	history, err := store.History(context.Background(), HistoryQuery{ProviderUUID: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Windows[0].Used != 95 || *history[1].Windows[0].Available != 90 {
		t.Fatalf("daily remaining extrema = %#v; want 5 and 90", history)
	}
}
