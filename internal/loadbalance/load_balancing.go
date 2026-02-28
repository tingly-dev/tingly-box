package loadbalance

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service represents a provider-model combination for load balancing
type Service struct {
	Provider   string       `yaml:"provider" json:"provider"`       // Provider name / uuid
	Model      string       `yaml:"model" json:"model"`             // Model name
	Weight     int          `yaml:"weight" json:"weight"`           // Weight for load balancing
	Active     bool         `yaml:"active" json:"active"`           // Whether this service is active
	TimeWindow int          `yaml:"time_window" json:"time_window"` // Statistics time window in seconds
	Stats      ServiceStats `yaml:"-" json:"-"`                     // Service usage statistics (stored in SQLite, not in config)
}

// ServiceID returns a unique identifier for the service
func (s *Service) ServiceID() string {
	return fmt.Sprintf("%s:%s", s.Provider, s.Model)
}

// PreferCompletions returns true if the model prefers the /v1/completions endpoint over /v1/chat/completions.
// This is used for models like Codex that don't support the chat completions endpoint.
func (s *Service) PreferCompletions() bool {
	// For now, all models with "codex" in their name (case insensitive) prefer completions
	// In the future, this can be extended to support more models or be configured per-model
	return strings.Contains(strings.ToLower(s.Model), "codex")
}

// InitializeStats initializes the service statistics if they are empty
func (s *Service) InitializeStats() {
	if s.Stats.ServiceID == "" {
		s.Stats = ServiceStats{
			ServiceID:   s.ServiceID(),
			TimeWindow:  s.TimeWindow,
			WindowStart: time.Now(),
		}
	}
}

// RecordUsage records usage for this service
func (s *Service) RecordUsage(inputTokens, outputTokens int) {
	s.InitializeStats()
	s.Stats.RecordUsage(inputTokens, outputTokens)
}

// GetWindowStats returns current window statistics for this service
func (s *Service) GetWindowStats() (requestCount int64, tokensConsumed int64) {
	s.InitializeStats()
	return s.Stats.GetWindowStats()
}

// ServiceStats tracks usage statistics for a service
type ServiceStats struct {
	ServiceID            string       `json:"service_id"`             // Unique service identifier
	RequestCount         int64        `json:"request_count"`          // Total request count
	LastUsed             time.Time    `json:"last_used"`              // Last usage timestamp
	WindowStart          time.Time    `json:"window_start"`           // Current time window start
	WindowRequestCount   int64        `json:"window_request_count"`   // Requests in current window
	WindowTokensConsumed int64        `json:"window_tokens_consumed"` // Tokens consumed in current window (input + output)
	WindowInputTokens    int64        `json:"window_input_tokens"`    // Input tokens in current window
	WindowOutputTokens   int64        `json:"window_output_tokens"`   // Output tokens in current window
	TimeWindow           int          `json:"time_window"`            // Copy of service's time window
	mutex                sync.RWMutex `json:"-"`                      // Thread safety

	// Latency tracking fields
	LatencySamples    []int64   `json:"-"` // Rolling window of latency samples (in ms)
	AvgLatencyMs      float64   `json:"avg_latency_ms"`      // Average latency in current window
	P50LatencyMs      float64   `json:"p50_latency_ms"`      // 50th percentile latency
	P95LatencyMs      float64   `json:"p95_latency_ms"`      // 95th percentile latency
	P99LatencyMs      float64   `json:"p99_latency_ms"`      // 99th percentile latency
	LastLatencyUpdate time.Time `json:"last_latency_update"` // When latency was last updated

	// Token speed tracking fields (tokens per second)
	SpeedSamples    []float64 `json:"-"`                 // Rolling window of token speed samples
	AvgTokenSpeed   float64   `json:"avg_token_speed"`   // Average tokens per second
	LastSpeedUpdate time.Time `json:"last_speed_update"` // When speed was last updated
}

// RecordUsage records a usage event for this service
func (ss *ServiceStats) RecordUsage(inputTokens, outputTokens int) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	now := time.Now()

	// Check if we need to reset the time window
	if now.Sub(ss.WindowStart) >= time.Duration(ss.TimeWindow)*time.Second {
		ss.WindowStart = now
		ss.WindowRequestCount = 0
		ss.WindowTokensConsumed = 0
		ss.WindowInputTokens = 0
		ss.WindowOutputTokens = 0
	}

	ss.RequestCount++
	ss.WindowRequestCount++
	ss.WindowInputTokens += int64(inputTokens)
	ss.WindowOutputTokens += int64(outputTokens)
	ss.WindowTokensConsumed += int64(inputTokens + outputTokens)
	ss.LastUsed = now
}

