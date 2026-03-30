package toolruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type ConfigProvider func(providerUUID string) (*typ.ToolRuntimeConfig, bool)

type Runtime struct {
	configProvider ConfigProvider
	mu             sync.Mutex
	mcpSources     map[string]*mcpSource
}

type NativeToolSupport map[string]bool

func New(configProvider ConfigProvider) *Runtime {
	return &Runtime{
		configProvider: configProvider,
		mcpSources:     map[string]*mcpSource{},
	}
}

func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, source := range r.mcpSources {
		source.close()
		delete(r.mcpSources, id)
	}
}

func (r *Runtime) IsEnabledForProvider(provider *typ.Provider) bool {
	cfg := r.GetConfigForProvider(provider)
	return cfg != nil && bool(cfg.Enabled)
}

func (r *Runtime) GetConfigForProvider(provider *typ.Provider) *typ.ToolRuntimeConfig {
	if provider == nil || r.configProvider == nil {
		return typ.DefaultToolRuntimeConfig()
	}
	cfg, enabled := r.configProvider(provider.UUID)
	if !enabled {
		return nil
	}
	if cfg == nil {
		cfg = typ.DefaultToolRuntimeConfig()
	}
	typ.ApplyToolRuntimeDefaults(cfg)
	return cfg
}

func (r *Runtime) buildSources(config *typ.ToolRuntimeConfig) []Source {
	sources := make([]Source, 0, len(config.Sources))
	for _, sourceCfg := range config.Sources {
		if !bool(sourceCfg.Enabled) {
			continue
		}
		switch sourceCfg.Type {
		case typ.ToolSourceTypeBuiltin:
			if sourceCfg.Builtin == nil {
				cfg := typ.DefaultBuiltinToolSourceConfig()
				sourceCfg.Builtin = cfg.Builtin
			}
			sources = append(sources, newBuiltinSource(sourceCfg.ID, sourceCfg.Builtin))
		case typ.ToolSourceTypeMCP:
			sources = append(sources, r.getOrCreateMCPSource(sourceCfg.ID, sourceCfg.MCP))
		}
	}
	return sources
}

func (r *Runtime) getOrCreateMCPSource(id string, config *typ.MCPToolSourceConfig) Source {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.mcpSources[id]; ok {
		existing.config = config
		return existing
	}
	src := newMCPSource(id, config).(*mcpSource)
	r.mcpSources[id] = src
	return src
}

func (r *Runtime) IsRuntimeTool(provider *typ.Provider, name string) bool {
	tools, _ := r.ListTools(context.Background(), provider, nil)
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (r *Runtime) ListTools(ctx context.Context, provider *typ.Provider, nativeTools NativeToolSupport) ([]ToolDescriptor, error) {
	cfg := r.GetConfigForProvider(provider)
	if cfg == nil || !bool(cfg.Enabled) || !bool(cfg.AutoExpose) {
		return nil, nil
	}
	sources := r.buildSources(cfg)
	var tools []ToolDescriptor
	for _, source := range sources {
		listed, err := source.ListTools(ctx)
		if err != nil {
			logrus.WithError(err).Warnf("failed to list tools for source %s", source.ID())
			continue
		}
		for _, tool := range listed {
			if nativeTools != nil && nativeTools[tool.Name] {
				continue
			}
			tools = append(tools, tool)
		}
	}
	return tools, nil
}

func (r *Runtime) ExecuteTool(ctx context.Context, provider *typ.Provider, toolName string, argsJSON string) ToolResult {
	cfg := r.GetConfigForProvider(provider)
	if cfg == nil || !bool(cfg.Enabled) {
		return ToolResult{IsError: true, Error: "tool runtime disabled"}
	}
	for _, source := range r.buildSources(cfg) {
		if source.OwnsTool(toolName) {
			return source.CallTool(ctx, toolName, argsJSON)
		}
	}
	return ToolResult{IsError: true, Error: fmt.Sprintf("unknown runtime tool: %s", toolName)}
}

func mergeOpenAITools(existing []openai.ChatCompletionToolUnionParam, runtimeTools []ToolDescriptor) []openai.ChatCompletionToolUnionParam {
	seen := map[string]bool{}
	merged := make([]openai.ChatCompletionToolUnionParam, 0, len(existing)+len(runtimeTools))
	for _, tool := range existing {
		if fn := tool.GetFunction(); fn != nil {
			seen[fn.Name] = true
		}
		merged = append(merged, tool)
	}
	for _, tool := range runtimeTools {
		if seen[tool.Name] {
			continue
		}
		merged = append(merged, openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
			Name:        tool.Name,
			Description: openai.Opt(tool.Description),
			Parameters:  tool.Parameters,
		}))
	}
	return merged
}

