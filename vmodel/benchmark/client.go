// Package benchmark provides a load-testing client and an in-process server
// factory for the virtualmodel HTTP service. The server side is a thin
// factory over virtualmodel/virtualserver.Service so benchmarks reuse the
// same vmodel registries as production code; the client side is a pooled
// HTTP load generator that collects throughput / latency metrics.
//
// This package replaces the former pkg/benchmark, whose mock-server half
// duplicated virtualserver and whose Model type duplicated vmodel.Model.
package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type BenchmarkClient struct {
	baseURL    string
	httpClient *http.Client
	provider   string
	apiKey     string
}

type BenchmarkOptions struct {
	BaseURL          string
	Timeout          time.Duration
	MaxConns         int
	Provider         string // "openai" or "anthropic"
	APIKey           string
	DisableKeepAlive bool
}

type BenchmarkResult struct {
	TotalRequests    int           `json:"totalRequests"`
	SuccessRequests  int           `json:"successRequests"`
	FailedRequests   int           `json:"failedRequests"`
	TotalDuration    time.Duration `json:"totalDuration"`
	AvgResponseTime  time.Duration `json:"avgResponseTime"`
	MinResponseTime  time.Duration `json:"minResponseTime"`
	MaxResponseTime  time.Duration `json:"maxResponseTime"`
	RequestsPerSec   float64       `json:"requestsPerSec"`
	TotalBytes       int64         `json:"totalBytes"`
	ErrorRate        float64       `json:"errorRate"`
	StatusCodeCounts map[int]int   `json:"statusCodeCounts"`
}

type RequestResult struct {
	Duration   time.Duration
	StatusCode int
	Error      error
	Bytes      int64
}

// OpenAIChatRequest is a minimal request body for /v1/chat/completions used
// by the load tester. It is not a full SDK type — only the fields the
// benchmark client needs to construct a request.
type OpenAIChatRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
}

// AnthropicMessageRequest is a minimal request body for /v1/messages used by
// the load tester.
type AnthropicMessageRequest struct {
	Model     string                   `json:"model"`
	MaxTokens int                      `json:"max_tokens"`
	Messages  []map[string]interface{} `json:"messages"`
	Stream    bool                     `json:"stream,omitempty"`
}

func NewBenchmarkClient(opts *BenchmarkOptions) *BenchmarkClient {
	if opts == nil {
		opts = &BenchmarkOptions{
			Timeout:  30 * time.Second,
			MaxConns: 100,
			Provider: "openai",
		}
	}

	transport := &http.Transport{
		MaxIdleConns:        opts.MaxConns,
		MaxIdleConnsPerHost: opts.MaxConns,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   opts.DisableKeepAlive,
	}

	return &BenchmarkClient{
		baseURL:  opts.BaseURL,
		provider: opts.Provider,
		apiKey:   opts.APIKey,
		httpClient: &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
		},
	}
}

func (bc *BenchmarkClient) TestModelsEndpoint(concurrency int, totalRequests int) (*BenchmarkResult, error) {
	return bc.runBenchmark("/v1/models", "GET", nil, concurrency, totalRequests)
}

func (bc *BenchmarkClient) TestChatEndpoint(model string, messages []map[string]interface{}, concurrency int, totalRequests int) (*BenchmarkResult, error) {
	if bc.provider != "openai" {
		return nil, fmt.Errorf("chat endpoint is only supported for OpenAI provider")
	}

	request := OpenAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return bc.runBenchmark("/v1/chat/completions", "POST", body, concurrency, totalRequests)
}

func (bc *BenchmarkClient) TestMessagesEndpoint(model string, messages []map[string]interface{}, maxTokens int, concurrency int, totalRequests int) (*BenchmarkResult, error) {
	if bc.provider != "anthropic" {
		return nil, fmt.Errorf("messages endpoint is only supported for Anthropic provider")
	}

	request := AnthropicMessageRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  messages,
		Stream:    false,
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return bc.runBenchmark("/v1/messages", "POST", body, concurrency, totalRequests)
}

