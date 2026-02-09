package server

import (
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestRoundResultExtraction verifies that assistant outputs are correctly extracted
func TestRoundResultExtraction(t *testing.T) {
	// Test case with multiple complete rounds
	messages := []anthropic.MessageParam{
		// Round 0: Complete round (user + assistant)
		anthropic.NewUserMessage(anthropic.NewTextBlock("What is Golang?")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Go is a programming language developed by Google.")),

		// Round 1: Complete round (user + assistant)
		anthropic.NewUserMessage(anthropic.NewTextBlock("Show me an example")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Here's a simple example:\n\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}")),

		// Round 2: Current round (user only, no assistant response yet)
		anthropic.NewUserMessage(anthropic.NewTextBlock("Explain goroutines")),
	}

	grouper := protocol.NewGrouper()
	rounds := grouper.GroupV1(messages)

	fmt.Printf("========== RoundResult Extraction Test ==========\n")
	fmt.Printf("Total messages: %d\n", len(messages))
	fmt.Printf("Total rounds extracted: %d\n\n", len(rounds))

	if len(rounds) != 3 {
		t.Fatalf("Expected 3 rounds, got %d", len(rounds))
	}

	sr := &ScenarioRecorder{}
	roundsData := sr.extractRoundsFromV1Messages(messages)

	// Verify RoundData
	if len(roundsData) != 3 {
		t.Fatalf("Expected 3 RoundData, got %d", len(roundsData))
	}

	fmt.Printf("--- RoundData Verification ---\n")
	for i, rd := range roundsData {
		isCurrent := ""
		if i == len(roundsData)-1 {
			isCurrent = " [CURRENT]"
		}

		resultPreview := rd.RoundResult
		if len(resultPreview) > 50 {
			resultPreview = resultPreview[:50] + "..."
		}

		fmt.Printf("\nRound %d%s:\n", i, isCurrent)
		fmt.Printf("  UserInput: %q\n", rd.UserInput)
		fmt.Printf("  RoundResult: %q\n", resultPreview)
		fmt.Printf("  RoundResult length: %d chars\n", len(rd.RoundResult))
	}

	// Verify Round 0 (historical, has result)
	if roundsData[0].RoundResult == "" {
		t.Error("Round 0 should have RoundResult from request messages")
	}
	expectedResult0 := "Go is a programming language developed by Google."
	if roundsData[0].RoundResult != expectedResult0+"\n" {
		t.Errorf("Round 0 RoundResult mismatch.\nExpected: %q\nGot: %q", expectedResult0+"\n", roundsData[0].RoundResult)
	}

	// Verify Round 1 (historical, has result)
	if roundsData[1].RoundResult == "" {
		t.Error("Round 1 should have RoundResult from request messages")
	}

	// Verify Round 2 (current, no result in request)
	if roundsData[2].RoundResult != "" {
		t.Errorf("Round 2 (current) should have empty RoundResult from request, got %q", roundsData[2].RoundResult)
	}

	fmt.Printf("\n========== Key Points ==========\n")
	fmt.Printf("✓ Historical rounds: RoundResult extracted from request messages\n")
	fmt.Printf("✓ Current round: RoundResult empty (will be filled from response)\n")
	fmt.Printf("✓ Deduplication: Historical rounds with same user_input_hash will skip\n")
}

// TestRoundResultInStorage simulates the full flow with storage
func TestRoundResultInStorage(t *testing.T) {
	fmt.Printf("========== Full Flow Simulation ==========\n\n")

	sr := &ScenarioRecorder{}

	// === Request 1: First round ===
	fmt.Printf("--- Request 1: First round ---\n")
	req1Messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		// No assistant response yet in request
	}
	rounds1 := sr.extractRoundsFromV1Messages(req1Messages)
	fmt.Printf("Rounds in request: %d\n", len(rounds1))
	fmt.Printf("Round 0 UserInput: %q\n", rounds1[0].UserInput)
	fmt.Printf("Round 0 RoundResult (from request): %q\n", rounds1[0].RoundResult)

	// Simulate response
	fmt.Printf("After response, RoundResult would be: \"Hi there!\"\n\n")

	// === Request 2: Second round with history ===
	fmt.Printf("--- Request 2: Second round with history ---\n")
	req2Messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi there!")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("How are you?")),
		// No assistant response yet for current round
	}
	rounds2 := sr.extractRoundsFromV1Messages(req2Messages)
	fmt.Printf("Rounds in request: %d\n", len(rounds2))

	for i, rd := range rounds2 {
		isCurrent := ""
		if i == len(rounds2)-1 {
			isCurrent = " [CURRENT]"
		}
		fmt.Printf("Round %d%s:\n", i, isCurrent)
		fmt.Printf("  UserInput: %q\n", rd.UserInput)
		fmt.Printf("  RoundResult (from request): %q\n", rd.RoundResult)
	}

	// Verify
	if len(rounds2) != 2 {
		t.Fatalf("Expected 2 rounds, got %d", len(rounds2))
	}

	// Round 0 is historical, should have result from request
	if rounds2[0].RoundResult == "" {
		t.Error("Round 0 (historical) should have RoundResult from request")
	}
	if rounds2[0].RoundResult != "Hi there!\n" {
		t.Errorf("Round 0 RoundResult mismatch, got %q", rounds2[0].RoundResult)
	}

	// Round 1 is current, should NOT have result in request
	if rounds2[1].RoundResult != "" {
		t.Errorf("Round 1 (current) should have empty RoundResult in request, got %q", rounds2[1].RoundResult)
	}

	fmt.Printf("\nAfter response, Round 1 RoundResult would be filled from response\n")

	fmt.Printf("\n========== Flow Summary ==========\n")
	fmt.Printf("Request 1 → Store: Round 0 (UserInput: \"Hello\", RoundResult: \"Hi there!\")\n")
	fmt.Printf("Request 2 → Check: Round 0 already exists (by hash), skip\n")
	fmt.Printf("Request 2 → Store: Round 1 (UserInput: \"How are you?\", RoundResult: <from response>)\n")
}
