package mcpruntime

import (
	"bufio"
	"strings"
	"testing"
)

func TestNormalizeAndParseToolName(t *testing.T) {
	name := NormalizeToolName("search", "web_search")
	if name != "mcp__search__web_search" {
		t.Fatalf("unexpected normalized name: %s", name)
	}

	source, tool, ok := ParseNormalizedToolName(name)
	if !ok {
		t.Fatalf("expected normalized name to parse")
	}
	if source != "search" || tool != "web_search" {
		t.Fatalf("unexpected parse result: source=%s tool=%s", source, tool)
	}
}

func TestIsMCPToolName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{name: "mcp__search__web_search", want: true},
		{name: "mcp__onlyonesep", want: false},
		{name: "web_search", want: false},
		{name: "", want: false},
	}

	for _, tc := range cases {
		got := IsMCPToolName(tc.name)
		if got != tc.want {
			t.Fatalf("IsMCPToolName(%q)=%v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuildAllowList(t *testing.T) {
	allowAll, allowSet := buildAllowList(nil)
	if !allowAll || allowSet != nil {
		t.Fatalf("expected nil list to allow all")
	}

	allowAll, allowSet = buildAllowList([]string{"*"})
	if !allowAll || allowSet != nil {
		t.Fatalf("expected wildcard to allow all")
	}

	allowAll, allowSet = buildAllowList([]string{"web_search", "web_fetch"})
	if allowAll {
		t.Fatalf("expected explicit list to not allow all")
	}
	if !allowSet["web_search"] || !allowSet["web_fetch"] {
		t.Fatalf("expected allow set to include explicit names")
	}
}

func TestReadStdioFrame(t *testing.T) {
	in := "Content-Length: 17\r\n\r\n{\"jsonrpc\":\"2.0\"}"
	r := bufio.NewReader(strings.NewReader(in))
	body, err := readStdioFrame(r)
	if err != nil {
		t.Fatalf("readStdioFrame returned error: %v", err)
	}
	if string(body) != "{\"jsonrpc\":\"2.0\"}" {
		t.Fatalf("unexpected body: %s", string(body))
	}
}
