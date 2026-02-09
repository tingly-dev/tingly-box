package server

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestRoundExtraction tests the round extraction logic with a complex multi-round conversation
func TestRoundExtraction(t *testing.T) {
	// Construct a complex request with multiple rounds:
	// Round 0: User "Hello" -> Assistant "Hi!"
	// Round 1 (current): User "How are you?" -> (no assistant response yet in request)

	messages := []anthropic.MessageParam{
		// Round 0 - User message
		anthropic.NewUserMessage(
			anthropic.NewTextBlock("Hello, please help me with a task"),
		),
		// Round 0 - Assistant response
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("I'll help you with that."),
		),
		// Round 1 - User message (current round)
		anthropic.NewUserMessage(
			anthropic.NewTextBlock("How are you doing today?"),
		),
	}

	grouper := protocol.NewGrouper()
	rounds := grouper.GroupV1(messages)

	fmt.Printf("========== Round Extraction Test ==========\n")
	fmt.Printf("Total messages: %d\n", len(messages))
	fmt.Printf("Total rounds extracted: %d\n\n", len(rounds))

	// Verify we got 2 rounds
	if len(rounds) != 2 {
		t.Errorf("Expected 2 rounds, got %d", len(rounds))
	}

	// Print round details
	for i, round := range rounds {
		fmt.Printf("--- Round %d ---\n", i)
		fmt.Printf("  IsCurrentRound: %v\n", round.IsCurrentRound)
		fmt.Printf("  Message count: %d\n", len(round.Messages))

		// Print messages in this round
		for j, msg := range round.Messages {
			fmt.Printf("    Message %d: role=%s, content_blocks=%d\n", j, msg.Role, len(msg.Content))
			for k, block := range msg.Content {
				switch {
				case block.OfText != nil:
					fmt.Printf("      Block %d: [text] %q\n", k, block.OfText.Text)
				case block.OfToolUse != nil:
					fmt.Printf("      Block %d: [tool_use] id=%s, name=%s\n", k, block.OfToolUse.ID, block.OfToolUse.Name)
				case block.OfToolResult != nil:
					fmt.Printf("      Block %d: [tool_result] tool_use_id=%s\n", k, block.OfToolResult.ToolUseID)
				}
			}
		}
		fmt.Println()
	}

	// Verify Round 0
	if len(rounds) > 0 {
		round0 := rounds[0]
		if round0.IsCurrentRound {
			t.Error("Round 0 should not be marked as current round")
		}
		if len(round0.Messages) != 2 {
			t.Errorf("Round 0 should have 2 messages (user, assistant), got %d", len(round0.Messages))
		}
	}

	// Verify Round 1
	if len(rounds) > 1 {
		round1 := rounds[1]
		if !round1.IsCurrentRound {
			t.Error("Round 1 should be marked as current round")
		}
		if len(round1.Messages) != 1 {
			t.Errorf("Round 1 should have 1 message (user only), got %d", len(round1.Messages))
		}
	}

	// Now test extraction to RoundData
	fmt.Printf("========== RoundData Extraction Test ==========\n\n")

	sr := &ScenarioRecorder{}
	roundsData := sr.extractRoundsFromV1Messages(messages)

	fmt.Printf("Total RoundData extracted: %d\n\n", len(roundsData))

	for i, rd := range roundsData {
		fmt.Printf("--- RoundData %d ---\n", i)
		fmt.Printf("  RoundIndex: %d\n", rd.RoundIndex)
		fmt.Printf("  UserInput: %q\n", rd.UserInput)
		fmt.Printf("  UserInputHash: %s\n", rd.UserInputHash)
		fmt.Printf("  RoundResult: %q\n", rd.RoundResult)
		fmt.Printf("  FullMessages count: %d\n", len(rd.FullMessages))
		fmt.Println()
	}

	// Verify RoundData
	if len(roundsData) != 2 {
		t.Errorf("Expected 2 RoundData, got %d", len(roundsData))
	}

	// Check first round has correct user input
	if len(roundsData) > 0 {
		if roundsData[0].UserInput != "Hello, please help me with a task\n" {
			t.Errorf("Round 0 UserInput mismatch, got %q", roundsData[0].UserInput)
		}
		if roundsData[0].RoundIndex != 0 {
			t.Errorf("Round 0 RoundIndex should be 0, got %d", roundsData[0].RoundIndex)
		}
	}

	// Check second round (current) has correct user input
	if len(roundsData) > 1 {
		if roundsData[1].UserInput != "How are you doing today?\n" {
			t.Errorf("Round 1 UserInput mismatch, got %q", roundsData[1].UserInput)
		}
		if roundsData[1].RoundIndex != 1 {
			t.Errorf("Round 1 RoundIndex should be 1, got %d", roundsData[1].RoundIndex)
		}
	}

	fmt.Printf("========== Test Complete ==========\n")
}

