package interaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestInteractionBuilder_New creates a new builder
func TestInteractionBuilder_New(t *testing.T) {
	builder := NewBuilder()
	assert.NotNil(t, builder)
}

// TestInteractionBuilder_AddButton adds a button
func TestInteractionBuilder_AddButton(t *testing.T) {
	builder := NewBuilder()
	builder.AddButton("test-id", "Test Label", "test-value")
	interactions := builder.Build()

	assert.Len(t, interactions, 1)
	assert.Equal(t, "test-id", interactions[0].ID)
	assert.Equal(t, "Test Label", interactions[0].Label)
	assert.Equal(t, "test-value", interactions[0].Value)
	assert.Equal(t, ActionSelect, interactions[0].Type)
}

// TestInteractionBuilder_AddMultipleButtons adds multiple buttons
func TestInteractionBuilder_AddMultipleButtons(t *testing.T) {
	builder := NewBuilder()
	builder.AddButton("btn1", "Button 1", "val1")
	builder.AddButton("btn2", "Button 2", "val2")
	builder.AddButton("btn3", "Button 3", "val3")

	interactions := builder.Build()

	assert.Len(t, interactions, 3)

	// Verify each button
	assert.Equal(t, "btn1", interactions[0].ID)
	assert.Equal(t, "Button 1", interactions[0].Label)
	assert.Equal(t, "val1", interactions[0].Value)

	assert.Equal(t, "btn2", interactions[1].ID)
	assert.Equal(t, "btn3", interactions[2].ID)
}

// TestInteractionBuilder_AddConfirm adds yes/no buttons
func TestInteractionBuilder_AddConfirm(t *testing.T) {
	builder := NewBuilder()
	builder.AddConfirm("test-req")
	interactions := builder.Build()

	assert.Len(t, interactions, 2) // Yes and No buttons

	// Verify yes button
	assert.Equal(t, "test-req_yes", interactions[0].ID)
	assert.Equal(t, "Yes", interactions[0].Label)
	assert.Equal(t, "true", interactions[0].Value)
	assert.Equal(t, ActionConfirm, interactions[0].Type)

	// Verify no button
	assert.Equal(t, "test-req_no", interactions[1].ID)
	assert.Equal(t, "No", interactions[1].Label)
	assert.Equal(t, "false", interactions[1].Value)
}

// TestInteractionBuilder_AddAllowDeny adds allow and deny buttons
func TestInteractionBuilder_AddAllowDeny(t *testing.T) {
	builder := NewBuilder()
	builder.AddAllowDeny("test-123")
	interactions := builder.Build()

	assert.Len(t, interactions, 2)

	// Verify allow button
	assert.Equal(t, "test-123_allow", interactions[0].ID)
	assert.Equal(t, "Allow", interactions[0].Label)
	assert.Equal(t, "allow", interactions[0].Value)
	assert.Equal(t, ActionConfirm, interactions[0].Type)

	// Verify deny button
	assert.Equal(t, "test-123_deny", interactions[1].ID)
	assert.Equal(t, "Deny", interactions[1].Label)
	assert.Equal(t, "deny", interactions[1].Value)
	assert.Equal(t, ActionCancel, interactions[1].Type)
}

// TestInteractionBuilder_AddCancel adds cancel button
func TestInteractionBuilder_AddCancel(t *testing.T) {
	builder := NewBuilder()
	builder.AddCancel("test-cancel")
	interactions := builder.Build()

	assert.Len(t, interactions, 1)
	assert.Equal(t, "test-cancel_cancel", interactions[0].ID)
	assert.Equal(t, ActionCancel, interactions[0].Type)
	assert.Equal(t, "cancel", interactions[0].Value)
	assert.Equal(t, "Cancel", interactions[0].Label)
}

// TestInteractionBuilder_AddOptions adds option buttons
func TestInteractionBuilder_AddOptions(t *testing.T) {
	builder := NewBuilder()

	options := []Option{
		{Label: "Option A", Value: "a"},
		{Label: "Option B", Value: "b"},
		{Label: "Option C", Value: "c"},
	}

	builder.AddOptions("select-opt", options)
	interactions := builder.Build()

	assert.Len(t, interactions, 1) // AddOptions creates a single interaction with multiple options

	// Verify the interaction has the options
	assert.Equal(t, "select-opt", interactions[0].ID)
	assert.Equal(t, ActionSelect, interactions[0].Type)
	assert.NotNil(t, interactions[0].Options)
	assert.Len(t, interactions[0].Options, 3)

	// Verify option labels
	labels := make([]string, len(interactions[0].Options))
	for i, opt := range interactions[0].Options {
		labels[i] = opt.Label
	}
	assert.Equal(t, []string{"Option A", "Option B", "Option C"}, labels)
}

