package smartrouting

import (
	"fmt"
	"log"
	"strings"

	"tingly-box/internal/loadbalance"

	"github.com/gobwas/glob"
)

// Router evaluates requests against smart routing rules
type Router struct {
	rules []SmartRouting
}

// NewRouter creates a new smart routing router
func NewRouter(rules []SmartRouting) (*Router, error) {
	// Validate all rules
	for i, rule := range rules {
		if err := ValidateSmartRouting(&rule); err != nil {
			return nil, fmt.Errorf("rule[%d]: %w", i, err)
		}
	}

	return &Router{
		rules: rules,
	}, nil
}

// EvaluateRequest evaluates a request against smart routing rules
// Returns the matched services and true if a rule matched, otherwise empty and false
func (r *Router) EvaluateRequest(ctx *RequestContext) ([]loadbalance.Service, bool) {
	for _, rule := range r.rules {
		if r.evaluateRule(ctx, &rule) {
			return rule.Services, true
		}
	}
	return nil, false
}

// evaluateRule evaluates if a context matches a single rule
func (r *Router) evaluateRule(ctx *RequestContext, rule *SmartRouting) bool {
	// All operations must match (AND logic)
	for _, op := range rule.Ops {
		if !r.evaluateOp(ctx, &op) {
			return false
		}
	}
	return true
}

// evaluateOp evaluates if a context matches a single operation
func (r *Router) evaluateOp(ctx *RequestContext, op *SmartOp) bool {
	switch op.Position {
	case PositionModel:
		return r.evaluateModelOp(ctx, op)
	case PositionThinking:
		return r.evaluateThinkingOp(ctx, op)
	case PositionSystem:
		return r.evaluateSystemOp(ctx, op)
	case PositionUser:
		return r.evaluateUserOp(ctx, op)
	case PositionToolUse:
		return r.evaluateToolUseOp(ctx, op)
	case PositionToken:
		return r.evaluateTokenOp(ctx, op)
	default:
		return false
	}
}

// ValidateSmartRouting checks if the smart routing rule is valid
func ValidateSmartRouting(rule *SmartRouting) error {
	if rule.Description == "" {
		return fmt.Errorf("description cannot be empty")
	}

	if len(rule.Ops) == 0 {
		return fmt.Errorf("ops cannot be empty")
	}

	for i, op := range rule.Ops {
		if err := ValidateSmartOp(&op); err != nil {
			return fmt.Errorf("op[%d]: %w", i, err)
		}
	}

	if len(rule.Services) == 0 {
		return fmt.Errorf("services cannot be empty")
	}

	for i, svc := range rule.Services {
		if svc.Provider == "" {
			return fmt.Errorf("services[%d]: provider cannot be empty", i)
		}
		if svc.Model == "" {
			return fmt.Errorf("services[%d]: model cannot be empty", i)
		}
	}

	return nil
}

// ValidateSmartOp checks if the operation is valid for its position
func ValidateSmartOp(op *SmartOp) error {
	if !op.Position.IsValid() {
		return fmt.Errorf("invalid position: %s", op.Position)
	}

	if op.Operation == "" {
		return fmt.Errorf("operation cannot be empty")
	}

	// Validate operation is compatible with position
	if !isValidOperationForPosition(op.Position, op.Operation) {
		return fmt.Errorf("operation '%s' is not valid for position '%s'", op.Operation, op.Position)
	}

	// Validate value matches the expected type
	if err := validateOpValueType(op); err != nil {
		return err
	}

	return nil
}

// validateOpValueType checks if the value can be parsed as the expected type
func validateOpValueType(op *SmartOp) error {
	// Get the expected type from Operations registry
	expectedType := ValueTypeString // Default to string for backward compatibility
	for _, validOp := range Operations {
		if validOp.Position == op.Position && validOp.Operation == op.Operation {
			if validOp.Meta.Type != "" {
				expectedType = validOp.Meta.Type
			}
			break
		}
	}

	// Skip validation if no type specified (backward compatibility)
	if expectedType == ValueTypeString && op.Meta.Type == "" {
		return nil
	}

	// Validate the value can be parsed as expected type
	switch expectedType {
	case ValueTypeString:
		// Any string is valid
		return nil
	case ValueTypeInt:
		_, err := op.Int()
		return err
	case ValueTypeBool:
		_, err := op.Bool()
		return err
	default:
		return fmt.Errorf("unknown type: %s", expectedType)
	}
}

// isValidOperationForPosition checks if an operation is valid for a given position
// by looking it up in the global Operations registry
func isValidOperationForPosition(pos SmartOpPosition, op SmartOpOperation) bool {
	for _, validOp := range Operations {
		if validOp.Position == pos && validOp.Operation == op {
			return true
		}
	}
	return false
}

