package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// localProviderTarget describes a well-known local inference server.
type localProviderTarget struct {
	ID   string
	Name string
	URL  string // default base URL including port
}

var localProviderTargets = []localProviderTarget{
	{ID: "ollama",    Name: "Ollama",    URL: "http://localhost:11434/v1"},
	{ID: "lm-studio", Name: "LM Studio", URL: "http://localhost:1234/v1"},
	{ID: "localai",   Name: "LocalAI",   URL: "http://localhost:8080/v1"},
	{ID: "jan",       Name: "Jan",       URL: "http://localhost:1337/v1"},
	{ID: "vllm",      Name: "vLLM",      URL: "http://localhost:8000/v1"},
	{ID: "sglang",    Name: "SGLang",    URL: "http://localhost:30000/v1"},
}

// LocalModelProbeResult is the detection result for a single provider.
type LocalModelProbeResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Detected bool   `json:"detected"`
}

// LocalModelProbeResponse is returned by GET /api/v2/probe/local.
type LocalModelProbeResponse struct {
	Results []LocalModelProbeResult `json:"results"`
}

// HandleLocalModelProbe probes well-known localhost ports in parallel and
// returns which local inference servers are currently reachable. It uses a
// short timeout so the UI feels instant.
func (s *Server) HandleLocalModelProbe(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	results := make([]LocalModelProbeResult, len(localProviderTargets))
	var wg sync.WaitGroup

	for i, t := range localProviderTargets {
		wg.Add(1)
		go func(idx int, target localProviderTarget) {
			defer wg.Done()
			results[idx] = LocalModelProbeResult{
				ID:       target.ID,
				Name:     target.Name,
				URL:      target.URL,
				Detected: probeLocalURL(ctx, target.URL+"/models"),
			}
		}(i, t)
	}

	wg.Wait()
	c.JSON(http.StatusOK, LocalModelProbeResponse{Results: results})
}

// probeLocalURL does a lightweight GET to url and returns true if it gets any
// HTTP response (even an error status). A connection-refused or timeout means
// the service is not running.
func probeLocalURL(ctx context.Context, url string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
