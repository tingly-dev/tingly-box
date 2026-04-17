package tools

const (
	BuiltinAdvisorSourceID   = "advisor"
	BuiltinAdvisorSourceName = "Built-in Adviser"
	BuiltinAdvisorToolName   = "advisor"
)

var builtinAdvisorDefaultNames = []string{
	BuiltinAdvisorToolName,
}

// DefaultBuiltinAdvisorToolNames returns a copy of default builtin advisor tool names.
func DefaultBuiltinAdvisorToolNames() []string {
	out := make([]string, len(builtinAdvisorDefaultNames))
	copy(out, builtinAdvisorDefaultNames)
	return out
}
