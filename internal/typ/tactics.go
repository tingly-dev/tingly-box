package typ

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

// Global state for round-robin tactics (keyed by rule UUID)
// This allows multiple tactic instances to share the same state
var globalRoundRobinStreaks sync.Map

// Tactic bundles the strategy type and its parameters together
type Tactic struct {
	Type   loadbalance.TacticType `json:"type" yaml:"type"`
	Params TacticParams           `json:"params" yaml:"params"`
}

// UnmarshalJSON handles the polymorphic decoding of TacticParams
func (tc *Tactic) UnmarshalJSON(data []byte) error {
	type Alias Tactic
	aux := &struct {
		Params json.RawMessage `json:"params"`
		*Alias
	}{
		Alias: (*Alias)(tc),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Assign the concrete struct based on the type
	switch tc.Type {
	case loadbalance.TacticRoundRobin:
		tc.Params = &RoundRobinParams{}
	case loadbalance.TacticTokenBased:
		tc.Params = &TokenBasedParams{}
	case loadbalance.TacticHybrid:
		tc.Params = &HybridParams{}
	case loadbalance.TacticRandom:
		tc.Params = &RandomParams{}
	case loadbalance.TacticLatencyBased:
		tc.Params = &LatencyBasedParams{}
	case loadbalance.TacticSpeedBased:
		tc.Params = &SpeedBasedParams{}
	case loadbalance.TacticAdaptive:
		tc.Params = &AdaptiveParams{}
	default:
		return nil
	}

	if aux.Params != nil {
		return json.Unmarshal(aux.Params, tc.Params)
	}
	return nil
}

// Instantiate converts the configuration into functional logic
func (tc *Tactic) Instantiate() LoadBalancingTactic {
	if tc == nil {
		return defaultRoundRobinTactic
	}
	return CreateTacticWithTypedParams(tc.Type, tc.Params)
}

// ParseTacticFromMap creates a Tactic from a tactic type and parameter map.
// This is useful for parsing API request parameters into a Tactic configuration.
func ParseTacticFromMap(tacticType loadbalance.TacticType, params map[string]interface{}) Tactic {
	var tacticParams TacticParams
	switch tacticType {
	case loadbalance.TacticRoundRobin:
		if params != nil {
			tacticParams = &RoundRobinParams{
				RequestThreshold: getIntParamFromMap(params, "request_threshold", constant.DefaultRequestThreshold),
			}
		} else {
			tacticParams = DefaultRoundRobinParams()
		}
	case loadbalance.TacticRandom:
		tacticParams = DefaultRandomParams()
	case loadbalance.TacticTokenBased:
		if params != nil {
			tacticParams = &TokenBasedParams{
				TokenThreshold: getIntParamFromMap(params, "token_threshold", constant.DefaultTokenThreshold),
			}
		} else {
			tacticParams = DefaultTokenBasedParams()
		}
	case loadbalance.TacticHybrid:
		if params != nil {
			tacticParams = &HybridParams{
				RequestThreshold: getIntParamFromMap(params, "request_threshold", constant.DefaultRequestThreshold),
				TokenThreshold:   getIntParamFromMap(params, "token_threshold", constant.DefaultTokenThreshold),
			}
		} else {
			tacticParams = DefaultHybridParams()
		}
	case loadbalance.TacticLatencyBased:
		if params != nil {
			tacticParams = &LatencyBasedParams{
				LatencyThresholdMs: getIntParamFromMap(params, "latency_threshold_ms", constant.DefaultLatencyThresholdMs),
				SampleWindowSize:   int(getIntParamFromMap(params, "sample_window_size", int64(constant.DefaultLatencySampleWindow))),
				Percentile:         getFloatParamFromMap(params, "percentile", constant.DefaultLatencyPercentile),
				ComparisonMode:     getStringParamFromMap(params, "comparison_mode", constant.DefaultLatencyComparisonMode),
			}
		} else {
			tacticParams = DefaultLatencyBasedParams()
		}
	case loadbalance.TacticSpeedBased:
		if params != nil {
			tacticParams = &SpeedBasedParams{
				MinSamplesRequired: int(getIntParamFromMap(params, "min_samples_required", int64(constant.DefaultMinSpeedSamples))),
				SpeedThresholdTps:  getFloatParamFromMap(params, "speed_threshold_tps", constant.DefaultSpeedThresholdTps),
				SampleWindowSize:   int(getIntParamFromMap(params, "sample_window_size", int64(constant.DefaultSpeedSampleWindow))),
			}
		} else {
			tacticParams = DefaultSpeedBasedParams()
		}
	case loadbalance.TacticAdaptive:
		if params != nil {
			tacticParams = &AdaptiveParams{
				LatencyWeight: getFloatParamFromMap(params, "latency_weight", constant.DefaultLatencyWeight),
				TokenWeight:   getFloatParamFromMap(params, "token_weight", constant.DefaultTokenWeight),
				SpeedWeight:   getFloatParamFromMap(params, "speed_weight", constant.DefaultSpeedWeight),
				HealthWeight:  getFloatParamFromMap(params, "health_weight", constant.DefaultHealthWeight),
				MaxLatencyMs:  getIntParamFromMap(params, "max_latency_ms", constant.DefaultLatencyThresholdMs),
				MaxTokenUsage: getIntParamFromMap(params, "max_token_usage", constant.DefaultTokenThreshold),
				MinSpeedTps:   getFloatParamFromMap(params, "min_speed_tps", constant.DefaultSpeedThresholdTps),
				ScoringMode:   getStringParamFromMap(params, "scoring_mode", constant.DefaultScoringMode),
			}
		} else {
			tacticParams = DefaultAdaptiveParams()
		}
	default:
		tacticParams = DefaultRoundRobinParams()
	}

	return Tactic{
		Type:   tacticType,
		Params: tacticParams,
	}
}

// getIntParamFromMap safely extracts an int64 parameter from a map.
// Supports float64 (JSON numbers), int, and int64 types.
func getIntParamFromMap(params map[string]interface{}, key string, defaultValue int64) int64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return int64(v)
		case int:
			return int64(v)
		case int64:
			return v
		}
	}
	return defaultValue
}

