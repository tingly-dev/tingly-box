package toolinterceptor

import (
	"encoding/json"
	"fmt"

	"tingly-box/internal/typ"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// Interceptor handles tool interception and execution
type Interceptor struct {
	globalConfig  *typ.ToolInterceptorConfig
	searchHandler *SearchHandler
	fetchHandler  *FetchHandler
	cache         *Cache
}

// NewInterceptor creates a new tool interceptor with global configuration
func NewInterceptor(globalConfig *typ.ToolInterceptorConfig) *Interceptor {
	if globalConfig == nil {
		globalConfig = &typ.ToolInterceptorConfig{
			Enabled:      false,
			SearchAPI:    "brave",
			MaxResults:   10,
			MaxFetchSize: 1 * 1024 * 1024, // 1MB
			FetchTimeout: 30,              // 30 seconds
			MaxURLLength: 2000,
		}
	}

	cache := NewCache()
	handlerConfig := &Config{
		Enabled:      globalConfig.Enabled,
		SearchAPI:    globalConfig.SearchAPI,
		SearchKey:    globalConfig.SearchKey,
		MaxResults:   globalConfig.MaxResults,
		ProxyURL:     globalConfig.ProxyURL,
		MaxFetchSize: globalConfig.MaxFetchSize,
		FetchTimeout: globalConfig.FetchTimeout,
		MaxURLLength: globalConfig.MaxURLLength,
	}

	return &Interceptor{
		globalConfig:  globalConfig,
		searchHandler: NewSearchHandler(handlerConfig, cache),
		fetchHandler:  NewFetchHandlerWithConfig(cache, handlerConfig),
		cache:         cache,
	}
}

// IsEnabledForProvider checks if interceptor is enabled for a specific provider
func (i *Interceptor) IsEnabledForProvider(provider *typ.Provider) bool {
	if i.globalConfig == nil || !i.globalConfig.Enabled {
		return false
	}

	effectiveConfig, enabled := provider.GetEffectiveConfig(i.globalConfig)
	return enabled && effectiveConfig != nil
}

// GetConfigForProvider returns the effective config for a specific provider
func (i *Interceptor) GetConfigForProvider(provider *typ.Provider) *typ.ToolInterceptorConfig {
	effectiveConfig, enabled := provider.GetEffectiveConfig(i.globalConfig)
	if !enabled || effectiveConfig == nil {
		return &typ.ToolInterceptorConfig{Enabled: false}
	}

	return effectiveConfig
}

// InterceptOpenAIRequest intercepts tool calls in an OpenAI request
// Returns:
// - intercepted: true if any tools were intercepted
// - results: tool results to inject back
// - modifiedTools: tools that were not intercepted (to forward to provider)
func (i *Interceptor) InterceptOpenAIRequest(provider *typ.Provider, req *openai.ChatCompletionNewParams) (intercepted bool, results []ToolResult, modifiedTools []openai.ChatCompletionToolUnionParam) {
	// Check if enabled for this provider
	if !i.IsEnabledForProvider(provider) || len(req.Tools) == 0 {
		return false, nil, req.Tools
	}

	results = []ToolResult{}
	toolsToForward := []openai.ChatCompletionToolUnionParam{}

	// Filter tools - forward non-intercepted tools, note intercepted ones
	for _, toolUnion := range req.Tools {
		fn := toolUnion.GetFunction()
		if fn == nil {
			// Not a function tool, forward as-is
			toolsToForward = append(toolsToForward, toolUnion)
			continue
		}

		// Check if this tool should be intercepted
		if !ShouldInterceptTool(fn.Name) {
			toolsToForward = append(toolsToForward, toolUnion)
			continue
		}
		// This tool should be intercepted - don't add to toolsToForward
	}

	// Check if there are any tool calls in assistant messages that need to be executed
	// This happens when the LLM has already decided to use a tool
	for _, msgUnion := range req.Messages {
		msgMap, err := parseOpenAIMessage(msgUnion)
		if err != nil {
			continue
		}

		// Check if this is an assistant message with tool calls
		if msgMap["role"] != "assistant" {
			continue
		}

		toolCalls, ok := msgMap["tool_calls"].([]interface{})
		if !ok {
			continue
		}

		// Process each tool call
		for _, tc := range toolCalls {
			tcMap, ok := tc.(map[string]interface{})
			if !ok {
				continue
			}

			id, _ := tcMap["id"].(string)
			fnMap, ok := tcMap["function"].(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := fnMap["name"].(string)
			arguments, _ := fnMap["arguments"].(string)

			// Check if this tool should be intercepted
			if !ShouldInterceptTool(name) {
				continue
			}

			// Execute the tool
			result := i.executeTool(provider, name, arguments)

			results = append(results, ToolResult{
				ToolCallID: id,
				Content:    result.Content,
				Error:      result.Error,
				IsError:    result.IsError,
			})
		}
	}

	intercepted = len(results) > 0
	return intercepted, results, toolsToForward
}

// InterceptAnthropicRequest intercepts tool calls in an Anthropic request
func (i *Interceptor) InterceptAnthropicRequest(provider *typ.Provider, req *anthropic.MessageNewParams) (intercepted bool, results []ToolResult, modifiedTools []anthropic.ToolUnionParam) {
	// Check if enabled for this provider
	if !i.IsEnabledForProvider(provider) || len(req.Tools) == 0 {
		return false, nil, req.Tools
	}

	results = []ToolResult{}
	toolsToForward := []anthropic.ToolUnionParam{}

	// Filter tools - forward non-intercepted tools
	for _, toolUnion := range req.Tools {
		tool := toolUnion.OfTool
		if tool == nil {
			toolsToForward = append(toolsToForward, toolUnion)
			continue
		}

		// Check if this tool should be intercepted
		if !ShouldInterceptTool(tool.Name) {
			toolsToForward = append(toolsToForward, toolUnion)
			continue
		}
		// This tool should be intercepted - don't forward
	}

	// Check for tool_use blocks in messages that need to be executed
	for _, msg := range req.Messages {
		// Parse message to check for tool_use blocks
		msgMap, err := parseAnthropicMessage(msg)
		if err != nil {
			continue
		}

		if msgMap["role"] != "assistant" {
			continue
		}

		contentBlocks, ok := msgMap["content"].([]interface{})
		if !ok {
			continue
		}

		for _, block := range contentBlocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, ok := blockMap["type"].(string)
			if !ok || blockType != "tool_use" {
				continue
			}

			name, _ := blockMap["name"].(string)
			id, _ := blockMap["id"].(string)

			// Get input/arguments
			var inputStr string
			if input, ok := blockMap["input"].(string); ok {
				inputStr = input
			} else if inputJSON, err := json.Marshal(blockMap["input"]); err == nil {
				inputStr = string(inputJSON)
			}

			// Check if this tool should be intercepted
			if !ShouldInterceptTool(name) {
				continue
			}

			// Execute the tool
			result := i.executeTool(provider, name, inputStr)

			results = append(results, ToolResult{
				ToolCallID: id, // In Anthropic, this is tool_use_id
				Content:    result.Content,
				Error:      result.Error,
				IsError:    result.IsError,
			})
		}
	}

	intercepted = len(results) > 0
	return intercepted, results, toolsToForward
}

// ExecuteTool executes a tool by name with JSON arguments (public method for server use)
func (i *Interceptor) ExecuteTool(provider *typ.Provider, toolName string, argsJSON string) ToolResult {
	return i.executeTool(provider, toolName, argsJSON)
}

// PrepareOpenAIRequest pre-processes an OpenAI request before sending to provider
// Returns:
// - modifiedReq: the request with tools stripped and pre-injected results
// - hasPreInjectedResults: whether tool results were injected
func (i *Interceptor) PrepareOpenAIRequest(provider *typ.Provider, originalReq *openai.ChatCompletionNewParams) (modifiedReq *openai.ChatCompletionNewParams, hasPreInjectedResults bool) {
	// Create a mutable copy of the request
	modifiedReq = originalReq

	// Check if interception is enabled
	if !i.IsEnabledForProvider(provider) || len(originalReq.Tools) == 0 {
		return modifiedReq, false
	}

	// Intercept to strip tools and check for pre-existing tool calls
	intercepted, results, modifiedTools := i.InterceptOpenAIRequest(provider, originalReq)
	modifiedReq.Tools = modifiedTools

	// If there were pre-existing tool calls that were executed, inject results
	if intercepted && len(results) > 0 {
		// Build new messages list with original messages
		newMessages := make([]openai.ChatCompletionMessageParamUnion, len(originalReq.Messages))
		copy(newMessages, originalReq.Messages)

		// Inject tool result messages
		for _, result := range results {
			resultMsg := openai.ToolMessage(
				result.Content,
				result.ToolCallID,
			)
			newMessages = append(newMessages, resultMsg)
		}
		modifiedReq.Messages = newMessages
		return modifiedReq, true
	}

	// Strip tool_choice if all tools were intercepted
	if len(modifiedTools) == 0 && ShouldStripToolChoice(originalReq) {
		// Reset tool_choice to default (empty/zero value)
		// The empty ToolChoice will default to "auto" behavior
		modifiedReq.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{}
	}

	return modifiedReq, false
}

// PrepareAnthropicRequest pre-processes an Anthropic request before sending to provider
// Returns:
// - modifiedReq: the request with tools stripped and pre-injected results
// - hasPreInjectedResults: whether tool results were injected
func (i *Interceptor) PrepareAnthropicRequest(provider *typ.Provider, originalReq *anthropic.MessageNewParams) (modifiedReq *anthropic.MessageNewParams, hasPreInjectedResults bool) {
	// Create a mutable copy of the request
	modifiedReq = originalReq

	// Check if interception is enabled
	if !i.IsEnabledForProvider(provider) || len(originalReq.Tools) == 0 {
		return modifiedReq, false
	}

	// Intercept to strip tools and check for pre-existing tool calls
	intercepted, results, modifiedTools := i.InterceptAnthropicRequest(provider, originalReq)
	modifiedReq.Tools = modifiedTools

	// If there were pre-existing tool calls that were executed, inject results
	if intercepted && len(results) > 0 {
		// Build new messages list with original messages
		newMessages := make([]anthropic.MessageParam, len(originalReq.Messages))
		copy(newMessages, originalReq.Messages)

		// Inject tool result blocks
		for _, result := range results {
			resultBlock := CreateAnthropicToolResultBlock(result.ToolCallID, result.Content, result.IsError)
			// Wrap in a message
			resultMsg := anthropic.NewUserMessage(resultBlock)
			newMessages = append(newMessages, resultMsg)
		}
		modifiedReq.Messages = newMessages
		return modifiedReq, true
	}

	return modifiedReq, false
}

// executeTool executes a tool by name with JSON arguments
func (i *Interceptor) executeTool(provider *typ.Provider, toolName string, argsJSON string) ToolResult {
	// Determine handler type based on tool name
	handlerType, matched := MatchToolAlias(toolName)
	if !matched {
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Unknown tool: %s", toolName),
			IsError: true,
		}
	}

	switch handlerType {
	case HandlerTypeSearch:
		return i.executeSearch(provider, argsJSON)
	case HandlerTypeFetch:
		return i.executeFetch(provider, argsJSON)
	default:
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Unsupported handler type: %s", handlerType),
			IsError: true,
		}
	}
}