func (bc *BenchmarkClient) runBenchmark(endpoint, method string, body []byte, concurrency int, totalRequests int) (*BenchmarkResult, error) {
	url := bc.baseURL + endpoint

	results := make(chan RequestResult, totalRequests)
	semaphore := make(chan struct{}, concurrency)

	var wg sync.WaitGroup

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result := bc.makeRequest(url, method, body)
			results <- result
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return bc.collectResults(results, totalRequests), nil
}

func (bc *BenchmarkClient) makeRequest(url, method string, body []byte) RequestResult {
	start := time.Now()

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return RequestResult{
			Duration: time.Since(start),
			Error:    err,
		}
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if bc.apiKey != "" {
		if bc.provider == "openai" {
			req.Header.Set("Authorization", "Bearer "+bc.apiKey)
		} else if bc.provider == "anthropic" {
			req.Header.Set("x-api-key", bc.apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	}

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return RequestResult{
			Duration: time.Since(start),
			Error:    err,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return RequestResult{
			Duration:   time.Since(start),
			StatusCode: resp.StatusCode,
			Error:      err,
		}
	}

	return RequestResult{
		Duration:   time.Since(start),
		StatusCode: resp.StatusCode,
		Bytes:      int64(len(respBody)),
		Error:      nil,
	}
}

func (bc *BenchmarkClient) collectResults(results <-chan RequestResult, totalRequests int) *BenchmarkResult {
	var totalDuration time.Duration
	var minDuration time.Duration = time.Hour
	var maxDuration time.Duration
	var totalBytes int64
	successCount := 0
	failureCount := 0
	statusCodeCounts := make(map[int]int)

	for result := range results {
		totalDuration += result.Duration
		totalBytes += result.Bytes

		if result.Duration < minDuration {
			minDuration = result.Duration
		}
		if result.Duration > maxDuration {
			maxDuration = result.Duration
		}

		statusCodeCounts[result.StatusCode]++

		if result.Error != nil || result.StatusCode >= 400 {
			failureCount++
		} else {
			successCount++
		}
	}

	avgDuration := totalDuration / time.Duration(totalRequests)
	rps := float64(totalRequests) / totalDuration.Seconds()
	errorRate := float64(failureCount) / float64(totalRequests) * 100

	return &BenchmarkResult{
		TotalRequests:    totalRequests,
		SuccessRequests:  successCount,
		FailedRequests:   failureCount,
		TotalDuration:    totalDuration,
		AvgResponseTime:  avgDuration,
		MinResponseTime:  minDuration,
		MaxResponseTime:  maxDuration,
		RequestsPerSec:   rps,
		TotalBytes:       totalBytes,
		ErrorRate:        errorRate,
		StatusCodeCounts: statusCodeCounts,
	}
}

func (br *BenchmarkResult) PrintSummary() {
	fmt.Printf("\n=== Benchmark Results ===\n")
	fmt.Printf("Total Requests:    %d\n", br.TotalRequests)
	fmt.Printf("Successful:        %d\n", br.SuccessRequests)
	fmt.Printf("Failed:            %d\n", br.FailedRequests)
	fmt.Printf("Error Rate:        %.2f%%\n", br.ErrorRate)
	fmt.Printf("Total Duration:    %v\n", br.TotalDuration)
	fmt.Printf("Avg Response Time: %v\n", br.AvgResponseTime)
	fmt.Printf("Min Response Time: %v\n", br.MinResponseTime)
	fmt.Printf("Max Response Time: %v\n", br.MaxResponseTime)
	fmt.Printf("Requests/sec:      %.2f\n", br.RequestsPerSec)
	fmt.Printf("Total Bytes:       %d\n", br.TotalBytes)

	fmt.Printf("\nStatus Code Distribution:\n")
	for code, count := range br.StatusCodeCounts {
		fmt.Printf("  %d: %d\n", code, count)
	}
}