// getFloatParamFromMap safely extracts a float64 parameter from a map.
func getFloatParamFromMap(params map[string]interface{}, key string, defaultValue float64) float64 {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return defaultValue
}

// getStringParamFromMap safely extracts a string parameter from a map.
func getStringParamFromMap(params map[string]interface{}, key string, defaultValue string) string {
	if val, ok := params[key]; ok {
		switch v := val.(type) {
		case string:
			return v
		}
	}
	return defaultValue
}

// TacticParams represents parameters for different load balancing tactics
// This is a sealed type that can only be one of the specific tactic parameter types
type TacticParams interface {
	// Unexported methods make this a sealed type
	isTacticParams()
}

// RoundRobinParams holds parameters for round-robin tactic
type RoundRobinParams struct {
	RequestThreshold int64 `json:"request_threshold"` // Number of requests per service before switching
}

func (r RoundRobinParams) isTacticParams() {}

// TokenBasedParams holds parameters for token-based tactic
type TokenBasedParams struct {
	TokenThreshold int64 `json:"token_threshold"` // Threshold for token consumption before switching
}

func (t TokenBasedParams) isTacticParams() {}

// HybridParams holds parameters for hybrid tactic
type HybridParams struct {
	RequestThreshold int64 `json:"request_threshold"` // Request threshold for hybrid tactic
	TokenThreshold   int64 `json:"token_threshold"`   // Token threshold for hybrid tactic
}

func (h HybridParams) isTacticParams() {}

// RandomParams represents parameters for random tactic (currently empty but extensible)
type RandomParams struct{}

func (r RandomParams) isTacticParams() {}

// LatencyBasedParams holds parameters for latency-based tactic
type LatencyBasedParams struct {
	LatencyThresholdMs int64   `json:"latency_threshold_ms"` // Switch if latency exceeds this (ms)
	SampleWindowSize   int     `json:"sample_window_size"`   // Number of samples to keep
	Percentile         float64 `json:"percentile"`           // Which percentile to use (0.5 = p50, 0.95 = p95, 0.99 = p99)
	ComparisonMode     string  `json:"comparison_mode"`      // "avg", "p50", "p95", "p99"
}

func (l LatencyBasedParams) isTacticParams() {}

// SpeedBasedParams holds parameters for speed-based tactic
type SpeedBasedParams struct {
	MinSamplesRequired int     `json:"min_samples_required"` // Minimum samples before making decisions
	SpeedThresholdTps  float64 `json:"speed_threshold_tps"`  // Minimum acceptable tokens per second
	SampleWindowSize   int     `json:"sample_window_size"`   // Number of speed samples to keep
}

func (s SpeedBasedParams) isTacticParams() {}

// AdaptiveParams holds parameters for adaptive multi-dimensional tactic
type AdaptiveParams struct {
	LatencyWeight    float64 `json:"latency_weight"`    // Weight for latency (0-1)
	TokenWeight      float64 `json:"token_weight"`      // Weight for token usage (0-1)
	SpeedWeight      float64 `json:"speed_weight"`      // Weight for token speed (0-1)
	HealthWeight     float64 `json:"health_weight"`     // Weight for health status (0-1)
	MaxLatencyMs     int64   `json:"max_latency_ms"`    // Maximum acceptable latency
	MaxTokenUsage    int64   `json:"max_token_usage"`   // Maximum acceptable token usage
	MinSpeedTps      float64 `json:"min_speed_tps"`     // Minimum acceptable tokens per second
	ScoringMode      string  `json:"scoring_mode"`      // "weighted_sum", "multiplicative", "rank_based"
}

