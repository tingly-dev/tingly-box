package components

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// ConfirmOptions for confirmation dialog
type ConfirmOptions struct {
	DefaultYes  bool   // Default to Yes (default: true)
	CanGoBack   bool   // Allow back navigation
	Description string // Optional description text
}

// ConfirmModel is the bubbletea model for confirmation
type ConfirmModel struct {
	prompt   string
	opts     ConfirmOptions
	quitting bool
	aborting bool

	// Result
	Result tui.Result[bool]
}

// Confirm displays a yes/no prompt
func Confirm(prompt string, opts ...ConfirmOptions) (tui.Result[bool], error) {
	var opt ConfirmOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	m := &ConfirmModel{
		prompt: prompt,
		opts:   opt,
	}

	model, err := tui.RunProgram(m)
	if err != nil {
		return tui.Result[bool]{Value: false, Action: tui.ActionCancel}, err
	}

	result := model.(*ConfirmModel)
	return result.Result, nil
}

// Init implements tea.Model
func (m *ConfirmModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.Result = tui.Result[bool]{
				Value:  true,
				Action: tui.ActionConfirm,
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N"))):
			m.Result = tui.Result[bool]{
				Value:  false,
				Action: tui.ActionConfirm,
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Enter uses the default value
			m.Result = tui.Result[bool]{
				Value:  m.opts.DefaultYes,
				Action: tui.ActionConfirm,
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "left"))):
			if m.opts.CanGoBack {
				m.Result = tui.Result[bool]{
					Value:  false,
					Action: tui.ActionBack,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
			m.Result = tui.Result[bool]{
				Value:  false,
				Action: tui.ActionCancel,
			}
			m.aborting = true
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model
func (m *ConfirmModel) View() string {
	if m.quitting {
		return ""
	}

	// Build prompt
	var parts []string

	// Main prompt
	var yesText, noText string
	mutedStyle := lipgloss.NewStyle().Foreground(styles.Muted)
	if m.opts.DefaultYes {
		yesText = styles.SuccessStyle.Render("Y")
		noText = mutedStyle.Render("n")
	} else {
		yesText = mutedStyle.Render("Y")
		noText = styles.SuccessStyle.Render("n")
	}

	promptLine := styles.PromptStyle.Render("? "+m.prompt) + " " +
		"(" + yesText + "/" + noText + ")"
	parts = append(parts, promptLine)

	// Description
	if m.opts.Description != "" {
		parts = append(parts, styles.DescriptionStyle.Render("  "+m.opts.Description))
	}

	// Help
	helpText := "y: Yes  n: No  Enter: Default"
	if m.opts.CanGoBack {
		helpText += "  Esc: Back"
	}
	parts = append(parts, styles.HelpStyle.Render(helpText))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
