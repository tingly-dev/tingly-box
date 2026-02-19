package components

import (
	"context"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// StepResult indicates what the wizard should do after a step
type StepResult int

const (
	StepContinue StepResult = iota // Move to next step
	StepBack                       // Go back to previous step
	StepDone                       // Wizard complete
	StepSkip                       // Skip this step
	StepCancel                     // User cancelled
)

// Step defines a single step in the wizard
type Step[T any] struct {
	Name    string             // Step name shown in UI
	Skip    func(state T) bool // Return true to skip this step
	Execute func(ctx context.Context, state T) (T, StepResult, error)
}

// Wizard manages multi-step flows with back navigation
type Wizard[T any] struct {
	title   string
	steps   []Step[T]
	state   T
	current int
	history []int // Stack of step indices for back navigation
}

// WizardOption for customizing wizard behavior
type WizardOption[T any] func(*Wizard[T])

// WithTitle sets the wizard title
func WithTitle[T any](title string) WizardOption[T] {
	return func(w *Wizard[T]) {
		w.title = title
	}
}

// NewWizard creates a new wizard
func NewWizard[T any](initialState T, steps []Step[T], opts ...WizardOption[T]) *Wizard[T] {
	w := &Wizard[T]{
		title:   "Wizard",
		steps:   steps,
		state:   initialState,
		current: 0,
		history: []int{},
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run executes the wizard procedurally
func (w *Wizard[T]) Run() (T, error) {
	ctx := context.Background()

	for {
		// Skip current step if needed
		if w.shouldSkipCurrent() {
			if !w.moveToNext() {
				// No more steps
				return w.state, nil
			}
			continue
		}

		// Show wizard header
		w.showHeader()

		// Execute current step
		newState, result, err := w.steps[w.current].Execute(ctx, w.state)
		if err != nil {
			return w.state, err
		}
		w.state = newState

		// Handle result
		switch result {
		case StepContinue:
			if !w.moveToNext() {
				// No more steps
				return w.state, nil
			}
		case StepBack:
			w.moveToPrevious()
		case StepDone:
			return w.state, nil
		case StepCancel:
			return w.state, tui.ErrCancelled
		case StepSkip:
			if !w.moveToNext() {
				return w.state, nil
			}
		}
	}
}

func (w *Wizard[T]) shouldSkipCurrent() bool {
	if w.current >= len(w.steps) {
		return false
	}
	step := w.steps[w.current]
	return step.Skip != nil && step.Skip(w.state)
}

func (w *Wizard[T]) moveToNext() bool {
	// Find next non-skippable step
	for i := w.current + 1; i < len(w.steps); i++ {
		if w.steps[i].Skip == nil || !w.steps[i].Skip(w.state) {
			w.history = append(w.history, w.current)
			w.current = i
			return true
		}
	}
	// No more steps
	return false
}

func (w *Wizard[T]) moveToPrevious() {
	if len(w.history) > 0 {
		w.current = w.history[len(w.history)-1]
		w.history = w.history[:len(w.history)-1]
	}
}

func (w *Wizard[T]) showHeader() {
	// Count active (non-skipped) steps
	activeSteps := 0
	currentActiveIndex := 0
	for i, step := range w.steps {
		if step.Skip == nil || !step.Skip(w.state) {
			if i < w.current {
				currentActiveIndex++
			}
			activeSteps++
		}
	}

	// Title
	title := styles.TitleStyle.Render(w.title)
	fmt.Println(title)

	// Step indicator
	stepText := fmt.Sprintf("Step %d/%d: %s", currentActiveIndex+1, activeSteps, w.steps[w.current].Name)
	fmt.Println(styles.SubtitleStyle.Render(stepText))

	// Separator
	sep := strings.Repeat("─", 60)
	fmt.Println(styles.DescriptionStyle.Render(sep))

	// Progress dots
	var dots strings.Builder
	for i := 0; i < activeSteps; i++ {
		if i <= currentActiveIndex {
			dots.WriteString(styles.ProgressDotActive.String())
		} else {
			dots.WriteString(styles.ProgressDotInactive.String())
		}
		if i < activeSteps-1 {
			dots.WriteString(" ")
		}
	}
	fmt.Println(styles.ProgressStyle.Render(dots.String()))
	fmt.Println()
}

// RunWizard is a convenience function to run a wizard
func RunWizard[T any](title string, initialState T, steps []Step[T]) (T, error) {
	w := NewWizard(initialState, steps, WithTitle[T](title))
	return w.Run()
}
