package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tingly-dev/tingly-box/internal/tui"
	"github.com/tingly-dev/tingly-box/internal/tui/styles"
)

// MultiSelectItem represents an item in a multi-select list
type MultiSelectItem[T any] struct {
	Title       string
	Description string
	Value       T
	Selected    bool
}

// MultiSelectOptions for customization
type MultiSelectOptions struct {
	Initial   map[any]bool // Initial selection state (by Value)
	PageSize  int          // Number of items to show (default: 8)
	CanGoBack bool         // Allow back navigation (default: true)
}

// MultiSelectModel is the bubbletea model for multi-selection
type MultiSelectModel[T any] struct {
	list     list.Model
	items    []MultiSelectItem[T]
	quitting bool
	canBack  bool

	// Result
	Result tui.Result[[]T]
}

// MultiSelect displays a multi-select list and returns the selected items
func MultiSelect[T any](prompt string, items []MultiSelectItem[T], opts ...MultiSelectOptions) (tui.Result[[]T], error) {
	var opt MultiSelectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.PageSize == 0 {
		opt.PageSize = 8
	}
	if opt.Initial == nil {
		opt.Initial = make(map[any]bool)
	}

	// Convert items to list items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		// Check initial selection
		selected := item.Selected
		if opt.Initial != nil {
			if v, ok := opt.Initial[fmt.Sprintf("%v", item.Value)]; ok {
				selected = v
			}
		}
		items[i].Selected = selected
		listItems[i] = multiItemWrapper[T]{item: items[i], index: i}
	}

	// Custom delegate for checkboxes
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(styles.Primary).
		Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(styles.Primary)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(styles.Foreground)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(styles.Muted)

	l := list.New(listItems, delegate, 60, min(len(items), opt.PageSize)+4)
	l.Title = prompt
	l.Styles.Title = styles.TitleStyle
	l.Styles.TitleBar = lipgloss.NewStyle()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(len(items) > opt.PageSize)

	m := &MultiSelectModel[T]{
		list:    l,
		items:   items,
		canBack: opt.CanGoBack,
	}

	model, err := tui.RunProgram(m)
	if err != nil {
		return tui.Result[[]T]{Value: nil, Action: tui.ActionCancel}, err
	}

	result := model.(*MultiSelectModel[T])
	return result.Result, nil
}

// multiItemWrapper wraps MultiSelectItem for list.Item interface
type multiItemWrapper[T any] struct {
	item  MultiSelectItem[T]
	index int
}

func (i multiItemWrapper[T]) FilterValue() string {
	return i.item.Title
}

func (i multiItemWrapper[T]) Title() string {
	if i.item.Selected {
		return "☑ " + i.item.Title
	}
	return "☐ " + i.item.Title
}

func (i multiItemWrapper[T]) Description() string {
	return i.item.Description
}

// Init implements tea.Model
func (m *MultiSelectModel[T]) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model
func (m *MultiSelectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// Confirm selection - collect selected items
			var selected []T
			for _, item := range m.items {
				if item.Selected {
					selected = append(selected, item.Value)
				}
			}
			m.Result = tui.Result[[]T]{
				Value:  selected,
				Action: tui.ActionConfirm,
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys(" "))):
			// Toggle selection
			if i, ok := m.list.SelectedItem().(multiItemWrapper[T]); ok {
				m.items[i.index].Selected = !m.items[i.index].Selected
				// Update the list item
				newItems := make([]list.Item, len(m.items))
				for idx, item := range m.items {
					newItems[idx] = multiItemWrapper[T]{item: item, index: idx}
				}
				m.list.SetItems(newItems)
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
			// Select all
			for i := range m.items {
				m.items[i].Selected = true
			}
			newItems := make([]list.Item, len(m.items))
			for idx, item := range m.items {
				newItems[idx] = multiItemWrapper[T]{item: item, index: idx}
			}
			m.list.SetItems(newItems)
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			// Select none
			for i := range m.items {
				m.items[i].Selected = false
			}
			newItems := make([]list.Item, len(m.items))
			for idx, item := range m.items {
				newItems[idx] = multiItemWrapper[T]{item: item, index: idx}
			}
			m.list.SetItems(newItems)
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "left", "h", "backspace"))):
			// Back navigation
			if m.canBack {
				m.Result = tui.Result[[]T]{
					Value:  nil,
					Action: tui.ActionBack,
				}
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
			// Cancel
			m.Result = tui.Result[[]T]{
				Value:  nil,
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
func (m *MultiSelectModel[T]) View() string {
	if m.quitting {
		return ""
	}

	view := m.list.View()

	// Count selected
	selectedCount := 0
	for _, item := range m.items {
		if item.Selected {
			selectedCount++
		}
	}

	// Help footer
	helpParts := []string{
		"↑/↓: Navigate",
		"Space: Toggle",
		"a: All",
		"n: None",
		fmt.Sprintf("(%d selected)", selectedCount),
	}
	if m.canBack {
		helpParts = append(helpParts, "Esc: Back")
	}
	helpParts = append(helpParts, "Enter: Confirm", "Ctrl+C: Quit")

	help := styles.HelpStyle.Render(strings.Join(helpParts, "  "))
	return view + "\n" + help
}
