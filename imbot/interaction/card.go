package interaction

// CardActionStyle represents the visual style of a card action.
type CardActionStyle string

const (
	// CardActionStyleDefault is the default neutral action style.
	CardActionStyleDefault CardActionStyle = "default"
	// CardActionStylePrimary highlights the primary action.
	CardActionStylePrimary CardActionStyle = "primary"
	// CardActionStyleDanger marks a destructive or risky action.
	CardActionStyleDanger CardActionStyle = "danger"
)

// Card represents a platform-neutral message card.
type Card struct {
	ID       string         `json:"id,omitempty"`
	Title    string         `json:"title,omitempty"`
	Text     string         `json:"text,omitempty"`
	Sections []CardSection  `json:"sections,omitempty"`
	Actions  []CardAction   `json:"actions,omitempty"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// CardSection represents a content section within a card.
type CardSection struct {
	Title  string      `json:"title,omitempty"`
	Text   string      `json:"text,omitempty"`
	Fields []CardField `json:"fields,omitempty"`
}

// CardField represents a label/value field inside a card section.
type CardField struct {
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
	Short bool   `json:"short,omitempty"`
}

// CardAction represents a platform-neutral card action.
type CardAction struct {
	ID       string          `json:"id,omitempty"`
	Label    string          `json:"label,omitempty"`
	Value    string          `json:"value,omitempty"`
	Type     ActionType      `json:"type,omitempty"`
	URL      string          `json:"url,omitempty"`
	Style    CardActionStyle `json:"style,omitempty"`
	Disabled bool            `json:"disabled,omitempty"`
	Meta     map[string]any  `json:"meta,omitempty"`
}

// CardBuilder builds platform-neutral cards with a fluent API.
type CardBuilder struct {
	card Card
}

// NewCard creates a new card builder.
func NewCard(id string) *CardBuilder {
	return NewCardBuilder(id)
}

// NewCardBuilder creates a new card builder.
func NewCardBuilder(id string) *CardBuilder {
	return &CardBuilder{
		card: Card{
			ID:       id,
			Sections: make([]CardSection, 0),
			Actions:  make([]CardAction, 0),
			Meta:     make(map[string]any),
		},
	}
}

// WithTitle sets the card title.
func (b *CardBuilder) WithTitle(title string) *CardBuilder {
	b.card.Title = title
	return b
}

// WithText sets the card main text.
func (b *CardBuilder) WithText(text string) *CardBuilder {
	b.card.Text = text
	return b
}

// WithMeta sets a metadata value on the card.
func (b *CardBuilder) WithMeta(key string, value any) *CardBuilder {
	if b.card.Meta == nil {
		b.card.Meta = make(map[string]any)
	}
	b.card.Meta[key] = value
	return b
}

// AddSection adds a pre-built section to the card.
func (b *CardBuilder) AddSection(section CardSection) *CardBuilder {
	b.card.Sections = append(b.card.Sections, section)
	return b
}

// AddSectionBuilder adds a built section to the card.
func (b *CardBuilder) AddSectionBuilder(section *CardSectionBuilder) *CardBuilder {
	if section == nil {
		return b
	}
	return b.AddSection(section.Build())
}

// AddAction adds a pre-built action to the card.
func (b *CardBuilder) AddAction(action CardAction) *CardBuilder {
	b.card.Actions = append(b.card.Actions, action)
	return b
}

// AddActionBuilder adds a built action to the card.
func (b *CardBuilder) AddActionBuilder(action *CardActionBuilder) *CardBuilder {
	if action == nil {
		return b
	}
	return b.AddAction(action.Build())
}

// AddActions adds multiple actions to the card.
func (b *CardBuilder) AddActions(actions ...CardAction) *CardBuilder {
	b.card.Actions = append(b.card.Actions, actions...)
	return b
}

// ClearSections removes all sections from the card.
func (b *CardBuilder) ClearSections() *CardBuilder {
	b.card.Sections = make([]CardSection, 0)
	return b
}

// ClearActions removes all actions from the card.
func (b *CardBuilder) ClearActions() *CardBuilder {
	b.card.Actions = make([]CardAction, 0)
	return b
}

// Build returns the card.
func (b *CardBuilder) Build() Card {
	return b.card
}

// BuildPtr returns the card as a pointer.
func (b *CardBuilder) BuildPtr() *Card {
	card := b.Build()
	return &card
}

// CardSectionBuilder builds card sections with a fluent API.
type CardSectionBuilder struct {
	section CardSection
}

// NewCardSection creates a new card section builder.
func NewCardSection() *CardSectionBuilder {
	return &CardSectionBuilder{
		section: CardSection{
			Fields: make([]CardField, 0),
		},
	}
}

// WithTitle sets the section title.
func (b *CardSectionBuilder) WithTitle(title string) *CardSectionBuilder {
	b.section.Title = title
	return b
}

// WithText sets the section text.
func (b *CardSectionBuilder) WithText(text string) *CardSectionBuilder {
	b.section.Text = text
	return b
}

// AddField adds a full-width field to the section.
func (b *CardSectionBuilder) AddField(label, value string) *CardSectionBuilder {
	b.section.Fields = append(b.section.Fields, CardField{
		Label: label,
		Value: value,
	})
	return b
}

// AddShortField adds a compact field to the section.
func (b *CardSectionBuilder) AddShortField(label, value string) *CardSectionBuilder {
	b.section.Fields = append(b.section.Fields, CardField{
		Label: label,
		Value: value,
		Short: true,
	})
	return b
}

// Build returns the section.
func (b *CardSectionBuilder) Build() CardSection {
	return b.section
}

// CardActionBuilder builds card actions with a fluent API.
type CardActionBuilder struct {
	action CardAction
}

// NewCardAction creates a new card action builder.
func NewCardAction(id, label string) *CardActionBuilder {
	return &CardActionBuilder{
		action: CardAction{
			ID:    id,
			Label: label,
			Type:  ActionCustom,
			Style: CardActionStyleDefault,
			Meta:  make(map[string]any),
		},
	}
}

// CallbackCardAction creates a callback-style card action builder.
func CallbackCardAction(id, label, value string) *CardActionBuilder {
	return NewCardAction(id, label).WithValue(value)
}

// URLCardAction creates a URL card action builder.
func URLCardAction(id, label, url string) *CardActionBuilder {
	return NewCardAction(id, label).WithURL(url)
}

// WithValue sets the action value.
func (b *CardActionBuilder) WithValue(value string) *CardActionBuilder {
	b.action.Value = value
	return b
}

// WithType sets the action type.
func (b *CardActionBuilder) WithType(actionType ActionType) *CardActionBuilder {
	b.action.Type = actionType
	return b
}

// WithURL sets the action URL.
func (b *CardActionBuilder) WithURL(url string) *CardActionBuilder {
	b.action.URL = url
	return b
}

// WithStyle sets the action style.
func (b *CardActionBuilder) WithStyle(style CardActionStyle) *CardActionBuilder {
	b.action.Style = style
	return b
}

// WithDisabled sets whether the action is disabled.
func (b *CardActionBuilder) WithDisabled(disabled bool) *CardActionBuilder {
	b.action.Disabled = disabled
	return b
}

// WithMeta sets a metadata value on the action.
func (b *CardActionBuilder) WithMeta(key string, value any) *CardActionBuilder {
	if b.action.Meta == nil {
		b.action.Meta = make(map[string]any)
	}
	b.action.Meta[key] = value
	return b
}

// Build returns the action.
func (b *CardActionBuilder) Build() CardAction {
	return b.action
}

// ToInteractions converts card actions to platform-neutral interactions.
func (c Card) ToInteractions() []Interaction {
	interactions := make([]Interaction, 0, len(c.Actions))
	for _, action := range c.Actions {
		interactions = append(interactions, Interaction{
			ID:    action.ID,
			Type:  action.Type,
			Label: action.Label,
			Value: action.Value,
			Meta:  cloneCardMeta(action.Meta),
		})
	}
	return interactions
}

func cloneCardMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	cloned := make(map[string]any, len(meta))
	for key, value := range meta {
		cloned[key] = value
	}
	return cloned
}
