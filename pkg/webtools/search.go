package webtools

import (
	"context"
	"encoding/json"
	"fmt"
)

// SearchTool 搜索工具 (MCP 工具格式)
type SearchTool struct {
	webtools *WebTools
}

// Name 工具名称
func (t *SearchTool) Name() string {
	return "web_search"
}

// Description 工具描述
func (t *SearchTool) Description() string {
	return "Search the web for information using Google or other search engines. Returns search results with titles, URLs, and snippets."
}

// Parameters 参数定义
func (t *SearchTool) Parameters() map[string]Parameter {
	return map[string]Parameter{
		"query": {
			Type:        "string",
			Description: "The search query string",
			Required:    true,
		},
		"num_results": {
			Type:        "integer",
			Description: "Number of results to return (default: 10)",
			Required:    false,
			Default:     10,
		},
		"engine": {
			Type:        "string",
			Description: "Search engine to use (google, bing, duckduckgo)",
			Required:    false,
			Default:     "google",
		},
	}
}

// Execute 执行搜索
func (t *SearchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("missing required parameter: query")
	}

	numResults := 10
	if nr, ok := params["num_results"].(float64); ok {
		numResults = int(nr)
	}

	engine := "google"
	if e, ok := params["engine"].(string); ok {
		engine = e
	}

	results, err := t.webtools.Search(ctx, query, numResults)
	if err != nil {
		return nil, err
	}

	// 转换为 MCP 格式
	type SearchResponse struct {
		Results []SearchResult `json:"results"`
		Query   string        `json:"query"`
		Engine  string        `json:"engine"`
	}

	return SearchResponse{
		Results: results,
		Query:   query,
		Engine:  engine,
	}, nil
}

// ToJSON 转换为 JSON (用于 MCP)
func (t *SearchTool) ToJSON() string {
	params := make(map[string]map[string]interface{})
	for k, v := range t.Parameters() {
		params[k] = map[string]interface{}{
			"type":        v.Type,
			"description": v.Description,
			"required":    v.Required,
		}
		if v.Default != nil {
			params[k]["default"] = v.Default
		}
	}

	out := map[string]interface{}{
		"name":        t.Name(),
		"description": t.Description(),
		"parameters": map[string]interface{}{
			"type":       "object",
			"properties": params,
		},
	}

	b, _ := json.MarshalIndent(out, "", "  ")
	return string(b)
}