// GetWindowStats returns current window statistics
func (ss *ServiceStats) GetWindowStats() (requestCount int64, tokensConsumed int64) {
	// Check if window has expired without locking first
	if time.Since(ss.WindowStart) >= time.Duration(ss.TimeWindow)*time.Second {
		// Reset the window when it expires - ResetWindow handles locking internally
		ss.ResetWindow()
		return 0, 0
	}

	// Now get the read lock for normal operation
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.WindowRequestCount, ss.WindowTokensConsumed
}

// GetWindowTokenDetails returns current window input and output token details
func (ss *ServiceStats) GetWindowTokenDetails() (requestCount int64, inputTokens int64, outputTokens int64) {
	// Check if window has expired without locking first
	if time.Since(ss.WindowStart) >= time.Duration(ss.TimeWindow)*time.Second {
		// Reset the window when it expires - ResetWindow handles locking internally
		ss.ResetWindow()
		return 0, 0, 0
	}

	// Now get the read lock for normal operation
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.WindowRequestCount, ss.WindowInputTokens, ss.WindowOutputTokens
}

// IsWindowExpired checks if the current time window has expired
func (ss *ServiceStats) IsWindowExpired() bool {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return time.Since(ss.WindowStart) >= time.Duration(ss.TimeWindow)*time.Second
}

// ResetWindow resets the time window statistics
func (ss *ServiceStats) ResetWindow() {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	ss.WindowStart = time.Now()
	ss.WindowRequestCount = 0
	ss.WindowTokensConsumed = 0
	ss.WindowInputTokens = 0
	ss.WindowOutputTokens = 0
}

// GetStats returns a copy of current statistics
func (ss *ServiceStats) GetStats() ServiceStats {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ServiceStats{
		ServiceID:            ss.ServiceID,
		RequestCount:         ss.RequestCount,
		LastUsed:             ss.LastUsed,
		WindowStart:          ss.WindowStart,
		WindowRequestCount:   ss.WindowRequestCount,
		WindowTokensConsumed: ss.WindowTokensConsumed,
		WindowInputTokens:    ss.WindowInputTokens,
		WindowOutputTokens:   ss.WindowOutputTokens,
		TimeWindow:           ss.TimeWindow,
		AvgLatencyMs:         ss.AvgLatencyMs,
		P50LatencyMs:         ss.P50LatencyMs,
		P95LatencyMs:         ss.P95LatencyMs,
		P99LatencyMs:         ss.P99LatencyMs,
		LastLatencyUpdate:    ss.LastLatencyUpdate,
		AvgTokenSpeed:        ss.AvgTokenSpeed,
		LastSpeedUpdate:      ss.LastSpeedUpdate,
	}
}

// RecordLatency records a latency sample for this service
func (ss *ServiceStats) RecordLatency(latencyMs int64, maxSamples int) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	// Initialize samples slice if needed
	if ss.LatencySamples == nil {
		ss.LatencySamples = make([]int64, 0, maxSamples)
	}

	// Add new sample
	ss.LatencySamples = append(ss.LatencySamples, latencyMs)

	// Remove oldest sample if we exceed max samples
	if len(ss.LatencySamples) > maxSamples {
		ss.LatencySamples = ss.LatencySamples[len(ss.LatencySamples)-maxSamples:]
	}

	// Recalculate statistics
	ss.recalculateLatencyStats()
	ss.LastLatencyUpdate = time.Now()
}

// recalculateLatencyStats recalculates latency statistics from samples
// Must be called with mutex held
func (ss *ServiceStats) recalculateLatencyStats() {
	if len(ss.LatencySamples) == 0 {
		ss.AvgLatencyMs = 0
		ss.P50LatencyMs = 0
		ss.P95LatencyMs = 0
		ss.P99LatencyMs = 0
		return
	}

	// Calculate average
	var sum int64
	for _, v := range ss.LatencySamples {
		sum += v
	}
	ss.AvgLatencyMs = float64(sum) / float64(len(ss.LatencySamples))

	// Sort for percentile calculation
	sorted := make([]int64, len(ss.LatencySamples))
	copy(sorted, ss.LatencySamples)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentiles
	ss.P50LatencyMs = percentile(sorted, 0.50)
	ss.P95LatencyMs = percentile(sorted, 0.95)
	ss.P99LatencyMs = percentile(sorted, 0.99)
}

