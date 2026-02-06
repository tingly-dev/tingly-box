package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRecordRoundsUpsert tests that RoundResult can be updated for existing records
func TestRecordRoundsUpsert(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "prompt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the db subdirectory
	dbDir := filepath.Join(tempDir, "db")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("Failed to create db dir: %v", err)
	}

	store, err := NewMemoryStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "test-session-upsert"

	// === Scenario 1: First request - only UserInput, no RoundResult yet ===
	records1 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "provider-1",
			ProviderName:  "Test Provider",
			Model:         "test-model",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    0,
			UserInput:     "What is Golang?",
			UserInputHash: ComputeUserInputHash("What is Golang?"),
			RoundResult:   "", // Empty - response not received yet
			FullMessages:  `{"role":"user","content":[{"type":"text","text":"What is Golang?"}]}`,
		},
	}

	err = store.RecordRounds(records1)
	if err != nil {
		t.Fatalf("Failed to record initial rounds: %v", err)
	}

	// Verify the record was created
	var saved MemoryRoundRecord
	err = store.db.Where("session_id = ? AND user_input_hash = ?",
		sessionID, ComputeUserInputHash("What is Golang?")).
		First(&saved).Error
	if err != nil {
		t.Fatalf("Failed to find saved record: %v", err)
	}

	if saved.RoundResult != "" {
		t.Errorf("Expected empty RoundResult, got %q", saved.RoundResult)
	}
	t.Logf("✓ Initial record created with empty RoundResult")

	// === Scenario 2: Second request - same UserInput, now with RoundResult ===
	records2 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "provider-1",
			ProviderName:  "Test Provider",
			Model:         "test-model",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    0,
			UserInput:     "What is Golang?",
			UserInputHash: ComputeUserInputHash("What is Golang?"),
			RoundResult:   "Go is a programming language by Google.", // Now we have the response
			FullMessages:  `{"role":"user","content":[{"type":"text","text":"What is Golang?"}]}`,
			InputTokens:   10,
			OutputTokens:  50,
		},
	}

	err = store.RecordRounds(records2)
	if err != nil {
		t.Fatalf("Failed to update rounds: %v", err)
	}

	// Verify RoundResult was updated
	err = store.db.Where("session_id = ? AND user_input_hash = ?",
		sessionID, ComputeUserInputHash("What is Golang?")).
		First(&saved).Error
	if err != nil {
		t.Fatalf("Failed to find updated record: %v", err)
	}

	if saved.RoundResult != "Go is a programming language by Google." {
		t.Errorf("RoundResult not updated. Expected %q, got %q",
			"Go is a programming language by Google.", saved.RoundResult)
	}
	if saved.InputTokens != 10 {
		t.Errorf("InputTokens not updated. Expected 10, got %d", saved.InputTokens)
	}
	if saved.OutputTokens != 50 {
		t.Errorf("OutputTokens not updated. Expected 50, got %d", saved.OutputTokens)
	}
	t.Logf("✓ RoundResult updated: %q", saved.RoundResult)

	// === Scenario 3: Third request - don't overwrite existing RoundResult ===
	records3 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "provider-1",
			ProviderName:  "Test Provider",
			Model:         "test-model",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    0,
			UserInput:     "What is Golang?",
			UserInputHash: ComputeUserInputHash("What is Golang?"),
			RoundResult:   "Different response", // Should NOT overwrite
			FullMessages:  `{"role":"user","content":[{"type":"text","text":"What is Golang?"}]}`,
		},
	}

	err = store.RecordRounds(records3)
	if err != nil {
		t.Fatalf("Failed to process records: %v", err)
	}

	// Verify RoundResult was NOT overwritten
	err = store.db.Where("session_id = ? AND user_input_hash = ?",
		sessionID, ComputeUserInputHash("What is Golang?")).
		First(&saved).Error
	if err != nil {
		t.Fatalf("Failed to find record: %v", err)
	}

	if saved.RoundResult == "Different response" {
		t.Error("RoundResult should not be overwritten when it already has a value")
	}
	if saved.RoundResult != "Go is a programming language by Google." {
		t.Errorf("RoundResult was incorrectly overwritten. Expected %q, got %q",
			"Go is a programming language by Google.", saved.RoundResult)
	}
	t.Logf("✓ Existing RoundResult preserved: %q", saved.RoundResult)

	t.Log("\n========== Upsert Test Summary ==========")
	t.Log("✓ Empty RoundResult can be updated")
	t.Log("✓ Existing RoundResult is preserved (not overwritten)")
	t.Log("✓ Token counts can be updated")
}

