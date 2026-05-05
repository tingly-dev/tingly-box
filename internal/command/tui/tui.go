// Package tui provides interactive terminal prompts for the tingly-box CLI.
//
// It is intentionally small and self contained: a handful of single-shot
// prompts (Confirm, Input, Select, MultiSelect, Spinner) plus a Wizard runner
// that strings them together with a shared header, breadcrumb and help line.
package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Common navigation outcomes.
var (
	ErrCancelled = errors.New("cancelled")
	ErrBack      = errors.New("back")
)

// Action describes how a prompt was dismissed.
type Action int

const (
	ActionConfirm Action = iota // user confirmed (Enter / y)
	ActionBack                  // user requested to go back (Esc / ←)
	ActionCancel                // user aborted (Ctrl+C / q)
)

// Result wraps a prompt value with the dismissal Action.
type Result[T any] struct {
	Value  T
	Action Action
}

func (r Result[T]) IsConfirm() bool { return r.Action == ActionConfirm }
func (r Result[T]) IsBack() bool    { return r.Action == ActionBack }
func (r Result[T]) IsCancel() bool  { return r.Action == ActionCancel }

// run runs a tea program in inline mode (no alt-screen) so prompts compose
// naturally with normal stdout output.
func run(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m).Run()
}

// ---------- theme ----------

var (
	colAccent  = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#B4BEFE"}
	colSuccess = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#A6E3A1"}
	colDanger  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F38BA8"}
	colText    = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#CDD6F4"}
	colMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9099B0"}
	colSubtle  = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#585B70"}
)

var (
	titleStyle  = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	stepStyle   = lipgloss.NewStyle().Foreground(colMuted)
	crumbDone   = lipgloss.NewStyle().Foreground(colSuccess)
	crumbActive = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	crumbTodo   = lipgloss.NewStyle().Foreground(colSubtle)
	crumbSep    = lipgloss.NewStyle().Foreground(colSubtle).SetString(" › ")

	promptStyle = lipgloss.NewStyle().Foreground(colText).Bold(true)
	qMark       = lipgloss.NewStyle().Foreground(colAccent).Bold(true).SetString("?")
	helpStyle   = lipgloss.NewStyle().Foreground(colSubtle)
	keyStyle    = lipgloss.NewStyle().Foreground(colMuted).Bold(true)

	itemStyle    = lipgloss.NewStyle().Foreground(colText)
	itemSelStyle = lipgloss.NewStyle().Foreground(colAccent).Bold(true)
	descStyle    = lipgloss.NewStyle().Foreground(colMuted)
	descSelStyle = lipgloss.NewStyle().Foreground(colMuted).Italic(true)

	errorStyle   = lipgloss.NewStyle().Foreground(colDanger)
	successStyle = lipgloss.NewStyle().Foreground(colSuccess).Bold(true)
	valueStyle   = lipgloss.NewStyle().Foreground(colAccent)

	cursorStr   = itemSelStyle.Render("❯")
	noCursorStr = " "
	checkOn     = itemSelStyle.Render("◉")
	checkOff    = lipgloss.NewStyle().Foreground(colSubtle).Render("○")
)

// helpLine renders a compact "key: action  key: action" footer.
func helpLine(items ...[2]string) string {
	out := ""
	for i, kv := range items {
		if i > 0 {
			out += helpStyle.Render("  ")
		}
		out += keyStyle.Render(kv[0]) + helpStyle.Render(" "+kv[1])
	}
	return helpStyle.Render(out)
}