func (a AdaptiveParams) isTacticParams() {}

// Helper constructors for creating tactic parameters
func NewRoundRobinParams(threshold int64) TacticParams {
	return RoundRobinParams{RequestThreshold: threshold}
}

func NewTokenBasedParams(threshold int64) TacticParams {
	return TokenBasedParams{TokenThreshold: threshold}
}

func NewHybridParams(requestThreshold, tokenThreshold int64) TacticParams {
	return HybridParams{
		RequestThreshold: requestThreshold,
		TokenThreshold:   tokenThreshold,
	}
}

func NewRandomParams() TacticParams {
	return RandomParams{}
}

func NewLatencyBasedParams(latencyThresholdMs int64, sampleWindowSize int, percentile float64, comparisonMode string) TacticParams {
	return LatencyBasedParams{
		LatencyThresholdMs: latencyThresholdMs,
		SampleWindowSize:   sampleWindowSize,
		Percentile:         percentile,
		ComparisonMode:     comparisonMode,
	}
}

func NewSpeedBasedParams(minSamplesRequired int, speedThresholdTps float64, sampleWindowSize int) TacticParams {
	return SpeedBasedParams{
		MinSamplesRequired: minSamplesRequired,
		SpeedThresholdTps:  speedThresholdTps,
		SampleWindowSize:   sampleWindowSize,
	}
}

func NewAdaptiveParams(latencyWeight, tokenWeight, speedWeight, healthWeight float64, maxLatencyMs int64, maxTokenUsage int64, minSpeedTps float64, scoringMode string) TacticParams {
	return AdaptiveParams{
		LatencyWeight: latencyWeight,
		TokenWeight:   tokenWeight,
		SpeedWeight:   speedWeight,
		HealthWeight:  healthWeight,
		MaxLatencyMs:  maxLatencyMs,
		MaxTokenUsage: maxTokenUsage,
		MinSpeedTps:   minSpeedTps,
		ScoringMode:   scoringMode,
	}
}

// DefaultParams returns default parameters for each tactic type
func DefaultRoundRobinParams() TacticParams {
	return RoundRobinParams{RequestThreshold: constant.DefaultRequestThreshold}
}

func DefaultTokenBasedParams() TacticParams {
	return TokenBasedParams{TokenThreshold: constant.DefaultTokenThreshold}
}

func DefaultHybridParams() TacticParams {
	return HybridParams{
		RequestThreshold: constant.DefaultRequestThreshold,
		TokenThreshold:   constant.DefaultTokenThreshold,
	}
}

func DefaultRandomParams() TacticParams {
	return RandomParams{}
}

func DefaultLatencyBasedParams() TacticParams {
	return LatencyBasedParams{
		LatencyThresholdMs: constant.DefaultLatencyThresholdMs,
		SampleWindowSize:   constant.DefaultLatencySampleWindow,
		Percentile:         constant.DefaultLatencyPercentile,
		ComparisonMode:     constant.DefaultLatencyComparisonMode,
	}
}

func DefaultSpeedBasedParams() TacticParams {
	return SpeedBasedParams{
		MinSamplesRequired: constant.DefaultMinSpeedSamples,
		SpeedThresholdTps:  constant.DefaultSpeedThresholdTps,
		SampleWindowSize:   constant.DefaultSpeedSampleWindow,
	}
}

func DefaultAdaptiveParams() TacticParams {
	return AdaptiveParams{
		LatencyWeight: constant.DefaultLatencyWeight,
		TokenWeight:   constant.DefaultTokenWeight,
		SpeedWeight:   constant.DefaultSpeedWeight,
		HealthWeight:  constant.DefaultHealthWeight,
		MaxLatencyMs:  constant.DefaultLatencyThresholdMs,
		MaxTokenUsage: constant.DefaultTokenThreshold,
		MinSpeedTps:   constant.DefaultSpeedThresholdTps,
		ScoringMode:   constant.DefaultScoringMode,
	}
}

// Type assertion helpers for TacticParams
func AsRoundRobinParams(p TacticParams) (RoundRobinParams, bool) {
	rp, ok := p.(RoundRobinParams)
	return rp, ok
}

func AsTokenBasedParams(p TacticParams) (TokenBasedParams, bool) {
	tp, ok := p.(TokenBasedParams)
	return tp, ok
}

func AsHybridParams(p TacticParams) (HybridParams, bool) {
	hp, ok := p.(HybridParams)
	return hp, ok
}

func AsRandomParams(p TacticParams) (RandomParams, bool) {
	rp, ok := p.(RandomParams)
	return rp, ok
}

