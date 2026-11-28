package tools

// ToolReadinessChecker reports whether a builtin tool is ready to serve requests
// given the resolved environment for its source.
type ToolReadinessChecker interface {
	IsReady(env map[string]string) bool
}

// webSearchReadiness covers the legacy Serper path. The runtime resolves
// provider-backed DeepSeek readiness before applying this checker.
type webSearchReadiness struct{}

func (webSearchReadiness) IsReady(env map[string]string) bool {
	return env["SERPER_API_KEY"] != ""
}

// webFetchReadiness has no external key requirement.
type webFetchReadiness struct{}

func (webFetchReadiness) IsReady(_ map[string]string) bool { return true }

// WebtoolReadinessCheckers maps each builtin webtools tool name to its checker.
var WebtoolReadinessCheckers = map[string]ToolReadinessChecker{
	BuiltinWebSearchToolName: webSearchReadiness{},
	BuiltinWebFetchToolName:  webFetchReadiness{},
}
