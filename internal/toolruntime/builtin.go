package toolruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	BuiltinToolSearch = "web_search"
	BuiltinToolFetch  = "web_fetch"
)

type builtinSource struct {
	id     string
	search *builtinSearchHandler
	cache  *builtinCache
	config *builtinConfig
}

func newBuiltinSource(id string, config *typ.BuiltinToolSourceConfig) Source {
	handlerConfig := &builtinConfig{
		SearchAPI:    config.SearchAPI,
		SearchKey:    config.SearchKey,
		MaxResults:   config.MaxResults,
		ProxyURL:     config.ProxyURL,
		MaxFetchSize: config.MaxFetchSize,
		FetchTimeout: config.FetchTimeout,
		MaxURLLength: config.MaxURLLength,
	}
	cache := newBuiltinCache()
	return &builtinSource{
		id:     id,
		search: newBuiltinSearchHandler(handlerConfig, cache),
		cache:  cache,
		config: handlerConfig,
	}
}

func (s *builtinSource) ID() string { return s.id }

func (s *builtinSource) OwnsTool(name string) bool {
	return name == BuiltinToolSearch || name == BuiltinToolFetch
}

func (s *builtinSource) ListTools(context.Context) ([]ToolDescriptor, error) {
	return []ToolDescriptor{
		{
			Name:        BuiltinToolSearch,
			Description: "Search the web for current information.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{"type": "string"},
					"count": map[string]interface{}{"type": "integer"},
				},
				"required": []string{"query"},
			},
			SourceID: s.id,
		},
		{
			Name:        BuiltinToolFetch,
			Description: "Fetch a URL and extract readable page content.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"url": map[string]interface{}{"type": "string"},
				},
				"required": []string{"url"},
			},
			SourceID: s.id,
		},
	}, nil
}

func (s *builtinSource) CallTool(_ context.Context, name string, arguments string) ToolResult {
	switch name {
	case BuiltinToolSearch:
		var req builtinSearchRequest
		if err := json.Unmarshal([]byte(arguments), &req); err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("invalid search arguments: %v", err)}
		}
		if req.Query == "" {
			return ToolResult{IsError: true, Error: "search query is required"}
		}
		results, err := s.search.SearchWithConfig(req.Query, req.Count, s.config)
		if err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("search failed: %v", err)}
		}
		return ToolResult{Content: formatBuiltinSearchResults(req.Query, results)}
	case BuiltinToolFetch:
		var req builtinFetchRequest
		if err := json.Unmarshal([]byte(arguments), &req); err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("invalid fetch arguments: %v", err)}
		}
		if req.URL == "" {
			return ToolResult{IsError: true, Error: "url is required"}
		}
		if err := validateBuiltinFetchURL(req.URL, s.config); err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("fetch blocked: %v", err)}
		}
		result, err := s.fetchURL(req.URL)
		if err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("fetch failed: %v", err)}
		}
		content, err := json.Marshal(result)
		if err != nil {
			return ToolResult{IsError: true, Error: fmt.Sprintf("failed to encode fetch result: %v", err)}
		}
		return ToolResult{Content: string(content)}
	default:
		return ToolResult{IsError: true, Error: fmt.Sprintf("unknown builtin tool: %s", name)}
	}
}