func AsLatencyBasedParams(p TacticParams) (LatencyBasedParams, bool) {
	// Try pointer type first (used by ParseTacticFromMap and UnmarshalJSON)
	if lp, ok := p.(*LatencyBasedParams); ok {
		return *lp, true
	}
	// Try value type
	lp, ok := p.(LatencyBasedParams)
	return lp, ok
}

func AsSpeedBasedParams(p TacticParams) (SpeedBasedParams, bool) {
	// Try pointer type first
	if sp, ok := p.(*SpeedBasedParams); ok {
		return *sp, true
	}
	// Try value type
	sp, ok := p.(SpeedBasedParams)
	return sp, ok
}

func AsAdaptiveParams(p TacticParams) (AdaptiveParams, bool) {
	// Try pointer type first
	if ap, ok := p.(*AdaptiveParams); ok {
		return *ap, true
	}
	// Try value type
	ap, ok := p.(AdaptiveParams)
	return ap, ok
}

// LoadBalancingTactic defines the interface for load balancing strategies
type LoadBalancingTactic interface {
	SelectService(rule *Rule) *loadbalance.Service
	GetName() string
	GetType() loadbalance.TacticType
}

// RoundRobinTactic implements round-robin load balancing based on request count
type RoundRobinTactic struct {
	RequestThreshold int64 // Number of requests per service before switching
}

// NewRoundRobinTactic creates a new round-robin tactic with optional threshold parameter
func NewRoundRobinTactic(requestThreshold ...int64) *RoundRobinTactic {
	threshold := constant.DefaultRequestThreshold // Default 100 requests per service
	if len(requestThreshold) > 0 && requestThreshold[0] > 0 {
		threshold = requestThreshold[0]
	}
	return &RoundRobinTactic{RequestThreshold: threshold}
}

// SelectService selects the next service based on round-robin with request threshold
func (rr *RoundRobinTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services once to avoid duplicate filtering
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If only one service, return it directly
	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Use rule UUID as key for global streaks (allows state sharing across tactic instances)
	ruleKey := rule.UUID
	if ruleKey == "" {
		// Fallback to rule pointer if UUID is empty (shouldn't happen in normal operation)
		ruleKey = string(fmt.Sprintf("%p", rule))
	}

	// Get current streak for this specific rule (tracks consecutive requests to current service)
	val, _ := globalRoundRobinStreaks.LoadOrStore(ruleKey, int64(0))
	// Handle both int and int64 types for compatibility
	var currentStreak int64
	switch v := val.(type) {
	case int64:
		currentStreak = v
	case int:
		currentStreak = int64(v)
	default:
		currentStreak = 0
	}

	// Find current service by ID and get its index
	var currentIndex int = 0
	if rule.CurrentServiceID != "" {
		for i, svc := range activeServices {
			svcID := svc.ServiceID()
			if svcID == rule.CurrentServiceID {
				currentIndex = i
				break
			}
		}
	}
	currentService := activeServices[currentIndex]

	// If current service hasn't exceeded threshold, keep using it and increment streak
	if currentStreak < rr.RequestThreshold {
		globalRoundRobinStreaks.Store(ruleKey, currentStreak+1)
		return currentService
	}

	// Current service exceeded threshold, move to next service AND reset streak
	nextIndex := (currentIndex + 1) % len(activeServices)
	nextService := activeServices[nextIndex]

	// Update the rule's current service ID
	rule.CurrentServiceID = nextService.ServiceID()

	// Reset streak for the new service (set to 1 because we're using it now)
	globalRoundRobinStreaks.Store(ruleKey, int64(1))

	return nextService
}

func (rr *RoundRobinTactic) GetName() string {
	return "Round Robin"
}

func (rr *RoundRobinTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticRoundRobin
}

// TokenBasedTactic implements load balancing based on token consumption
type TokenBasedTactic struct {
	TokenThreshold int64 // Threshold for token consumption before switching
}

// NewTokenBasedTactic creates a new token-based tactic
func NewTokenBasedTactic(tokenThreshold int64) *TokenBasedTactic {
	if tokenThreshold <= 0 {
		tokenThreshold = constant.DefaultTokenThreshold // Default threshold
	}
	return &TokenBasedTactic{TokenThreshold: tokenThreshold}
}

