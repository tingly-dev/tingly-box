package interaction

// Builder builds platform-agnostic interactions with a fluent API
type Builder struct {
	interactions []Interaction
}

// NewBuilder creates a new interaction builder
func NewBuilder() *Builder {
	return &Builder{
		interactions: make([]Interaction, 0),
	}
}

// AddButton adds a single button interaction
func (b *Builder) AddButton(id, label, value string) *Builder {
	b.interactions = append(b.interactions, Interaction{
		ID:    id,
		Type:  ActionSelect,
		Label: label,
		Value: value,
	})
	return b
}

// AddConfirm adds a confirm/deny pair of buttons
func (b *Builder) AddConfirm(id string) *Builder {
	b.interactions = append(b.interactions,
		Interaction{
			ID:    id + "_yes",
			Type:  ActionConfirm,
			Label: "Yes",
			Value: "true",
		},
		Interaction{
			ID:    id + "_no",
			Type:  ActionConfirm,
			Label: "No",
			Value: "false",
		},
	)
	return b
}

// AddAllowDeny adds an Allow/Deny pair of buttons (for permissions)
func (b *Builder) AddAllowDeny(id string) *Builder {
	b.interactions = append(b.interactions,
		Interaction{
			ID:    id + "_allow",
			Type:  ActionConfirm,
			Label: "Allow",
			Value: "allow",
		},
		Interaction{
			ID:    id + "_deny",
			Type:  ActionCancel,
			Label: "Deny",
			Value: "deny",
		},
	)
	return b
}

// AddOptions adds multiple selectable options
func (b *Builder) AddOptions(id string, options []Option) *Builder {
	b.interactions = append(b.interactions, Interaction{
		ID:      id,
		Type:    ActionSelect,
		Options: options,
	})
	return b
}

// AddOption adds a single selectable option
func (b *Builder) AddOption(id, label, value string) *Builder {
	b.interactions = append(b.interactions, Interaction{
		ID:    id,
		Type:  ActionSelect,
		Label: label,
		Value: value,
	})
	return b
}

// AddNavigation adds prev/next navigation buttons
func (b *Builder) AddNavigation(id string, hasPrev, hasNext bool) *Builder {
	if hasPrev {
		b.interactions = append(b.interactions, Interaction{
			ID:    id + "_prev",
			Type:  ActionNavigate,
			Label: "Previous",
			Value: "prev",
		})
	}
	if hasNext {
		b.interactions = append(b.interactions, Interaction{
			ID:    id + "_next",
			Type:  ActionNavigate,
			Label: "Next",
			Value: "next",
		})
	}
	return b
}

// AddCancel adds a cancel button
func (b *Builder) AddCancel(id string) *Builder {
	b.interactions = append(b.interactions, Interaction{
		ID:    id + "_cancel",
		Type:  ActionCancel,
		Label: "Cancel",
		Value: "cancel",
	})
	return b
}

// AddCustom adds a custom interaction
func (b *Builder) AddCustom(id, label, value string, actionType ActionType) *Builder {
	b.interactions = append(b.interactions, Interaction{
		ID:    id,
		Type:  actionType,
		Label: label,
		Value: value,
	})
	return b
}

// Add adds a pre-built interaction
func (b *Builder) Add(interaction Interaction) *Builder {
	b.interactions = append(b.interactions, interaction)
	return b
}

// AddAll adds multiple interactions
func (b *Builder) AddAll(interactions []Interaction) *Builder {
	b.interactions = append(b.interactions, interactions...)
	return b
}

// Build returns the interaction list
func (b *Builder) Build() []Interaction {
	return b.interactions
}

// Clear removes all interactions
func (b *Builder) Clear() *Builder {
	b.interactions = make([]Interaction, 0)
	return b
}

// Count returns the number of interactions
func (b *Builder) Count() int {
	return len(b.interactions)
}

// Empty returns true if there are no interactions
func (b *Builder) Empty() bool {
	return len(b.interactions) == 0
}

// Helper functions for common button labels

// YesButton creates a yes/confirm button
func YesButton(id string) Interaction {
	return Interaction{
		ID:    id + "_yes",
		Type:  ActionConfirm,
		Label: "Yes",
		Value: "true",
	}
}

// NoButton creates a no/deny button
func NoButton(id string) Interaction {
	return Interaction{
		ID:    id + "_no",
		Type:  ActionCancel,
		Label: "No",
		Value: "false",
	}
}

// AllowButton creates an allow button
func AllowButton(id string) Interaction {
	return Interaction{
		ID:    id + "_allow",
		Type:  ActionConfirm,
		Label: "Allow",
		Value: "allow",
	}
}

// DenyButton creates a deny button
func DenyButton(id string) Interaction {
	return Interaction{
		ID:    id + "_deny",
		Type:  ActionCancel,
		Label: "Deny",
		Value: "deny",
	}
}

// CancelButton creates a cancel button
func CancelButton(id string) Interaction {
	return Interaction{
		ID:    id + "_cancel",
		Type:  ActionCancel,
		Label: "Cancel",
		Value: "cancel",
	}
}

// ConfirmButton creates a confirm button
func ConfirmButton(id string) Interaction {
	return Interaction{
		ID:    id + "_confirm",
		Type:  ActionConfirm,
		Label: "Confirm",
		Value: "confirm",
	}
}
