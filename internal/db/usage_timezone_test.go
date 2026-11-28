package db

import (
	"strings"
	"testing"
	"time"
)

// These tests pin the timezone contract between the browser and the store:
// usage_records.timestamp is stored as UTC, and every range bound is compared
// as UTC. SQLite compares the TEXT timestamp column lexicographically, so a
// stored row and a bound carrying different UTC offsets compare wrongly even
// when the instants are in order — the bug behind #2797 (an empty dashboard
// right after local midnight when the server runs in UTC and the browser does
// not).

var (
	tzUTCplus8  = time.FixedZone("UTC+8", 8*60*60)
	tzUTCminus8 = time.FixedZone("UTC-8", -8*60*60)
)

func usageRecordAt(provider string, at time.Time, tokens int) *UsageRecord {
	return &UsageRecord{
		ProviderUUID: provider,
		ProviderName: provider,
		Model:        "model-x",
		Scenario:     "scenario",
		Timestamp:    at,
		InputTokens:  tokens,
		Status:       "success",
	}
}

// TestUsageRangeMatchesAcrossServerAndClientTimezones is the #2797 regression:
// the server (and so the DB) is in UTC while the browser is in another zone.
// The "today" window is computed in the browser's zone but must select the same
// instants from the UTC store.
func TestUsageRangeMatchesAcrossServerAndClientTimezones(t *testing.T) {
	// One record written by a UTC server in the early hours of the browser's
	// local day.
	store := newUsageStoreForTest(t)
	// 2026-11-26 01:58 in UTC+8 == 2026-11-25 17:58 UTC.
	recordInstant := time.Date(2026, 11, 25, 17, 58, 0, 0, time.UTC)
	if err := store.RecordUsage(usageRecordAt("prov-a", recordInstant, 10)); err != nil {
		t.Fatalf("record: %v", err)
	}

	cases := []struct {
		name  string
		start time.Time // browser-local midnight
		end   time.Time // browser "now", 2026-11-26 01:58 local
	}{
		{
			name:  "browser east of the server (UTC+8)",
			start: time.Date(2026, 11, 26, 0, 0, 0, 0, tzUTCplus8),
			end:   time.Date(2026, 11, 26, 1, 58, 0, 0, tzUTCplus8),
		},
		{
			name: "browser west of the server (UTC-8)",
			// A record at 09:00 UTC is 01:00 in UTC-8, also after local midnight.
			start: time.Date(2026, 11, 26, 0, 0, 0, 0, tzUTCminus8),
			end:   time.Date(2026, 11, 26, 1, 58, 0, 0, tzUTCminus8),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The west case needs its own record instant inside its window.
			if tc.start.Location() == tzUTCminus8 {
				if err := store.RecordUsage(usageRecordAt("prov-b", time.Date(2026, 11, 26, 9, 0, 0, 0, time.UTC), 5)); err != nil {
					t.Fatalf("record: %v", err)
				}
			}

			stats, err := store.GetAggregatedStats(UsageStatsQuery{
				GroupBy:   "model",
				StartTime: tc.start,
				EndTime:   tc.end,
				Limit:     1000,
				SortBy:    "total_tokens",
				SortOrder: "desc",
			})
			if err != nil {
				t.Fatalf("stats: %v", err)
			}
			if len(stats) == 0 {
				t.Fatalf("GetAggregatedStats returned nothing; the record at %s is inside [%s, %s]",
					recordInstant, tc.start, tc.end)
			}

			records, total, err := store.GetRecords(tc.start, tc.end, nil, 50, 0)
			if err != nil {
				t.Fatalf("records: %v", err)
			}
			if total == 0 || len(records) == 0 {
				t.Fatalf("GetRecords returned nothing; expected the record inside [%s, %s]", tc.start, tc.end)
			}
		})
	}
}

// TestRecordUsageStoresUTC pins that any timestamp handed to the store is
// persisted in UTC, whatever zone the caller used.
func TestRecordUsageStoresUTC(t *testing.T) {
	store := newUsageStoreForTest(t)
	local := time.Date(2026, 11, 26, 1, 58, 0, 0, tzUTCplus8) // 2026-11-25 17:58 UTC
	if err := store.RecordUsage(usageRecordAt("prov-a", local, 10)); err != nil {
		t.Fatalf("record: %v", err)
	}

	var stored string
	if err := store.db.Raw(`SELECT CAST(timestamp AS TEXT) FROM usage_records LIMIT 1`).Scan(&stored).Error; err != nil {
		t.Fatalf("read raw timestamp: %v", err)
	}
	if !strings.HasSuffix(stored, "+00:00") {
		t.Fatalf("stored timestamp %q is not UTC (+00:00)", stored)
	}
}

// TestNormalizeUsageTimestampsToUTC covers the compatibility migration: rows
// written by an older server in a non-UTC zone are rewritten to UTC without
// changing the instant, so UTC range bounds match them afterwards.
func TestNormalizeUsageTimestampsToUTC(t *testing.T) {
	store := newUsageStoreForTest(t)

	// Simulate a legacy row written by a UTC+8 server.
	const legacy = "2026-11-26 01:58:00+08:00"
	if err := store.db.Exec(`INSERT INTO usage_records
		(provider_uuid, provider_name, model, scenario, timestamp, input_tokens, output_tokens, total_tokens, status)
		VALUES ('prov-a', 'prov-a', 'model-x', 'scenario', ?, 10, 0, 10, 'success')`, legacy).Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	if err := normalizeUsageTimestampsToUTC(store.db); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	var stored string
	if err := store.db.Raw(`SELECT CAST(timestamp AS TEXT) FROM usage_records LIMIT 1`).Scan(&stored).Error; err != nil {
		t.Fatalf("read raw timestamp: %v", err)
	}
	if !strings.HasSuffix(stored, "+00:00") {
		t.Fatalf("normalized timestamp %q is not UTC (+00:00)", stored)
	}

	// The same instant (2026-11-25 17:58 UTC) must still be found by a UTC range.
	start := time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 25, 18, 0, 0, 0, time.UTC)
	_, total, err := store.GetRecords(start, end, nil, 10, 0)
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if total != 1 {
		t.Fatalf("after normalization the record is not found by its UTC range (total=%d)", total)
	}

	// Idempotent: a second run changes nothing.
	if err := normalizeUsageTimestampsToUTC(store.db); err != nil {
		t.Fatalf("normalize twice: %v", err)
	}
	var again string
	if err := store.db.Raw(`SELECT CAST(timestamp AS TEXT) FROM usage_records LIMIT 1`).Scan(&again).Error; err != nil {
		t.Fatalf("read raw timestamp: %v", err)
	}
	if again != stored {
		t.Fatalf("migration is not idempotent: %q -> %q", stored, again)
	}
}