// SelectService selects service based on token consumption thresholds
func (tb *TokenBasedTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services once to avoid duplicate filtering
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// Get current service by ID
	var currentService *loadbalance.Service
	if rule.CurrentServiceID != "" {
		for _, svc := range activeServices {
			if svc.ServiceID() == rule.CurrentServiceID {
				currentService = svc
				break
			}
		}
	}
	// Default to first service if not found
	if currentService == nil && len(activeServices) > 0 {
		currentService = activeServices[0]
	}
	if currentService == nil {
		return nil
	}

	_, windowTokens := currentService.GetWindowStats()

	// If current service hasn't exceeded threshold, keep using it
	if windowTokens < tb.TokenThreshold {
		return currentService
	}

	// Find service with lowest token usage in current window
	var selectedService *loadbalance.Service
	var lowestTokens int64 = -1

	for _, service := range activeServices {
		_, windowTokens := service.GetWindowStats()
		if lowestTokens == -1 || windowTokens < lowestTokens {
			lowestTokens = windowTokens
			selectedService = service
		}
	}

	return selectedService
}

func (tb *TokenBasedTactic) GetName() string {
	return "Token Based"
}

func (tb *TokenBasedTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticTokenBased
}

// HybridTactic implements a hybrid load balancing strategy
type HybridTactic struct {
	RequestThreshold int64 // Threshold for request count before switching
	TokenThreshold   int64 // Threshold for token consumption before switching
}

// NewHybridTactic creates a new hybrid tactic
func NewHybridTactic(requestThreshold, tokenThreshold int64) *HybridTactic {
	if requestThreshold <= 0 {
		requestThreshold = constant.DefaultRequestThreshold // Default
	}
	if tokenThreshold <= 0 {
		tokenThreshold = constant.DefaultTokenThreshold // Default
	}
	return &HybridTactic{
		RequestThreshold: requestThreshold,
		TokenThreshold:   tokenThreshold,
	}
}

// SelectService selects service based on both request count and token consumption
func (ht *HybridTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services once to avoid duplicate filtering
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// Get current service by ID
	var currentService *loadbalance.Service
	if rule.CurrentServiceID != "" {
		for _, svc := range activeServices {
			if svc.ServiceID() == rule.CurrentServiceID {
				currentService = svc
				break
			}
		}
	}
	// Default to first service if not found
	if currentService == nil && len(activeServices) > 0 {
		currentService = activeServices[0]
	}
	if currentService == nil {
		return nil
	}

	requests, tokens := currentService.GetWindowStats()

	// If current service hasn't exceeded either threshold, keep using it
	if requests < ht.RequestThreshold && tokens < ht.TokenThreshold {
		return currentService
	}

	// Score services based on combined usage (lower score is better)
	var selectedService *loadbalance.Service
	var lowestScore int64 = -1

	for _, service := range activeServices {
		requests, tokens := service.GetWindowStats()
		// Weight tokens more heavily than requests
		score := requests*10 + tokens

		if lowestScore == -1 || score < lowestScore {
			lowestScore = score
			selectedService = service
		}
	}

	return selectedService
}

func (ht *HybridTactic) GetName() string {
	return "Hybrid"
}

func (ht *HybridTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticHybrid
}

// RandomTactic implements random selection with weighted probability
type RandomTactic struct{}

// NewRandomTactic creates a new random tactic
func NewRandomTactic() *RandomTactic {
	return &RandomTactic{}
}

// SelectService selects a service randomly based on weights
func (rt *RandomTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Use the rule's method to get active services
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	var totalWeight int
	for _, service := range activeServices {
		if service.Weight > 0 {
			totalWeight += service.Weight
		}
	}

	// If no weights or all weights are 0, select randomly
	if totalWeight == 0 {
		return activeServices[rand.Intn(len(activeServices))]
	}

	// Weighted random selection
	random := rand.Intn(totalWeight)

	currentWeight := 0
	for _, service := range activeServices {
		currentWeight += service.Weight
		if random < currentWeight {
			return service
		}
	}

	// Fallback (shouldn't reach here)
	return activeServices[0]
}

func (rt *RandomTactic) GetName() string {
	return "Random"
}

func (rt *RandomTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticRandom
}

// LatencyBasedTactic implements load balancing based on response latency
type LatencyBasedTactic struct {
	LatencyThresholdMs int64   // Switch if latency exceeds this (ms)
	SampleWindowSize   int     // Number of samples to keep
	Percentile         float64 // Which percentile to use
	ComparisonMode     string  // "avg", "p50", "p95", "p99"
}

// NewLatencyBasedTactic creates a new latency-based tactic
func NewLatencyBasedTactic(latencyThresholdMs int64, sampleWindowSize int, percentile float64, comparisonMode string) *LatencyBasedTactic {
	if latencyThresholdMs <= 0 {
		latencyThresholdMs = constant.DefaultLatencyThresholdMs
	}
	if sampleWindowSize <= 0 {
		sampleWindowSize = constant.DefaultLatencySampleWindow
	}
	if percentile <= 0 || percentile >= 1 {
		percentile = constant.DefaultLatencyPercentile
	}
	if comparisonMode == "" {
		comparisonMode = constant.DefaultLatencyComparisonMode
	}
	return &LatencyBasedTactic{
		LatencyThresholdMs: latencyThresholdMs,
		SampleWindowSize:   sampleWindowSize,
		Percentile:         percentile,
		ComparisonMode:     comparisonMode,
	}
}

