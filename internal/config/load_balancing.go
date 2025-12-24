package config

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
	Stats      ServiceStats `yaml:"stats" json:"stats"`             // Service usage statistics
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
	}
}

// TacticType represents different load balancing strategies
type TacticType int

const (
	TacticRoundRobin TacticType = iota // Rotate by request count
	TacticTokenBased                   // Rotate by token consumption
	TacticHybrid                       // Hybrid: request count or tokens, whichever comes first
	TacticRandom                       // Random selection with weighted probability
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
	default:
		return TacticRoundRobin // default
	}
}
