package claude

import (
	"slices"
	"testing"
)

func TestBuildCommonArgsSeparatesAvailableToolsFromApprovalRules(t *testing.T) {
	args := BuildCommonArgs(DefaultConfig(), CommonOptions{
		AvailableTools: []string{"Read", "Edit", "Bash"},
		AllowedTools:   []string{"Read"},
		PermissionMode: "manual",
	})

	if !hasArgPair(args, "--tools", "Read,Edit,Bash") {
		t.Fatalf("missing available tools in %q", args)
	}
	if !hasArgPair(args, "--allowedTools", "Read") {
		t.Fatalf("missing approval allow-list in %q", args)
	}
	if !hasArgPair(args, "--permission-mode", "manual") {
		t.Fatalf("missing manual permission mode in %q", args)
	}
}

func hasArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if slices.Equal(args[i:i+2], []string{key, value}) {
			return true
		}
	}
	return false
}