// executeSearch executes a search tool
func (i *Interceptor) executeSearch(provider *typ.Provider, argsJSON string) ToolResult {
	// Get provider-specific config
	providerConfig := i.GetConfigForProvider(provider)
	if providerConfig == nil || !providerConfig.Enabled {
		return ToolResult{
			Content: "",
			Error:   "Search is not enabled for this provider",
			IsError: true,
		}
	}

	// Parse search arguments
	var searchReq SearchRequest
	if err := json.Unmarshal([]byte(argsJSON), &searchReq); err != nil {
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Invalid search arguments: %v", err),
			IsError: true,
		}
	}

	if searchReq.Query == "" {
		return ToolResult{
			Content: "",
			Error:   "Search query is required",
			IsError: true,
		}
	}

	// Execute search with provider-specific config
	handlerConfig := &Config{
		Enabled:      providerConfig.Enabled,
		SearchAPI:    providerConfig.SearchAPI,
		SearchKey:    providerConfig.SearchKey,
		MaxResults:   providerConfig.MaxResults,
		ProxyURL:     providerConfig.ProxyURL,
		MaxFetchSize: providerConfig.MaxFetchSize,
		FetchTimeout: providerConfig.FetchTimeout,
		MaxURLLength: providerConfig.MaxURLLength,
	}
	results, err := i.searchHandler.SearchWithConfig(searchReq.Query, searchReq.Count, handlerConfig)
	if err != nil {
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Search failed: %v", err),
			IsError: true,
		}
	}

	// Format results
	return ToolResult{
		Content: FormatSearchResults(results),
		IsError: false,
	}
}

