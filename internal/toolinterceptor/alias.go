package toolinterceptor

// HandlerType represents the type of tool handler
type HandlerType string

const (
	HandlerTypeSearch HandlerType = "internal_search"
	HandlerTypeFetch  HandlerType = "internal_fetch"
	HandlerTypeNone   HandlerType = ""
)

// toolAliases maps various tool names to internal handler types
var toolAliases = map[string]HandlerType{
	// Search aliases
	"web_search":    HandlerTypeSearch,
	"Google Search": HandlerTypeSearch,
	"search":        HandlerTypeSearch,
	"bing_search":   HandlerTypeSearch,

	// Fetch aliases
	"web_fetch":        HandlerTypeFetch,
	"browse":           HandlerTypeFetch,
	"read_url":         HandlerTypeFetch,
	"get_page_content": HandlerTypeFetch,
}

// MatchToolAlias checks if a tool name matches any known alias and returns the handler type
func MatchToolAlias(toolName string) (HandlerType, bool) {
	handlerType, matched := toolAliases[toolName]
	return handlerType, matched
}

// IsSearchTool checks if a tool name is a search tool alias
func IsSearchTool(toolName string) bool {
	handlerType, matched := MatchToolAlias(toolName)
	return matched && handlerType == HandlerTypeSearch
}

// IsFetchTool checks if a tool name is a fetch tool alias
func IsFetchTool(toolName string) bool {
	handlerType, matched := MatchToolAlias(toolName)
	return matched && handlerType == HandlerTypeFetch
}

// ShouldInterceptTool checks if a tool should be intercepted based on its name
func ShouldInterceptTool(toolName string) bool {
	_, matched := MatchToolAlias(toolName)
	return matched
}