// percentile calculates the percentile value from a sorted slice
func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return float64(sorted[0])
	}
	if p >= 1 {
		return float64(sorted[len(sorted)-1])
	}

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1
	if upper >= len(sorted) {
		return float64(sorted[lower])
	}
	fraction := index - float64(lower)
	return float64(sorted[lower]) + fraction*float64(sorted[upper]-sorted[lower])
}

// GetLatencyStats returns current latency statistics
func (ss *ServiceStats) GetLatencyStats() (avg, p50, p95, p99 float64, sampleCount int) {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.AvgLatencyMs, ss.P50LatencyMs, ss.P95LatencyMs, ss.P99LatencyMs, len(ss.LatencySamples)
}

// RecordTokenSpeed records a token speed sample (tokens per second)
func (ss *ServiceStats) RecordTokenSpeed(speedTps float64, maxSamples int) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	// Initialize samples slice if needed
	if ss.SpeedSamples == nil {
		ss.SpeedSamples = make([]float64, 0, maxSamples)
	}

	// Add new sample
	ss.SpeedSamples = append(ss.SpeedSamples, speedTps)

	// Remove oldest sample if we exceed max samples
	if len(ss.SpeedSamples) > maxSamples {
		ss.SpeedSamples = ss.SpeedSamples[len(ss.SpeedSamples)-maxSamples:]
	}

	// Recalculate average
	ss.recalculateSpeedStats()
	ss.LastSpeedUpdate = time.Now()
}

// recalculateSpeedStats recalculates token speed statistics
// Must be called with mutex held
func (ss *ServiceStats) recalculateSpeedStats() {
	if len(ss.SpeedSamples) == 0 {
		ss.AvgTokenSpeed = 0
		return
	}

	var sum float64
	for _, v := range ss.SpeedSamples {
		sum += v
	}
	ss.AvgTokenSpeed = sum / float64(len(ss.SpeedSamples))
}

// GetTokenSpeedStats returns current token speed statistics
func (ss *ServiceStats) GetTokenSpeedStats() (avgSpeed float64, sampleCount int) {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.AvgTokenSpeed, len(ss.SpeedSamples)
}

// TacticType represents different load balancing strategies
type TacticType int

const (
	TacticRoundRobin TacticType = iota // Rotate by request count
	TacticTokenBased                   // Rotate by token consumption
	TacticHybrid                       // Hybrid: request count or tokens, whichever comes first
	TacticRandom                       // Random selection with weighted probability
	TacticLatencyBased                 // Route based on response latency
	TacticSpeedBased                   // Route based on token generation speed
	TacticAdaptive                     // Composite multi-dimensional routing
)

// MarshalJSON implements json.Marshaler for TacticType
func (tt TacticType) MarshalJSON() ([]byte, error) {
	return json.Marshal(tt.String())
}

// UnmarshalJSON implements json.Unmarshaler for TacticType
func (tt *TacticType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try unmarshaling as integer for backward compatibility
		var i int
		if err2 := json.Unmarshal(data, &i); err2 != nil {
			return err
		}
		*tt = TacticType(i)
		return nil
	}
	*tt = ParseTacticType(s)
	return nil
}

// String returns string representation of TacticType
func (tt TacticType) String() string {
	switch tt {
	case TacticRoundRobin:
		return "round_robin"
	case TacticTokenBased:
		return "token_based"
	case TacticHybrid:
		return "hybrid"
	case TacticRandom:
		return "random"
	case TacticLatencyBased:
		return "latency_based"
	case TacticSpeedBased:
		return "speed_based"
	case TacticAdaptive:
		return "adaptive"
	default:
		return "unknown"
	}
}

// ParseTacticType parses string to TacticType
func ParseTacticType(s string) TacticType {
	switch s {
	case "round_robin":
		return TacticRoundRobin
	case "token_based":
		return TacticTokenBased
	case "hybrid":
		return TacticHybrid
	case "random":
		return TacticRandom
	case "latency_based":
		return TacticLatencyBased
	case "speed_based":
		return TacticSpeedBased
	case "adaptive":
		return TacticAdaptive
	default:
		return TacticRoundRobin // default
	}
}