// executeFetch executes a fetch tool
func (i *Interceptor) executeFetch(provider *typ.Provider, argsJSON string) ToolResult {
	// Parse fetch arguments
	var fetchReq FetchRequest
	if err := json.Unmarshal([]byte(argsJSON), &fetchReq); err != nil {
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Invalid fetch arguments: %v", err),
			IsError: true,
		}
	}

	if fetchReq.URL == "" {
		return ToolResult{
			Content: "",
			Error:   "URL is required",
			IsError: true,
		}
	}

	// Execute fetch
	content, err := i.fetchHandler.FetchAndExtract(fetchReq.URL)
	if err != nil {
		return ToolResult{
			Content: "",
			Error:   fmt.Sprintf("Fetch failed: %v", err),
			IsError: true,
		}
	}

	return ToolResult{
		Content: content,
		IsError: false,
	}
}

// parseOpenAIMessage parses an OpenAI message union type
func parseOpenAIMessage(msgUnion openai.ChatCompletionMessageParamUnion) (map[string]interface{}, error) {
	var msgMap map[string]interface{}

	// Marshal to JSON and back to parse the union type
	bytes, err := json.Marshal(msgUnion)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(bytes, &msgMap)
	return msgMap, err
}

// parseAnthropicMessage parses an Anthropic message param
func parseAnthropicMessage(msg anthropic.MessageParam) (map[string]interface{}, error) {
	var msgMap map[string]interface{}

	bytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(bytes, &msgMap)
	return msgMap, err
}

