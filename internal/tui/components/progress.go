package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// WithProgress runs an async operation with a spinner
func WithProgress[T any](message string, fn func() (T, error)) (T, error) {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.ProgressStyle

	m := &ProgressModel[T]{
		message: message,
		fn:      fn,
		spinner: s,
	}

	_, err := tui.RunProgram(m)
	if err != nil {
		var zero T
		return zero, err
	}

	return m.result, m.err
}

// ProgressModel shows a spinner while running an async operation
type ProgressModel[T any] struct {
	message string
	fn      func() (T, error)
	done    bool
	result  T
	err     error
	spinner spinner.Model
}

// Init implements tea.Model
func (m *ProgressModel[T]) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.runOperation,
	)
}

func (m *ProgressModel[T]) runOperation() tea.Msg {
	result, err := m.fn()
	return operationDoneMsg[T]{result: result, err: err}
}

type operationDoneMsg[T any] struct {
	result T
	err    error
}

// Update implements tea.Model
func (m *ProgressModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case operationDoneMsg[T]:
		m.done = true
		m.result = msg.result
		m.err = msg.err
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model
func (m *ProgressModel[T]) View() string {
	if m.done {
		if m.err != nil {
			return styles.ErrorStyle.Render("✗ "+m.message) + "\n"
		}
		return styles.SuccessStyle.Render("✓ "+m.message) + "\n"
	}

	return m.spinner.View() + " " + m.message + "..."
}