func mergeAnthropicTools(existing []anthropic.ToolUnionParam, runtimeTools []ToolDescriptor) []anthropic.ToolUnionParam {
	seen := map[string]bool{}
	merged := make([]anthropic.ToolUnionParam, 0, len(existing)+len(runtimeTools))
	for _, tool := range existing {
		if t := tool.OfTool; t != nil {
			seen[t.Name] = true
		}
		merged = append(merged, tool)
	}
	for _, tool := range runtimeTools {
		if seen[tool.Name] {
			continue
		}
		merged = append(merged, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropicparam.Opt[string]{Value: tool.Description},
				InputSchema: buildAnthropicSchema(tool.Parameters),
			},
		})
	}
	return merged
}

func mergeAnthropicBetaTools(existing []anthropic.BetaToolUnionParam, runtimeTools []ToolDescriptor) []anthropic.BetaToolUnionParam {
	seen := map[string]bool{}
	merged := make([]anthropic.BetaToolUnionParam, 0, len(existing)+len(runtimeTools))
	for _, tool := range existing {
		if t := tool.OfTool; t != nil {
			seen[t.Name] = true
		}
		merged = append(merged, tool)
	}
	for _, tool := range runtimeTools {
		if seen[tool.Name] {
			continue
		}
		merged = append(merged, anthropic.BetaToolUnionParam{
			OfTool: &anthropic.BetaToolParam{
				Name:        tool.Name,
				Description: anthropicparam.Opt[string]{Value: tool.Description},
				InputSchema: buildAnthropicBetaSchema(tool.Parameters),
			},
		})
	}
	return merged
}

func (r *Runtime) InjectOpenAITools(ctx context.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, nativeTools NativeToolSupport) *openai.ChatCompletionNewParams {
	modifiedReq := originalReq
	runtimeTools, _ := r.ListTools(ctx, provider, nativeTools)
	if len(runtimeTools) > 0 {
		modifiedReq.Tools = mergeOpenAITools(modifiedReq.Tools, runtimeTools)
	}
	return modifiedReq
}

func (r *Runtime) PrepareOpenAIRequest(ctx context.Context, provider *typ.Provider, originalReq *openai.ChatCompletionNewParams, nativeTools NativeToolSupport) (*openai.ChatCompletionNewParams, bool) {
	modifiedReq := r.InjectOpenAITools(ctx, provider, originalReq, nativeTools)

	results := []ToolResult{}
	for _, msgUnion := range modifiedReq.Messages {
		msgMap, err := parseOpenAIMessage(msgUnion)
		if err != nil || msgMap["role"] != "assistant" {
			continue
		}
		toolCalls, ok := msgMap["tool_calls"].([]interface{})
		if !ok {
			continue
		}
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
			args, _ := fnMap["arguments"].(string)
			if !r.IsRuntimeTool(provider, name) {
				continue
			}
			result := r.ExecuteTool(ctx, provider, name, args)
			result.ToolCallID = id
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return modifiedReq, false
	}
	newMessages := append([]openai.ChatCompletionMessageParamUnion{}, modifiedReq.Messages...)
	for _, result := range results {
		content := result.Content
		if result.IsError {
			content = "Error: " + result.Error
		}
		newMessages = append(newMessages, openai.ToolMessage(content, result.ToolCallID))
	}
	modifiedReq.Messages = newMessages
	return modifiedReq, true
}