// TestInteractionBuilder_ComplexBuild builds a complex interaction set
func TestInteractionBuilder_ComplexBuild(t *testing.T) {
	builder := NewBuilder()

	// Add some options
	builder.AddOptions("choose", []Option{
		{Label: "Red", Value: "red"},
		{Label: "Blue", Value: "blue"},
	})

	// Add yes/no buttons
	builder.AddConfirm("confirm")

	// Add cancel button
	builder.AddCancel("cancel")

	interactions := builder.Build()

	assert.Len(t, interactions, 4) // 1 option group + 2 for confirm + 1 cancel = 4

	// Verify we have the expected types
	hasYes := false
	hasNo := false
	hasCancel := false
	hasOptions := false

	for _, interaction := range interactions {
		if interaction.ID == "confirm_yes" {
			hasYes = true
		}
		if interaction.ID == "confirm_no" {
			hasNo = true
		}
		if interaction.ID == "cancel_cancel" {
			hasCancel = true
		}
		if interaction.ID == "choose" {
			hasOptions = true
		}
	}

	assert.True(t, hasYes, "Should have yes button")
	assert.True(t, hasNo, "Should have no button")
	assert.True(t, hasCancel, "Should have cancel button")
	assert.True(t, hasOptions, "Should have option group")
}

// TestInteractionBuilder_Chaining tests method chaining
func TestInteractionBuilder_Chaining(t *testing.T) {
	interactions := NewBuilder().
		AddButton("btn1", "Button 1", "val1").
		AddButton("btn2", "Button 2", "val2").
		AddCancel("cancel").
		Build()

	assert.Len(t, interactions, 3)
}

// TestInteractionBuilder_Clear clears all interactions
func TestInteractionBuilder_Clear(t *testing.T) {
	builder := NewBuilder()
	builder.AddButton("btn1", "Button 1", "val1")
	assert.Equal(t, 1, builder.Count())

	builder.Clear()
	assert.Equal(t, 0, builder.Count())
	assert.True(t, builder.Empty())
}

// TestInteractionBuilder_Empty checks if builder is empty
func TestInteractionBuilder_Empty(t *testing.T) {
	builder := NewBuilder()
	assert.True(t, builder.Empty())

	builder.AddButton("btn1", "Button 1", "val1")
	assert.False(t, builder.Empty())
}

// TestInteractionResponse_IsCancel checks if response is cancel
func TestInteractionResponse_IsCancel(t *testing.T) {
	tests := []struct {
		name     string
		response InteractionResponse
		want     bool
	}{
		{
			name: "cancel action",
			response: InteractionResponse{
				Action: Interaction{Type: ActionCancel, Value: "cancel"},
			},
			want: true,
		},
		{
			name: "confirm action",
			response: InteractionResponse{
				Action: Interaction{Type: ActionConfirm, Value: "confirm"},
			},
			want: false,
		},
		{
			name: "select action",
			response: InteractionResponse{
				Action: Interaction{Type: ActionSelect, Value: "option1"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.IsCancel()
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestInteractionResponse_IsConfirm checks if response is confirm
func TestInteractionResponse_IsConfirm(t *testing.T) {
	tests := []struct {
		name     string
		response InteractionResponse
		want     bool
	}{
		{
			name: "confirm with true value",
			response: InteractionResponse{
				Action: Interaction{Type: ActionConfirm, Value: "true"},
			},
			want: true,
		},
		{
			name: "confirm with false value",
			response: InteractionResponse{
				Action: Interaction{Type: ActionConfirm, Value: "false"},
			},
			want: false,
		},
		{
			name: "confirm with confirm value",
			response: InteractionResponse{
				Action: Interaction{Type: ActionConfirm, Value: "confirm"},
			},
			want: false, // IsConfirm requires Value == "true"
		},
		{
			name: "allow is not true",
			response: InteractionResponse{
				Action: Interaction{Type: ActionConfirm, Value: "allow"},
			},
			want: false, // IsConfirm requires Value == "true"
		},
		{
			name: "cancel action",
			response: InteractionResponse{
				Action: Interaction{Type: ActionCancel, Value: "cancel"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.IsConfirm()
			assert.Equal(t, tt.want, result)
		})
	}
}
