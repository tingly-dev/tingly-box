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
	"web_search":       HandlerTypeSearch,
	"Google Search":    HandlerTypeSearch,
	"search":           HandlerTypeSearch,
	"bing_search":      HandlerTypeSearch,
	"google_search":    HandlerTypeSearch,
	"duckduckgo":       HandlerTypeSearch,
	"ddg_search":       HandlerTypeSearch,

	// Fetch aliases
	"web_fetch":        HandlerTypeFetch,
	"browse":           HandlerTypeFetch,
	"read_url":         HandlerTypeFetch,
	"get_page_content": HandlerTypeFetch,
	"url_fetch":        HandlerTypeFetch,
	"fetch_url":        HandlerTypeFetch,
}

// nativeSearchTools lists tool names that indicate the model has native search capability
var nativeSearchTools = map[string]bool{
	"web_search":    true,
	"Google Search": true,
	"google_search": true,
	"bing_search":   true,
	"duckduckgo":    true,
	"ddg_search":    true,
	"search":        true,
}

// nativeFetchTools lists tool names that indicate the model has native fetch capability
var nativeFetchTools = map[string]bool{
	"web_fetch":        true,
	"browse":           true,
	"read_url":         true,
	"get_page_content": true,
	"url_fetch":        true,
	"fetch_url":        true,
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

// HasNativeSearchTool checks if the model has a native search tool
func HasNativeSearchTool(toolNames []string) bool {
	for _, name := range toolNames {
		if nativeSearchTools[name] {
			return true
		}
	}
	return false
}

// HasNativeFetchTool checks if the model has a native fetch tool
func HasNativeFetchTool(toolNames []string) bool {
	for _, name := range toolNames {
		if nativeFetchTools[name] {
			return true
		}
	}
	return false
}

// ShouldInterceptSearch determines if search should be intercepted based on mode and native tools
// preferLocal: true = hard open (always intercept), false = soft open (skip if model has native)
// returns true if should intercept
func ShouldInterceptSearch(preferLocal bool, modelToolNames []string) bool {
	if preferLocal {
		// Hard open: always intercept
		return true
	}
	// Soft open: only intercept if model doesn't have native search
	return !HasNativeSearchTool(modelToolNames)
}

// ShouldInterceptFetch determines if fetch should be intercepted based on mode and native tools
// preferLocal: true = hard open (always intercept), false = soft open (skip if model has native)
// returns true if should intercept
func ShouldInterceptFetch(preferLocal bool, modelToolNames []string) bool {
	if preferLocal {
		// Hard open: always intercept
		return true
	}
	// Soft open: only intercept if model doesn't have native fetch
	return !HasNativeFetchTool(modelToolNames)
}
