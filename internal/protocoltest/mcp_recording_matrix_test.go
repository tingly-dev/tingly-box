package protocoltest

import (
	"testing"

	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

func TestMCPStageRecordingMatrixPersistsStableBoundaries(t *testing.T) {
	recordDir := t.TempDir()
	matrix := DefaultMatrix().
		WithMCPEnabled().
		WithProtocolStage().
		WithMCPStageCoverage().
		WithRecordDir(recordDir).
		OnlyScenarios(MCPStageOwnedToolScenarioName)

	results := matrix.ExecuteAll()
	if len(results) != 26 {
		t.Fatalf("matrix results = %d, want 26", len(results))
	}
	for _, result := range results {
		if result.Skipped || !result.Passed {
			t.Fatalf("matrix case %s passed=%v skipped=%v errors=%#v", result.Name, result.Passed, result.Skipped, result.Errors)
		}
	}

	records, err := readPersistedRequestRecordArtifacts(recordDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != len(results) {
		t.Fatalf("persisted records = %d, want %d", len(records), len(results))
	}
	for _, record := range records {
		if record == nil || record.Outcome != requestrecord.OutcomeSucceeded {
			t.Fatalf("persisted record = %#v", record)
		}
		if len(record.ProviderExchanges) != 2 {
			t.Fatalf("record %s provider exchanges = %d, want 2", record.RequestID, len(record.ProviderExchanges))
		}
		if record.FinalResponse == nil {
			t.Fatalf("record %s final response is missing", record.RequestID)
		}
	}
}