// getLatencyForService returns the latency value based on comparison mode
func (lt *LatencyBasedTactic) getLatencyForService(service *loadbalance.Service) float64 {
	avg, p50, p95, p99, sampleCount := service.Stats.GetLatencyStats()

	// If no samples available, return a high latency to deprioritize this service
	if sampleCount == 0 {
		return float64(lt.LatencyThresholdMs) * 2
	}

	switch lt.ComparisonMode {
	case "p50":
		return p50
	case "p95":
		return p95
	case "p99":
		return p99
	case "avg":
		fallthrough
	default:
		return avg
	}
}

// SelectService selects service based on latency
func (lt *LatencyBasedTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If only one service, return it directly
	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Get current service by ID
	var currentService *loadbalance.Service
	if rule.CurrentServiceID != "" {
		for _, svc := range activeServices {
			if svc.ServiceID() == rule.CurrentServiceID {
				currentService = svc
				break
			}
		}
	}
	// Default to first service if not found
	if currentService == nil && len(activeServices) > 0 {
		currentService = activeServices[0]
	}
	if currentService == nil {
		return nil
	}

	// Check if current service has exceeded latency threshold
	currentLatency := lt.getLatencyForService(currentService)
	if int64(currentLatency) < lt.LatencyThresholdMs {
		// Current service is within threshold, keep using it
		return currentService
	}

	// Find service with lowest latency
	var selectedService *loadbalance.Service
	var lowestLatency float64 = -1

	for _, service := range activeServices {
		latency := lt.getLatencyForService(service)
		if lowestLatency == -1 || latency < lowestLatency {
			lowestLatency = latency
			selectedService = service
		}
	}

	return selectedService
}

func (lt *LatencyBasedTactic) GetName() string {
	return "Latency Based"
}

func (lt *LatencyBasedTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticLatencyBased
}

// SpeedBasedTactic implements load balancing based on token generation speed
type SpeedBasedTactic struct {
	MinSamplesRequired int     // Minimum samples before making decisions
	SpeedThresholdTps  float64 // Minimum acceptable tokens per second
	SampleWindowSize   int     // Number of speed samples to keep
}

// NewSpeedBasedTactic creates a new speed-based tactic
func NewSpeedBasedTactic(minSamplesRequired int, speedThresholdTps float64, sampleWindowSize int) *SpeedBasedTactic {
	if minSamplesRequired <= 0 {
		minSamplesRequired = constant.DefaultMinSpeedSamples
	}
	if speedThresholdTps <= 0 {
		speedThresholdTps = constant.DefaultSpeedThresholdTps
	}
	if sampleWindowSize <= 0 {
		sampleWindowSize = constant.DefaultSpeedSampleWindow
	}
	return &SpeedBasedTactic{
		MinSamplesRequired: minSamplesRequired,
		SpeedThresholdTps:  speedThresholdTps,
		SampleWindowSize:   sampleWindowSize,
	}
}

// SelectService selects service based on token generation speed
func (st *SpeedBasedTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If only one service, return it directly
	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Filter services with sufficient samples and find the one with highest speed
	var selectedService *loadbalance.Service
	var highestSpeed float64 = -1
	hasValidService := false

	for _, service := range activeServices {
		avgSpeed, sampleCount := service.Stats.GetTokenSpeedStats()

		// Skip services with insufficient samples
		if sampleCount < st.MinSamplesRequired {
			continue
		}

		// Check if this service meets the speed threshold
		if avgSpeed >= st.SpeedThresholdTps {
			hasValidService = true
			if avgSpeed > highestSpeed {
				highestSpeed = avgSpeed
				selectedService = service
			}
		}
	}

	// If no service meets the threshold, fall back to the one with highest speed regardless of threshold
	if !hasValidService {
		for _, service := range activeServices {
			avgSpeed, sampleCount := service.Stats.GetTokenSpeedStats()
			if sampleCount < st.MinSamplesRequired {
				continue
			}
			if avgSpeed > highestSpeed {
				highestSpeed = avgSpeed
				selectedService = service
			}
		}
	}

	// If still no service has enough samples, fall back to round-robin
	if selectedService == nil {
		return activeServices[0]
	}

	return selectedService
}

func (st *SpeedBasedTactic) GetName() string {
	return "Speed Based"
}

func (st *SpeedBasedTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticSpeedBased
}

