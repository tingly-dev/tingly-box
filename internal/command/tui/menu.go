package tui

import (
	"fmt"
)

// RunTUI renders the top-level TUI mode menu. On entry the user picks a
// mode — QuickStart (the guided wizard), Provider, Rule, or Agent —
// and is dropped into that mode. After a mode returns, the menu is
// shown again until the user exits.
func RunTUI(mgr TUIManager) error {
	for {
		items := []SelectItem[string]{
			{
				Title:       "QuickStart",
				Description: "Guided setup wizard — credential, model, rules, agent",
				Value:       "quickstart",
			},
			{
				Title:       "Provider",
				Description: "Add, list, edit, delete AI providers",
				Value:       "provider",
			},
			{
				Title:       "Rule",
				Description: "Add, list, edit, delete routing rules",
				Value:       "rule",
			},
			{
				Title:       "Agent",
				Description: "Apply, view, restore agent config (claude-code, opencode, codex…)",
				Value:       "agent",
			},
			{
				Title:       "Exit",
				Description: "Leave the TUI",
				Value:       "exit",
			},
		}

		header := titleStyle.Render("Tingly Box · TUI")
		r, err := Select("Pick a mode:", items, SelectOptions{
			Header:   header,
			PageSize: 10,
		})
		if err != nil {
			return err
		}
		if r.IsCancel() {
			return nil
		}

		switch r.Value {
		case "quickstart":
			if err := RunQuickstart(mgr); err != nil {
				printModeError("QuickStart", err)
			}
		case "provider":
			if err := RunProviderMode(mgr); err != nil {
				printModeError("Provider", err)
			}
		case "rule":
			if err := RunRuleMode(mgr); err != nil {
				printModeError("Rule", err)
			}
		case "agent":
			if err := RunAgentMode(mgr); err != nil {
				printModeError("Agent", err)
			}
		case "exit":
			return nil
		}
	}
}

func printModeError(mode string, err error) {
	if err == ErrCancelled {
		return
	}
	fmt.Println(errorStyle.Render(fmt.Sprintf("✗ %s mode: %v", mode, err)))
}
