package core

// Actions are the platform-neutral way to attach interactive controls to an
// outbound message. They replace the previous convention of pre-rendering a
// platform payload and passing it through SendMessageOptions.Metadata, which
// forced every caller to know which platform it was talking to — and silently
// produced button-less messages when it guessed wrong.
//
// The division of labour is: the caller describes intent (a labelled action
// that carries some callback data), the platform decides presentation
// (Telegram inline keyboard, Feishu card buttons, numbered text, ...).
//
// Not every capability should be neutral. Three tiers:
//
//   - Tier 1, universal: a labelled action with callback data, or a link.
//     Every platform can express it. That is Action's own fields.
//   - Tier 2, capability-gated: something several platforms can do in
//     different ways (opening a mini app). Expressed as an ActionKind; the
//     platform picks the implementation, and Fallback says what happens where
//     it does not exist.
//   - Tier 3, platform-only: capabilities worth no abstraction at all
//     (Telegram's pay or switch_inline_query). These do NOT get neutral
//     fields; they travel in Ext, populated by constructors in the platform
//     package, so the caller's import declares the intent and it stays
//     greppable. See imbot/platform/telegram for examples.
//
// The rule that makes this work: a platform that cannot render an action must
// apply Fallback rather than dropping it silently.

// ActionKind is the neutral semantic of an action. The zero value is a plain
// callback action, which is what nearly every action is.
type ActionKind string

const (
	// ActionCallback emits a callback to the bot carrying CallbackData.
	ActionCallback ActionKind = ""
	// ActionOpenURL opens URL in the user's browser.
	ActionOpenURL ActionKind = "open_url"
	// ActionOpenMiniApp opens an embedded mini app at URL. Platforms without
	// an embedded webview apply Fallback (FallbackAsURL is the sensible one).
	ActionOpenMiniApp ActionKind = "open_mini_app"
)

// FallbackPolicy declares what a platform does with an action it cannot
// render. Declaring this is mandatory in spirit: the zero value drops the
// action, so a caller that cares must say so.
type FallbackPolicy string

const (
	// FallbackDrop omits the action. The zero value, and correct for actions
	// that are pure convenience.
	FallbackDrop FallbackPolicy = ""
	// FallbackAsURL degrades to a plain link action when URL is set.
	FallbackAsURL FallbackPolicy = "as_url"
)

// Only the policies a platform actually honours are defined here. An
// "append it to the body as numbered text" policy would be useful for
// platforms with no interactive support, but nothing implements it yet — and a
// policy that silently does nothing is precisely the failure this seam exists
// to remove. Add it together with its first implementation.

// Action is one message-scoped interactive control.
//
// "Message-scoped" is deliberate: Telegram also has a chat-scoped reply
// keyboard, which is a different concept with a different lifetime. Do not
// conflate the two under one name.
type Action struct {
	// ID is a stable identifier for the action, used for logging and for
	// platforms whose button payloads carry a separate id field.
	ID string
	// Label is the user-visible button text.
	Label string
	// Payload is what comes back when the action fires: ordered segments, with
	// each platform free to deliver them however it can. See payload.go.
	Payload Payload
	// CallbackData is the flat colon-joined form of Payload.
	//
	// Deprecated: this is Telegram's wire encoding, not an identity — see the
	// Payload doc comment for what that cost. Producers inside imbot's own
	// interaction and menu packages still build it, so renderers accept it and
	// parse it into segments. New code sets Payload.
	CallbackData string
	// URL is the link opened by ActionOpenURL / ActionOpenMiniApp.
	URL string
	// Kind is the neutral semantic; zero value means a callback action.
	Kind ActionKind
	// Ext carries Tier 3, platform-specific action data. Only the platform
	// named by the key reads its entry; the others apply Fallback. Populate it
	// through the platform package's constructors rather than by hand.
	Ext map[Platform]any
	// Fallback declares the behaviour on platforms that cannot render this
	// action.
	Fallback FallbackPolicy
}

// ExtFor returns this action's Tier 3 payload for a platform, if any.
func (a Action) ExtFor(platform Platform) (any, bool) {
	if a.Ext == nil {
		return nil, false
	}
	v, ok := a.Ext[platform]
	return v, ok
}

// EffectivePayload returns the action's payload, parsing the deprecated
// CallbackData when that is all the producer supplied. Renderers call this
// rather than reading either field directly, so the compatibility path lives
// in one place and disappears in one edit.
func (a Action) EffectivePayload() Payload {
	if len(a.Payload) > 0 {
		return a.Payload
	}
	return PayloadFromCallbackData(a.CallbackData)
}

// IsLink reports whether the action should be rendered as a link rather than
// as a callback control.
func (a Action) IsLink() bool {
	return a.URL != "" && (a.Kind == ActionOpenURL || a.EffectivePayload().IsEmpty())
}

// ActionSet is a laid-out group of message-scoped actions. Rows are preserved
// because layout is meaningful — the directory browser depends on it — and a
// platform that has no notion of rows can always flatten.
type ActionSet struct {
	Rows [][]Action
}

// NewActionSet creates an empty action set.
func NewActionSet() *ActionSet {
	return &ActionSet{}
}

// AddRow appends a row of actions. Empty rows are ignored.
func (s *ActionSet) AddRow(actions ...Action) *ActionSet {
	if len(actions) == 0 {
		return s
	}
	s.Rows = append(s.Rows, actions)
	return s
}

// IsEmpty reports whether the set carries no actions at all.
func (s *ActionSet) IsEmpty() bool {
	if s == nil {
		return true
	}
	for _, row := range s.Rows {
		if len(row) > 0 {
			return false
		}
	}
	return true
}

// Flatten returns every action in row order, for platforms with no row concept.
func (s *ActionSet) Flatten() []Action {
	if s == nil {
		return nil
	}
	var out []Action
	for _, row := range s.Rows {
		out = append(out, row...)
	}
	return out
}
