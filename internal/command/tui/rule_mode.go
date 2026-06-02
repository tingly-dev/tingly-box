package tui

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RunRuleMode is the entry point for the Rule mode loop.
func RunRuleMode(mgr TUIManager) error {
	for {
		items := []SelectItem[string]{
			{Title: "List", Description: "Show all routing rules", Value: "list"},
			{Title: "Add", Description: "Create a new rule", Value: "add"},
			{Title: "Edit", Description: "Re-pick the service on an existing rule", Value: "edit"},
			{Title: "Delete", Description: "Remove a rule", Value: "delete"},
			{Title: "Back", Description: "Return to the main menu", Value: "back"},
		}
		r, err := Select("Rule:", items, SelectOptions{
			Header:    titleStyle.Render("Tingly Box · TUI · Rule"),
			CanGoBack: true,
			PageSize:  10,
		})
		if err != nil {
			return err
		}
		if r.IsCancel() || r.IsBack() || r.Value == "back" {
			return nil
		}

		var opErr error
		switch r.Value {
		case "list":
			opErr = ruleList(mgr)
		case "add":
			opErr = ruleAdd(mgr)
		case "edit":
			opErr = ruleEdit(mgr)
		case "delete":
			opErr = ruleDelete(mgr)
		}
		if opErr != nil && opErr != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", opErr)))
		}
	}
}

func ruleList(mgr TUIManager) error {
	rules := mgr.ListRules()
	if len(rules) == 0 {
		fmt.Println(descStyle.Render("No rules configured."))
		return nil
	}
	fmt.Println()
	fmt.Println(promptStyle.Render(fmt.Sprintf("%d rule(s):", len(rules))))
	for i := range rules {
		r := &rules[i]
		svc := formatRuleService(mgr, r)
		fmt.Printf("  %d. %s  %s\n",
			i+1,
			valueStyle.Render(r.RequestModel),
			descStyle.Render("("+string(r.Scenario)+")"))
		fmt.Println(descStyle.Render(fmt.Sprintf("     uuid:    %s", r.UUID)))
		fmt.Println(descStyle.Render(fmt.Sprintf("     service: %s", svc)))
	}
	fmt.Println()
	return nil
}

func formatRuleService(mgr TUIManager, r *typ.Rule) string {
	if len(r.Services) == 0 {
		return "(none)"
	}
	s := r.Services[0]
	label := s.Provider
	if p, err := mgr.GetProvider(s.Provider); err == nil && p != nil {
		label = p.Name
	}
	extra := ""
	if len(r.Services) > 1 {
		extra = fmt.Sprintf(" (+%d more)", len(r.Services)-1)
	}
	return fmt.Sprintf("%s:%s%s", label, s.Model, extra)
}

func ruleAdd(mgr TUIManager) error {
	rmR, err := Input("Request model (e.g. gpt-4o, claude-3-5-sonnet):", InputOptions{Required: true, CanGoBack: true})
	if err != nil || rmR.IsCancel() || rmR.IsBack() {
		return nil
	}

	scenarios := typ.BuiltinScenarios()
	scnItems := make([]SelectItem[typ.RuleScenario], 0, len(scenarios))
	for _, s := range scenarios {
		scnItems = append(scnItems, SelectItem[typ.RuleScenario]{Title: string(s), Value: s})
	}
	scnR, err := Select("Scenario:", scnItems, SelectOptions{CanGoBack: true, PageSize: 12, Initial: typ.ScenarioOpenAI})
	if err != nil || scnR.IsCancel() || scnR.IsBack() {
		return nil
	}

	if existing := mgr.GetGlobalConfig().GetRuleByRequestModelAndScenario(rmR.Value, scnR.Value); existing != nil {
		return fmt.Errorf("a rule for %q + %q already exists (uuid %s); use Edit instead",
			rmR.Value, scnR.Value, existing.UUID)
	}

	svc, err := pickRuleService(mgr)
	if err != nil || svc == nil {
		return err
	}

	rule := typ.Rule{
		UUID:         uuid.New().String(),
		Scenario:     scnR.Value,
		RequestModel: rmR.Value,
		Services:     []*loadbalance.Service{svc},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active: true,
	}

	cfm, err := Confirm("Save this rule?", ConfirmOptions{DefaultYes: true, CanGoBack: true,
		Description: fmt.Sprintf("%s · %s → %s:%s", rmR.Value, scnR.Value, providerName(mgr, svc.Provider), svc.Model)})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}
	if err := mgr.AddRule(rule); err != nil {
		return err
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ Rule added (uuid: %s).", rule.UUID)))
	return nil
}

