package webtools

import (
	"context"
	"encoding/json"
	"fmt"
)

type FetchTool struct {
	webtools *WebTools
}

func (t *FetchTool) Name() string {
	return "web_fetch"
}

func (t *FetchTool) Description() string {
	return "Fetch and extract content from a web page. Supports plain text, HTML, and markdown extraction. Can also extract specific information using a prompt."
}

func (t *FetchTool) Parameters() map[string]Parameter {
	return map[string]Parameter{
		"url": {
			Type:        "string",
			Description: "The URL of the web page to fetch",
			Required:    true,
		},
		"extract": {
			Type:        "string",
			Description: "Extraction mode: text (plain text), html (raw HTML), markdown (converted to markdown)",
			Required:    false,
			Default:     "text",
		},
		"prompt": {
			Type:        "string",
			Description: "Optional prompt to extract specific information from the page using AI",
			Required:    false,
		},
	}
}

func (t *FetchTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("missing required parameter: url")
	}

	extract := "text"
	if e, ok := params["extract"].(string); ok {
		extract = e
	}

	result, err := t.webtools.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}

	type FetchResponse struct {
		Content string `json:"content"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		Extract string `json:"extract_mode"`
	}

	return FetchResponse{
		Content: result.Content,
		Title:   result.Title,
		URL:     result.URL,
		Extract: extract,
	}, nil
}

func (t *FetchTool) ToJSON() string {
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
