package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrRequired is returned by Input when a required field is left blank.
var ErrRequired = errors.New("this field is required")

// ============================================================================
// Confirm
// ============================================================================

// ConfirmOptions tunes a Confirm prompt.
type ConfirmOptions struct {
	Header      string // optional header (rendered above the prompt)
	DefaultYes  bool   // pressing Enter chooses Yes
	CanGoBack   bool   // allow Esc/← to go back
	Description string // optional one-line description
}

// Confirm shows a y/n prompt and returns the selection.
func Confirm(prompt string, opts ...ConfirmOptions) (Result[bool], error) {
	var opt ConfirmOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	m := &confirmModel{prompt: prompt, opts: opt, selectedYes: opt.DefaultYes}
	out, err := run(m)
	if err != nil {
		return Result[bool]{Action: ActionCancel}, err
	}
	return out.(*confirmModel).result, nil
}

type confirmModel struct {
	prompt      string
	opts        ConfirmOptions
	selectedYes bool
	done        bool
	result      Result[bool]
}

func (m *confirmModel) Init() tea.Cmd { return nil }

func (m *confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, kY):
			m.result = Result[bool]{Value: true, Action: ActionConfirm}
			m.done = true
			return m, tea.Quit
		case key.Matches(k, kN):
			m.result = Result[bool]{Value: false, Action: ActionConfirm}
			m.done = true
			return m, tea.Quit
		case key.Matches(k, kLeft):
			m.selectedYes = true
		case key.Matches(k, kRight):
			m.selectedYes = false
		case key.Matches(k, kTab):
			m.selectedYes = !m.selectedYes
		case key.Matches(k, kEnter):
			m.result = Result[bool]{Value: m.selectedYes, Action: ActionConfirm}
			m.done = true
			return m, tea.Quit
		case key.Matches(k, kBack):
			if m.opts.CanGoBack {
				m.result = Result[bool]{Action: ActionBack}
				m.done = true
				return m, tea.Quit
			}
		case key.Matches(k, kQuit):
			m.result = Result[bool]{Action: ActionCancel}
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *confirmModel) View() string {
	if m.done {
		// leave a single confirmed line behind in scrollback
		ans := "no"
		if m.result.IsConfirm() && m.result.Value {
			ans = "yes"
		}
		if m.result.IsBack() {
			ans = "(back)"
		} else if m.result.IsCancel() {
			ans = "(cancelled)"
		}
		return renderAnsweredPrompt(m.opts.Header, m.prompt, ans)
	}

	yes := "  Yes  "
	no := "  No  "
	if m.selectedYes {
		yes = itemSelStyle.Render("▸ Yes ◂")
		no = descStyle.Render("  No  ")
	} else {
		yes = descStyle.Render("  Yes ")
		no = itemSelStyle.Render("▸ No ◂")
	}

	var parts []string
	if h := strings.TrimRight(m.opts.Header, "\n"); h != "" {
		parts = append(parts, h, "")
	}
	parts = append(parts, qMark.String()+" "+promptStyle.Render(m.prompt))
	if m.opts.Description != "" {
		parts = append(parts, descStyle.Render("  "+m.opts.Description))
	}
	parts = append(parts, "  "+yes+"   "+no)

	help := helpLine(
		[2]string{"y/n", "answer"},
		[2]string{"←/→", "toggle"},
		[2]string{"↵", "confirm"},
	)
	if m.opts.CanGoBack {
		help += "  " + helpLine([2]string{"esc", "back"})
	}
	help += "  " + helpLine([2]string{"^c", "quit"})
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ============================================================================
// Input
// ============================================================================

// InputOptions tunes an Input prompt.
type InputOptions struct {
	Header      string
	Placeholder string
	Required    bool
	Mask        bool
	Initial     string
	Validate    func(string) error
	CharLimit   int
	CanGoBack   bool
}

// Input shows a text-input prompt and returns the entered value.
func Input(prompt string, opts ...InputOptions) (Result[string], error) {
	var opt InputOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	ti := textinput.New()
	ti.Placeholder = opt.Placeholder
	ti.SetValue(opt.Initial)
	ti.CharLimit = opt.CharLimit
	ti.Width = 60
	ti.Prompt = ""
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(colText)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(colSubtle)
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(colAccent)
	if opt.Mask {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	ti.Focus()

	m := &inputModel{ti: ti, prompt: prompt, opts: opt}
	out, err := run(m)
	if err != nil {
		return Result[string]{Action: ActionCancel}, err
	}
	return out.(*inputModel).result, nil
}

type inputModel struct {
	ti     textinput.Model
	prompt string
	opts   InputOptions
	err    error
	done   bool
	result Result[string]
}

func (m *inputModel) Init() tea.Cmd { return textinput.Blink }

func (m *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, kEnter):
			val := m.ti.Value()
			if m.opts.Required && val == "" {
				m.err = ErrRequired
				return m, nil
			}
			if m.opts.Validate != nil {
				if err := m.opts.Validate(val); err != nil {
					m.err = err
					return m, nil
				}
			}
			m.result = Result[string]{Value: val, Action: ActionConfirm}
			m.done = true
			return m, tea.Quit
		case key.Matches(k, kEsc):
			if m.opts.CanGoBack {
				m.result = Result[string]{Value: m.ti.Value(), Action: ActionBack}
				m.done = true
				return m, tea.Quit
			}
		case key.Matches(k, kQuit):
			m.result = Result[string]{Action: ActionCancel}
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	m.err = nil
	return m, cmd
}

func (m *inputModel) View() string {
	if m.done {
		ans := m.result.Value
		if m.opts.Mask && ans != "" {
			ans = strings.Repeat("•", len(ans))
		}
		if m.result.IsBack() {
			ans = "(back)"
		} else if m.result.IsCancel() {
			ans = "(cancelled)"
		} else if ans == "" {
			ans = descStyle.Render("(empty)")
		}
		return renderAnsweredPrompt(m.opts.Header, m.prompt, ans)
	}

	var parts []string
	if h := strings.TrimRight(m.opts.Header, "\n"); h != "" {
		parts = append(parts, h, "")
	}

	parts = append(parts, qMark.String()+" "+promptStyle.Render(m.prompt))
	parts = append(parts, "  "+valueStyle.Render("›")+" "+m.ti.View())

	if m.err != nil {
		parts = append(parts, errorStyle.Render("  ✗ "+m.err.Error()))
	}

	help := helpLine([2]string{"↵", "confirm"})
	if m.opts.CanGoBack {
		help += "  " + helpLine([2]string{"esc", "back"})
	}
	help += "  " + helpLine([2]string{"^c", "quit"})
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ============================================================================
// Select
// ============================================================================

// SelectItem is a single row in a Select prompt.
type SelectItem[T any] struct {
	Title       string
	Description string
	Value       T
}

// SelectOptions tunes a Select prompt.
type SelectOptions struct {
	Header    string
	Initial   any  // initial selection value (matched via fmt %v)
	PageSize  int  // visible items, default 8
	CanGoBack bool // allow Esc/← to go back
}

// Select shows a one-of selection list.
func Select[T any](prompt string, items []SelectItem[T], opts ...SelectOptions) (Result[T], error) {
	var opt SelectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.PageSize <= 0 {
		opt.PageSize = 8
	}
	if len(items) == 0 {
		return Result[T]{Action: ActionCancel}, fmt.Errorf("no items to select")
	}

	visible := make([]int, len(items))
	for i := range items {
		visible[i] = i
	}

	cur := 0
	if opt.Initial != nil {
		want := fmt.Sprintf("%v", opt.Initial)
		for i, it := range items {
			if fmt.Sprintf("%v", it.Value) == want {
				cur = i
				break
			}
		}
	}

	m := &selectModel[T]{prompt: prompt, items: items, opts: opt, visible: visible, cursor: cur}
	out, err := run(m)
	if err != nil {
		var zero T
		return Result[T]{Value: zero, Action: ActionCancel}, err
	}
	return out.(*selectModel[T]).result, nil
}

type selectModel[T any] struct {
	prompt   string
	items    []SelectItem[T] // all items (never mutated)
	opts     SelectOptions
	filter   string
	visible  []int  // indices into items that pass the current filter
	cursor   int    // index into visible
	offset   int
	done     bool
	result   Result[T]
	selTitle string // title of the confirmed item, captured on Enter
}

func (m *selectModel[T]) Init() tea.Cmd { return nil }

func (m *selectModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.Type {
	case tea.KeyCtrlC:
		var zero T
		m.result = Result[T]{Value: zero, Action: ActionCancel}
		m.done = true
		return m, tea.Quit

	case tea.KeyEsc:
		if m.filter != "" {
			m.filter = ""
			m.refilter()
			return m, nil
		}
		if m.opts.CanGoBack {
			var zero T
			m.result = Result[T]{Value: zero, Action: ActionBack}
			m.done = true
			return m, tea.Quit
		}

	case tea.KeyEnter:
		if len(m.visible) == 0 {
			return m, nil
		}
		idx := m.visible[m.cursor]
		m.selTitle = m.items[idx].Title
		m.result = Result[T]{Value: m.items[idx].Value, Action: ActionConfirm}
		m.done = true
		return m, tea.Quit

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			m.scroll()
		}
	case tea.KeyDown:
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			m.scroll()
		}
	case tea.KeyPgUp:
		m.cursor -= m.opts.PageSize
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.scroll()
	case tea.KeyPgDown:
		m.cursor += m.opts.PageSize
		if m.cursor >= len(m.visible) {
			m.cursor = max(0, len(m.visible)-1)
		}
		m.scroll()
	case tea.KeyHome:
		m.cursor = 0
		m.scroll()
	case tea.KeyEnd:
		m.cursor = max(0, len(m.visible)-1)
		m.scroll()

	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.refilter()
		}

	case tea.KeyRunes:
		m.filter += string(k.Runes)
		m.cursor = 0
		m.refilter()

	case tea.KeySpace:
		m.filter += " "
		m.refilter()
	}
	return m, nil
}

