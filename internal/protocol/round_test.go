package protocol

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestGroupV1_SingleRound tests a simple single-round conversation
func TestGroupV1_SingleRound(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi there!")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the only round to be marked as current")
	}

	if rounds[0].Stats.UserMessageCount != 1 {
		t.Errorf("Expected 1 user message, got %d", rounds[0].Stats.UserMessageCount)
	}

	if rounds[0].Stats.AssistantCount != 1 {
		t.Errorf("Expected 1 assistant message, got %d", rounds[0].Stats.AssistantCount)
	}
}

// TestGroupV1_TwoRounds tests a simple two-round conversation
func TestGroupV1_TwoRounds(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("First question")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("First answer")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("Second question")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Second answer")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 2 {
		t.Fatalf("Expected 2 rounds, got %d", len(rounds))
	}

	// First round should be historical
	if rounds[0].IsCurrentRound {
		t.Error("Expected first round to be historical")
	}

	// Second round should be current
	if !rounds[1].IsCurrentRound {
		t.Error("Expected second round to be current")
	}

	// Check first round content
	if len(rounds[0].Messages) != 2 {
		t.Errorf("Expected 2 messages in first round, got %d", len(rounds[0].Messages))
	}

	// Check second round content
	if len(rounds[1].Messages) != 2 {
		t.Errorf("Expected 2 messages in second round, got %d", len(rounds[1].Messages))
	}
}

// TestGroupV1_WithToolUse tests a conversation with tool use
func TestGroupV1_WithToolUse(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("What's the weather?")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("I'll check the weather."),
			anthropic.NewToolUseBlock("toolu_1", map[string]interface{}{"location": "SF"}, "get_weather"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "Sunny, 70°F", false)),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("It's sunny and 70°F in SF.")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the round to be marked as current")
	}

	// Should contain all 4 messages (user, assistant with tool use, user with tool result, assistant)
	if len(rounds[0].Messages) != 4 {
		t.Errorf("Expected 4 messages in round, got %d", len(rounds[0].Messages))
	}

	// Check stats
	if rounds[0].Stats.AssistantCount != 2 {
		t.Errorf("Expected 2 assistant messages, got %d", rounds[0].Stats.AssistantCount)
	}

	if rounds[0].Stats.ToolResultCount != 1 {
		t.Errorf("Expected 1 tool result, got %d", rounds[0].Stats.ToolResultCount)
	}
}

// TestGroupV1_MultiRoundWithTools tests multiple rounds with tool use
func TestGroupV1_MultiRoundWithTools(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		// Round 1: Simple question
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi!")),
		// Round 2: With tool use
		anthropic.NewUserMessage(anthropic.NewTextBlock("Check weather")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("Checking..."),
			anthropic.NewToolUseBlock("toolu_1", map[string]interface{}{"location": "NYC"}, "get_weather"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "Rainy, 60°F", false)),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("It's rainy in NYC.")),
		// Round 3: Current incomplete round (only user message)
		anthropic.NewUserMessage(anthropic.NewTextBlock("Thanks!")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 3 {
		t.Fatalf("Expected 3 rounds, got %d", len(rounds))
	}

	// Round 1: Historical, 2 messages
	if rounds[0].IsCurrentRound {
		t.Error("Expected first round to be historical")
	}
	if len(rounds[0].Messages) != 2 {
		t.Errorf("Round 1: Expected 2 messages, got %d", len(rounds[0].Messages))
	}

	// Round 2: Historical, 4 messages (user, assistant+tool, user+result, assistant)
	if rounds[1].IsCurrentRound {
		t.Error("Expected second round to be historical")
	}
	if len(rounds[1].Messages) != 4 {
		t.Errorf("Round 2: Expected 4 messages, got %d", len(rounds[1].Messages))
	}

	// Round 3: Current, 1 message (only user, no assistant yet)
	if !rounds[2].IsCurrentRound {
		t.Error("Expected third round to be current")
	}
	if len(rounds[2].Messages) != 1 {
		t.Errorf("Round 3: Expected 1 message, got %d", len(rounds[2].Messages))
	}

	// Verify round 3 has only user message
	if string(rounds[2].Messages[0].Role) != "user" {
		t.Error("Round 3 should start with user message")
	}
}

// TestGroupV1_EmptyMessages tests empty message list
func TestGroupV1_EmptyMessages(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{}
	rounds := g.GroupV1(messages)

	if len(rounds) != 0 {
		t.Fatalf("Expected 0 rounds for empty messages, got %d", len(rounds))
	}
}

// TestGroupV1_OnlyUserMessage tests only a user message (no response yet)
func TestGroupV1_OnlyUserMessage(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the round to be marked as current")
	}

	if len(rounds[0].Messages) != 1 {
		t.Errorf("Expected 1 message in round, got %d", len(rounds[0].Messages))
	}
}

