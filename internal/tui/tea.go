package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// Common errors
var (
	ErrCancelled = errors.New("cancelled")
	ErrBack      = errors.New("back")
)

// Action represents user's navigation action
type Action int

const (
	ActionConfirm Action = iota // User confirmed selection/input
	ActionBack                  // User pressed back/escape
	ActionCancel                // User cancelled (Ctrl+C)
)

// Result wraps component output with navigation context
type Result[T any] struct {
	Value  T
	Action Action
}

// IsConfirm returns true if user confirmed
func (r Result[T]) IsConfirm() bool {
	return r.Action == ActionConfirm
}

// IsBack returns true if user went back
func (r Result[T]) IsBack() bool {
	return r.Action == ActionBack
}

// IsCancel returns true if user cancelled
func (r Result[T]) IsCancel() bool {
	return r.Action == ActionCancel
}

// RunProgram is a helper to run a tea program and handle cancellation
func RunProgram(m tea.Model) (tea.Model, error) {
	p := tea.NewProgram(m)
	model, err := p.Run()
	if err != nil {
		return nil, err
	}
	return model, nil
}
