package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

//go:embed anthropic_test.txt
var rawBody []byte

func TestAnthropicBetaMessagesRequest_UnmarshalJSON(t *testing.T) {
	jsonString := "{\"stream\":true}"

	var req AnthropicBetaMessagesRequest
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize rawBody into BetaMessageNewParams: %v", err)
	}

	if req.Stream != true {
		t.Fatal("Failed to deserialize rawBody into BetaMessageNewParams")
	}
}

func TestBetaDecode(t *testing.T) {
	var jsonString string
	if err := json.Unmarshal(rawBody, &jsonString); err != nil {
		panic(fmt.Sprintf("第一次解码失败: %v", err))
	}

	// Test that rawBody (a JSON string) can be deserialized into BetaMessageNewParams
	var req anthropic.BetaMessageNewParams
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize rawBody into BetaMessageNewParams: %v", err)
	}

	d, _ := json.MarshalIndent(req, "", "    ")
	fmt.Println(string(d))

	// Print messages for debugging
	t.Logf("Model: %s", req.Model)
	t.Logf("Messages count: %d", len(req.Messages))
	for i, msg := range req.Messages {
		t.Logf("Message[%d] Role: %s", i, msg.Role)
		t.Logf("Message[%d] Content blocks: %d", i, len(msg.Content))
		for j, block := range msg.Content {
			// Handle different block types
			if block.OfText != nil {
				contentStr := fmt.Sprintf("%v", block.OfText.Text)
				const maxLen = 200
				if len(contentStr) > maxLen {
					contentStr = contentStr[:maxLen] + "..."
				}
				t.Logf("  Content[%d] (text): %s", j, contentStr)
			} else if block.OfToolResult != nil {
				// Tool result block - check what fields are available
				t.Logf("  Content[%d] (tool_result) ToolUseID: %s", j, block.OfToolResult.ToolUseID)
				t.Logf("  Content[%d] (tool_result) IsError: %v", j, block.OfToolResult.IsError)
				// Content is an array of BetaToolResultBlockParamContentUnion
				t.Logf("  Content[%d] (tool_result) Content blocks: %d", j, len(block.OfToolResult.Content))
				for k, contentBlock := range block.OfToolResult.Content {
					if contentBlock.OfText != nil {
						contentStr := fmt.Sprintf("%v", contentBlock.OfText.Text)
						const maxLen = 200
						if len(contentStr) > maxLen {
							contentStr = contentStr[:maxLen] + "..."
						}
						t.Logf("    Content[%d][%d] (text): %s", j, k, contentStr)
					} else if contentBlock.OfImage != nil {
						t.Logf("    Content[%d][%d] (image)", j, k)
					} else {
						d, _ := json.MarshalIndent(contentBlock, "", "    ")
						t.Logf("    Content[%d][%d] (other): %s", j, k, string(d))
					}
				}
			} else {
				d, _ := json.MarshalIndent(block, "", "    ")
				t.Logf("  Content[%d] (other): %s", j, string(d))
			}
		}
	}

	// Verify basic fields
	if req.Model != "tingly/cc" {
		t.Errorf("Expected model 'tingly/cc', got '%s'", req.Model)
	}

	// Verify messages exist
	if len(req.Messages) == 0 {
		t.Fatal("Expected at least one message, got none")
	}

	// Verify first message is from user
	firstMsg := req.Messages[0]
	if firstMsg.Role != anthropic.BetaMessageParamRoleUser {
		t.Errorf("Expected first message role 'user', got '%s'", firstMsg.Role)
	}

	// Verify content blocks exist in first message
	if len(firstMsg.Content) == 0 {
		t.Fatal("Expected at least one content block in first message, got none")
	}
}

func TestDecode(t *testing.T) {
	// 2. 第一次解码：得到 JSON 字符串
	var jsonString string
	if err := json.Unmarshal(rawBody, &jsonString); err != nil {
		panic(fmt.Sprintf("第一次解码失败: %v", err))
	}

	// Test that rawBody (a JSON string) can be deserialized into BetaMessageNewParams
	var req anthropic.MessageNewParams
	if err := json.Unmarshal([]byte(jsonString), &req); err != nil {
		t.Fatalf("Failed to deserialize rawBody into BetaMessageNewParams: %v", err)
	}

	d, _ := json.MarshalIndent(req, "", "    ")
	fmt.Println(string(d))

	// Print messages for debugging
	t.Logf("Model: %s", req.Model)
	t.Logf("Messages count: %d", len(req.Messages))
	for i, msg := range req.Messages {
		t.Logf("Message[%d] Role: %s", i, msg.Role)
		t.Logf("Message[%d] Content blocks: %d", i, len(msg.Content))
		for j, block := range msg.Content {
			// Handle different block types
			if block.OfText != nil {
				contentStr := fmt.Sprintf("%v", block.OfText.Text)
				const maxLen = 200
				if len(contentStr) > maxLen {
					contentStr = contentStr[:maxLen] + "..."
				}
				t.Logf("  Content[%d] (text): %s", j, contentStr)
			} else if block.OfToolResult != nil {
				// Tool result block - check what fields are available
				t.Logf("  Content[%d] (tool_result) ToolUseID: %s", j, block.OfToolResult.ToolUseID)
				t.Logf("  Content[%d] (tool_result) IsError: %v", j, block.OfToolResult.IsError)
				// Content is an array of BetaToolResultBlockParamContentUnion
				t.Logf("  Content[%d] (tool_result) Content blocks: %d", j, len(block.OfToolResult.Content))
				for k, contentBlock := range block.OfToolResult.Content {
					if contentBlock.OfText != nil {
						contentStr := fmt.Sprintf("%v", contentBlock.OfText.Text)
						const maxLen = 200
						if len(contentStr) > maxLen {
							contentStr = contentStr[:maxLen] + "..."
						}
						t.Logf("    Content[%d][%d] (text): %s", j, k, contentStr)
					} else if contentBlock.OfImage != nil {
						t.Logf("    Content[%d][%d] (image)", j, k)
					} else {
						d, _ := json.MarshalIndent(contentBlock, "", "    ")
						t.Logf("    Content[%d][%d] (other): %s", j, k, string(d))
					}
				}
			} else {
				d, _ := json.MarshalIndent(block, "", "    ")
				t.Logf("  Content[%d] (other): %s", j, string(d))
			}
		}
	}

	// Verify basic fields
	if req.Model != "tingly/cc" {
		t.Errorf("Expected model 'tingly/cc', got '%s'", req.Model)
	}

	// Verify messages exist
	if len(req.Messages) == 0 {
		t.Fatal("Expected at least one message, got none")
	}

	// Verify first message is from user
	firstMsg := req.Messages[0]
	if firstMsg.Role != anthropic.MessageParamRoleUser {
		t.Errorf("Expected first message role 'user', got '%s'", firstMsg.Role)
	}

	// Verify content blocks exist in first message
	if len(firstMsg.Content) == 0 {
		t.Fatal("Expected at least one content block in first message, got none")
	}
}
