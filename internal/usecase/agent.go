package usecase

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AgentUseCase implements Agent Apply/Show/Restore request assembly and
// routing-rule resolution. It holds a *serverconfig.Config directly (not an
// AppManager — see .design/usecase-layer.md, "Construction").
//
// Apply and Restore are thin wrappers over agent.AgentApply
// (internal/agent/rule_bridge.go), which already satisfies the use-case
// contract on its own (Request-in/Result-out, no I/O) — this package does
// not reimplement file-writing logic that already lives there. What this
// package owns is the piece that was duplicated per caller: routing-key
// lookup and pre-apply rule resolution.
type AgentUseCase struct {
	cfg  *serverconfig.Config
	host string
}

// NewAgentUseCase constructs an AgentUseCase over the given config. host is
// passed through to agent.NewAgentApply — see its docs (pure hostname, port
// is handled internally).
func NewAgentUseCase(cfg *serverconfig.Config, host string) *AgentUseCase {
	return &AgentUseCase{cfg: cfg, host: host}
}

// ErrUnsupportedAgentType means RoutingKey was asked about an agent type
// with no registered routing key.
type ErrUnsupportedAgentType struct {
	AgentType agent.AgentType
}

func (e ErrUnsupportedAgentType) Error() string {
	return fmt.Sprintf("unsupported agent type: %s", e.AgentType)
}

// RoutingKey returns the canonical (RequestModel, Scenario) pair that
// identifies the routing rule for an agent type. This is the single source
// of truth for that mapping — it replaces two independent, hand-copied
// tables that had already drifted:
//   - internal/command/agent_command.go's agentRoutingKey (errored on an
//     unsupported type)
//   - internal/command/tui/agent_mode.go's agentRequestModel (silently fell
//     back to string(t) instead of erroring)
//
// This version errors on an unsupported type (the CLI's stricter behavior),
// since a silent fallback produces a routing key that matches no real rule
// and fails confusingly downstream instead of failing here with a clear
// cause.
func (uc *AgentUseCase) RoutingKey(agentType agent.AgentType) (requestModel string, scenario typ.RuleScenario, err error) {
	switch agentType {
	case agent.AgentTypeClaudeCode:
		return "cc", typ.ScenarioClaudeCode, nil
	case agent.AgentTypeOpenCode:
		return "tingly-opencode", typ.ScenarioOpenCode, nil
	case agent.AgentTypeCodex:
		return "tingly-codex", typ.ScenarioCodex, nil
	case agent.AgentTypeDsh:
		return "tingly-dsh", typ.ScenarioDsh, nil
	default:
		return "", "", ErrUnsupportedAgentType{AgentType: agentType}
	}
}

// ResolveRoutingRequest identifies the agent type to resolve routing for.
type ResolveRoutingRequest struct {
	AgentType agent.AgentType `json:"agent_type"`
}

// ResolveRoutingResult reports what routing rule (if any) already exists
// for the agent type, and whether its configured provider is still valid.
// RuleFound false means no rule exists yet for this agent's routing key;
// ServiceUsable false (with RuleFound true) means the rule exists but its
// primary service has no usable provider — callers decide what to do about
// either case (prompt, warn, proceed with config-files-only), this method
// only reports the facts.
type ResolveRoutingResult struct {
	RequestModel  string           `json:"request_model"`
	Scenario      typ.RuleScenario `json:"scenario"`
	RuleFound     bool             `json:"rule_found"`
	RuleActive    bool             `json:"rule_active"`
	ResponseModel string           `json:"response_model,omitempty"`
	ServiceUsable bool             `json:"service_usable"`
	ProviderUUID  string           `json:"provider_uuid,omitempty"`
	ProviderName  string           `json:"provider_name,omitempty"`
	Model         string           `json:"model,omitempty"`
}

// ResolveRouting looks up the existing routing rule for an agent type
// (mirrors the rule-lookup half of resolveAgentConfigFromRules in
// internal/command/agent_command.go, without any prompting or warning
// output — those stay in the caller).
func (uc *AgentUseCase) ResolveRouting(req ResolveRoutingRequest) (ResolveRoutingResult, error) {
	requestModel, scenario, err := uc.RoutingKey(req.AgentType)
	if err != nil {
		return ResolveRoutingResult{}, err
	}

	res := ResolveRoutingResult{RequestModel: requestModel, Scenario: scenario}

	rule := uc.cfg.GetRuleByRequestModelAndScenario(requestModel, scenario)
	if rule == nil {
		return res, nil
	}
	res.RuleFound = true
	res.RuleActive = rule.Active
	res.ResponseModel = rule.ResponseModel
	if len(rule.Services) == 0 {
		return res, nil
	}

	service := rule.Services[0]
	res.ProviderUUID = service.Provider
	res.ProviderName = service.Provider
	res.Model = service.Model
	if service.Provider == "" || service.Model == "" {
		return res, nil
	}

	provider, err := uc.cfg.GetProviderByUUID(service.Provider)
	if err != nil || provider == nil {
		return res, nil
	}

	res.ServiceUsable = true
	res.ProviderName = provider.Name
	return res, nil
}

// ShowRequest identifies the agent type to show.
type ShowRequest struct {
	AgentType agent.AgentType `json:"agent_type"`
}

// ShowResult is the assembled data for the Show operation — replaces the
// data-gathering half of showAgentConfig in
// internal/command/agent_command.go. Rendering (fmt.Printf calls) stays in
// the caller.
type ShowResult struct {
	Info    agent.AgentInfo      `json:"info"`
	Routing ResolveRoutingResult `json:"routing"`
}

// Show assembles agent info plus its current routing rule status.
func (uc *AgentUseCase) Show(req ShowRequest) (ShowResult, error) {
	info, ok := agent.GetAgentInfo(req.AgentType)
	if !ok {
		return ShowResult{}, ErrUnsupportedAgentType{AgentType: req.AgentType}
	}

	routing, err := uc.ResolveRouting(ResolveRoutingRequest{AgentType: req.AgentType})
	if err != nil {
		return ShowResult{}, err
	}

	return ShowResult{Info: info, Routing: routing}, nil
}

// Apply configures an agent (writes its config files, optionally syncing a
// routing rule). Thin wrapper over agent.AgentApply.ApplyAgent, which
// already has no I/O of its own.
func (uc *AgentUseCase) Apply(req *agent.ApplyAgentRequest) (*agent.ApplyAgentResult, error) {
	return agent.NewAgentApply(uc.cfg, uc.host).ApplyAgent(req)
}

// Restore restores an agent's config files from their most recent backup.
// Thin wrapper over agent.AgentApply.RestoreAgent, which already has no I/O
// of its own.
func (uc *AgentUseCase) Restore(req *agent.RestoreAgentRequest) (*agent.RestoreAgentResult, error) {
	return agent.NewAgentApply(uc.cfg, uc.host).RestoreAgent(req)
}