// AdaptiveTactic implements composite multi-dimensional routing
type AdaptiveTactic struct {
	LatencyWeight float64 // Weight for latency (0-1)
	TokenWeight   float64 // Weight for token usage (0-1)
	SpeedWeight   float64 // Weight for token speed (0-1)
	HealthWeight  float64 // Weight for health status (0-1)
	MaxLatencyMs  int64   // Maximum acceptable latency for normalization
	MaxTokenUsage int64   // Maximum acceptable token usage for normalization
	MinSpeedTps   float64 // Minimum acceptable tokens per second for normalization
	ScoringMode   string  // "weighted_sum", "multiplicative", "rank_based"
}

// NewAdaptiveTactic creates a new adaptive multi-dimensional tactic
func NewAdaptiveTactic(latencyWeight, tokenWeight, speedWeight, healthWeight float64, maxLatencyMs int64, maxTokenUsage int64, minSpeedTps float64, scoringMode string) *AdaptiveTactic {
	// Use defaults if not provided (0 or negative values)
	if latencyWeight <= 0 {
		latencyWeight = constant.DefaultLatencyWeight
	}
	if tokenWeight <= 0 {
		tokenWeight = constant.DefaultTokenWeight
	}
	if speedWeight <= 0 {
		speedWeight = constant.DefaultSpeedWeight
	}
	if healthWeight <= 0 {
		healthWeight = constant.DefaultHealthWeight
	}
	if maxLatencyMs <= 0 {
		maxLatencyMs = constant.DefaultLatencyThresholdMs
	}
	if maxTokenUsage <= 0 {
		maxTokenUsage = constant.DefaultTokenThreshold
	}
	if minSpeedTps <= 0 {
		minSpeedTps = constant.DefaultSpeedThresholdTps
	}
	if scoringMode == "" {
		scoringMode = constant.DefaultScoringMode
	}

	return &AdaptiveTactic{
		LatencyWeight: latencyWeight,
		TokenWeight:   tokenWeight,
		SpeedWeight:   speedWeight,
		HealthWeight:  healthWeight,
		MaxLatencyMs:  maxLatencyMs,
		MaxTokenUsage: maxTokenUsage,
		MinSpeedTps:   minSpeedTps,
		ScoringMode:   scoringMode,
	}
}

// calculateScore calculates a composite score for a service (higher is better)
func (at *AdaptiveTactic) calculateScore(service *loadbalance.Service) float64 {
	// Get metrics
	avgLatency, _, _, _, latencySampleCount := service.Stats.GetLatencyStats()
	avgSpeed, speedSampleCount := service.Stats.GetTokenSpeedStats()
	_, tokensConsumed := service.GetWindowStats()

	// Normalize metrics to 0-1 scale (higher is better)
	// For latency: lower is better, so invert
	var latencyScore float64
	if latencySampleCount > 0 {
		latencyScore = 1.0 - (avgLatency / float64(at.MaxLatencyMs))
		if latencyScore < 0 {
			latencyScore = 0
		}
	} else {
		latencyScore = 0.5 // Neutral if no data
	}

	// For tokens: lower is better, so invert
	var tokenScore float64
	if at.MaxTokenUsage > 0 {
		tokenScore = 1.0 - (float64(tokensConsumed) / float64(at.MaxTokenUsage))
		if tokenScore < 0 {
			tokenScore = 0
		}
	} else {
		tokenScore = 0.5
	}

	// For speed: higher is better
	var speedScore float64
	if speedSampleCount > 0 {
		speedScore = avgSpeed / (at.MinSpeedTps * 2) // Normalize against 2x minimum
		if speedScore > 1 {
			speedScore = 1
		}
	} else {
		speedScore = 0.5 // Neutral if no data
	}

	// Health score: always 1 (health is checked separately before calling this tactic)
	healthScore := 1.0

	// Calculate composite score based on scoring mode
	var compositeScore float64
	switch at.ScoringMode {
	case "multiplicative":
		// Multiplicative scoring (all dimensions must be good)
		compositeScore = latencyScore * at.LatencyWeight +
			tokenScore*at.TokenWeight +
			speedScore*at.SpeedWeight +
			healthScore*at.HealthWeight
	case "rank_based":
		// For rank-based, we'll handle in SelectService
		compositeScore = latencyScore*at.LatencyWeight +
			tokenScore*at.TokenWeight +
			speedScore*at.SpeedWeight +
			healthScore*at.HealthWeight
	case "weighted_sum":
		fallthrough
	default:
		// Weighted sum (default)
		compositeScore = latencyScore*at.LatencyWeight +
			tokenScore*at.TokenWeight +
			speedScore*at.SpeedWeight +
			healthScore*at.HealthWeight
	}

	return compositeScore
}

