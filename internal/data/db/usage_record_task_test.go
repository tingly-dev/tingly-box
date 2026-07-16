package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetTaskUsageTotals(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewUsageStore(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	records := []*UsageRecord{
		{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", TaskID: "task-1", RunID: "run-1", InputTokens: 100, OutputTokens: 40, CacheInputTokens: 10},
		{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", TaskID: "task-1", RunID: "run-2", InputTokens: 60, OutputTokens: 20},
		{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", TaskID: "task-2", InputTokens: 999, OutputTokens: 999},
		{ProviderUUID: "p", ProviderName: "P", Model: "m", Scenario: "s", InputTokens: 5, OutputTokens: 5},
	}
	for _, record := range records {
		if err := store.RecordUsage(record); err != nil {
			t.Fatal(err)
		}
	}
	totals, err := store.GetTaskUsageTotals("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if totals.Requests != 2 || totals.InputTokens != 160 || totals.OutputTokens != 60 || totals.CacheInputTokens != 10 || totals.TotalTokens != 220 {
		t.Fatalf("totals = %+v", totals)
	}
	empty, err := store.GetTaskUsageTotals("task-none")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Requests != 0 || empty.TotalTokens != 0 {
		t.Fatalf("empty totals = %+v", empty)
	}
}
