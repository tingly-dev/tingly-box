# internal/command/tui

Interactive terminal prompts for the tingly-box CLI. A small, self-contained
package: a handful of single-shot prompts plus a generic Wizard runner that
strings them together with a shared header, breadcrumb, and help line.

---

## Structure

```
internal/command/tui/
  tui.go          core types, shared theme, run() helper
  prompts.go      Confirm, Input, Select, MultiSelect
  wizard.go       Step[S], RunWizard[S], WithSpinner[T]
  quickstart.go   Quickstart wizard — the only consumer
```

Everything is in one flat package. The package is a leaf: only
`internal/command/quickstart.go` imports it. To avoid a cycle, the
`QuickstartManager` interface is defined here; `AppManager` satisfies it
implicitly.

---

## Prompts

Each prompt is a self-contained bubbletea program. All share the same
`Header string` option — the wizard passes its rendered breadcrumb down so
the step counter and progress crumbs appear above every question inside the
same tea frame, not as a preceding `fmt.Println`.

After a prompt resolves it collapses to a single trace line that stays in the
terminal scrollback:

```
✓ Pick a provider: OpenAI
```

Users can scroll up and see every answer they gave.

### Confirm

Two visible buttons (`Yes` / `No`) that the user can toggle with `←`/`→`,
`Tab`, or `y`/`n`. `Enter` confirms the highlighted button. The current
selection is always visible, so there is no ambiguity about what pressing
`Enter` will do.

### Input

Wraps `bubbles/textinput` with an optional `Validate func(string) error`.
Validation runs on every keystroke; errors appear inline below the field.
There is no submit-and-fail cycle.

### Select

A flat list with a `❯` cursor and an instant fzf-style filter.

**Filter:** typing any printable character narrows the list in real-time. A
`filter:` hint appears above the list while active. The list is never mutated —
a `visible []int` slice (indices into the immutable `items`) is rebuilt on each
keystroke. Moving the cursor moves through `visible`.

**Esc is two-stage:** the first `Esc` clears the filter; a second `Esc` on an
empty filter triggers back navigation. This gives users a clear escape hatch
without accidentally leaving the prompt mid-search.

**No vim aliases in Select.** Navigation uses `tea.KeyMsg.Type` directly
(`KeyUp`, `KeyDown`, `KeyEnter`, `KeyEsc`) rather than `bubbles/key.Matches`.
This means `tea.KeyRunes` always feeds the filter — pressing `k` to search for
"kubernetes" works as expected and does not move the cursor up. The `k`/`j`
aliases are intentionally absent here.

### MultiSelect

Like Select but with `◉`/`○` checkboxes. `Space` toggles the item under the
cursor; `Enter` confirms the full selection. `k`/`j` vim aliases are kept
because there is no freeform filter input.

### Spinner

`WithSpinner[T](message, fn)` runs `fn` in a goroutine and renders an animated
`spinner.Points` while it blocks. On completion: `✓ message` on success,
`✗ message` on failure. The result line stays in the scrollback like other
prompt traces.

---

## Wizard

### Generic state

```go
type Step[S any] struct {
    Name    string
    Skip    func(state S) bool
    Execute func(ctx StepContext, state S) (S, StepResult, error)
}

func RunWizard[S any](title string, initial S, steps []Step[S]) (S, error)
```

Each step receives the current state, returns a (possibly mutated) copy, and
signals what the wizard should do next: `StepContinue`, `StepBack`, `StepSkip`,
`StepDone`, or `StepCancel`. The wizard carries arbitrary application state `S`
without interface boxing or type assertions.

### Back navigation

A `history []int` stack records step indices. `advance()` pushes before moving
forward; `retreat()` pops. Back always returns to wherever the user actually
came from, even when some steps were auto-skipped, with no special-casing.

### Breadcrumb

The header rendered above every prompt:

```
Tingly Box · Quickstart   Step 3/7
✓ Welcome › ✓ Credential › ❯ Provider › · API Style › · Model › · Rules › · Agent › · Done
```

`✓` done, `❯` active, `·` upcoming. Named steps tell users what is coming, not
just how far along they are.

---

## Theme

All colours are `lipgloss.AdaptiveColor` with separate light/dark values:

```go
colAccent  = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#B4BEFE"}
colSuccess = lipgloss.AdaptiveColor{Light: "#16A34A", Dark: "#A6E3A1"}
colDanger  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F38BA8"}
colText    = lipgloss.AdaptiveColor{Light: "#1F2937", Dark: "#CDD6F4"}
colMuted   = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9099B0"}
colSubtle  = lipgloss.AdaptiveColor{Light: "#9CA3AF", Dark: "#585B70"}
```

No colour is hard-coded outside `tui.go`. Every prompt renders its help footer
through the same `helpLine` function, producing a uniform `key: action` format.

---

## Quickstart flow

### Step order: Provider before API Style

Users pick a provider template first. The description of each template lists
which API styles it supports (`openai · anthropic`). Only after choosing a
provider does the API Style step appear — at that point the choice is fully in
context: the user knows which provider they picked and can see which styles it
offers.

This avoids asking users to know upfront whether a provider speaks the OpenAI
or Anthropic wire protocol — a prerequisite most users lack.

### Never auto-skip

Even when a provider template supports only one API style, the API Style step
is still shown. It presents the constraint as context ("only style supported
by X") and lets the user confirm or go back. Auto-skipping hides information
and breaks the back-navigation mental model: users wonder why a step they
passed through is unreachable via Esc.

### Skip Welcome for returning users

The Welcome step has `Skip: qsHasProviders`. Users who already have a provider
configured land directly on the Credential step — they are adding a second
provider, not being onboarded, and reading the onboarding copy again would
be confusing.

---

## Adding a step

1. Add a field to your state struct.
2. Write an `Execute` function: `func(ctx StepContext, state S) (S, StepResult, error)`.
3. Optionally write a `Skip` predicate: `func(state S) bool`.
4. Append `Step[S]{Name: "...", Execute: ..., Skip: ...}` to the slice passed
   to `RunWizard`.

The breadcrumb, step counter, and back navigation update automatically.
