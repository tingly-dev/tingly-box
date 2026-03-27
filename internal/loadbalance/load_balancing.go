package loadbalance

import (
	"encoding/json"
	"fmt"
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
	LatencySamples    []int64   `json:"-"`                   // Rolling window of latency samples (in ms)
	AvgLatencyMs      float64   `json:"avg_latency_ms"`      // Average latency in current window
	P50LatencyMs      float64   `json:"p50_latency_ms"`      // 50th percentile latency
	P95LatencyMs      float64   `json:"p95_latency_ms"`      // 95th percentile latency
	P99LatencyMs      float64   `json:"p99_latency_ms"`      // 99th percentile latency
	LastLatencyUpdate time.Time `json:"last_latency_update"` // When latency was last updated

	// Token speed tracking fields (tokens per second)
	SpeedSamples    []float64 `json:"-"`                 // Rolling window of token speed samples
	AvgTokenSpeed   float64   `json:"avg_token_speed"`   // Average tokens per second
	LastSpeedUpdate time.Time `json:"last_speed_update"` // When speed was last updated

	// TTFT (Time To First Token) tracking fields
	TTFTSamples    []int64   `json:"-"`                // Rolling window of TTFT samples (in ms)
	AvgTTFTMs      float64   `json:"avg_ttft_ms"`      // Average TTFT in milliseconds
	P50TTFTMs      float64   `json:"p50_ttft_ms"`      // 50th percentile TTFT
	P95TTFTMs      float64   `json:"p95_ttft_ms"`      // 95th percentile TTFT
	P99TTFTMs      float64   `json:"p99_ttft_ms"`      // 99th percentile TTFT
	LastTTFTUpdate time.Time `json:"last_ttft_update"` // When TTFT was last updated

	// Cache tracking fields
	WindowCacheHits   int64   `json:"window_cache_hits"`   // Cache hits in current window
	WindowCacheMisses int64   `json:"window_cache_misses"` // Cache misses in current window
	CacheHitRate      float64 `json:"cache_hit_rate"`      // Cache hit rate (hits / total)

	// Cost tracking fields (token-based)
	WindowCostTokens int64 `json:"window_cost_tokens"` // Total tokens as cost proxy in current window
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
		ss.WindowCacheHits = 0
		ss.WindowCacheMisses = 0
		ss.CacheHitRate = 0
		ss.WindowCostTokens = 0
	}

	totalTokens := int64(inputTokens + outputTokens)

	ss.RequestCount++
	ss.WindowRequestCount++
	ss.WindowInputTokens += int64(inputTokens)
	ss.WindowOutputTokens += int64(outputTokens)
	ss.WindowTokensConsumed += totalTokens
	ss.WindowCostTokens += totalTokens // Track cost as total tokens
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
	ss.WindowCacheHits = 0
	ss.WindowCacheMisses = 0
	ss.CacheHitRate = 0
	ss.WindowCostTokens = 0
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
		AvgTTFTMs:            ss.AvgTTFTMs,
		P50TTFTMs:            ss.P50TTFTMs,
		P95TTFTMs:            ss.P95TTFTMs,
		P99TTFTMs:            ss.P99TTFTMs,
		LastTTFTUpdate:       ss.LastTTFTUpdate,
		WindowCacheHits:      ss.WindowCacheHits,
		WindowCacheMisses:    ss.WindowCacheMisses,
		CacheHitRate:         ss.CacheHitRate,
		WindowCostTokens:     ss.WindowCostTokens,
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

// RecordTTFT records a Time To First Token sample (in milliseconds)
func (ss *ServiceStats) RecordTTFT(ttftMs int64, maxSamples int) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	// Initialize samples slice if needed
	if ss.TTFTSamples == nil {
		ss.TTFTSamples = make([]int64, 0, maxSamples)
	}

	// Add new sample
	ss.TTFTSamples = append(ss.TTFTSamples, ttftMs)

	// Remove oldest sample if we exceed max samples
	if len(ss.TTFTSamples) > maxSamples {
		ss.TTFTSamples = ss.TTFTSamples[len(ss.TTFTSamples)-maxSamples:]
	}

	// Recalculate statistics (reuse percentile logic)
	ss.recalculateTTFTStats()
	ss.LastTTFTUpdate = time.Now()
}

