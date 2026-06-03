package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// RunProviderMode is the entry point for the Provider mode loop. It returns
// nil when the user backs out to the top-level menu.
func RunProviderMode(mgr TUIManager) error {
	for {
		items := []SelectItem[string]{
			{Title: "List", Description: "Show all configured providers", Value: "list"},
			{Title: "Add", Description: "Add a new provider", Value: "add"},
			{Title: "Edit", Description: "Edit an existing provider", Value: "edit"},
			{Title: "Delete", Description: "Remove a provider", Value: "delete"},
			{Title: "Refresh models", Description: "Re-fetch model list from a provider", Value: "refresh"},
			{Title: "Back", Description: "Return to the main menu", Value: "back"},
		}
		r, err := Select("Provider:", items, SelectOptions{
			Header:    titleStyle.Render("Tingly Box · TUI · Provider"),
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
			opErr = providerList(mgr)
		case "add":
			opErr = providerAdd(mgr)
		case "edit":
			opErr = providerEdit(mgr)
		case "delete":
			opErr = providerDelete(mgr)
		case "refresh":
			opErr = providerRefreshModels(mgr)
		}
		if opErr != nil && opErr != ErrCancelled {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %v", opErr)))
		}
	}
}

func providerList(mgr TUIManager) error {
	providers := mgr.ListProviders()
	if len(providers) == 0 {
		fmt.Println(descStyle.Render("No providers configured."))
		Pause("")
		return nil
	}
	fmt.Println()
	fmt.Println(promptStyle.Render(fmt.Sprintf("%d provider(s):", len(providers))))
	for i, p := range providers {
		status := successStyle.Render("enabled")
		if !p.Enabled {
			status = errorStyle.Render("disabled")
		}
		fmt.Printf("  %d. %s  %s\n", i+1, valueStyle.Render(p.Name), status)
		fmt.Println(descStyle.Render(fmt.Sprintf("     uuid: %s", p.UUID)))
		fmt.Println(descStyle.Render(fmt.Sprintf("     %s · %s", p.APIStyle, p.APIBase)))
	}
	fmt.Println()
	Pause("")
	return nil
}

func providerAdd(mgr TUIManager) error {
	nameR, err := Input("Provider name:", InputOptions{Required: true, CanGoBack: true})
	if err != nil || nameR.IsCancel() || nameR.IsBack() {
		return nil
	}

	styleR, err := Select("API style:", []SelectItem[protocol.APIStyle]{
		{Title: "OpenAI-compatible", Description: "/v1/chat/completions endpoint", Value: protocol.APIStyleOpenAI},
		{Title: "Anthropic-compatible", Description: "/v1/messages endpoint", Value: protocol.APIStyleAnthropic},
	}, SelectOptions{CanGoBack: true})
	if err != nil || styleR.IsCancel() || styleR.IsBack() {
		return nil
	}

	defURL := "https://api.example.com/v1"
	if styleR.Value == protocol.APIStyleAnthropic {
		defURL = "https://api.anthropic.com"
	}
	baseR, err := Input("Base URL:", InputOptions{Placeholder: defURL, CanGoBack: true})
	if err != nil || baseR.IsCancel() || baseR.IsBack() {
		return nil
	}
	apiBase := baseR.Value
	if apiBase == "" {
		apiBase = defURL
	}

	tokenR, err := Input("API key:", InputOptions{Required: true, Mask: true, CanGoBack: true})
	if err != nil || tokenR.IsCancel() || tokenR.IsBack() {
		return nil
	}

	proxyR, err := Input("Proxy URL (optional):", InputOptions{Placeholder: "e.g. http://localhost:7890", CanGoBack: true})
	if err != nil || proxyR.IsCancel() || proxyR.IsBack() {
		return nil
	}

	uuid, err := mgr.AddProvider(nameR.Value, apiBase, tokenR.Value, styleR.Value)
	if err != nil {
		return err
	}
	if proxyR.Value != "" {
		if p, gerr := mgr.GetProvider(uuid); gerr == nil && p != nil {
			p.ProxyURL = proxyR.Value
			if uerr := mgr.UpdateProviderByUUID(uuid, p); uerr != nil {
				return fmt.Errorf("save proxy: %w", uerr)
			}
		}
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ Provider '%s' added (uuid: %s).", nameR.Value, uuid)))
	Pause("")
	return nil
}

func pickProvider(mgr TUIManager, prompt string) (*typ.Provider, error) {
	providers := mgr.ListProviders()
	if len(providers) == 0 {
		fmt.Println(descStyle.Render("No providers configured. Add one from Provider → Add first."))
		Pause("")
		return nil, nil
	}
	sort.Slice(providers, func(i, j int) bool {
		return strings.ToLower(providers[i].Name) < strings.ToLower(providers[j].Name)
	})
	items := make([]SelectItem[string], 0, len(providers))
	for _, p := range providers {
		items = append(items, SelectItem[string]{
			Title:       p.Name,
			Description: fmt.Sprintf("%s · %s", p.APIStyle, p.APIBase),
			Value:       p.UUID,
		})
	}
	r, err := Select(prompt, items, SelectOptions{CanGoBack: true, PageSize: 10})
	if err != nil {
		return nil, err
	}
	if r.IsCancel() || r.IsBack() {
		return nil, nil
	}
	for _, p := range providers {
		if p.UUID == r.Value {
			return p, nil
		}
	}
	return nil, nil
}

func providerEdit(mgr TUIManager) error {
	p, err := pickProvider(mgr, "Select provider to edit:")
	if err != nil || p == nil {
		return err
	}

	nameR, err := Input("Name:", InputOptions{Initial: p.Name, Required: true, CanGoBack: true})
	if err != nil || nameR.IsCancel() || nameR.IsBack() {
		return nil
	}
	baseR, err := Input("Base URL:", InputOptions{Initial: p.APIBase, Required: true, CanGoBack: true})
	if err != nil || baseR.IsCancel() || baseR.IsBack() {
		return nil
	}
	tokenR, err := Input("API key (leave blank to keep):", InputOptions{Mask: true, CanGoBack: true})
	if err != nil || tokenR.IsCancel() || tokenR.IsBack() {
		return nil
	}
	styleR, err := Select("API style:", []SelectItem[protocol.APIStyle]{
		{Title: "OpenAI-compatible", Value: protocol.APIStyleOpenAI},
		{Title: "Anthropic-compatible", Value: protocol.APIStyleAnthropic},
	}, SelectOptions{CanGoBack: true, Initial: p.APIStyle})
	if err != nil || styleR.IsCancel() || styleR.IsBack() {
		return nil
	}
	proxyR, err := Input("Proxy URL:", InputOptions{Initial: p.ProxyURL, CanGoBack: true})
	if err != nil || proxyR.IsCancel() || proxyR.IsBack() {
		return nil
	}

	p.Name = nameR.Value
	p.APIBase = baseR.Value
	p.APIStyle = styleR.Value
	p.ProxyURL = proxyR.Value
	if tokenR.Value != "" {
		p.Token = tokenR.Value
	}
	if err := mgr.UpdateProviderByUUID(p.UUID, p); err != nil {
		return err
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ Provider '%s' updated.", p.Name)))
	Pause("")
	return nil
}

func providerDelete(mgr TUIManager) error {
	p, err := pickProvider(mgr, "Select provider to delete:")
	if err != nil || p == nil {
		return err
	}
	confirm, err := Confirm(fmt.Sprintf("Delete provider '%s'?", p.Name), ConfirmOptions{
		DefaultYes: false, CanGoBack: true,
		Description: "This is irreversible. Rules referencing it will be left dangling.",
	})
	if err != nil || !confirm.IsConfirm() || !confirm.Value {
		return nil
	}
	if err := mgr.DeleteProviderByUUID(p.UUID); err != nil {
		return err
	}
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ Provider '%s' deleted.", p.Name)))
	Pause("")
	return nil
}

func providerRefreshModels(mgr TUIManager) error {
	p, err := pickProvider(mgr, "Refresh models for which provider?")
	if err != nil || p == nil {
		return err
	}
	_, err = WithSpinner(fmt.Sprintf("Fetching models from %s", p.Name), func() (struct{}, error) {
		return struct{}{}, mgr.FetchAndSaveProviderModels(p.UUID)
	})
	if err != nil {
		return err
	}
	models := availableModels(mgr, p)
	fmt.Println(successStyle.Render(fmt.Sprintf("✓ %d model(s) available for %s.", len(models), p.Name)))
	for _, m := range models {
		fmt.Println(descStyle.Render("  - " + m))
	}
	fmt.Println()
	Pause("")
	return nil
}
