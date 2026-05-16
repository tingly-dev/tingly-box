package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// QuickstartManager is the surface the wizard needs from the host (the CLI's
// AppManager). Defined here as an interface so this package stays a leaf.
type QuickstartManager interface {
	ListProviders() []*typ.Provider
	GetProvider(name string) (*typ.Provider, error)
	AddProvider(name, apiBase, token string, apiStyle protocol.APIStyle) (string, error)
	SaveConfig() error

	GetServerPort() int
	SetupServerWithPort(port int) error
	StartServer() error

	GetGlobalConfig() *serverconfig.Config
	FetchAndSaveProviderModels(providerUUID string) error
}

// quickstartState is the wizard's accumulated state.
type quickstartState struct {
	mgr QuickstartManager

	apiStyle    protocol.APIStyle
	provider    *typ.Provider
	useExisting bool

	selectedTemplate *data.ProviderTemplate // nil = custom provider
	providerName     string
	apiBase          string
	apiToken         string
	proxyURL         string
	model            string
	startServer      bool

	// Apply-agent sub-flow
	selectedAgents      []agent.AgentType
	ccUnified           bool
	ccInstallStatusLine bool
	agentResults        []*agent.ApplyAgentResult
}

// RunQuickstart runs the interactive Tingly Box quickstart wizard.
func RunQuickstart(mgr QuickstartManager) error {
	steps := []Step[quickstartState]{
		{Name: "Welcome", Execute: qsWelcome, Skip: qsHasProviders},
		{Name: "Credential", Execute: qsCredential, Skip: qsHasNoProviders},
		{Name: "Provider", Execute: qsProvider, Skip: qsUsingExisting},
		{Name: "API Style", Execute: qsAPIStyle, Skip: qsUsingExisting},
		{Name: "Details", Execute: qsDetails, Skip: qsUsingExisting},
		{Name: "Model", Execute: qsModel},
		{Name: "Rules", Execute: qsRules},
		{Name: "Token", Execute: qsShowToken},
		{Name: "Agent", Execute: qsAgent},
		{Name: "Done", Execute: qsDone},
	}

	final, err := RunWizard("Tingly Box · Quickstart", quickstartState{mgr: mgr}, steps)
	if err != nil {
		if err == ErrCancelled {
			fmt.Println(descStyle.Render("\nSetup cancelled."))
			return nil
		}
		return err
	}

	if final.startServer {
		return startServer(mgr)
	}
	fmt.Println()
	fmt.Println(descStyle.Render("You can start the server later with: ") + valueStyle.Render("tingly-box start"))
	return nil
}

// ---------- skip predicates ----------

func qsHasNoProviders(s quickstartState) bool { return len(s.mgr.ListProviders()) == 0 }
func qsHasProviders(s quickstartState) bool   { return len(s.mgr.ListProviders()) > 0 }
func qsUsingExisting(s quickstartState) bool  { return s.useExisting }

// ---------- steps ----------

