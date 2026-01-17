package smartrouting

import (
	"fmt"
	"strconv"
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

	return nil
}

// isValidOperationForPosition checks if an operation is valid for a given position
func isValidOperationForPosition(pos SmartOpPosition, op SmartOpOperation) bool {
	validOps := map[SmartOpPosition][]SmartOpOperation{
		PositionModel:    {OpModelContains, OpModelGlob, OpModelEquals},
		PositionThinking: {OpThinkingEnabled, OpThinkingDisabled},
		PositionSystem:   {OpSystemAnyContains, OpSystemRegex},
		PositionUser:     {OpUserAnyContains, OpUserContains, OpUserRegex, OpUserRequestType},
		PositionToolUse:  {OpToolUseIs, OpToolUseContains},
		PositionToken:    {OpTokenGe, OpTokenGt, OpTokenLe, OpTokenLt},
	}

	ops, ok := validOps[pos]
	if !ok {
		return false
	}

	for _, validOp := range ops {
		if op == validOp {
			return true
		}
	}
	return false
}

// evaluateModelOp evaluates operations on the model field
func (r *Router) evaluateModelOp(ctx *RequestContext, op *SmartOp) bool {
	model := ctx.Model

	switch op.Operation {
	case OpModelContains:
		return strings.Contains(model, op.Value)
	case OpModelGlob:
		g, err := glob.Compile(op.Value)
		if err != nil {
			return false
		}
		return g.Match(model)
	case OpModelEquals:
		return model == op.Value
	default:
		return false
	}
}

// evaluateThinkingOp evaluates operations on the thinking field
func (r *Router) evaluateThinkingOp(ctx *RequestContext, op *SmartOp) bool {
	enabled := ctx.ThinkingEnabled

	switch op.Operation {
	case OpThinkingEnabled:
		// Value can be "true", "yes", "1" or empty (just checking enabled state)
		if op.Value == "" || strings.ToLower(op.Value) == "true" || strings.ToLower(op.Value) == "yes" || op.Value == "1" {
			return enabled
		}
		return false
	case OpThinkingDisabled:
		if op.Value == "" || strings.ToLower(op.Value) == "true" || strings.ToLower(op.Value) == "yes" || op.Value == "1" {
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

	switch op.Operation {
	case OpSystemAnyContains:
		return strings.Contains(combined, op.Value)
	case OpSystemRegex:
		// Basic regex support - can be extended with regexp package
		matched, err := stringsMatch(combined, op.Value, true)
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

	switch op.Operation {
	case OpUserAnyContains:
		return strings.Contains(combined, op.Value)
	case OpUserContains:
		latest := ctx.GetLatestUserMessage()
		return strings.Contains(latest, op.Value)
	case OpUserRegex:
		matched, err := stringsMatch(combined, op.Value, true)
		if err != nil {
			return false
		}
		return matched
	case OpUserRequestType:
		return ctx.LatestContentType == op.Value
	default:
		return false
	}
}

// evaluateToolUseOp evaluates operations on the tool_use field
func (r *Router) evaluateToolUseOp(ctx *RequestContext, op *SmartOp) bool {
	// Check if any tool use matches
	for _, toolUse := range ctx.ToolUses {
		switch op.Operation {
		case OpToolUseIs:
			if toolUse == op.Value {
				return true
			}
		case OpToolUseContains:
			if strings.Contains(toolUse, op.Value) {
				return true
			}
		}
	}
	return false
}

// evaluateTokenOp evaluates operations on the token count field
func (r *Router) evaluateTokenOp(ctx *RequestContext, op *SmartOp) bool {
	tokens := ctx.EstimatedTokens
	target, err := strconv.Atoi(op.Value)
	if err != nil {
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
