package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StepResult signals what the wizard should do after a step.
type StepResult int

const (
	StepContinue StepResult = iota // advance to the next step
	StepBack                       // go back to the previous step
	StepDone                       // wizard finished successfully
	StepSkip                       // skip this step
	StepCancel                     // user cancelled
)

// StepContext carries presentation info passed to each Step's Execute.
type StepContext struct {
	Header string // ready-to-print breadcrumb/title
	Index  int    // 0-indexed position among the active (non-skipped) steps
	Total  int    // total active steps
}

// Step describes a single phase in a wizard.
type Step[S any] struct {
	Name    string
	Skip    func(state S) bool
	Execute func(ctx StepContext, state S) (S, StepResult, error)
}

// RunWizard executes the wizard and returns the final state.
func RunWizard[S any](title string, initial S, steps []Step[S]) (S, error) {
	w := wizard[S]{title: title, steps: steps, state: initial}
	return w.run()
}

type wizard[S any] struct {
	title   string
	steps   []Step[S]
	state   S
	current int
	history []int
}

func (w *wizard[S]) run() (S, error) {
	for {
		if w.current >= len(w.steps) {
			return w.state, nil
		}
		// auto-skip
		if step := w.steps[w.current]; step.Skip != nil && step.Skip(w.state) {
			if !w.advance() {
				return w.state, nil
			}
			continue
		}

		ctx := StepContext{
			Header: w.renderHeader(),
			Index:  w.activeIndex(),
			Total:  w.activeTotal(),
		}

		newState, result, err := w.steps[w.current].Execute(ctx, w.state)
		if err != nil {
			return w.state, err
		}
		w.state = newState

		switch result {
		case StepContinue, StepSkip:
			if !w.advance() {
				return w.state, nil
			}
		case StepBack:
			w.retreat()
		case StepDone:
			return w.state, nil
		case StepCancel:
			return w.state, ErrCancelled
		}
	}
}

func (w *wizard[S]) advance() bool {
	for i := w.current + 1; i < len(w.steps); i++ {
		if w.steps[i].Skip == nil || !w.steps[i].Skip(w.state) {
			w.history = append(w.history, w.current)
			w.current = i
			return true
		}
	}
	return false
}

func (w *wizard[S]) retreat() {
	if n := len(w.history); n > 0 {
		w.current = w.history[n-1]
		w.history = w.history[:n-1]
	}
}

func (w *wizard[S]) activeIndex() int {
	idx := 0
	for i := 0; i < w.current; i++ {
		if step := w.steps[i]; step.Skip == nil || !step.Skip(w.state) {
			idx++
		}
	}
	return idx
}

func (w *wizard[S]) activeTotal() int {
	n := 0
	for _, step := range w.steps {
		if step.Skip == nil || !step.Skip(w.state) {
			n++
		}
	}
	return n
}

// renderHeader returns the title line plus a one-line breadcrumb showing the
// progression through the wizard's active steps.
func (w *wizard[S]) renderHeader() string {
	cur := w.activeIndex()

	var crumbs []string
	idx := 0
	for _, step := range w.steps {
		if step.Skip != nil && step.Skip(w.state) {
			continue
		}
		var c string
		switch {
		case idx < cur:
			c = crumbDone.Render("✓ " + step.Name)
		case idx == cur:
			c = crumbActive.Render("❯ " + step.Name)
		default:
			c = crumbTodo.Render("· " + step.Name)
		}
		crumbs = append(crumbs, c)
		idx++
	}

	title := titleStyle.Render(w.title)
	step := stepStyle.Render(fmt.Sprintf("Step %d/%d", cur+1, w.activeTotal()))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		title+"  "+step,
		strings.Join(crumbs, crumbSep.String()),
	)
}

// ============================================================================
// Spinner
// ============================================================================

// WithSpinner renders an animated spinner while fn runs and returns its
// result.
func WithSpinner[T any](message string, fn func() (T, error)) (T, error) {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(colAccent)

	m := &spinModel[T]{message: message, fn: fn, spinner: s}
	out, err := run(m)
	if err != nil {
		var zero T
		return zero, err
	}
	final := out.(*spinModel[T])
	return final.result, final.err
}

type spinModel[T any] struct {
	message string
	fn      func() (T, error)
	spinner spinner.Model
	done    bool
	result  T
	err     error
}

type spinDoneMsg[T any] struct {
	result T
	err    error
}

func (m *spinModel[T]) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		r, err := m.fn()
		return spinDoneMsg[T]{result: r, err: err}
	})
}

func (m *spinModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case spinDoneMsg[T]:
		m.done = true
		m.result = v.result
		m.err = v.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	case tea.KeyMsg:
		if v.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *spinModel[T]) View() string {
	if m.done {
		if m.err != nil {
			return errorStyle.Render("✗ "+m.message) + "\n"
		}
		return successStyle.Render("✓ "+m.message) + "\n"
	}
	return m.spinner.View() + " " + descStyle.Render(m.message+"...")
}
