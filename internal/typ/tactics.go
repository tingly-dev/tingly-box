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

// Pre-created singleton tactic instances
var (
	defaultRoundRobinTactic = NewRoundRobinTactic()
	defaultTokenBasedTactic = NewTokenBasedTactic(constant.DefaultTokenThreshold)
	defaultHybridTactic     = NewHybridTactic(constant.DefaultRequestThreshold, constant.DefaultTokenThreshold)
	defaultRandomTactic     = NewRandomTactic()
)

// IsValidTactic checks if the given tactic string is valid
func IsValidTactic(tacticStr string) bool {
	// Map of valid tactic names
	validTactics := map[string]bool{
		"round_robin": true,
		"token_based": true,
		"hybrid":      true,
		"random":      true,
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
	default:
		return defaultRoundRobinTactic
	}
}