// SelectService selects service based on composite multi-dimensional scoring
func (at *AdaptiveTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	// If only one service, return it directly
	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Calculate scores for all services
	type serviceScore struct {
		service *loadbalance.Service
		score   float64
	}
	scores := make([]serviceScore, 0, len(activeServices))

	for _, service := range activeServices {
		score := at.calculateScore(service)
		scores = append(scores, serviceScore{service: service, score: score})
	}

	// Find service with highest score
	var selectedService *loadbalance.Service
	var highestScore float64 = -1

	for _, ss := range scores {
		if ss.score > highestScore {
			highestScore = ss.score
			selectedService = ss.service
		}
	}

	return selectedService
}

func (at *AdaptiveTactic) GetName() string {
	return "Adaptive"
}

func (at *AdaptiveTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticAdaptive
}

// Pre-created singleton tactic instances
var (
	defaultRoundRobinTactic   = NewRoundRobinTactic()
	defaultTokenBasedTactic   = NewTokenBasedTactic(constant.DefaultTokenThreshold)
	defaultHybridTactic       = NewHybridTactic(constant.DefaultRequestThreshold, constant.DefaultTokenThreshold)
	defaultRandomTactic       = NewRandomTactic()
	defaultLatencyBasedTactic = NewLatencyBasedTactic(
		constant.DefaultLatencyThresholdMs,
		constant.DefaultLatencySampleWindow,
		constant.DefaultLatencyPercentile,
		constant.DefaultLatencyComparisonMode,
	)
	defaultSpeedBasedTactic = NewSpeedBasedTactic(
		constant.DefaultMinSpeedSamples,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultSpeedSampleWindow,
	)
	defaultAdaptiveTactic = NewAdaptiveTactic(
		constant.DefaultLatencyWeight,
		constant.DefaultTokenWeight,
		constant.DefaultSpeedWeight,
		constant.DefaultHealthWeight,
		constant.DefaultLatencyThresholdMs,
		constant.DefaultTokenThreshold,
		constant.DefaultSpeedThresholdTps,
		constant.DefaultScoringMode,
	)
)

// IsValidTactic checks if the given tactic string is valid
func IsValidTactic(tacticStr string) bool {
	// Map of valid tactic names
	validTactics := map[string]bool{
		"round_robin":   true,
		"token_based":   true,
		"hybrid":        true,
		"random":        true,
		"latency_based": true,
		"speed_based":   true,
		"adaptive":      true,
	}

	// Convert to lowercase for case-insensitive comparison
	input := strings.ToLower(tacticStr)
	return validTactics[input]
}

func CreateTacticWithTypedParams(tacticType loadbalance.TacticType, params TacticParams) LoadBalancingTactic {
	switch tacticType {
	case loadbalance.TacticRoundRobin:
		if rp, ok := params.(*RoundRobinParams); ok {
			return NewRoundRobinTactic(rp.RequestThreshold)
		}
	case loadbalance.TacticTokenBased:
		if tp, ok := params.(*TokenBasedParams); ok {
			return NewTokenBasedTactic(tp.TokenThreshold)
		}
	case loadbalance.TacticHybrid:
		if hp, ok := params.(*HybridParams); ok {
			return NewHybridTactic(hp.RequestThreshold, hp.TokenThreshold)
		}
	case loadbalance.TacticRandom:
		return defaultRandomTactic
	case loadbalance.TacticLatencyBased:
		if lp, ok := params.(*LatencyBasedParams); ok {
			return NewLatencyBasedTactic(lp.LatencyThresholdMs, lp.SampleWindowSize, lp.Percentile, lp.ComparisonMode)
		}
		return defaultLatencyBasedTactic
	case loadbalance.TacticSpeedBased:
		if sp, ok := params.(*SpeedBasedParams); ok {
			return NewSpeedBasedTactic(sp.MinSamplesRequired, sp.SpeedThresholdTps, sp.SampleWindowSize)
		}
		return defaultSpeedBasedTactic
	case loadbalance.TacticAdaptive:
		if ap, ok := params.(*AdaptiveParams); ok {
			return NewAdaptiveTactic(ap.LatencyWeight, ap.TokenWeight, ap.SpeedWeight, ap.HealthWeight, ap.MaxLatencyMs, ap.MaxTokenUsage, ap.MinSpeedTps, ap.ScoringMode)
		}
		return defaultAdaptiveTactic
	}
	return GetDefaultTactic(tacticType)
}

func GetDefaultTactic(tType loadbalance.TacticType) LoadBalancingTactic {
	switch tType {
	case loadbalance.TacticTokenBased:
		return defaultTokenBasedTactic
	case loadbalance.TacticHybrid:
		return defaultHybridTactic
	case loadbalance.TacticRandom:
		return defaultRandomTactic
	case loadbalance.TacticLatencyBased:
		return defaultLatencyBasedTactic
	case loadbalance.TacticSpeedBased:
		return defaultSpeedBasedTactic
	case loadbalance.TacticAdaptive:
		return defaultAdaptiveTactic
	default:
		return defaultRoundRobinTactic
	}
}
