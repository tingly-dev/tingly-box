// Package deepseek implements DeepSeek's read-only, per-request billed web APIs.
package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const PriceCNY = 0.005 // CNY per successful request producing results.

// Service resolves credentials on every call, so rotations and disabled providers
// take effect without restarting an MCP subprocess or persisting another secret.
type Service struct {
	Resolve func() *typ.Provider
	Record  func(context.Context, *typ.Provider, string, bool, time.Duration, error)
}

func (s *Service) Available() bool { return s != nil && s.Resolve != nil && s.Resolve() != nil }

// Call preserves the upstream status and JSON body for gateway passthrough.
func (s *Service) Call(ctx context.Context, operation string, payload []byte) (body []byte, status int, err error) {
	if operation != "search" && operation != "fetch" {
		return nil, 400, fmt.Errorf("unsupported web operation")
	}
	if s == nil || s.Resolve == nil {
		return nil, 503, fmt.Errorf("no enabled DeepSeek API-key provider configured")
	}
	p := s.Resolve()
	if p == nil {
		return nil, 503, fmt.Errorf("no enabled DeepSeek API-key provider configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	started := time.Now()
	defer func() {
		if s.Record != nil {
			s.Record(ctx, p, operation, status == 200 && HasResults(operation, body), time.Since(started), err)
		}
	}()
	base := p.APIBaseOpenAI
	if base == "" {
		base = p.APIBase
	}
	u, parseErr := url.Parse(base)
	if parseErr != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, 502, fmt.Errorf("invalid DeepSeek provider base URL")
	}
	// DeepSeek also offers an Anthropic base; tools always live under /v1.
	u.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1"), "/anthropic") + "/v1/tools/" + operation
	u.RawQuery, u.Fragment = "", ""
	httpClient := *client.CreateHTTPClientWithProxy(p.ProxyURL)
	httpClient.Timeout = 30 * time.Second
	// Never forward the provider credential across redirects.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(1<<(attempt-1)) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, 504, ctx.Err()
			case <-timer.C:
			}
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
		if e != nil {
			return nil, 502, e
		}
		req.Header.Set("Authorization", "Bearer "+p.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, e := httpClient.Do(req)
		if e != nil {
			return nil, 502, fmt.Errorf("DeepSeek %s request failed: %w", operation, e)
		}
		status = resp.StatusCode
		body, e = io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
		resp.Body.Close()
		if e != nil {
			return nil, 502, fmt.Errorf("read DeepSeek response: %w", e)
		}
		if len(body) > 8*1024*1024 {
			return nil, 502, fmt.Errorf("DeepSeek response exceeds size limit")
		}
		if status == 200 {
			if err := validateResponse(operation, body); err != nil {
				// A malformed 200 must not be retried: the successful upstream
				// call may already have been billed.
				return nil, 502, err
			}
			return body, status, nil
		}
		if (status == 408 || status == 429 || status >= 500) && attempt < 2 {
			continue
		}
		if status == 402 {
			return body, status, fmt.Errorf("insufficient balance: top up your DeepSeek balance")
		}
		var envelope struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &envelope)
		return body, status, fmt.Errorf("DeepSeek %s failed (HTTP %d): %s", operation, status, envelope.Error.Message)
	}
	panic("unreachable")
}

func validateResponse(operation string, body []byte) error {
	if operation == "search" {
		var response struct {
			Results *[]json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(body, &response); err != nil || response.Results == nil {
			return fmt.Errorf("invalid DeepSeek search response: expected results array")
		}
	} else {
		var response struct {
			Content *string `json:"content"`
		}
		if err := json.Unmarshal(body, &response); err != nil || response.Content == nil {
			return fmt.Errorf("invalid DeepSeek fetch response: expected content string")
		}
	}
	return nil
}

// HasResults applies DeepSeek's non-empty-result requirement for billing.
func HasResults(operation string, body []byte) bool {
	if operation == "search" {
		var response struct {
			Results []json.RawMessage `json:"results"`
		}
		return json.Unmarshal(body, &response) == nil && len(response.Results) > 0
	}
	var response struct {
		Content string `json:"content"`
	}
	return json.Unmarshal(body, &response) == nil && strings.TrimSpace(response.Content) != ""
}

// Tool maps the stable built-in MCP arguments and renders model-oriented text.
// ori_question is passed only when explicitly supplied, never from conversation.
func (s *Service) Tool(ctx context.Context, operation string, args map[string]any) (string, error) {
	payload := map[string]any{}
	if operation == "search" {
		query, _ := args["query"].(string)
		if strings.TrimSpace(query) == "" {
			return "", fmt.Errorf("query is required")
		}
		var allowed []string
		domains := func(value any) []string {
			var result []string
			switch values := value.(type) {
			case []any:
				for _, v := range values {
					if text, ok := v.(string); ok && text != "" {
						result = append(result, text)
					}
				}
			case []string:
				result = values
			}
			return result
		}
		for _, domain := range domains(args["allowed_domains"]) {
			allowed = append(allowed, "site:"+domain)
		}
		if len(allowed) > 0 {
			query += " (" + strings.Join(allowed, " OR ") + ")"
		}
		for _, domain := range domains(args["blocked_domains"]) {
			query += " -site:" + domain
		}
		payload["query"] = query
		for _, key := range []string{"count", "need_content", "ori_question", "localization"} {
			if value, ok := args[key]; ok {
				payload[key] = value
			}
		}
	} else {
		target, _ := args["url"].(string)
		u, err := url.Parse(target)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", fmt.Errorf("url must be an HTTP/HTTPS URL with a host and without credentials")
		}
		payload["url"] = target
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	body, _, err := s.Call(ctx, operation, data)
	if err != nil {
		return "", err
	}
	if operation == "fetch" {
		var page struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return "", err
		}
		return fmt.Sprintf("# %s\n\n%s", page.Title, page.Content), nil
	}
	var response struct {
		Results []struct {
			Title       string  `json:"title"`
			URL         string  `json:"url"`
			Summary     string  `json:"summary"`
			Snippet     string  `json:"snippet"`
			Content     string  `json:"content"`
			Date        *string `json:"date"`
			RerankScore float64 `json:"rerank_score"`
			Authority   int     `json:"authority"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Web Search Results for: %s\n\n", args["query"])
	if len(response.Results) == 0 {
		out.WriteString("No results found.\n")
	}
	full, _ := args["need_content"].(bool)
	for i, item := range response.Results {
		text := item.Summary
		if text == "" {
			text = item.Snippet
		}
		if full && item.Content != "" {
			text = item.Content
		}
		fmt.Fprintf(&out, "%d. **[%s](%s)**\n   rerank_score: %g; authority: %d", i+1, item.Title, item.URL, item.RerankScore, item.Authority)
		if item.Date != nil {
			fmt.Fprintf(&out, "; date: %s", *item.Date)
		}
		fmt.Fprintf(&out, "\n\n%s\n\n", text)
	}
	return out.String(), nil
}
