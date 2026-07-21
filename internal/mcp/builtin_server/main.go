package builtinserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
)

// Endpoints are variables so tests can point them at a local httptest server.
var (
	serperAPIEndpoint  = "https://google.serper.dev/search"
	jinaReaderEndpoint = "https://r.jina.ai/"
)

// The Serper backend can transiently fail or rate-limit under parallel bursts,
// sometimes as an HTTP 200 whose body carries no results; retry with a short
// backoff instead of surfacing a silent empty result.
const webSearchMaxAttempts = 3

var webSearchRetryBaseWait = 500 * time.Millisecond

// Serve starts the builtin MCP server on stdio.
// This is the main entry point for the builtin MCP server.
func Serve() error {
	// Create MCP server
	mcpServer := server.NewMCPServer(
		"tingly-box-builtin",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register web search tool
	webSearchTool := mcp.Tool{
		Name:        mcptools.BuiltinWebSearchToolName,
		Description: "Search web pages with Serper and return top organic results.",
	}

	// Set input schema via JSON conversion
	if err := setToolInputSchema(&webSearchTool, map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query.",
			},
			"allowed_domains": map[string]interface{}{
				"type":        "array",
				"items":       map[string]string{"type": "string"},
				"description": "Optional domain allow list.",
			},
			"blocked_domains": map[string]interface{}{
				"type":        "array",
				"items":       map[string]string{"type": "string"},
				"description": "Optional domain block list.",
			},
		},
		"required": []string{"query"},
	}); err != nil {
		return fmt.Errorf("failed to set web search schema: %w", err)
	}

	mcpServer.AddTool(webSearchTool, handleWebSearch)

	// Register web fetch tool
	webFetchTool := mcp.Tool{
		Name:        mcptools.BuiltinWebFetchToolName,
		Description: "Fetch and convert a URL to markdown-like text via Jina Reader.",
	}

	if err := setToolInputSchema(&webFetchTool, map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "Target URL.",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "Extraction instruction for content focus.",
			},
		},
		"required": []string{"url", "prompt"},
	}); err != nil {
		return fmt.Errorf("failed to set web fetch schema: %w", err)
	}

	mcpServer.AddTool(webFetchTool, handleWebFetch)

	// Serve stdio (blocking call)
	if err := server.ServeStdio(mcpServer); err != nil {
		return fmt.Errorf("error serving MCP server: %w", err)
	}

	return nil
}

// setToolInputSchema sets the input schema for a tool
func setToolInputSchema(tool *mcp.Tool, schema map[string]interface{}) error {
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return err
	}

	var inputSchema mcp.ToolInputSchema
	if err := json.Unmarshal(schemaBytes, &inputSchema); err != nil {
		return err
	}

	tool.InputSchema = inputSchema
	return nil
}

// handleWebSearch implements the web search tool handler
func handleWebSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract arguments
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: invalid arguments"},
			},
			IsError: true,
		}, nil
	}

	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: query is required"},
			},
			IsError: true,
		}, nil
	}

	// Call web search implementation
	result, err := webSearchImpl(ctx, args)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: " + err.Error()},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: result},
		},
	}, nil
}

// handleWebFetch implements the web fetch tool handler
func handleWebFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract arguments
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: invalid arguments"},
			},
			IsError: true,
		}, nil
	}

	url, ok := args["url"].(string)
	if !ok || strings.TrimSpace(url) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: url is required"},
			},
			IsError: true,
		}, nil
	}

	prompt, ok := args["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: prompt is required"},
			},
			IsError: true,
		}, nil
	}

	// Call web fetch implementation
	result, err := webFetchImpl(ctx, args)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{Type: "text", Text: "Error: " + err.Error()},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: result},
		},
	}, nil
}

