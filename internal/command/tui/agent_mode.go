package tui

import (
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/usecase"
)

// RunAgentMode is the entry point for the Agent mode loop. Users pick an
// agent type (from ListAgentInfo, so every supported scenario is listed)
// and then choose Apply, Show, or Restore for that agent.
func RunAgentMode(cfg *serverconfig.Config) error {
	for {
		info, err := pickAgent(cfg)
		if err != nil {
			return err
		}
		if info == nil {
			return nil
		}
		if err := runAgentSubmenu(cfg, *info); err != nil && err != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", err)))
		}
	}
}

// pickAgent renders the list of all agents from ListAgentInfo, annotating
// each with whether a routing rule is currently wired up for it.
func pickAgent(cfg *serverconfig.Config) (*agent.AgentInfo, error) {
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
			Description: agentStatusLabel(cfg, info),
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

func agentStatusLabel(cfg *serverconfig.Config, info agent.AgentInfo) string {
	if cfg == nil {
		return info.Description
	}
	requestModel, scenario, err := usecase.NewAgentUseCase(cfg, "localhost").RoutingKey(info.Type)
	if err != nil {
		if info.Description != "" {
			return "not configured · " + info.Description
		}
		return "not configured"
	}
	rule := cfg.GetRuleByRequestModelAndScenario(requestModel, scenario)
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

func runAgentSubmenu(cfg *serverconfig.Config, info agent.AgentInfo) error {
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
			opErr = agentApply(cfg, info)
		case "show":
			opErr = agentShow(cfg, info)
		case "restore":
			opErr = agentRestore(cfg, info)
		}
		if opErr != nil && opErr != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", opErr)))
		}
	}
}

func agentApply(cfg *serverconfig.Config, info agent.AgentInfo) error {
	// Apply is fundamentally "point the agent CLI at tb" — it writes the
	// agent's config files (e.g. ~/.claude/settings.json) so the agent
	// talks to this box. Picking provider+model is a separate concern:
	// it wires/updates the routing rule for the agent's scenario. The
	// underlying ApplyAgent supports an empty provider/model ("config
	// files only" mode), so we offer that here too.
	req := &agent.ApplyAgentRequest{
		AgentType: info.Type,
		Unified:   true,
		Yes:       true,
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
		p, err := pickProvider(cfg, "Provider for "+info.Name+":")
		if err != nil || p == nil {
			return err
		}
		model, err := pickProviderModel(cfg, p, "Model for "+info.Name+":")
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
		return usecase.NewAgentUseCase(cfg, "localhost").Apply(req)
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

func agentShow(cfg *serverconfig.Config, info agent.AgentInfo) error {
	if cfg == nil {
		return fmt.Errorf("global config not available")
	}
	result, err := usecase.NewAgentUseCase(cfg, "localhost").Show(usecase.ShowRequest{AgentType: info.Type})
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println(promptStyle.Render(result.Info.Name) + "  " + descStyle.Render("("+string(result.Info.Scenario)+")"))

	if result.Routing.RuleFound {
		fmt.Println(descStyle.Render("  routing rule:"))
		fmt.Println(descStyle.Render(fmt.Sprintf("    request-model: %s  active: %v", result.Routing.RequestModel, result.Routing.RuleActive)))
		if result.Routing.ProviderName != "" || result.Routing.Model != "" {
			fmt.Println(descStyle.Render(fmt.Sprintf("    service:       %s:%s", result.Routing.ProviderName, result.Routing.Model)))
		}
	} else {
		fmt.Println(descStyle.Render("  no routing rule configured."))
	}
	fmt.Println(descStyle.Render("  config files:"))
	for _, f := range result.Info.ConfigFiles {
		fmt.Println(descStyle.Render("    - " + f))
	}
	fmt.Println()
	Pause("")
	return nil
}

func agentRestore(cfg *serverconfig.Config, info agent.AgentInfo) error {
	fmt.Println(descStyle.Render("Files restored from their most recent backup:"))
	for _, f := range info.ConfigFiles {
		fmt.Println(descStyle.Render("  - " + f))
	}
	cfm, err := Confirm("Restore now?", ConfirmOptions{DefaultYes: false, CanGoBack: true})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}
	res, err := usecase.NewAgentUseCase(cfg, "localhost").Restore(&agent.RestoreAgentRequest{
		AgentType: info.Type,
		Yes:       true,
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