// TestBetaRoundExtraction tests the beta message round extraction
func TestBetaRoundExtraction(t *testing.T) {
	t.Skip("Beta message API needs further investigation - skipping for now")
}

// TestUserInputHash tests that identical user inputs produce the same hash
func TestUserInputHash(t *testing.T) {
	input1 := "Hello, how are you?"
	input2 := "Hello, how are you?"
	input3 := "Different message"

	hash1 := db.ComputeUserInputHash(input1)
	hash2 := db.ComputeUserInputHash(input2)
	hash3 := db.ComputeUserInputHash(input3)

	fmt.Printf("========== Hash Test ==========\n")
	fmt.Printf("Input 1: %q\n", input1)
	fmt.Printf("Hash 1: %s\n\n", hash1)

	fmt.Printf("Input 2: %q\n", input2)
	fmt.Printf("Hash 2: %s\n\n", hash2)

	fmt.Printf("Input 3: %q\n", input3)
	fmt.Printf("Hash 3: %s\n\n", hash3)

	if hash1 != hash2 {
		t.Error("Same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("Different inputs should produce different hashes")
	}

	fmt.Printf("========== Hash Test Complete ==========\n")
}

// TestFullMessagesSerialization tests that full messages can be serialized to JSON
func TestFullMessagesSerialization(t *testing.T) {
	sr := &ScenarioRecorder{}

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(
			anthropic.NewTextBlock("Test message"),
		),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("Test response"),
		),
	}

	normalized := sr.normalizeV1Messages(messages)

	fmt.Printf("========== Serialization Test ==========\n")
	fmt.Printf("Normalized %d messages\n\n", len(normalized))

	// Serialize to JSON
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	fmt.Printf("JSON output:\n%s\n\n", string(data))

	// Deserialize and verify
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 messages after round-trip, got %d", len(result))
	}

	fmt.Printf("========== Serialization Test Complete ==========\n")
}

// TestLargeConversation tests a more realistic large conversation
func TestLargeConversation(t *testing.T) {
	// Create a conversation with 5 rounds
	var messages []anthropic.MessageParam

	questions := []string{
		"What is Golang?",
		"Show me an example",
		"Explain goroutines",
		"How do I handle errors?",
		"Tell me about channels",
	}

	for i, q := range questions {
		// User message
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(q)))

		// Assistant response (except for the last one which is current)
		if i < len(questions)-1 {
			messages = append(messages, anthropic.NewAssistantMessage(
				anthropic.NewTextBlock(fmt.Sprintf("Answer to: %s", q)),
			))
		}
	}

	grouper := protocol.NewGrouper()
	rounds := grouper.GroupV1(messages)

	fmt.Printf("========== Large Conversation Test ==========\n")
	fmt.Printf("Total messages: %d\n", len(messages))
	fmt.Printf("Total rounds: %d\n\n", len(rounds))

	if len(rounds) != 5 {
		t.Errorf("Expected 5 rounds, got %d", len(rounds))
	}

	for i, round := range rounds {
		isCurrent := ""
		if round.IsCurrentRound {
			isCurrent = " [CURRENT]"
		}
		fmt.Printf("Round %d%s: %d messages\n", i, isCurrent, len(round.Messages))
	}

	sr := &ScenarioRecorder{}
	roundsData := sr.extractRoundsFromV1Messages(messages)

	fmt.Printf("\nExtracted %d RoundData\n\n", len(roundsData))

	for i, rd := range roundsData {
		isCurrent := ""
		if i == len(roundsData)-1 {
			isCurrent = " [CURRENT]"
		}
		fmt.Printf("Round %d%s: UserInput=%q\n", i, isCurrent, rd.UserInput)
	}

	fmt.Printf("\n========== Large Conversation Test Complete ==========\n")
}