func ruleEdit(mgr TUIManager) error {
	rule, err := pickRule(mgr, "Select rule to edit:")
	if err != nil || rule == nil {
		return err
	}
	fmt.Println(descStyle.Render(fmt.Sprintf("Current service: %s", formatRuleService(mgr, rule))))

	svc, err := pickRuleService(mgr)
	if err != nil || svc == nil {
		return err
	}
	updated := *rule
	updated.Services = []*loadbalance.Service{svc}

	cfm, err := Confirm("Apply update?", ConfirmOptions{DefaultYes: true, CanGoBack: true,
		Description: fmt.Sprintf("new service: %s:%s", providerName(mgr, svc.Provider), svc.Model)})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}
	if err := mgr.UpdateRule(rule.UUID, updated); err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓ Rule updated."))
	return nil
}

func ruleDelete(mgr TUIManager) error {
	rule, err := pickRule(mgr, "Select rule to delete:")
	if err != nil || rule == nil {
		return err
	}
	cfm, err := Confirm(fmt.Sprintf("Delete rule '%s' (%s)?", rule.RequestModel, rule.Scenario), ConfirmOptions{
		DefaultYes: false, CanGoBack: true,
	})
	if err != nil || !cfm.IsConfirm() || !cfm.Value {
		return nil
	}
	if err := mgr.DeleteRule(rule.UUID); err != nil {
		return err
	}
	fmt.Println(successStyle.Render("✓ Rule deleted."))
	return nil
}

func pickRule(mgr TUIManager, prompt string) (*typ.Rule, error) {
	rules := mgr.ListRules()
	if len(rules) == 0 {
		fmt.Println(descStyle.Render("No rules configured."))
		return nil, nil
	}
	items := make([]SelectItem[string], 0, len(rules))
	for i := range rules {
		r := &rules[i]
		items = append(items, SelectItem[string]{
			Title:       r.RequestModel,
			Description: fmt.Sprintf("%s · %s", r.Scenario, formatRuleService(mgr, r)),
			Value:       r.UUID,
		})
	}
	sel, err := Select(prompt, items, SelectOptions{CanGoBack: true, PageSize: 12})
	if err != nil {
		return nil, err
	}
	if sel.IsCancel() || sel.IsBack() {
		return nil, nil
	}
	return mgr.GetRuleByUUID(sel.Value), nil
}

func pickRuleService(mgr TUIManager) (*loadbalance.Service, error) {
	p, err := pickProvider(mgr, "Provider for this rule:")
	if err != nil || p == nil {
		return nil, err
	}
	model, err := pickProviderModel(mgr, p, "Model on "+p.Name+":")
	if err != nil || model == "" {
		return nil, err
	}
	return &loadbalance.Service{
		Provider: p.UUID,
		Model:    model,
		Weight:   1,
		Active:   true,
	}, nil
}

// pickProviderModel offers a Select over the provider's models. If no
// models are cached yet it first refreshes them from the provider so the
// user gets a picker rather than a free-form Input. The list still
// includes a "Custom…" escape hatch for vendors that don't return a
// listable catalog. Returns ("", nil) on cancel.
func pickProviderModel(mgr TUIManager, p *typ.Provider, prompt string) (string, error) {
	models := cachedModels(mgr, p.UUID)
	if len(models) == 0 {
		_, _ = WithSpinner("Fetching models from "+p.Name, func() (struct{}, error) {
			return struct{}{}, mgr.FetchAndSaveProviderModels(p.UUID)
		})
		models = cachedModels(mgr, p.UUID)
	}

	if len(models) == 0 {
		r, err := Input(prompt, InputOptions{
			Placeholder: "e.g. gpt-4o, claude-3-5-sonnet-20241022",
			Required:    true,
			CanGoBack:   true,
		})
		if err != nil || r.IsCancel() || r.IsBack() {
			return "", err
		}
		return r.Value, nil
	}

	items := make([]SelectItem[string], 0, len(models)+1)
	for _, m := range models {
		items = append(items, SelectItem[string]{Title: m, Value: m})
	}
	items = append(items, SelectItem[string]{Title: "Custom…", Description: "Enter a model name manually", Value: ""})

	sel, err := Select(prompt, items, SelectOptions{CanGoBack: true, PageSize: 12})
	if err != nil || sel.IsCancel() || sel.IsBack() {
		return "", err
	}
	if sel.Value != "" {
		return sel.Value, nil
	}
	r, err := Input("Model name:", InputOptions{Required: true, CanGoBack: true})
	if err != nil || r.IsCancel() || r.IsBack() {
		return "", err
	}
	return r.Value, nil
}

func cachedModels(mgr TUIManager, providerUUID string) []string {
	cfg := mgr.GetGlobalConfig()
	if cfg == nil {
		return nil
	}
	mm := cfg.GetModelManager()
	if mm == nil {
		return nil
	}
	return mm.GetModels(providerUUID)
}

func providerName(mgr TUIManager, uuid string) string {
	if p, err := mgr.GetProvider(uuid); err == nil && p != nil {
		return p.Name
	}
	return uuid
}