// TestMultipleRoundsUpsert tests upsert behavior with multiple rounds
func TestMultipleRoundsUpsert(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "prompt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the db subdirectory
	dbDir := filepath.Join(tempDir, "db")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		t.Fatalf("Failed to create db dir: %v", err)
	}

	store, err := NewMemoryStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "test-multi-upsert"

	// Request 1: Two rounds, both without results
	records1 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "p1",
			ProviderName:  "Test",
			Model:         "m1",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    0,
			UserInput:     "Q1",
			UserInputHash: ComputeUserInputHash("Q1"),
			RoundResult:   "",
		},
		{
			Scenario:      "test",
			ProviderUUID:  "p1",
			ProviderName:  "Test",
			Model:         "m1",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    1,
			UserInput:     "Q2",
			UserInputHash: ComputeUserInputHash("Q2"),
			RoundResult:   "",
		},
	}

	err = store.RecordRounds(records1)
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Request 2: Same rounds, now with results for Q1, Q2 still empty
	records2 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "p1",
			ProviderName:  "Test",
			Model:         "m1",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    0,
			UserInput:     "Q1",
			UserInputHash: ComputeUserInputHash("Q1"),
			RoundResult:   "A1", // Update this
		},
		{
			Scenario:      "test",
			ProviderUUID:  "p1",
			ProviderName:  "Test",
			Model:         "m1",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    1,
			UserInput:     "Q2",
			UserInputHash: ComputeUserInputHash("Q2"),
			RoundResult:   "", // Still empty
		},
	}

	err = store.RecordRounds(records2)
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Verify: Q1 should have result, Q2 should still be empty
	var rounds []MemoryRoundRecord
	err = store.db.Where("session_id = ?", sessionID).Order("round_index").Find(&rounds).Error
	if err != nil {
		t.Fatalf("Failed to find rounds: %v", err)
	}

	if len(rounds) != 2 {
		t.Fatalf("Expected 2 rounds, got %d", len(rounds))
	}

	if rounds[0].RoundResult != "A1" {
		t.Errorf("Round 0: expected RoundResult %q, got %q", "A1", rounds[0].RoundResult)
	}
	if rounds[1].RoundResult != "" {
		t.Errorf("Round 1: expected empty RoundResult, got %q", rounds[1].RoundResult)
	}

	t.Log("✓ Multiple rounds upsert works correctly")

	// Request 3: Now Q2 gets result
	records3 := []*MemoryRoundRecord{
		{
			Scenario:      "test",
			ProviderUUID:  "p1",
			ProviderName:  "Test",
			Model:         "m1",
			Protocol:      ProtocolAnthropic,
			SessionID:     sessionID,
			RoundIndex:    1,
			UserInput:     "Q2",
			UserInputHash: ComputeUserInputHash("Q2"),
			RoundResult:   "A2", // Update Q2
		},
	}

	err = store.RecordRounds(records3)
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}

	// Verify both have results now
	err = store.db.Where("session_id = ?", sessionID).Order("round_index").Find(&rounds).Error
	if err != nil {
		t.Fatalf("Failed to find rounds: %v", err)
	}

	if rounds[0].RoundResult != "A1" {
		t.Errorf("Round 0: expected RoundResult %q, got %q", "A1", rounds[0].RoundResult)
	}
	if rounds[1].RoundResult != "A2" {
		t.Errorf("Round 1: expected RoundResult %q, got %q", "A2", rounds[1].RoundResult)
	}

	t.Log("✓ Progressive RoundResult updates work correctly")
}
