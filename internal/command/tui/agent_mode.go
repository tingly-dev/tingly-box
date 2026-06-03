package tui

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RunAgentMode is the entry point for the Agent mode loop. Users pick an
// agent type (from ListAgentInfo, so every supported scenario is listed)
// and then choose Apply, Show, or Restore for that agent.
func RunAgentMode(mgr TUIManager) error {
	for {
		info, err := pickAgent(mgr)
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if err := runAgentSubmenu(mgr, *info); err != nil && err != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", err)))
		}
	}
}

// pickAgent renders the list of all agents from ListAgentInfo, annotating
// each with whether a routing rule is currently wired up for it.
func pickAgent(mgr TUIManager) (*agent.AgentInfo, error) {
	infos := agent.ListAgentInfo()
	if len(infos) == 0 {
		fmt.Println(descStyle.Render("No agents registered."))
		Pause("")
		return nil, nil
	}

	items := make([]SelectItem[agent.AgentType], 0, len(infos)+1)
	for _, info := range infos {
		items = append(items, SelectItem[agent.AgentType]{
			Title:       info.Name,
			Description: agentStatusLabel(mgr, info),
			Value:       info.Type,
		})
	}
	items = append(items, SelectItem[agent.AgentType]{Title: "Back", Value: ""})

	r, err := Select("Agent:", items, SelectOptions{
		Header:    titleStyle.Render("Tingly Box · TUI · Agent"),
		CanGoBack: true,
		PageSize:  12,
	})
	if err != nil {
		return nil, err
	}
	if r.IsCancel() || r.IsBack() || r.Value == "" {
		return nil, nil
	}
	if info, ok := agent.GetAgentInfo(r.Value); ok {
		return &info, nil
	}
	return nil, nil
}

func agentStatusLabel(mgr TUIManager, info agent.AgentInfo) string {
	cfg := mgr.GetGlobalConfig()
	if cfg == nil {
		return info.Description
	}
	rule := cfg.GetRuleByRequestModelAndScenario(agentRequestModel(info.Type), typ.RuleScenario(info.Scenario))
	if rule != nil && len(rule.Services) > 0 && rule.Services[0].Provider != "" {
		if p, err := cfg.GetProviderByUUID(rule.Services[0].Provider); err == nil && p != nil {
			return fmt.Sprintf("configured · %s:%s", p.Name, rule.Services[0].Model)
		}
		return "configured"
	}
	if info.Description != "" {
		return "not configured · " + info.Description
	}
	return "not configured"
}

// agentRequestModel returns the canonical request model used to look up the
// routing rule for an agent type. Mirrors agentRoutingKey in
// internal/command/agent_command.go — keep these in sync.
func agentRequestModel(t agent.AgentType) string {
	switch t {
	case agent.AgentTypeClaudeCode:
		return "tingly/cc"
	case agent.AgentTypeOpenCode:
		return "tingly-opencode"
	case agent.AgentTypeCodex:
		return "tingly-codex"
	default:
		return string(t)
	}
}

func runAgentSubmenu(mgr TUIManager, info agent.AgentInfo) error {
	for {
		items := []SelectItem[string]{
			{Title: "Apply", Description: "Pick provider + model and write config files", Value: "apply"},
			{Title: "Show", Description: "Display the agent's current routing rule + config files", Value: "show"},
			{Title: "Restore", Description: "Roll config files back to their latest backup", Value: "restore"},
			{Title: "Back", Value: "back"},
		}
		r, err := Select(info.Name+":", items, SelectOptions{
			Header:    titleStyle.Render("Tingly Box · TUI · Agent · " + info.Name),
			CanGoBack: true,
			PageSize:  8,
		})
		if err != nil {
			return err
		}
		if r.IsCancel() || r.IsBack() || r.Value == "back" {
			return nil
		}

		var opErr error
		switch r.Value {
		case "apply":
			opErr = agentApply(mgr, info)
		case "show":
			opErr = agentShow(mgr, info)
		case "restore":
			opErr = agentRestore(mgr, info)
		}
		if opErr != nil && opErr != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", opErr)))
		}
	}
}

