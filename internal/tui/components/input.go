package components

import (
	"errors"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// Validation errors
var (
	ErrRequired = errors.New("this field is required")
)

// InputOptions for text input customization
type InputOptions struct {
	Placeholder string             // Placeholder text
	Required    bool               // Whether input is required
	Mask        bool               // Mask input (for passwords)
	Initial     string             // Initial value
	Validate    func(string) error // Validation function
	CharLimit   int                // Character limit (0 = unlimited)
	ShowHelp    bool               // Show help footer
	CanGoBack   bool               // Allow back navigation
}

// InputModel is the bubbletea model for text input
type InputModel struct {
	textInput textinput.Model
	prompt    string
	opts      InputOptions
	err       error
	quitting  bool

	// Result
	Result tui.Result[string]
}

// Input displays a text input prompt
func Input(prompt string, opts ...InputOptions) (tui.Result[string], error) {
	var opt InputOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	ti := textinput.New()
	ti.Placeholder = opt.Placeholder
	ti.SetValue(opt.Initial)
	ti.CharLimit = opt.CharLimit
	ti.Width = 50

	if opt.Mask {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}

	ti.Focus()

	m := &InputModel{
		textInput: ti,
		prompt:    prompt,
		opts:      opt,
	}

	model, err := tui.RunProgram(m)
	if err != nil {
		return tui.Result[string]{Value: "", Action: tui.ActionCancel}, err
	}

	result := model.(*InputModel)
	return result.Result, nil
}

// Init implements tea.Model
func (m *InputModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implements tea.Model
func (m *InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Validate and confirm
			value := m.textInput.Value()

			// Check required
			if m.opts.Required && value == "" {
				m.err = ErrRequired
				return m, nil
			}

			// Custom validation
			if m.opts.Validate != nil {
				if err := m.opts.Validate(value); err != nil {
					m.err = err
					return m, nil
				}
			}

			m.Result = tui.Result[string]{
				Value:  value,
				Action: tui.ActionConfirm,
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "backspace"))):
			// Back navigation (only if input is empty or with Ctrl)
			if m.canGoBack() && (m.textInput.Value() == "" || key.Matches(msg, key.NewBinding(key.WithKeys("esc")))) {
				m.Result = tui.Result[string]{
					Value:  m.textInput.Value(), // Preserve current input
					Action: tui.ActionBack,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "ctrl+d"))):
			// Cancel
			m.Result = tui.Result[string]{
				Value:  "",
				Action: tui.ActionCancel,
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.err = nil // Clear error on any input
	return m, cmd
}

func (m *InputModel) canGoBack() bool {
	return m.opts.CanGoBack
}

// View implements tea.Model
func (m *InputModel) View() string {
	if m.quitting {
		return ""
	}

	// Prompt style
	promptStyle := styles.PromptStyle
	if m.err != nil {
		promptStyle = styles.ErrorStyle
	}

	// Build view
	var parts []string

	// Prompt
	parts = append(parts, promptStyle.Render("? "+m.prompt))

	// Input field
	inputView := m.textInput.View()
	if m.textInput.Focused() {
		inputView = styles.FocusedBorder.Render(inputView)
	} else {
		inputView = styles.BlurredBorder.Render(inputView)
	}
	parts = append(parts, inputView)

	// Error message
	if m.err != nil {
		parts = append(parts, styles.ErrorStyle.Render("  "+m.err.Error()))
	}

	// Help
	helpText := "Enter: Confirm"
	if m.opts.CanGoBack {
		helpText += "  Esc: Back"
	}
	helpText += "  Ctrl+C: Cancel"
	parts = append(parts, styles.HelpStyle.Render(helpText))

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
