package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// SelectItem represents an item in a selection list
type SelectItem[T any] struct {
	Title       string
	Description string
	Value       T
}

// SelectOptions for customization
type SelectOptions struct {
	Initial    any    // Initial selection value
	PageSize   int    // Number of items to show (default: 8)
	ShowHelp   bool   // Show help footer (default: true)
	CanGoBack  bool   // Allow back navigation (default: true)
	BackLabel  string // Label for back action (default: "Back")
	EmptyLabel string // Label when no items
}

// SelectModel is the bubbletea model for selection
type SelectModel[T any] struct {
	list     list.Model
	items    []SelectItem[T]
	quitting bool
	selected bool
	canBack  bool

	// Result
	Result tui.Result[T]
}

// Select displays a selection list and returns the selected item's value
func Select[T any](prompt string, items []SelectItem[T], opts ...SelectOptions) (tui.Result[T], error) {
	var opt SelectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.PageSize == 0 {
		opt.PageSize = 8
	}
	if opt.BackLabel == "" {
		opt.BackLabel = "Back"
	}
	if opt.EmptyLabel == "" {
		opt.EmptyLabel = "No items available"
	}

	// Convert items to list items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = itemWrapper[T]{item: item}
	}

	// Setup list
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(styles.Primary).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(styles.Primary)

	l := list.New(listItems, delegate, 60, min(len(items), opt.PageSize)+4)
	l.Title = prompt
	l.Styles.Title = styles.TitleStyle
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(len(items) > opt.PageSize)

	// Find initial selection
	if opt.Initial != nil {
		for i, item := range items {
			if fmt.Sprintf("%v", item.Value) == fmt.Sprintf("%v", opt.Initial) {
				l.Select(i)
				break
			}
		}
	}

	m := &SelectModel[T]{
		list:    l,
		items:   items,
		canBack: opt.CanGoBack,
	}

	model, err := tui.RunProgram(m)
	if err != nil {
		var zero T
		return tui.Result[T]{Value: zero, Action: tui.ActionCancel}, err
	}

	result := model.(*SelectModel[T])
	return result.Result, nil
}

// itemWrapper wraps SelectItem for list.Item interface
type itemWrapper[T any] struct {
	item SelectItem[T]
}

func (i itemWrapper[T]) FilterValue() string {
	return i.item.Title
}

func (i itemWrapper[T]) Title() string {
	return i.item.Title
}

func (i itemWrapper[T]) Description() string {
	return i.item.Description
}

// Init implements tea.Model
func (m *SelectModel[T]) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *SelectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Confirm selection
			i, ok := m.list.SelectedItem().(itemWrapper[T])
			if ok {
				m.Result = tui.Result[T]{
					Value:  i.item.Value,
					Action: tui.ActionConfirm,
				}
				m.selected = true
			}
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "left", "h", "backspace"))):
			// Back navigation
			if m.canBack {
				var zero T
				m.Result = tui.Result[T]{
					Value:  zero,
					Action: tui.ActionBack,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
			// Cancel
			var zero T
			m.Result = tui.Result[T]{
				Value:  zero,
				Action: tui.ActionCancel,
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model
func (m *SelectModel[T]) View() string {
	if m.quitting || m.selected {
		return ""
	}

	view := m.list.View()

	// Add help footer
	helpText := "↑/↓: Navigate  Enter: Select"
	if m.canBack {
		helpText += "  Esc/←: Back"
	}
	helpText += "  Ctrl+C: Quit"

	help := styles.HelpStyle.Render(helpText)
	return view + "\n" + help
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