// refilter recomputes visible from the current filter and clamps the cursor.
func (m *selectModel[T]) refilter() {
	m.visible = m.visible[:0]
	q := strings.ToLower(m.filter)
	for i, it := range m.items {
		if q == "" ||
			strings.Contains(strings.ToLower(it.Title), q) ||
			strings.Contains(strings.ToLower(it.Description), q) {
			m.visible = append(m.visible, i)
		}
	}
	if len(m.visible) == 0 {
		m.cursor = 0
	} else if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.offset = 0
}

func (m *selectModel[T]) scroll() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+m.opts.PageSize {
		m.offset = m.cursor - m.opts.PageSize + 1
	}
}

func (m *selectModel[T]) View() string {
	if m.done {
		var ans string
		switch {
		case m.result.IsBack():
			ans = "(back)"
		case m.result.IsCancel():
			ans = "(cancelled)"
		default:
			ans = m.selTitle
		}
		return renderAnsweredPrompt(m.opts.Header, m.prompt, ans)
	}

	var parts []string
	if h := strings.TrimRight(m.opts.Header, "\n"); h != "" {
		parts = append(parts, h, "")
	}
	parts = append(parts, qMark.String()+" "+promptStyle.Render(m.prompt))

	if len(m.visible) == 0 {
		parts = append(parts, descStyle.Render("  (no matches — backspace to clear)"))
	} else {
		end := m.offset + m.opts.PageSize
		if end > len(m.visible) {
			end = len(m.visible)
		}
		for i := m.offset; i < end; i++ {
			it := m.items[m.visible[i]]
			var line string
			if i == m.cursor {
				line = "  " + cursorStr + " " + itemSelStyle.Render(it.Title)
				if it.Description != "" {
					line += "  " + descSelStyle.Render(it.Description)
				}
			} else {
				line = "  " + noCursorStr + " " + itemStyle.Render(it.Title)
				if it.Description != "" {
					line += "  " + descStyle.Render(it.Description)
				}
			}
			parts = append(parts, line)
		}
	}

	// filter bar / pagination counter
	if m.filter != "" {
		bar := valueStyle.Render("  / ") + itemSelStyle.Render(m.filter) + valueStyle.Render("▋")
		if len(m.visible) < len(m.items) {
			bar += "  " + descStyle.Render(fmt.Sprintf("%d/%d", len(m.visible), len(m.items)))
		}
		parts = append(parts, bar)
	} else if len(m.items) > m.opts.PageSize {
		pos := 0
		if len(m.visible) > 0 {
			pos = m.cursor + 1
		}
		parts = append(parts, descStyle.Render(fmt.Sprintf("  %d/%d", pos, len(m.visible))))
	}

	help := helpLine([2]string{"↑/↓", "move"}, [2]string{"↵", "select"})
	if m.filter != "" {
		help += "  " + helpLine([2]string{"esc", "clear filter"})
	} else if m.opts.CanGoBack {
		help += "  " + helpLine([2]string{"esc", "back"})
	}
	help += "  " + helpLine([2]string{"^c", "quit"})
	if m.filter == "" {
		help += "  " + helpLine([2]string{"type", "filter"})
	}
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ============================================================================
// MultiSelect
// ============================================================================

// MultiSelectItem is a single row in a MultiSelect prompt.
type MultiSelectItem[T any] struct {
	Title       string
	Description string
	Value       T
	Selected    bool
}

// MultiSelectOptions tunes a MultiSelect prompt.
type MultiSelectOptions struct {
	Header    string
	Initial   map[any]bool
	PageSize  int
	CanGoBack bool
}

// MultiSelect shows a many-of selection list.
func MultiSelect[T any](prompt string, items []MultiSelectItem[T], opts ...MultiSelectOptions) (Result[[]T], error) {
	var opt MultiSelectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.PageSize <= 0 {
		opt.PageSize = 8
	}

	for i := range items {
		if opt.Initial != nil {
			if v, ok := opt.Initial[fmt.Sprintf("%v", items[i].Value)]; ok {
				items[i].Selected = v
			}
		}
	}

	m := &multiModel[T]{prompt: prompt, items: items, opts: opt}
	out, err := run(m)
	if err != nil {
		return Result[[]T]{Action: ActionCancel}, err
	}
	return out.(*multiModel[T]).result, nil
}

type multiModel[T any] struct {
	prompt string
	items  []MultiSelectItem[T]
	opts   MultiSelectOptions
	cursor int
	offset int
	done   bool
	result Result[[]T]
}

func (m *multiModel[T]) Init() tea.Cmd { return nil }

func (m *multiModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(k, kUp):
			if m.cursor > 0 {
				m.cursor--
			}
			m.scroll()
		case key.Matches(k, kDown):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
			m.scroll()
		case key.Matches(k, kSpace):
			m.items[m.cursor].Selected = !m.items[m.cursor].Selected
		case key.Matches(k, kSelectAll):
			for i := range m.items {
				m.items[i].Selected = true
			}
		case key.Matches(k, kSelectNone):
			for i := range m.items {
				m.items[i].Selected = false
			}
		case key.Matches(k, kEnter):
			var sel []T
			for _, it := range m.items {
				if it.Selected {
					sel = append(sel, it.Value)
				}
			}
			m.result = Result[[]T]{Value: sel, Action: ActionConfirm}
			m.done = true
			return m, tea.Quit
		case key.Matches(k, kBack):
			if m.opts.CanGoBack {
				m.result = Result[[]T]{Action: ActionBack}
				m.done = true
				return m, tea.Quit
			}
		case key.Matches(k, kQuit):
			m.result = Result[[]T]{Action: ActionCancel}
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *multiModel[T]) scroll() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+m.opts.PageSize {
		m.offset = m.cursor - m.opts.PageSize + 1
	}
}