// evaluateModelOp evaluates operations on the model field
func (r *Router) evaluateModelOp(ctx *RequestContext, op *SmartOp) bool {
	model := ctx.Model
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid model value '%s': %v", op.Value, err)
		return false
	}

	switch op.Operation {
	case OpModelContains:
		return strings.Contains(model, value)
	case OpModelGlob:
		g, err := glob.Compile(value)
		if err != nil {
			log.Printf("[smart_routing] invalid glob pattern '%s' in model operation: %v", value, err)
			return false
		}
		return g.Match(model)
	case OpModelEquals:
		return model == value
	default:
		return false
	}
}

// evaluateThinkingOp evaluates operations on the thinking field
func (r *Router) evaluateThinkingOp(ctx *RequestContext, op *SmartOp) bool {
	enabled := ctx.ThinkingEnabled

	switch op.Operation {
	case OpThinkingEnabled:
		// Parse value as bool; empty string defaults to true (just checking enabled state)
		val, err := op.Bool()
		if err != nil && op.Value != "" {
			log.Printf("[smart_routing] invalid thinking value '%s': %v", op.Value, err)
			return false
		}
		// If value parsed successfully and is true, check if enabled
		// If value is empty, just check if enabled
		if op.Value == "" || val {
			return enabled
		}
		return false
	case OpThinkingDisabled:
		val, err := op.Bool()
		if err != nil && op.Value != "" {
			log.Printf("[smart_routing] invalid thinking value '%s': %v", op.Value, err)
			return false
		}
		if op.Value == "" || val {
			return !enabled
		}
		return false
	default:
		return false
	}
}

// evaluateSystemOp evaluates operations on the system message field
func (r *Router) evaluateSystemOp(ctx *RequestContext, op *SmartOp) bool {
	combined := ctx.CombineMessages(ctx.SystemMessages)
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid system value '%s': %v", op.Value, err)
		return false
	}

	switch op.Operation {
	case OpSystemAnyContains:
		return strings.Contains(combined, value)
	case OpSystemRegex:
		// Basic regex support - can be extended with regexp package
		matched, err := stringsMatch(combined, value, true)
		if err != nil {
			return false
		}
		return matched
	default:
		return false
	}
}

// evaluateUserOp evaluates operations on the user message field
func (r *Router) evaluateUserOp(ctx *RequestContext, op *SmartOp) bool {
	combined := ctx.CombineMessages(ctx.UserMessages)
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid user value '%s': %v", op.Value, err)
		return false
	}

	switch op.Operation {
	case OpUserAnyContains:
		return strings.Contains(combined, value)
	case OpUserContains:
		// Check if latest role is user
		if ctx.LatestRole != "user" {
			return false
		}
		latest := ctx.GetLatestUserMessage()
		return strings.Contains(latest, value)
	case OpUserRegex:
		matched, err := stringsMatch(combined, value, true)
		if err != nil {
			return false
		}
		return matched
	case OpUserRequestType:
		return ctx.LatestContentType == value
	default:
		return false
	}
}

// evaluateToolUseOp evaluates operations on the tool_use field
func (r *Router) evaluateToolUseOp(ctx *RequestContext, op *SmartOp) bool {
	value, err := op.String()
	if err != nil {
		log.Printf("[smart_routing] invalid tool_use value '%s': %v", op.Value, err)
		return false
	}

	// Check if any tool use matches
	for _, toolUse := range ctx.ToolUses {
		if op.Operation == OpToolUseIs && toolUse == value {
			return true
		}
	}
	return false
}

// evaluateTokenOp evaluates operations on the token count field
func (r *Router) evaluateTokenOp(ctx *RequestContext, op *SmartOp) bool {
	tokens := ctx.EstimatedTokens
	target, err := op.Int()
	if err != nil {
		log.Printf("[smart_routing] invalid token value '%s': %v", op.Value, err)
		return false
	}

	switch op.Operation {
	case OpTokenGe:
		return tokens >= target
	case OpTokenGt:
		return tokens > target
	case OpTokenLe:
		return tokens <= target
	case OpTokenLt:
		return tokens < target
	default:
		return false
	}
}

// stringsMatch provides basic regex matching support
// For now, it provides simple pattern matching with support for:
// - Wildcards (*)
// - Character classes ([abc])
// - Alternatives (a|b)
func stringsMatch(text, pattern string, useRegex bool) (bool, error) {
	if !useRegex {
		return strings.Contains(text, pattern), nil
	}

	// For simple patterns, use glob
	// For complex regex, we'd use the regexp package
	// This is a simplified implementation
	g, err := glob.Compile(pattern)
	if err != nil {
		log.Printf("[smart_routing] invalid glob/regex pattern '%s', falling back to contains: %v", pattern, err)
		// Try as simple contains
		return strings.Contains(text, pattern), nil
	}
	return g.Match(text), nil
}

// EstimateTokens estimates token count from text (rough approximation: 4 chars per token)
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Rough approximation: ~4 characters per token
	return len(text) / 4
}
