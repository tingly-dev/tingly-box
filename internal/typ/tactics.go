package typ

import (
	"encoding/json"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

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
	case loadbalance.TacticTokenBased:
		tc.Params = &TokenBasedParams{}
	case loadbalance.TacticRandom:
		tc.Params = &RandomParams{}
	case loadbalance.TacticLatencyBased:
		tc.Params = &LatencyBasedParams{}
	case loadbalance.TacticSpeedBased:
		tc.Params = &SpeedBasedParams{}
	case loadbalance.TacticCapacityBased:
		tc.Params = &CapacityBasedParams{}
	case loadbalance.TacticTier:
		tc.Params = &TierParams{}
	default:
		return nil
	}

	if aux.Params != nil {
		return json.Unmarshal(aux.Params, tc.Params)
	}
	return nil
}

// Instantiate converts the configuration into functional logic.
// An unset tactic (nil or Type==0) resolves to Random, matching the documented
// default in Rule.GetTacticType(); these previously disagreed (Adaptive vs
// Random), so unconfigured rules silently ran Adaptive.
func (tc *Tactic) Instantiate() LoadBalancingTactic {
	if tc == nil {
		return defaultRandomTactic
	}
	return CreateTacticWithTypedParams(tc.Type, tc.Params)
}

// ParseTacticFromMap creates a Tactic from a tactic type and parameter map.
// This is useful for parsing API request parameters into a Tactic configuration.
func ParseTacticFromMap(tacticType loadbalance.TacticType, params map[string]interface{}) Tactic {
	var tacticParams TacticParams
	switch tacticType {
	case loadbalance.TacticTokenBased:
		if params != nil {
			tacticParams = &TokenBasedParams{
				TokenThreshold: getIntParamFromMap(params, "token_threshold", constant.DefaultTokenThreshold),
			}
		} else {
			tacticParams = DefaultTokenBasedParams()
		}
	case loadbalance.TacticRandom:
		tacticParams = DefaultRandomParams()
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
	case loadbalance.TacticCapacityBased:
		tacticParams = DefaultCapacityBasedParams()
	case loadbalance.TacticTier:
		if params != nil {
			tacticParams = &TierParams{
				WithinTierTactic: loadbalance.ParseTacticType(
					getStringParamFromMap(params, "within_tier_tactic", "random"),
				),
			}
		} else {
			tacticParams = DefaultTierParams()
		}
	default:
		tacticType = loadbalance.TacticRandom
		tacticParams = DefaultRandomParams()
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

// TokenBasedParams holds parameters for token-based tactic
type TokenBasedParams struct {
	TokenThreshold int64 `json:"token_threshold"` // Threshold for token consumption before switching
}

func (t TokenBasedParams) isTacticParams() {}

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

// NewRandomParams creates parameters for the random tactic.
func NewRandomParams() TacticParams {
	return RandomParams{}
}

// DefaultParams returns default parameters for each tactic type
func DefaultTokenBasedParams() TacticParams {
	return TokenBasedParams{TokenThreshold: constant.DefaultTokenThreshold}
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

// Type assertion helpers for TacticParams. They accept both the pointer and
// value forms because UnmarshalJSON / ParseTacticFromMap store pointers while
// hand-built configs may use values.
func AsRandomParams(p TacticParams) (RandomParams, bool) {
	if rp, ok := p.(*RandomParams); ok {
		return *rp, true
	}
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

// LoadBalancingTactic defines the interface for load balancing strategies
type LoadBalancingTactic interface {
	SelectService(rule *Rule) *loadbalance.Service
	GetName() string
	GetType() loadbalance.TacticType
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

// resolveCurrentService returns the service matching rule.CurrentServiceID
// among the given services, defaulting to the first when unset or not found.
// Sticky tactics (token/latency) use it to keep serving from the current
// service until a threshold forces a switch.
func resolveCurrentService(rule *Rule, services []*loadbalance.Service) *loadbalance.Service {
	if rule.CurrentServiceID != "" {
		for _, svc := range services {
			if svc.ServiceID() == rule.CurrentServiceID {
				return svc
			}
		}
	}
	if len(services) > 0 {
		return services[0]
	}
	return nil
}

// SelectService selects service based on token consumption thresholds
func (tb *TokenBasedTactic) SelectService(rule *Rule) *loadbalance.Service {
	// Get active services once to avoid duplicate filtering
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	currentService := resolveCurrentService(rule, activeServices)
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

	currentService := resolveCurrentService(rule, activeServices)
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

	// Find service with highest speed, handling uninitialized services gracefully
	var selectedService *loadbalance.Service
	var highestSpeed float64 = -1
	hasValidService := false

	for _, service := range activeServices {
		avgSpeed, sampleCount := service.Stats.GetTokenSpeedStats()

		// For services without enough samples, assign an exploratory score to allow cold-start
		// This prevents starvation of new services that need initial traffic to collect metrics
		if sampleCount < st.MinSamplesRequired {
			// Use 50% of threshold as exploratory score - allows new services to compete
			// without completely overriding services with proven performance data
			exploratoryScore := st.SpeedThresholdTps * 0.5
			if exploratoryScore > highestSpeed {
				highestSpeed = exploratoryScore
				selectedService = service
			}
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

			// Apply same exploratory logic for consistency
			if sampleCount < st.MinSamplesRequired {
				exploratoryScore := st.SpeedThresholdTps * 0.5
				if exploratoryScore > highestSpeed {
					highestSpeed = exploratoryScore
					selectedService = service
				}
				continue
			}

			if avgSpeed > highestSpeed {
				highestSpeed = avgSpeed
				selectedService = service
			}
		}
	}

	// Final fallback (should rarely reach here due to exploratory scoring)
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

// Pre-created singleton tactic instances
var (
	defaultTokenBasedTactic   = NewTokenBasedTactic(constant.DefaultTokenThreshold)
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
)

// IsValidTactic checks if the given tactic string is valid.
// Deprecated aliases are accepted and mapped by loadbalance.ParseTacticType:
// round_robin/hybrid → token_based, priority → tier, and adaptive → random
// (the adaptive scorer is legacy; configuring it by name selects random).
func IsValidTactic(tacticStr string) bool {
	validTactics := map[string]bool{
		"round_robin":   true, // deprecated → token_based
		"token_based":   true,
		"hybrid":        true, // deprecated → token_based
		"random":        true,
		"latency_based": true,
		"speed_based":   true,
		"adaptive":      true, // legacy → random
		"tier":          true,
		"priority":      true, // deprecated → tier
	}

	// Convert to lowercase for case-insensitive comparison
	input := strings.ToLower(tacticStr)
	return validTactics[input]
}

func CreateTacticWithTypedParams(tacticType loadbalance.TacticType, params TacticParams) LoadBalancingTactic {
	switch tacticType {
	case loadbalance.TacticTokenBased:
		if tp, ok := params.(*TokenBasedParams); ok {
			return NewTokenBasedTactic(tp.TokenThreshold)
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
	case loadbalance.TacticCapacityBased:
		return GetCapacityBasedTactic()
	case loadbalance.TacticTier:
		within := loadbalance.TacticRandom
		if pp, ok := params.(*TierParams); ok && pp != nil && pp.WithinTierTactic != 0 {
			within = pp.WithinTierTactic
		}
		return NewTierTactic(within)
	}
	return GetDefaultTactic(tacticType)
}

func GetDefaultTactic(tType loadbalance.TacticType) LoadBalancingTactic {
	switch tType {
	case loadbalance.TacticTokenBased:
		return defaultTokenBasedTactic
	case loadbalance.TacticRandom:
		return defaultRandomTactic
	case loadbalance.TacticLatencyBased:
		return defaultLatencyBasedTactic
	case loadbalance.TacticSpeedBased:
		return defaultSpeedBasedTactic
	case loadbalance.TacticCapacityBased:
		return GetCapacityBasedTactic()
	case loadbalance.TacticTier:
		return defaultTierTactic
	default:
		// Unset/unknown tactic type: default to Random to match
		// Rule.GetTacticType(), which documents Random as the default.
		return defaultRandomTactic
	}
}

// CapacityBasedParams holds parameters for capacity-based load balancing
type CapacityBasedParams struct{}

// isTacticParams implements TacticParams interface
func (c CapacityBasedParams) isTacticParams() {}

// TierParams holds parameters for the tier-based failover tactic.
// WithinTierTactic decides how to share load among services that share
// the same Tier value (i.e. that are "tied" at a tier).
type TierParams struct {
	WithinTierTactic loadbalance.TacticType `json:"within_tier_tactic"`
}

func (p TierParams) isTacticParams() {}

// DefaultTierParams returns the default tier-tactic params.
// Random within a tier is a sensible default: it spreads load across
// equally-tiered services without requiring extra config.
func DefaultTierParams() TacticParams {
	return &TierParams{WithinTierTactic: loadbalance.TacticRandom}
}

// DefaultCapacityBasedParams returns default capacity-based parameters
func DefaultCapacityBasedParams() TacticParams {
	return &CapacityBasedParams{}
}

// CapacityBasedTactic implements capacity-based load balancing
// It selects services based on available capacity (weighted random)
type CapacityBasedTactic struct{}

// NewCapacityBasedTactic creates a new capacity-based tactic
func NewCapacityBasedTactic() *CapacityBasedTactic {
	return &CapacityBasedTactic{}
}

// SelectService selects a service using capacity-based weighted random.
// Capacity is determined by Service.ModelCapacity (from rule config).
// Higher capacity = higher probability of selection.
func (cbt *CapacityBasedTactic) SelectService(rule *Rule) *loadbalance.Service {
	activeServices := rule.GetActiveServices()
	if len(activeServices) == 0 {
		return nil
	}

	if len(activeServices) == 1 {
		return activeServices[0]
	}

	// Calculate weights based on ModelCapacity
	// Higher capacity = higher weight = higher probability
	var totalWeight int64
	type weightedService struct {
		service *loadbalance.Service
		weight  int64
	}
	weighted := make([]weightedService, 0, len(activeServices))

	for _, svc := range activeServices {
		// Use ModelCapacity if set, otherwise use Weight as fallback
		weight := int64(100) // default weight
		if svc.ModelCapacity != nil && *svc.ModelCapacity > 0 {
			weight = int64(*svc.ModelCapacity)
		} else if svc.Weight > 0 {
			weight = int64(svc.Weight)
		}
		weighted = append(weighted, weightedService{svc, weight})
		totalWeight += weight
	}

	if totalWeight == 0 {
		return activeServices[0]
	}

	// Weighted random selection
	r := rand.Int63n(totalWeight)
	cumulative := int64(0)
	for _, ws := range weighted {
		cumulative += ws.weight
		if r < cumulative {
			return ws.service
		}
	}

	return weighted[len(weighted)-1].service
}

// GetName returns the tactic name
func (cbt *CapacityBasedTactic) GetName() string {
	return "Capacity Based"
}

// GetType returns the tactic type
func (cbt *CapacityBasedTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticCapacityBased
}

// capacityBasedTactic is the pre-created singleton, matching the other
// default tactics above. (It was previously lazy-initialized without
// synchronization, racing on the concurrent selection path.)
var capacityBasedTactic = NewCapacityBasedTactic()

// GetCapacityBasedTactic returns the capacity-based tactic singleton
func GetCapacityBasedTactic() *CapacityBasedTactic {
	return capacityBasedTactic
}

// TierTactic implements tier-based failover load balancing.
//
// Services are bucketed by Service.Tier (ascending; lower tier number tried first;
// T0 is the highest-priority tier). The lowest-tier bucket containing at least one service
// whose circuit breaker permits a request is selected. Within that
// bucket, the WithinTierTactic (e.g. random, token-based) chooses the
// final service. This yields:
//
//   - "Direct + fallback" when each service has a distinct Tier.
//   - "Two equivalent services share a tier, with a backup tier below"
//     when several services share the same Tier.
//
// Recovery is automatic: every request reconsiders the buckets from the
// top, so once a lower-tier service's breaker closes the routing
// returns to it without any extra coordination.
type TierTactic struct {
	WithinTierTactic loadbalance.TacticType
}

// NewTierTactic creates a tier tactic with the given sub-tactic
// used to break ties within a tier.
func NewTierTactic(within loadbalance.TacticType) *TierTactic {
	if within == 0 || within == loadbalance.TacticTier {
		within = loadbalance.TacticRandom
	}
	return &TierTactic{WithinTierTactic: within}
}

// SelectService returns the highest-priority service whose breaker is
// closed (or half-open and unclaimed). It returns nil when every active
// service is currently tripped — callers should surface the original
// upstream error in that case.
//
// Selection is two-phase per tier: candidates are gathered with the
// non-consuming IsAvailable read, the sub-tactic picks one, and only the
// picked service claims a breaker slot via Allow. Claiming for every
// candidate up front would consume half-open probe slots of services that
// are never dispatched; with no outcome ever reported, those slots stayed
// taken and the service could never finish recovering.
func (pt *TierTactic) SelectService(rule *Rule) *loadbalance.Service {
	active := rule.GetActiveServices()
	if len(active) == 0 {
		return nil
	}

	// Group by Tier, deterministic ascending iteration.
	buckets := groupServicesByTier(active)

	// Pick the highest-priority bucket that has at least one breaker-
	// permitted service. If every bucket is tripped we fall back to the
	// highest-priority bucket regardless — better to surface a real
	// upstream error than to reject the request locally.
	var fallback []*loadbalance.Service
	for _, group := range buckets {
		if fallback == nil {
			fallback = group.services
		}
		chosen := PickBreakerAvailable(rule.UUID, group.services, func(candidates []*loadbalance.Service) *loadbalance.Service {
			return pt.pickWithinTier(rule, candidates)
		})
		if chosen != nil {
			return chosen
		}
	}
	if len(fallback) > 0 {
		return pt.pickWithinTier(rule, fallback)
	}
	return active[0]
}

// PickBreakerAvailable runs a two-phase, breaker-aware selection over
// candidates: gather the breaker-available subset with the non-consuming
// IsAvailable read (rule-scoped), let pick choose among that subset, then
// claim the picked service's breaker slot via Allow. A pick whose claim
// fails (its half-open probe is already in flight) is dropped and the
// remainder re-picked, so exactly one probe reaches a recovering service.
//
// It returns nil when no candidate is breaker-available or claimable —
// callers own the degrade decision (TierTactic moves to the next bucket;
// the horizontal path in LoadBalancer.SelectService falls back to an
// unfiltered pick so the client sees the real upstream error).
func PickBreakerAvailable(ruleUUID string, candidates []*loadbalance.Service, pick func([]*loadbalance.Service) *loadbalance.Service) *loadbalance.Service {
	store := loadbalance.DefaultBreakerStore()
	avail := make([]*loadbalance.Service, 0, len(candidates))
	for _, svc := range candidates {
		if store.IsAvailable(ruleUUID, svc.ServiceID()) {
			avail = append(avail, svc)
		}
	}
	for len(avail) > 0 {
		chosen := pick(avail)
		if chosen == nil {
			return nil
		}
		if store.Allow(ruleUUID, chosen.ServiceID()) {
			return chosen
		}
		avail = removeServiceByID(avail, chosen.ServiceID())
	}
	return nil
}

// removeServiceByID returns services without the entry matching serviceID.
func removeServiceByID(services []*loadbalance.Service, serviceID string) []*loadbalance.Service {
	out := make([]*loadbalance.Service, 0, len(services))
	for _, svc := range services {
		if svc.ServiceID() != serviceID {
			out = append(out, svc)
		}
	}
	return out
}

func (pt *TierTactic) pickWithinTier(rule *Rule, services []*loadbalance.Service) *loadbalance.Service {
	if len(services) == 1 {
		return services[0]
	}
	// Construct an ephemeral Rule view containing only the tier's
	// services so the sub-tactic operates on the right pool.
	sub := *rule
	sub.Services = services
	sub.CurrentServiceID = ""
	tactic := GetDefaultTactic(pt.WithinTierTactic)
	if tactic == nil {
		return services[0]
	}
	if chosen := tactic.SelectService(&sub); chosen != nil {
		return chosen
	}
	return services[0]
}

func (pt *TierTactic) GetName() string {
	return "Tier"
}

func (pt *TierTactic) GetType() loadbalance.TacticType {
	return loadbalance.TacticTier
}

// tierBucket holds services that share the same Tier value.
type tierBucket struct {
	tier     int
	services []*loadbalance.Service
}

// groupServicesByTier buckets services by their Tier field and
// returns the buckets sorted ascending — lower number is tried first (0 = T0,
// the highest-priority tier). There is no special treatment for any value.
func groupServicesByTier(services []*loadbalance.Service) []tierBucket {
	groups := make(map[int][]*loadbalance.Service)
	for _, svc := range services {
		groups[svc.Tier] = append(groups[svc.Tier], svc)
	}

	keys := make([]int, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	// Pure ascending: lower number = higher priority = tried first.
	sort.Ints(keys)

	out := make([]tierBucket, 0, len(keys))
	for _, k := range keys {
		out = append(out, tierBucket{tier: k, services: groups[k]})
	}
	return out
}

// IsAffinityEligible reports whether target is a service the routing strategy
// would actually select right now, so session affinity can decide whether a
// pin is still valid. ruleUUID scopes the breaker store: each rule has
// independent breaker state per service, so eligibility reflects only the
// traffic this rule observes (a service failing under another rule does not
// demote a pin here). It is config-shape driven rather than tactic-label
// driven — "tier" is just the emergent shape of a multi-layer rule, so this
// answers the same question for every shape:
//
//   - one service           → the only service; eligible whenever present.
//   - one tier, many services → eligible iff target's own breaker is available
//     (don't stick a session to a dead peer when healthy peers exist).
//   - many tiers             → eligible iff target is breaker-available AND its
//     tier is the highest-priority tier that currently has any available
//     service (don't stay on a fallback tier after the primary recovers).
//
// PromotionHold de-jitters batch return-to-primary: when a higher-priority tier
// has just recovered (within DefaultPromotionHold), it only takes NEW sessions.
// A session already pinned to a lower tier is kept there until the recovered
// primary has stayed healthy past the hold — so a freshly-recovered primary
// doesn't vacuum all fallback-tier sessions back at once and re-trip under
// full load. This mirrors the "low tier has an inherent, lower-priority
// stickiness" intent: the primary must prove stable to outweigh it.
//
// It mirrors TierTactic.SelectService's bucket walk but uses the non-consuming
// breaker read (IsAvailable) so it never steals the half-open probe. When
// every service is tripped it falls back to "target is in the lowest-numbered
// tier" (matching TierTactic's degrade-don't-disappear behavior) so a pin is
// honored rather than wedging.
func IsAffinityEligible(ruleUUID string, services []*loadbalance.Service, target *loadbalance.Service) bool {
	if target == nil || !target.Active {
		return false
	}
	// Mirror GetActiveServices: inactive services are not selectable, so they
	// must not influence the tier-availability computation (e.g. an inactive
	// service whose breaker happens to be closed must not make its tier look
	// "available" and wrongly demote a healthy pin in a lower tier).
	active := make([]*loadbalance.Service, 0, len(services))
	for _, svc := range services {
		if svc != nil && svc.Active {
			active = append(active, svc)
		}
	}
	buckets := groupServicesByTier(active)
	if len(buckets) == 0 {
		return false
	}

	store := loadbalance.DefaultBreakerStore()
	hold := loadbalance.DefaultPromotionHold
	for _, group := range buckets {
		available := false
		targetAvailableHere := false
		for _, svc := range group.services {
			if store.IsAvailable(ruleUUID, svc.ServiceID()) {
				available = true
				if svc.ServiceID() == target.ServiceID() {
					targetAvailableHere = true
				}
			}
		}
		if !available {
			continue
		}
		// This is the top available tier.
		if targetAvailableHere {
			// The pin is already on the best tier — always eligible.
			return true
		}
		// The pin is on a lower tier while a higher-priority tier is
		// available. Default to returning to the primary, UNLESS the primary
		// just recovered and is still within PromotionHold — then keep the
		// low-tier pin a little longer to avoid a batch flip-flop.
		if tierWithinPromotionHold(ruleUUID, store, group.services, hold) {
			return true
		}
		return false
	}
	// Every service is tripped — honor a pin to the lowest-numbered (highest
	// priority) tier, which groupServicesByTier returns first, so the request
	// still surfaces a real upstream error instead of wedging.
	return target.Tier == buckets[0].tier
}

// tierWithinPromotionHold reports whether any available service in the tier
// recovered within the hold window. A tier counts as "freshly recovered" if at
// least one of its available services is within PromotionHold; services that
// never tripped or recovered long ago do not trigger the hold.
func tierWithinPromotionHold(ruleUUID string, store *loadbalance.BreakerStore, services []*loadbalance.Service, hold time.Duration) bool {
	for _, svc := range services {
		if store.WithinPromotionHold(ruleUUID, svc.ServiceID(), hold) {
			return true
		}
	}
	return false
}

// Pre-created singleton priority tactic for the default-tactic registry.
var defaultTierTactic = NewTierTactic(loadbalance.TacticRandom)