func agentApply(mgr TUIManager, info agent.AgentInfo) error {
	// Apply is fundamentally "point the agent CLI at tb" — it writes the
	// agent's config files (e.g. ~/.claude/settings.json) so the agent
	// talks to this box. Picking provider+model is a separate concern:
	// it wires/updates the routing rule for the agent's scenario. The
	// underlying ApplyAgent supports an empty provider/model ("config
	// files only" mode), so we offer that here too.
	req := &agent.ApplyAgentRequest{
		AgentType: info.Type,
		Unified:   true,
		Force:     true,
	}

	wireRule, err := Confirm("Also wire a routing rule (pick provider + model)?", ConfirmOptions{
		Header:      titleStyle.Render("Tingly Box · TUI · Agent · " + info.Name + " · Apply"),
		DefaultYes:  true,
		CanGoBack:   true,
		Description: "No = only write the agent's config files; keep whatever routing rule is already set up.",
	})
	if err != nil || wireRule.IsCancel() || wireRule.IsBack() {
		return nil
	}

	if wireRule.Value {
		p, err := pickProvider(mgr, "Provider for "+info.Name+":")
		if err != nil || p == nil {
			return err
		}
		model, err := pickProviderModel(mgr, p, "Model for "+info.Name+":")
		if err != nil || model == "" {
			return err
		}
		req.Provider = p.UUID
		req.Model = model
	}

	if info.Type == agent.AgentTypeClaudeCode {
		uni, err := Confirm("Use unified mode? (single config for all models)", ConfirmOptions{DefaultYes: true, CanGoBack: true})
		if err != nil || uni.IsCancel() {
			return nil
		}
		if uni.IsBack() {
			return nil
		}
		req.Unified = uni.Value

		sl, err := Confirm("Install Claude Code status line script?", ConfirmOptions{DefaultYes: false, CanGoBack: true})
		if err != nil || sl.IsCancel() {
			return nil
		}
		if sl.IsBack() {
			return nil
		}
		req.InstallStatusLine = sl.Value
	}

	res, err := WithSpinner(fmt.Sprintf("Applying %s configuration", info.Name), func() (*agent.ApplyAgentResult, error) {
		return agent.NewAgentApply(mgr.GetGlobalConfig(), "localhost").ApplyAgent(req)
	})
	if err != nil {
		return err
	}
	if res != nil {
		if res.Success {
			fmt.Println(successStyle.Render("✓ " + strings.TrimSpace(res.Message)))
		} else {
			fmt.Println(errorStyle.Render("✗ " + strings.TrimSpace(res.Message)))
		}
	}
	Pause("")
	return nil
}

func agentShow(mgr TUIManager, info agent.AgentInfo) error {
	cfg := mgr.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not available")
	}
	fmt.Println()
	fmt.Println(promptStyle.Render(info.Name) + "  " + descStyle.Render("("+string(info.Scenario)+")"))

	rule := cfg.GetRuleByRequestModelAndScenario(agentRequestModel(info.Type), typ.RuleScenario(info.Scenario))
	if rule != nil {
		fmt.Println(descStyle.Render("  routing rule:"))
		fmt.Println(descStyle.Render(fmt.Sprintf("    request-model: %s  active: %v", rule.RequestModel, rule.Active)))
		if len(rule.Services) > 0 {
			svc := rule.Services[0]
			pname := svc.Provider
			if p, err := cfg.GetProviderByUUID(svc.Provider); err == nil && p != nil {
				pname = p.Name
			}
			fmt.Println(descStyle.Render(fmt.Sprintf("    service:       %s:%s", pname, svc.Model)))
		}
	} else {
		fmt.Println(descStyle.Render("  no routing rule configured."))
	}
	fmt.Println(descStyle.Render("  config files:"))
	for _, f := range info.ConfigFiles {
		fmt.Println(descStyle.Render("    - " + f))
	}
	fmt.Println()
	Pause("")
	return nil
}

func agentRestore(mgr TUIManager, info agent.AgentInfo) error {
	fmt.Println(descStyle.Render("Files restored from their most recent backup:"))
	for _, f := range info.ConfigFiles {
		fmt.Println(descStyle.Render("  - " + f))
	}
	cfm, err := Confirm("Restore now?", ConfirmOptions{DefaultYes: false, CanGoBack: true})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}
	res, err := agent.NewAgentApply(mgr.GetGlobalConfig(), "localhost").RestoreAgent(&agent.RestoreAgentRequest{
		AgentType: info.Type,
		Force:     true,
	})
	if err != nil {
		return err
	}
	if res != nil {
		if res.Success {
			fmt.Println(successStyle.Render("✓ " + strings.TrimSpace(res.Message)))
		} else {
			fmt.Println(errorStyle.Render("✗ " + strings.TrimSpace(res.Message)))
		}
	}
	Pause("")
	return nil
}