// webSearchImpl implements the actual web search logic
func webSearchImpl(ctx context.Context, args map[string]interface{}) (string, error) {
	// Extract query
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}

	// Get SERPER_API_KEY from environment
	apiKey := os.Getenv("SERPER_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("SERPER_API_KEY environment variable is not set")
	}

	// Build search query with optional domain filters as a single q value
	finalQuery := query
	if allowedDomains, ok := args["allowed_domains"].([]interface{}); ok && len(allowedDomains) > 0 {
		var siteExprs []string
		for _, d := range allowedDomains {
			if domainStr, ok := d.(string); ok {
				siteExprs = append(siteExprs, "site:"+domainStr)
			}
		}
		if len(siteExprs) > 0 {
			finalQuery = finalQuery + " (" + strings.Join(siteExprs, " OR ") + ")"
		}
	}
	if blockedDomains, ok := args["blocked_domains"].([]interface{}); ok && len(blockedDomains) > 0 {
		for _, d := range blockedDomains {
			if domainStr, ok := d.(string); ok {
				finalQuery = finalQuery + " -site:" + domainStr
			}
		}
	}

	// Build POST payload
	payload := map[string]interface{}{
		"q":   finalQuery,
		"num": 10,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	var result serperSearchResult
	var lastErr error
	for attempt := 1; attempt <= webSearchMaxAttempts; attempt++ {
		if attempt > 1 {
			wait := webSearchRetryBaseWait << (attempt - 2) // 500ms, 1s, ...
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}

		var retryable bool
		result, retryable, lastErr = serperSearchOnce(ctx, client, apiKey, payloadBytes)
		if lastErr != nil {
			if !retryable {
				return "", lastErr
			}
			fmt.Fprintf(os.Stderr, "mcp_web_search: attempt %d/%d failed: %v\n", attempt, webSearchMaxAttempts, lastErr)
			continue
		}
		if len(result.Organic) > 0 || result.AnswerBox.Answer != "" {
			break
		}
		fmt.Fprintf(os.Stderr, "mcp_web_search: attempt %d/%d returned no results for %q\n", attempt, webSearchMaxAttempts, finalQuery)
	}
	if lastErr != nil {
		return "", lastErr
	}

	// Format results
	var output strings.Builder
	output.WriteString(fmt.Sprintf("# Web Search Results for: %s\n\n", query))

	if result.AnswerBox.Answer != "" {
		output.WriteString(fmt.Sprintf("**Quick Answer:** %s\n\n", result.AnswerBox.Answer))
	}

	for i, item := range result.Organic {
		if i >= 10 { // Limit to top 10 results
			break
		}
		output.WriteString(fmt.Sprintf("%d. **[%s](%s)**\n", i+1, item.Title, item.Link))
		output.WriteString(fmt.Sprintf("   %s\n\n", item.Snippet))
	}

	if result.AnswerBox.Answer == "" && len(result.Organic) == 0 {
		output.WriteString("No results found for this query. Try rewording it or removing site: filters.\n")
	}

	return output.String(), nil
}

// serperSearchResult mirrors the fields of a Serper response we consume.
// searchParameters is echoed back on every well-formed response, so its
// absence on an HTTP 200 marks a backend error body rather than a result set.
type serperSearchResult struct {
	SearchParameters json.RawMessage `json:"searchParameters"`
	Message          string          `json:"message"`
	Organic          []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"organic"`
	AnswerBox struct {
		Answer string `json:"answer"`
	} `json:"answerBox"`
}

// serperSearchOnce performs a single Serper API call. retryable reports
// whether the failure is worth another attempt (network errors, 429/5xx,
// malformed 200 bodies) as opposed to a hard client error such as a bad key.
func serperSearchOnce(ctx context.Context, client *http.Client, apiKey string, payload []byte) (serperSearchResult, bool, error) {
	var result serperSearchResult

	req, err := http.NewRequestWithContext(ctx, "POST", serperAPIEndpoint, bytes.NewReader(payload))
	if err != nil {
		return result, false, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return result, true, fmt.Errorf("failed to execute search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096000))
	if err != nil {
		return result, true, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return result, retryable, fmt.Errorf("search failed with status %d: %s", resp.StatusCode, truncateForError(body))
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return result, true, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Message != "" {
		return result, true, fmt.Errorf("search backend returned error: %s", result.Message)
	}
	if len(result.SearchParameters) == 0 {
		return result, true, fmt.Errorf("search backend returned malformed response: %s", truncateForError(body))
	}

	return result, true, nil
}

func truncateForError(body []byte) string {
	const max = 512
	if len(body) > max {
		return string(body[:max]) + "..."
	}
	return string(body)
}

// webFetchImpl implements the actual web fetch logic
func webFetchImpl(ctx context.Context, args map[string]interface{}) (string, error) {
	// Extract URL
	targetURL, ok := args["url"].(string)
	if !ok || strings.TrimSpace(targetURL) == "" {
		return "", fmt.Errorf("url is required")
	}

	// Validate URL format
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		return "", fmt.Errorf("invalid URL format")
	}

	// Build Jina Reader URL
	fetchURL := jinaReaderEndpoint + targetURL

	// Add optional prompt parameter
	prompt := ""
	if p, ok := args["prompt"].(string); ok && strings.TrimSpace(p) != "" {
		prompt = strings.TrimSpace(p)
		fetchURL += "/" + url.QueryEscape(prompt)
	}

	// Make HTTP request
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "Tingly-Box/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	content := string(body)

	// Add metadata header
	var output strings.Builder
	output.WriteString(fmt.Sprintf("# Fetched Content from: %s\n\n", targetURL))
	if prompt != "" {
		output.WriteString(fmt.Sprintf("**Extraction Prompt:** %s\n\n", prompt))
	}
	output.WriteString("---\n\n")
	output.WriteString(content)

	return output.String(), nil
}
