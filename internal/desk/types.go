package desk

// PermissionMode is Claude Code's permission mode for a turn, passed as
// --permission-mode. Empty means "not overridden": the settings file's
// defaultMode (profile or user) decides, then the CLI default
// (.design/remote-cc-profile.md §2.1).
const (
	PermissionInherit           = ""
	PermissionDefault           = "default"
	PermissionPlan              = "plan"
	PermissionAcceptEdits       = "acceptEdits"
	PermissionDontAsk           = "dontAsk"
	PermissionBypassPermissions = "bypassPermissions"
	// PermissionAuto delegates decisions to Claude Code's rule classifier.
	// It is not a bypass: a call the classifier will not decide still
	// reaches the host as an approval request.
	PermissionAuto = "auto"
)

// PermissionModes lists the selectable modes in display order.
var PermissionModes = []string{
	PermissionDefault, PermissionAcceptEdits, PermissionAuto, PermissionPlan, PermissionDontAsk, PermissionBypassPermissions,
}

// ValidPermissionMode reports whether m is empty or one of PermissionModes.
func ValidPermissionMode(m string) bool {
	if m == PermissionInherit {
		return true
	}
	for _, v := range PermissionModes {
		if v == m {
			return true
		}
	}
	return false
}

// autoApproves reports whether the host should answer permission requests
// itself. Only bypassPermissions promises unconditional approval; every
// other mode keeps Claude Code's own deny/plan/classifier semantics (the
// same policy as @cc's own noApprovalModes).
func autoApproves(mode string) bool { return mode == PermissionBypassPermissions }
