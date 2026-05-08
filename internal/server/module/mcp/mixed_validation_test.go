package mcp

import (
	"testing"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

func TestValidateAndNormalizeMixedStash_OK(t *testing.T) {
	results, err := validateAndNormalizeMixedStash(
		[]string{"toolu_external", "toolu_external"},
		[]ToolExecutionResult{
			{ToolUseID: "toolu_virtual", Contents: []mcpruntime.ToolContent{{Type: mcpruntime.ContentTypeText, Text: "ok"}}, IsError: false},
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 || results[0].ToolUseID != "toolu_virtual" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestValidateAndNormalizeMixedStash_NoAnchors(t *testing.T) {
	_, err := validateAndNormalizeMixedStash(
		[]string{"", ""},
		[]ToolExecutionResult{{ToolUseID: "toolu_virtual", Contents: []mcpruntime.ToolContent{{Type: mcpruntime.ContentTypeText, Text: "ok"}}}},
	)
	if err == nil {
		t.Fatalf("expected error when anchors are empty")
	}
}

func TestValidateAndNormalizeMixedStash_NoVirtualIDs(t *testing.T) {
	_, err := validateAndNormalizeMixedStash(
		[]string{"toolu_external"},
		[]ToolExecutionResult{{ToolUseID: "", Contents: []mcpruntime.ToolContent{{Type: mcpruntime.ContentTypeText, Text: "ok"}}}},
	)
	if err == nil {
		t.Fatalf("expected error when virtual tool ids are empty")
	}
}