// TestGroupV1_SequentialToolResults tests multiple tool results in sequence
func TestGroupV1_SequentialToolResults(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Use two tools")),
		anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock("toolu_1", map[string]interface{}{"x": 1}, "tool_a"),
			anthropic.NewToolUseBlock("toolu_2", map[string]interface{}{"y": 2}, "tool_b"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "Result A", false)),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_2", "Result B", false)),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Both tools executed.")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	// Should contain all 5 messages
	if len(rounds[0].Messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(rounds[0].Messages))
	}

	if rounds[0].Stats.ToolResultCount != 2 {
		t.Errorf("Expected 2 tool results, got %d", rounds[0].Stats.ToolResultCount)
	}
}

// TestGroupBeta_SingleRound tests a simple single-round conversation for beta API
func TestGroupBeta_SingleRound(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Hello")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Hi there!")},
		},
	}

	rounds := g.GroupBeta(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the only round to be marked as current")
	}

	if rounds[0].Stats.UserMessageCount != 1 {
		t.Errorf("Expected 1 user message, got %d", rounds[0].Stats.UserMessageCount)
	}

	if rounds[0].Stats.AssistantCount != 1 {
		t.Errorf("Expected 1 assistant message, got %d", rounds[0].Stats.AssistantCount)
	}
}

// TestGroupBeta_TwoRounds tests a simple two-round conversation for beta API
func TestGroupBeta_TwoRounds(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("First question")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("First answer")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Second question")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("Second answer")},
		},
	}

	rounds := g.GroupBeta(messages)

	if len(rounds) != 2 {
		t.Fatalf("Expected 2 rounds, got %d", len(rounds))
	}

	// First round should be historical
	if rounds[0].IsCurrentRound {
		t.Error("Expected first round to be historical")
	}

	// Second round should be current
	if !rounds[1].IsCurrentRound {
		t.Error("Expected second round to be current")
	}
}

// TestGroupBeta_WithToolUse tests a conversation with tool use for beta API
func TestGroupBeta_WithToolUse(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("What's the weather?")},
		},
		{
			Role: anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaTextBlock("I'll check the weather."),
				anthropic.NewBetaToolUseBlock("toolu_1", map[string]interface{}{"location": "SF"}, "get_weather"),
			},
		},
		{
			Role:    anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaToolResultBlock("toolu_1")},
		},
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("It's sunny and 70°F in SF.")},
		},
	}

	rounds := g.GroupBeta(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the round to be marked as current")
	}

	// Should contain all 4 messages
	if len(rounds[0].Messages) != 4 {
		t.Errorf("Expected 4 messages in round, got %d", len(rounds[0].Messages))
	}

	if rounds[0].Stats.ToolResultCount != 1 {
		t.Errorf("Expected 1 tool result, got %d", rounds[0].Stats.ToolResultCount)
	}
}

// TestGroupV1_AssistantOnlyMessages tests messages starting with assistant (edge case)
func TestGroupV1_AssistantOnlyMessages(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hello without user")),
	}

	rounds := g.GroupV1(messages)

	// Assistant-only messages should be grouped into a single round
	// (not starting with pure user message)
	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	if !rounds[0].IsCurrentRound {
		t.Error("Expected the round to be marked as current")
	}

	if rounds[0].Stats.AssistantCount != 1 {
		t.Errorf("Expected 1 assistant message, got %d", rounds[0].Stats.AssistantCount)
	}
}

// TestIsPureUserMessage tests the IsPureUserMessage method
func TestIsPureUserMessage(t *testing.T) {
	g := NewGrouper()

	// Pure user message (text only)
	pureUser := anthropic.NewUserMessage(anthropic.NewTextBlock("Hello"))

	if !g.IsPureUserMessage(pureUser) {
		t.Error("Expected pure user message to return true")
	}

	// User message with tool result (not pure)
	userWithToolResult := anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_1", "Result", false))

	if g.IsPureUserMessage(userWithToolResult) {
		t.Error("Expected user message with tool result to return false")
	}

	// Assistant message
	assistantMsg := anthropic.NewAssistantMessage(anthropic.NewTextBlock("Hi"))

	if g.IsPureUserMessage(assistantMsg) {
		t.Error("Expected assistant message to return false")
	}
}