// recalculateTTFTStats recalculates TTFT statistics from samples
// Must be called with mutex held
func (ss *ServiceStats) recalculateTTFTStats() {
	if len(ss.TTFTSamples) == 0 {
		ss.AvgTTFTMs = 0
		ss.P50TTFTMs = 0
		ss.P95TTFTMs = 0
		ss.P99TTFTMs = 0
		return
	}

	// Calculate average
	var sum int64
	for _, v := range ss.TTFTSamples {
		sum += v
	}
	ss.AvgTTFTMs = float64(sum) / float64(len(ss.TTFTSamples))

	// Sort for percentile calculation
	sorted := make([]int64, len(ss.TTFTSamples))
	copy(sorted, ss.TTFTSamples)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentiles (reuse existing percentile function)
	ss.P50TTFTMs = percentile(sorted, 0.50)
	ss.P95TTFTMs = percentile(sorted, 0.95)
	ss.P99TTFTMs = percentile(sorted, 0.99)
}

// GetTTFTStats returns current TTFT statistics
func (ss *ServiceStats) GetTTFTStats() (avg, p50, p95, p99 float64, sampleCount int) {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.AvgTTFTMs, ss.P50TTFTMs, ss.P95TTFTMs, ss.P99TTFTMs, len(ss.TTFTSamples)
}

// RecordCacheHit records a cache hit or miss event
func (ss *ServiceStats) RecordCacheHit(isHit bool) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	if isHit {
		ss.WindowCacheHits++
	} else {
		ss.WindowCacheMisses++
	}

	// Recalculate hit rate
	total := ss.WindowCacheHits + ss.WindowCacheMisses
	if total > 0 {
		ss.CacheHitRate = float64(ss.WindowCacheHits) / float64(total)
	} else {
		ss.CacheHitRate = 0
	}
}

// GetCacheStats returns current cache statistics
func (ss *ServiceStats) GetCacheStats() (hitRate float64, hits int64, misses int64) {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.CacheHitRate, ss.WindowCacheHits, ss.WindowCacheMisses
}

// GetCostMetrics returns cost-related metrics (token-based)
func (ss *ServiceStats) GetCostMetrics() int64 {
	ss.mutex.RLock()
	defer ss.mutex.RUnlock()

	return ss.WindowCostTokens
}

// TacticType represents different load balancing strategies
type TacticType int

const (
	_                  TacticType = iota // 0: deprecated round_robin → token_based
	TacticTokenBased                     // Rotate by token consumption
	_                                    // 2: deprecated hybrid → token_based
	TacticRandom                         // Random selection with weighted probability
	TacticLatencyBased                   // Route based on response latency
	TacticSpeedBased                     // Route based on token generation speed
	TacticAdaptive                       // Composite multi-dimensional routing
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
	case TacticTokenBased:
		return "token_based"
	case TacticRandom:
		return "random"
	case TacticLatencyBased:
		return "latency_based"
	case TacticSpeedBased:
		return "speed_based"
	case TacticAdaptive:
		return "adaptive"
	default:
		return "token_based"
	}
}

// ParseTacticType parses string to TacticType
func ParseTacticType(s string) TacticType {
	switch s {
	case "round_robin": // deprecated, map to token_based
		return TacticTokenBased
	case "token_based":
		return TacticTokenBased
	case "hybrid": // deprecated, map to token_based
		return TacticTokenBased
	case "random":
		return TacticRandom
	case "latency_based":
		return TacticLatencyBased
	case "speed_based":
		return TacticSpeedBased
	case "adaptive":
		return TacticAdaptive
	default:
		return TacticAdaptive // default to adaptive
	}
}