func qsWelcome(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	r, err := Confirm("Ready to set up your first AI provider?", ConfirmOptions{
		Header:      ctx.Header,
		DefaultYes:  true,
		Description: "We'll add a credential, pick a model, and wire up routing rules.",
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsCancel():
		return s, StepCancel, nil
	case !r.Value:
		return s, StepCancel, nil
	}
	return s, StepContinue, nil
}

func qsCredential(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	providers := s.mgr.ListProviders()

	items := []SelectItem[string]{
		{Title: "Add new credential", Description: "Configure a fresh AI provider", Value: "new"},
	}
	for _, p := range providers {
		items = append(items, SelectItem[string]{
			Title:       p.Name,
			Description: fmt.Sprintf("%s · %s", p.APIStyle, p.APIBase),
			Value:       p.Name,
		})
	}

	r, err := Select(fmt.Sprintf("Found %d existing credential(s) - pick one or add new:", len(providers)), items, SelectOptions{
		Header:    ctx.Header,
		CanGoBack: true,
		PageSize:  10,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}

	if r.Value == "new" {
		s.useExisting = false
		return s, StepContinue, nil
	}
	for _, p := range providers {
		if p.Name == r.Value {
			s.provider = p
			s.useExisting = true
			break
		}
	}
	return s, StepContinue, nil
}

func qsProvider(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	cfg := s.mgr.GetGlobalConfig()
	var tm *data.TemplateManager
	if cfg != nil {
		tm = cfg.GetTemplateManager()
	}
	if tm == nil {
		tm = data.NewEmbeddedOnlyTemplateManager()
	}
	_ = tm.Initialize(context.Background())

	// All non-OAuth templates that have at least one usable base URL.
	var avail []*data.ProviderTemplate
	for _, t := range tm.GetAllTemplates() {
		if !t.Valid || t.AuthType == "oauth" {
			continue
		}
		if t.BaseURLOpenAI == "" && t.BaseURLAnthropic == "" {
			continue
		}
		avail = append(avail, t)
	}
	sort.Slice(avail, func(i, j int) bool { return avail[i].Name < avail[j].Name })

	items := []SelectItem[string]{
		{Title: "Custom", Description: "Enter base URL and token manually", Value: "custom"},
	}
	for _, t := range avail {
		items = append(items, SelectItem[string]{
			Title:       t.Name,
			Description: templateStylesLabel(t),
			Value:       t.ID,
		})
	}

	r, err := Select("Pick a provider:", items, SelectOptions{
		Header:    ctx.Header,
		CanGoBack: true,
		PageSize:  10,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}

	s.providerName = r.Value
	s.selectedTemplate = nil
	s.apiBase = "" // chosen in the API Style step
	if r.Value != "custom" {
		for _, t := range avail {
			if t.ID == r.Value {
				s.selectedTemplate = t
				break
			}
		}
	}
	return s, StepContinue, nil
}

func qsAPIStyle(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	var items []SelectItem[protocol.APIStyle]
	prompt := "Pick the API style:"

	if s.selectedTemplate == nil {
		// Custom: both styles available.
		items = []SelectItem[protocol.APIStyle]{
			{Title: "OpenAI-compatible", Description: "/v1/chat/completions endpoint", Value: protocol.APIStyleOpenAI},
			{Title: "Anthropic-compatible", Description: "/v1/messages endpoint", Value: protocol.APIStyleAnthropic},
		}
	} else {
		t := s.selectedTemplate
		hasOpenAI := t.BaseURLOpenAI != ""
		hasAnthropic := t.BaseURLAnthropic != ""
		soleHint := ""
		if hasOpenAI != hasAnthropic { // exactly one supported
			soleHint = "only style supported by " + t.Name
		}
		prompt = fmt.Sprintf("Pick the API style for %s:", t.Name)

		if hasOpenAI {
			desc := t.BaseURLOpenAI
			if soleHint != "" {
				desc = soleHint + " · " + desc
			}
			items = append(items, SelectItem[protocol.APIStyle]{
				Title: "OpenAI-compatible", Description: desc, Value: protocol.APIStyleOpenAI,
			})
		}
		if hasAnthropic {
			desc := t.BaseURLAnthropic
			if soleHint != "" {
				desc = soleHint + " · " + desc
			}
			items = append(items, SelectItem[protocol.APIStyle]{
				Title: "Anthropic-compatible", Description: desc, Value: protocol.APIStyleAnthropic,
			})
		}
	}

	r, err := Select(prompt, items, SelectOptions{
		Header:    ctx.Header,
		Initial:   s.apiStyle,
		CanGoBack: true,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}
	s.apiStyle = r.Value

	// Pre-fill the base URL from the template, now that the style is known.
	if s.selectedTemplate != nil {
		switch s.apiStyle {
		case protocol.APIStyleOpenAI:
			s.apiBase = s.selectedTemplate.BaseURLOpenAI
		case protocol.APIStyleAnthropic:
			s.apiBase = s.selectedTemplate.BaseURLAnthropic
		}
	}
	return s, StepContinue, nil
}

// templateStylesLabel returns a short description of which API styles a
// template supports, e.g. "openai · anthropic" or just "openai".
func templateStylesLabel(t *data.ProviderTemplate) string {
	var styles []string
	if t.BaseURLOpenAI != "" {
		styles = append(styles, "openai")
	}
	if t.BaseURLAnthropic != "" {
		styles = append(styles, "anthropic")
	}
	return strings.Join(styles, " · ")
}

func qsDetails(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	if s.providerName == "" || s.providerName == "custom" {
		r, err := Input("Provider name:", InputOptions{
			Header:      ctx.Header,
			Placeholder: "e.g. openai, anthropic",
			Required:    true,
			Initial:     "",
			CanGoBack:   true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if r.IsBack() {
			return s, StepBack, nil
		}
		if r.IsCancel() {
			return s, StepCancel, nil
		}
		s.providerName = r.Value
	}

	if existing, err := s.mgr.GetProvider(s.providerName); err == nil && existing != nil {
		r, err := Confirm(fmt.Sprintf("Provider '%s' already exists - reuse it?", s.providerName), ConfirmOptions{
			Header:     ctx.Header,
			DefaultYes: true,
			CanGoBack:  true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if r.IsBack() {
			return s, StepBack, nil
		}
		if r.IsConfirm() && r.Value {
			s.provider = existing
			return s, StepContinue, nil
		}
		nameR, err := Input("Pick a new name:", InputOptions{
			Header: ctx.Header, Required: true, CanGoBack: true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if nameR.IsBack() {
			return s, StepBack, nil
		}
		s.providerName = nameR.Value
	}

	defaultURL := s.apiBase
	if defaultURL == "" {
		if s.apiStyle == protocol.APIStyleAnthropic {
			defaultURL = "https://api.anthropic.com"
		} else {
			defaultURL = "https://api.example.com/v1"
		}
	}
	baseR, err := Input("Base URL:", InputOptions{
		Header:      ctx.Header,
		Placeholder: defaultURL,
		Initial:     s.apiBase,
		CanGoBack:   true,
	})
	if err != nil {
		return s, StepCancel, err
	}
	if baseR.IsBack() {
		return s, StepBack, nil
	}
	if baseR.Value != "" {
		s.apiBase = baseR.Value
	} else {
		s.apiBase = defaultURL
	}

	tokenR, err := Input("API key:", InputOptions{
		Header: ctx.Header, Required: true, Mask: true, CanGoBack: true,
	})
	if err != nil {
		return s, StepCancel, err
	}
	if tokenR.IsBack() {
		return s, StepBack, nil
	}
	s.apiToken = tokenR.Value

	proxyR, err := Input("Proxy URL (optional):", InputOptions{
		Header: ctx.Header, Placeholder: "e.g. http://localhost:7890", CanGoBack: true,
	})
	if err != nil {
		return s, StepCancel, err
	}
	if proxyR.IsBack() {
		return s, StepBack, nil
	}
	s.proxyURL = proxyR.Value

	uuid, err := s.mgr.AddProvider(s.providerName, s.apiBase, s.apiToken, s.apiStyle)
	if err != nil {
		return s, StepCancel, fmt.Errorf("failed to add provider: %w", err)
	}
	provider, err := s.mgr.GetProvider(uuid)
	if err != nil {
		return s, StepCancel, err
	}
	if s.proxyURL != "" {
		provider.ProxyURL = s.proxyURL
		if err := s.mgr.SaveConfig(); err != nil {
			return s, StepCancel, fmt.Errorf("failed to save proxy: %w", err)
		}
	}
	s.provider = provider
	return s, StepContinue, nil
}

func qsModel(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	models, _ := WithSpinner("Fetching models from provider", func() ([]string, error) {
		if err := s.mgr.FetchAndSaveProviderModels(s.provider.UUID); err != nil {
			return nil, err
		}
		cfg := s.mgr.GetGlobalConfig()
		if cfg == nil {
			return nil, nil
		}
		mm := cfg.GetModelManager()
		if mm == nil {
			return nil, nil
		}
		return mm.GetModels(s.provider.UUID), nil
	})

	if len(models) == 0 {
		r, err := Input("Couldn't fetch models - enter a model name:", InputOptions{
			Header:      ctx.Header,
			Placeholder: "e.g. gpt-4o, claude-sonnet-4-20250514",
			Required:    true,
			Initial:     s.model,
			CanGoBack:   true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if r.IsBack() {
			return s, StepBack, nil
		}
		if r.IsCancel() {
			return s, StepCancel, nil
		}
		s.model = r.Value
		return s, StepContinue, nil
	}

	items := []SelectItem[string]{
		{Title: "Custom…", Description: "Enter a model name manually", Value: ""},
	}
	for _, m := range models {
		items = append(items, SelectItem[string]{Title: m, Value: m})
	}
	r, err := Select(fmt.Sprintf("Pick a default model (%d available):", len(models)), items, SelectOptions{
		Header:    ctx.Header,
		Initial:   s.model,
		CanGoBack: true,
		PageSize:  12,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}

	if r.Value == "" {
		cr, err := Input("Model name:", InputOptions{
			Header: ctx.Header, Required: true, CanGoBack: true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if cr.IsBack() {
			return s, StepBack, nil
		}
		s.model = cr.Value
	} else {
		s.model = r.Value
	}
	return s, StepContinue, nil
}

func qsRules(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	cfg := s.mgr.GetGlobalConfig()
	if cfg == nil {
		return s, StepCancel, fmt.Errorf("global config not available")
	}

	defs := []struct {
		uuid string
		desc string
	}{
		{serverconfig.RuleUUIDBuiltinOpenAI, "OpenAI scenario"},
		{serverconfig.RuleUUIDBuiltinAnthropic, "Anthropic scenario"},
		{serverconfig.RuleUUIDBuiltinCC, "Claude Code · unified"},
		{serverconfig.RuleUUIDBuiltinCCDefault, "Claude Code · default"},
		{serverconfig.RuleUUIDBuiltinCCHaiku, "Claude Code · haiku"},
		{serverconfig.RuleUUIDBuiltinCCOpus, "Claude Code · opus"},
		{serverconfig.RuleUUIDBuiltinCCSonnet, "Claude Code · sonnet"},
		{serverconfig.RuleUUIDBuiltinCCSubagent, "Claude Code · subagent"},
		{"built-in-opencode", "OpenCode scenario"},
	}

	type rv struct{ uuid, desc string }
	var items []MultiSelectItem[rv]
	for _, d := range defs {
		rule := cfg.GetRuleByUUID(d.uuid)
		if rule == nil {
			continue
		}
		already := isRuleConfigured(rule, cfg)
		desc := ""
		if already {
			desc = "already configured"
		}
		items = append(items, MultiSelectItem[rv]{
			Title:       d.desc,
			Description: desc,
			Value:       rv{uuid: d.uuid, desc: d.desc},
			Selected:    !already,
		})
	}
	if len(items) == 0 {
		fmt.Println(descStyle.Render("No rules available to configure."))
		return s, StepContinue, nil
	}

	r, err := MultiSelect("Select rules to wire up:", items, MultiSelectOptions{
		Header: ctx.Header, CanGoBack: true, PageSize: 10,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}

	if len(r.Value) == 0 {
		return s, StepContinue, nil
	}

	svc := &loadbalance.Service{
		Provider: s.provider.UUID, Model: s.model, Weight: 1, Active: true,
	}
	configured := 0
	for _, sel := range r.Value {
		rule := cfg.GetRuleByUUID(sel.uuid)
		if rule == nil {
			continue
		}
		rule.Services = []*loadbalance.Service{svc}
		rule.Active = true
		if err := cfg.UpdateRule(sel.uuid, *rule); err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  ✗ %s: %v", sel.desc, err)))
			continue
		}
		fmt.Println(successStyle.Render("  ✓ ") + sel.desc)
		configured++
	}
	fmt.Println(descStyle.Render(fmt.Sprintf("\n%d rules configured.\n", configured)))
	return s, StepContinue, nil
}

// qsShowToken offers an optional preview of the tingly-box model token —
// the credential AI clients send to this box's OpenAI/Anthropic-compatible
// endpoints. Skipped by default; answering "no" advances without printing
// the secret. This is independent of upstream provider tokens.
func qsShowToken(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	cfg := s.mgr.GetGlobalConfig()
	if cfg == nil {
		return s, StepContinue, nil
	}
	modelToken := cfg.ModelToken
	if modelToken == "" {
		// Token is generated lazily at server start; nothing to show yet.
		return s, StepContinue, nil
	}

	r, err := Confirm("Reveal tingly box model token now?", ConfirmOptions{
		Header:      ctx.Header,
		DefaultYes:  false,
		CanGoBack:   true,
		Description: "Prints this box's model token (clients use it to call the box) — only reveal on a trusted screen.",
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}
	if !r.Value {
		return s, StepContinue, nil
	}

	port := s.mgr.GetServerPort()
	if port == 0 {
		port = 12580
	}

	fmt.Println()
	fmt.Println(promptStyle.Render("Tingly Box model token (copy into your AI client):"))
	fmt.Println(descStyle.Render("  Endpoint ") + valueStyle.Render(fmt.Sprintf("http://localhost:%d", port)))
	fmt.Println(descStyle.Render("  Token    ") + valueStyle.Render(modelToken))
	fmt.Println(descStyle.Render("  Tip: 'tingly-box token view model --reveal' to copy later, 'token refresh model' to rotate."))
	fmt.Println()
	return s, StepContinue, nil
}

func qsAgent(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	infos := agent.ListAgentInfo()

	items := make([]MultiSelectItem[agent.AgentType], 0, len(infos))
	for _, info := range infos {
		items = append(items, MultiSelectItem[agent.AgentType]{
			Title:       info.Name,
			Description: agentItemDescription(info),
			Value:       info.Type,
		})
	}

	r, err := MultiSelect("Configure AI coding agents now? (Space to toggle, Enter to continue, none = skip)", items, MultiSelectOptions{
		Header:    ctx.Header,
		CanGoBack: true,
		PageSize:  6,
	})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}

	s.selectedAgents = r.Value
	s.agentResults = nil
	if len(s.selectedAgents) == 0 {
		return s, StepContinue, nil
	}

	hasClaudeCode := false
	for _, t := range s.selectedAgents {
		if t == agent.AgentTypeClaudeCode {
			hasClaudeCode = true
			break
		}
	}

	if hasClaudeCode {
		uni, err := Confirm("Use unified mode for Claude Code? (single config for all models)", ConfirmOptions{
			Header:     ctx.Header,
			DefaultYes: true,
			CanGoBack:  true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if uni.IsBack() {
			return s, StepBack, nil
		}
		if uni.IsCancel() {
			return s, StepCancel, nil
		}
		s.ccUnified = uni.Value

		sl, err := Confirm("Install Claude Code status line script?", ConfirmOptions{
			Header:     ctx.Header,
			DefaultYes: false,
			CanGoBack:  true,
		})
		if err != nil {
			return s, StepCancel, err
		}
		if sl.IsBack() {
			return s, StepBack, nil
		}
		if sl.IsCancel() {
			return s, StepCancel, nil
		}
		s.ccInstallStatusLine = sl.Value
	}

	apply := agent.NewAgentApply(s.mgr.GetGlobalConfig(), "localhost")
	for _, t := range s.selectedAgents {
		t := t
		req := &agent.ApplyAgentRequest{
			AgentType:         t,
			Provider:          s.provider.UUID,
			Model:             s.model,
			Unified:           s.ccUnified,
			InstallStatusLine: s.ccInstallStatusLine,
			Force:             true,
		}
		label := string(t)
		if info, ok := agent.GetAgentInfo(t); ok {
			label = info.Name
		}
		res, err := WithSpinner(fmt.Sprintf("Applying %s configuration", label), func() (*agent.ApplyAgentResult, error) {
			return apply.ApplyAgent(req)
		})
		if err != nil {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  ✗ %s: %v", label, err)))
			continue
		}
		s.agentResults = append(s.agentResults, res)
		if res != nil && !res.Success {
			fmt.Println(errorStyle.Render(fmt.Sprintf("  ✗ %s: %s", label, res.Message)))
		}
	}
	return s, StepContinue, nil
}

// agentItemDescription returns a one-line summary of where the agent writes
// its configuration (used by the MultiSelect description column).
func agentItemDescription(info agent.AgentInfo) string {
	if len(info.ConfigFiles) == 0 {
		return info.Description
	}
	return "writes " + strings.Join(info.ConfigFiles, ", ")
}

func qsDone(ctx StepContext, s quickstartState) (quickstartState, StepResult, error) {
	port := s.mgr.GetServerPort()
	if port == 0 {
		port = 12580
	}

	summary := fmt.Sprintf(
		"%s\n\n  %s  %s\n  %s  %s\n  %s  %s\n",
		successStyle.Render("Setup complete!"),
		descStyle.Render("Provider"), valueStyle.Render(s.provider.Name),
		descStyle.Render("Model   "), valueStyle.Render(s.model),
		descStyle.Render("Server  "), valueStyle.Render(fmt.Sprintf("http://localhost:%d", port)),
	)
	fmt.Println()
	fmt.Println(ctx.Header)
	fmt.Println()
	fmt.Println(summary)

	if len(s.agentResults) > 0 {
		fmt.Println(descStyle.Render("Agents configured:"))
		for _, res := range s.agentResults {
			if res == nil {
				continue
			}
			marker := successStyle.Render("  ✓ ")
			if !res.Success {
				marker = errorStyle.Render("  ✗ ")
			}
			label := string(res.AgentType)
			if info, ok := agent.GetAgentInfo(res.AgentType); ok {
				label = info.Name
			}
			fmt.Println(marker + valueStyle.Render(label))
			for _, f := range res.ConfigFiles {
				fmt.Println(descStyle.Render("      " + f))
			}
		}
		fmt.Println()
	}

	r, err := Confirm("Start the server now?", ConfirmOptions{DefaultYes: false, CanGoBack: true})
	if err != nil {
		return s, StepCancel, err
	}
	switch {
	case r.IsBack():
		return s, StepBack, nil
	case r.IsCancel():
		return s, StepCancel, nil
	}
	s.startServer = r.Value
	return s, StepDone, nil
}

// ---------- helpers ----------

func isRuleConfigured(rule *typ.Rule, cfg *serverconfig.Config) bool {
	if rule == nil || !rule.Active || len(rule.Services) == 0 {
		return false
	}
	for _, svc := range rule.Services {
		if svc == nil || !svc.Active || svc.Provider == "" || svc.Model == "" {
			continue
		}
		if p, err := cfg.GetProviderByUUID(svc.Provider); err == nil && p != nil {
			return true
		}
	}
	return false
}

func startServer(mgr QuickstartManager) error {
	port := mgr.GetServerPort()
	if port == 0 {
		port = 12580
	}
	if err := mgr.SetupServerWithPort(port); err != nil {
		return fmt.Errorf("failed to setup server: %w", err)
	}
	if err := mgr.StartServer(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("Server started at http://localhost:%d", port)))
	fmt.Println(descStyle.Render("Press Ctrl+C to stop"))
	select {}
}
