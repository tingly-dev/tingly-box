package transform

import (
	"testing"
)

func TestIsNativeWebToolName_Search(t *testing.T) {
	cases := []struct {
		name   string
		expect bool
	}{
		{"web_search", true},
		{"WebSearch", true},
		{"websearch", true},
		{"WEB_SEARCH", true},
		{"web_fetch", false},
		{"mcp_web_search", false},
		{"mcp__tb__tingly_box_mcp__webtools__mcp_web_search", false},
	}
	for _, tc := range cases {
		got := isNativeWebToolName(tc.name, true, false)
		if got != tc.expect {
			t.Errorf("isNativeWebToolName(%q, stripSearch=true): got %v, want %v", tc.name, got, tc.expect)
		}
	}
}

func TestIsNativeWebToolName_Fetch(t *testing.T) {
	cases := []struct {
		name   string
		expect bool
	}{
		{"web_fetch", true},
		{"WebFetch", true},
		{"webfetch", true},
		{"web_search", false},
		{"WebSearch", false},
	}
	for _, tc := range cases {
		got := isNativeWebToolName(tc.name, false, true)
		if got != tc.expect {
			t.Errorf("isNativeWebToolName(%q, stripFetch=true): got %v, want %v", tc.name, got, tc.expect)
		}
	}
}