func (m *multiModel[T]) View() string {
	selectedCount := 0
	for _, it := range m.items {
		if it.Selected {
			selectedCount++
		}
	}

	if m.done {
		var ans string
		switch {
		case m.result.IsBack():
			ans = "(back)"
		case m.result.IsCancel():
			ans = "(cancelled)"
		default:
			ans = fmt.Sprintf("%d selected", selectedCount)
		}
		return renderAnsweredPrompt(m.opts.Header, m.prompt, ans)
	}

	var parts []string
	if h := strings.TrimRight(m.opts.Header, "\n"); h != "" {
		parts = append(parts, h, "")
	}
	parts = append(parts, qMark.String()+" "+promptStyle.Render(m.prompt))

	end := m.offset + m.opts.PageSize
	if end > len(m.items) {
		end = len(m.items)
	}

	for i := m.offset; i < end; i++ {
		it := m.items[i]
		check := checkOff
		if it.Selected {
			check = checkOn
		}
		var line string
		if i == m.cursor {
			line = "  " + cursorStr + " " + check + " " + itemSelStyle.Render(it.Title)
			if it.Description != "" {
				line += "  " + descSelStyle.Render(it.Description)
			}
		} else {
			line = "  " + noCursorStr + " " + check + " " + itemStyle.Render(it.Title)
			if it.Description != "" {
				line += "  " + descStyle.Render(it.Description)
			}
		}
		parts = append(parts, line)
	}

	footer := descStyle.Render(fmt.Sprintf("  %d/%d selected", selectedCount, len(m.items)))
	parts = append(parts, footer)

	help := helpLine(
		[2]string{"↑/↓", "navigate"},
		[2]string{"space", "toggle"},
		[2]string{"a/x", "all/none"},
		[2]string{"↵", "confirm"},
	)
	if m.opts.CanGoBack {
		help += "  " + helpLine([2]string{"esc", "back"})
	}
	help += "  " + helpLine([2]string{"^c", "quit"})
	parts = append(parts, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// ============================================================================
// Pause
// ============================================================================

// Pause renders a "press any key to continue" footer and blocks until a key
// is pressed. Use after List / Show / Refresh style operations so the
// printed output stays on screen instead of being pushed up by the next
// menu render. Errors are swallowed — Pause is best-effort UX glue, not
// a load-bearing prompt.
func Pause(message string) {
	if message == "" {
		message = "Press any key to continue..."
	}
	m := &pauseModel{message: message}
	_, _ = run(m)
}

type pauseModel struct {
	message string
	done    bool
}

func (m *pauseModel) Init() tea.Cmd { return nil }

func (m *pauseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *pauseModel) View() string {
	if m.done {
		return ""
	}
	return helpStyle.Render(m.message)
}

// ---------- helpers ----------

// renderAnsweredPrompt is shown after a prompt is dismissed - leaves a clean
// one-line "Q: A" trace in the scrollback so prior decisions are visible while
// the next prompt renders.
func renderAnsweredPrompt(header, prompt, answer string) string {
	tick := successStyle.Render("✓")
	line := tick + " " + promptStyle.Render(prompt) + " " + valueStyle.Render(answer)
	if h := strings.TrimRight(header, "\n"); h != "" {
		return h + "\n" + line + "\n"
	}
	return line + "\n"
}

// ---------- key bindings ----------

var (
	kEnter      = key.NewBinding(key.WithKeys("enter"))
	kEsc        = key.NewBinding(key.WithKeys("esc"))
	kQuit       = key.NewBinding(key.WithKeys("ctrl+c"))
	kBack       = key.NewBinding(key.WithKeys("esc", "left"))
	kUp         = key.NewBinding(key.WithKeys("up", "k"))
	kDown       = key.NewBinding(key.WithKeys("down", "j"))
	kLeft       = key.NewBinding(key.WithKeys("left", "h"))
	kRight      = key.NewBinding(key.WithKeys("right", "l"))
	kPgUp       = key.NewBinding(key.WithKeys("pgup"))
	kPgDown     = key.NewBinding(key.WithKeys("pgdown"))
	kHome       = key.NewBinding(key.WithKeys("home", "g"))
	kEnd        = key.NewBinding(key.WithKeys("end", "G"))
	kSpace      = key.NewBinding(key.WithKeys(" "))
	kTab        = key.NewBinding(key.WithKeys("tab"))
	kY          = key.NewBinding(key.WithKeys("y", "Y"))
	kN          = key.NewBinding(key.WithKeys("n", "N"))
	kSelectAll  = key.NewBinding(key.WithKeys("a"))
	kSelectNone = key.NewBinding(key.WithKeys("x"))
)