// CreateOpenAIToolResultMessage creates an OpenAI tool result message as a map

// CreateAnthropicToolResultBlock creates an Anthropic tool_result content block
func CreateAnthropicToolResultBlock(toolUseID, content string, isError bool) anthropic.ContentBlockParamUnion {
	resultContent := content
	if isError {
		resultContent = fmt.Sprintf("Error: %s", content)
	}

	// NewToolResultBlock returns ContentBlockParamUnion
	return anthropic.NewToolResultBlock(toolUseID, resultContent, isError)
}

// StripSearchFetchToolsAnthropic removes search/fetch tool definitions from Anthropic tools array
func StripSearchFetchToolsAnthropic(tools []anthropic.ToolUnionParam) []anthropic.ToolUnionParam {
	if tools == nil {
		return nil
	}

	filtered := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		t := tool.OfTool
		if t == nil {
			filtered = append(filtered, tool)
			continue
		}

		if !ShouldInterceptTool(t.Name) {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}

// ShouldStripToolChoice checks if tool_choice should be stripped (only contains search/fetch tools)
func ShouldStripToolChoice(req *openai.ChatCompletionNewParams) bool {
	// If tool_choice is "auto", we shouldn't strip it
	if req.ToolChoice.OfAuto.Value != "" {
		return false
	}

	// Check if tool_choice specifies only search/fetch tools
	if allowedTools := req.ToolChoice.OfAllowedTools; allowedTools != nil {
		// If any tool is not a search/fetch tool, don't strip
		// We need to marshal/unmarshal to inspect the contents
		bytes, _ := json.Marshal(allowedTools)
		var allowed []map[string]interface{}
		json.Unmarshal(bytes, &allowed)

		for _, toolRef := range allowed {
			if toolRef["type"] != "function" {
				return false
			}
			name, ok := toolRef["name"].(string)
			if !ok || !ShouldInterceptTool(name) {
				return false
			}
		}
		// All referenced tools are search/fetch tools
		return true
	}

	// Check function tool choice
	if funcChoice := req.ToolChoice.OfFunctionToolChoice; funcChoice != nil {
		if !ShouldInterceptTool(funcChoice.Function.Name) {
			return false
		}
		return true
	}

	return false
}

// SearchWithConfig executes a search with the given configuration
func (i *Interceptor) SearchWithConfig(query string, count int, config *Config) ([]SearchResult, error) {
	return i.searchHandler.SearchWithConfig(query, count, config)
}

// FetchAndExtract fetches a URL and extracts the main content
func (i *Interceptor) FetchAndExtract(url string) (string, error) {
	return i.fetchHandler.FetchAndExtract(url)
}