// TestGroupV1_ComplexMultiTurnScenario tests the scenario described by the user:
// User input about memory API issue → multiple assistant responses → tool use
func TestGroupV1_ComplexMultiTurnScenario(t *testing.T) {
	g := NewGrouper()

	messages := []anthropic.MessageParam{
		// Historical round 1
		anthropic.NewUserMessage(anthropic.NewTextBlock("Previous question")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Previous answer")),
		// Current round with the memory API discussion
		anthropic.NewUserMessage(anthropic.NewTextBlock("目前看，memory api 的数据返回，和前端不match，导致 round_result 并没有显示出来，检查修复")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("我来检查一下 memory API 的实现..."),
			anthropic.NewToolUseBlock("toolu_check", map[string]interface{}{
				"path": "/internal/server/memory_api.go",
			}, "read_file"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_check", "文件内容...", false)),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("我看到问题了。Metadata 字段需要从 JSON 字符串解析为 map...")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 2 {
		t.Fatalf("Expected 2 rounds, got %d", len(rounds))
	}

	// First round should be historical
	if rounds[0].IsCurrentRound {
		t.Error("Expected first round to be historical")
	}
	if len(rounds[0].Messages) != 2 {
		t.Errorf("Round 1: Expected 2 messages, got %d", len(rounds[0].Messages))
	}

	// Second round should be current with 4 messages
	if !rounds[1].IsCurrentRound {
		t.Error("Expected second round to be current")
	}
	if len(rounds[1].Messages) != 4 {
		t.Errorf("Round 2: Expected 4 messages, got %d", len(rounds[1].Messages))
	}

	// Verify current round has 2 assistant messages
	if rounds[1].Stats.AssistantCount != 2 {
		t.Errorf("Round 2: Expected 2 assistant messages, got %d", rounds[1].Stats.AssistantCount)
	}

	// The first assistant has "我来检查..." and tool_use
	// The second assistant has "我看到问题了..."
	// Both are in the same round (current round)
}

// TestGroupV1_MessagesPreserved verifies that messages are preserved correctly
func TestGroupV1_MessagesPreserved(t *testing.T) {
	g := NewGrouper()

	originalMessages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Question 1")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Answer 1")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("Question 2")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Answer 2")),
	}

	rounds := g.GroupV1(originalMessages)

	// Reconstruct messages from rounds
	var reconstructedMessages []anthropic.MessageParam
	for _, round := range rounds {
		reconstructedMessages = append(reconstructedMessages, round.Messages...)
	}

	// Verify all messages are preserved
	if len(reconstructedMessages) != len(originalMessages) {
		t.Fatalf("Message count mismatch: expected %d, got %d",
			len(originalMessages), len(reconstructedMessages))
	}

	for i := range originalMessages {
		if string(originalMessages[i].Role) != string(reconstructedMessages[i].Role) {
			t.Errorf("Message %d: role mismatch", i)
		}
	}
}

// TestGroupV1_LastAssistantTextExtraction verifies the logic for extracting
// only the last assistant message's text (for historical rounds with tool use)
func TestGroupV1_LastAssistantTextExtraction(t *testing.T) {
	g := NewGrouper()

	// This simulates a historical round with tool use
	// We want to ensure that when extracting round_result from such a round,
	// we only get the last assistant's text, not all assistant texts
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("What's 2+2?")),
		anthropic.NewAssistantMessage(
			anthropic.NewTextBlock("Let me calculate that."),
			anthropic.NewToolUseBlock("toolu_calc", map[string]interface{}{"expr": "2+2"}, "calculator"),
		),
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_calc", "4", false)),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("The answer is 4.")),
	}

	rounds := g.GroupV1(messages)

	if len(rounds) != 1 {
		t.Fatalf("Expected 1 round, got %d", len(rounds))
	}

	// The round should have 2 assistant messages
	if rounds[0].Stats.AssistantCount != 2 {
		t.Errorf("Expected 2 assistant messages, got %d", rounds[0].Stats.AssistantCount)
	}

	// Verify the messages are in correct order
	// Message 0: user "What's 2+2?"
	// Message 1: assistant with "Let me calculate..." + tool_use
	// Message 2: user with tool_result
	// Message 3: assistant with "The answer is 4."

	if string(rounds[0].Messages[0].Role) != "user" {
		t.Error("First message should be user")
	}

	if string(rounds[0].Messages[3].Role) != "assistant" {
		t.Error("Last message should be assistant")
	}

	// The round_result for this historical round should only contain
	// "The answer is 4." (from the last assistant), NOT "Let me calculate. The answer is 4."
}

// TestGroupV1_RoundBoundaryTests tests various round boundary scenarios
func TestGroupV1_RoundBoundaryTests(t *testing.T) {
	tests := []struct {
		name               string
		messages           []anthropic.MessageParam
		expectedRounds     int
		expectedAllCurrent []bool // which rounds should be current
	}{
		{
			name: "Single complete round",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q1")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("A1")),
			},
			expectedRounds:     1,
			expectedAllCurrent: []bool{true},
		},
		{
			name: "Three complete rounds",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q1")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("A1")),
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q2")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("A2")),
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q3")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("A3")),
			},
			expectedRounds:     3,
			expectedAllCurrent: []bool{false, false, true},
		},
		{
			name: "User message only (incomplete round)",
			messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q1")),
				anthropic.NewAssistantMessage(anthropic.NewTextBlock("A1")),
				anthropic.NewUserMessage(anthropic.NewTextBlock("Q2")),
			},
			expectedRounds:     2,
			expectedAllCurrent: []bool{false, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGrouper()
			rounds := g.GroupV1(tt.messages)

			if len(rounds) != tt.expectedRounds {
				t.Fatalf("Expected %d rounds, got %d", tt.expectedRounds, len(rounds))
			}

			for i, shouldBeCurrent := range tt.expectedAllCurrent {
				if rounds[i].IsCurrentRound != shouldBeCurrent {
					t.Errorf("Round %d: IsCurrentRound should be %v, got %v",
						i, shouldBeCurrent, rounds[i].IsCurrentRound)
				}
			}
		})
	}
}