func (r *Runtime) PrepareAnthropicRequest(ctx context.Context, provider *typ.Provider, originalReq *anthropic.MessageNewParams, nativeTools NativeToolSupport) (*anthropic.MessageNewParams, bool) {
	modifiedReq := originalReq
	runtimeTools, _ := r.ListTools(ctx, provider, nativeTools)
	if len(runtimeTools) > 0 {
		modifiedReq.Tools = mergeAnthropicTools(modifiedReq.Tools, runtimeTools)
	}
	results := []ToolResult{}
	for _, msg := range modifiedReq.Messages {
		msgMap, err := parseAnthropicMessage(msg)
		if err != nil || msgMap["role"] != "assistant" {
			continue
		}
		contentBlocks, ok := msgMap["content"].([]interface{})
		if !ok {
			continue
		}
		for _, block := range contentBlocks {
			blockMap, ok := block.(map[string]interface{})
			if !ok || blockMap["type"] != "tool_use" {
				continue
			}
			name, _ := blockMap["name"].(string)
			if !r.IsRuntimeTool(provider, name) {
				continue
			}
			id, _ := blockMap["id"].(string)
			inputBytes, _ := json.Marshal(blockMap["input"])
			result := r.ExecuteTool(ctx, provider, name, string(inputBytes))
			result.ToolCallID = id
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return modifiedReq, false
	}
	newMessages := append([]anthropic.MessageParam{}, modifiedReq.Messages...)
	for _, result := range results {
		content := result.Content
		if result.IsError {
			content = result.Error
		}
		newMessages = append(newMessages, anthropic.NewUserMessage(CreateAnthropicToolResultBlock(result.ToolCallID, content, result.IsError)))
	}
	modifiedReq.Messages = newMessages
	return modifiedReq, true
}

func (r *Runtime) PrepareAnthropicBetaRequest(ctx context.Context, provider *typ.Provider, originalReq *anthropic.BetaMessageNewParams, nativeTools NativeToolSupport) (*anthropic.BetaMessageNewParams, bool) {
	modifiedReq := originalReq
	runtimeTools, _ := r.ListTools(ctx, provider, nativeTools)
	if len(runtimeTools) > 0 {
		modifiedReq.Tools = mergeAnthropicBetaTools(modifiedReq.Tools, runtimeTools)
	}
	results := []ToolResult{}
	for _, msg := range modifiedReq.Messages {
		if msg.Role != anthropic.BetaMessageParamRoleAssistant {
			continue
		}
		for _, block := range msg.Content {
			if block.OfToolUse == nil || !r.IsRuntimeTool(provider, block.OfToolUse.Name) {
				continue
			}
			inputBytes, _ := json.Marshal(block.OfToolUse.Input)
			result := r.ExecuteTool(ctx, provider, block.OfToolUse.Name, string(inputBytes))
			result.ToolCallID = block.OfToolUse.ID
			results = append(results, result)
		}
	}
	if len(results) == 0 {
		return modifiedReq, false
	}
	newMessages := append([]anthropic.BetaMessageParam{}, modifiedReq.Messages...)
	for _, result := range results {
		content := result.Content
		if result.IsError {
			content = result.Error
		}
		newMessages = append(newMessages, anthropic.NewBetaUserMessage(CreateAnthropicBetaToolResultBlock(result.ToolCallID, content, result.IsError)))
	}
	modifiedReq.Messages = newMessages
	return modifiedReq, true
}

func parseOpenAIMessage(msgUnion openai.ChatCompletionMessageParamUnion) (map[string]interface{}, error) {
	var msgMap map[string]interface{}
	bytes, err := json.Marshal(msgUnion)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &msgMap)
	return msgMap, err
}

func parseAnthropicMessage(msg anthropic.MessageParam) (map[string]interface{}, error) {
	var msgMap map[string]interface{}
	bytes, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bytes, &msgMap)
	return msgMap, err
}

func CreateAnthropicToolResultBlock(toolUseID, content string, isError bool) anthropic.ContentBlockParamUnion {
	resultContent := content
	if isError {
		resultContent = fmt.Sprintf("Error: %s", content)
	}
	return anthropic.NewToolResultBlock(toolUseID, resultContent, isError)
}

func CreateAnthropicBetaToolResultBlock(toolUseID, content string, isError bool) anthropic.BetaContentBlockParamUnion {
	resultContent := content
	if isError {
		resultContent = fmt.Sprintf("Error: %s", content)
	}
	block := anthropic.NewBetaToolResultBlock(toolUseID, resultContent, isError)
	if block.OfToolResult != nil {
		block.OfToolResult.IsError = anthropic.Bool(isError)
		block.OfToolResult.Content = []anthropic.BetaToolResultBlockParamContentUnion{{
			OfText: &anthropic.BetaTextBlockParam{Text: resultContent},
		}}
	}
	return block
}

func buildAnthropicSchema(parameters map[string]interface{}) anthropic.ToolInputSchemaParam {
	schema := anthropic.ToolInputSchemaParam{}
	if properties, ok := parameters["properties"]; ok {
		schema.Properties = properties
	}
	if required, ok := parameters["required"].([]string); ok {
		schema.Required = required
	} else if requiredAny, ok := parameters["required"].([]interface{}); ok {
		required := make([]string, 0, len(requiredAny))
		for _, item := range requiredAny {
			if s, ok := item.(string); ok {
				required = append(required, s)
			}
		}
		schema.Required = required
	}
	if extra := map[string]interface{}{}; len(parameters) > 0 {
		for key, value := range parameters {
			if key == "properties" || key == "required" || key == "type" {
				continue
			}
			extra[key] = value
		}
		if len(extra) > 0 {
			schema.ExtraFields = extra
		}
	}
	return schema
}

func buildAnthropicBetaSchema(parameters map[string]interface{}) anthropic.BetaToolInputSchemaParam {
	schema := anthropic.BetaToolInputSchemaParam{}
	if properties, ok := parameters["properties"]; ok {
		schema.Properties = properties
	}
	if required, ok := parameters["required"].([]string); ok {
		schema.Required = required
	} else if requiredAny, ok := parameters["required"].([]interface{}); ok {
		required := make([]string, 0, len(requiredAny))
		for _, item := range requiredAny {
			if s, ok := item.(string); ok {
				required = append(required, s)
			}
		}
		schema.Required = required
	}
	if extra := map[string]interface{}{}; len(parameters) > 0 {
		for key, value := range parameters {
			if key == "properties" || key == "required" || key == "type" {
				continue
			}
			extra[key] = value
		}
		if len(extra) > 0 {
			schema.ExtraFields = extra
		}
	}
	return schema
}
