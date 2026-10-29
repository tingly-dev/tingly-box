package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func githubRuntime(repos []string) *Runtime {
	cfg := &typ.MCPRuntimeConfig{
		Sources:  []typ.MCPSourceConfig{{ID: "src-gh", Preset: "github", Transport: "http"}, {ID: "src-other", Transport: "http"}},
		Profiles: []typ.MCPProfile{{ID: "prof-scoped", GitHubRepos: repos}},
	}
	return NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
}

func TestCheckToolScope(t *testing.T) {
	r := githubRuntime([]string{"tingly-dev/tingly-box"})
	defer r.Close()
	gh := func(tool string) string { return NormalizeToolName("src-gh", tool) }
	cases := []struct {
		name, profile, tool string
		args                map[string]any
		wantErr             string
	}{
		{"in scope", "prof-scoped", gh("issue_read"), map[string]any{"owner": "Tingly-Dev", "repo": "tingly-box"}, ""},
		{"outside scope", "prof-scoped", gh("issue_read"), map[string]any{"owner": "other", "repo": "x"}, "outside this MCP profile's scope"},
		{"missing repo args", "prof-scoped", gh("list_issues"), map[string]any{}, "needs owner and repo"},
		{"get_me needs no repo", "prof-scoped", gh("get_me"), nil, ""},
		{"default profile unscoped", "default", gh("issue_read"), map[string]any{"owner": "other", "repo": "x"}, ""},
		{"other source untouched", "prof-scoped", NormalizeToolName("src-other", "issue_read"), map[string]any{"owner": "other", "repo": "x"}, ""},
	}
	for _, c := range cases {
		err := r.CheckToolScope(c.profile, c.tool, c.args)
		if c.wantErr == "" && err != nil || c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.wantErr)
		}
	}
}

func TestGitHubSubsetPolicy(t *testing.T) {
	src := typ.MCPSourceConfig{ID: "src-gh", Preset: "github", Tools: []string{"*"}}
	for tool, want := range map[string]bool{"issue_read": true, "get_job_logs": true, "merge_pull_request": false, "delete_file": false, "search_code": false, "actions_run_trigger": false} {
		if got, _ := EffectiveToolPolicy(src, tool); got != want {
			t.Errorf("%s enabled=%v want %v", tool, got, want)
		}
	}
}

func TestGitHubPresetHidesToolsOutsideSubsetEndToEnd(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "gh", Version: "1"}, nil)
	for _, name := range []string{"issue_read", "merge_pull_request", "delete_file", "search_code"} {
		server.AddTool(&sdk.Tool{Name: name, InputSchema: map[string]any{"type": "object"}}, func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{}, nil
		})
	}
	remote := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, &sdk.StreamableHTTPOptions{JSONResponse: true}))
	defer remote.Close()
	cfg := &typ.MCPRuntimeConfig{Sources: []typ.MCPSourceConfig{{ID: "src-gh", Origin: "external", Preset: "github", Transport: "http", Endpoint: remote.URL, Tools: []string{"*"}}}}
	rt := NewRuntime(func() *typ.MCPRuntimeConfig { return cfg })
	defer rt.Close()

	status, err := rt.DiscoverSource(context.Background(), "src-gh")
	require.NoError(t, err)
	var names []string
	for _, tool := range status.Tools {
		names = append(names, tool.Name)
	}
	require.Equal(t, []string{"issue_read"}, names)

	_, err = rt.CallTool(context.Background(), NormalizeToolName("src-gh", "merge_pull_request"), "{}")
	require.ErrorContains(t, err, "disabled by policy")
	_, err = rt.CallTool(context.Background(), NormalizeToolName("src-gh", "issue_read"), "{}")
	require.NoError(t, err)
}
