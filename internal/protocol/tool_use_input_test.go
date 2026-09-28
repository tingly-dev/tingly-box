package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolUseInput(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		want      map[string]any
		wantOK    bool
	}{
		{"blank (tool without parameters)", "", map[string]any{}, true},
		{"whitespace", "  \n", map[string]any{}, true},
		{"json null", "null", map[string]any{}, true},
		{"empty object", "{}", map[string]any{}, true},
		{"object", `{"location":"NYC","days":3}`, map[string]any{"location": "NYC", "days": float64(3)}, true},
		{"malformed", `{"location":`, map[string]any{}, false},
		{"array", `[1,2]`, map[string]any{}, false},
		{"string", `"NYC"`, map[string]any{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ToolUseInput(tt.arguments)
			assert.Equal(t, tt.want, got)
			assert.NotNil(t, got, "input must never be nil: it would be omitted or sent as null")
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
