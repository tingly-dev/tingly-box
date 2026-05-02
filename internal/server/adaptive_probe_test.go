package server

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/stretchr/testify/assert"
)

// TestDeterminePreferredEndpoint tests the logic for choosing between
// chat and responses endpoints based on availability and streaming support.
// This verifies the Chat-first priority strategy.
func TestDeterminePreferredEndpoint(t *testing.T) {
	ap := &AdaptiveProbe{}

	tests := []struct {
		name      string
		chat      *EndpointStatus
		responses *EndpointStatus
		want      string
	}{
		{
			name: "Chat with streaming - highest priority",
			chat: &EndpointStatus{
				Available:      true,
				SupportsStream: true,
				LatencyMs:      100,
			},
			responses: &EndpointStatus{
				Available:      true,
				SupportsStream: true,
				LatencyMs:      50,
			},
			want: string(db.EndpointTypeChat), // Chat preferred even if slower
		},
		{
			name: "Responses with streaming (Chat no streaming)",
			chat: &EndpointStatus{
				Available:      true,
				SupportsStream: false,
				LatencyMs:      100,
			},
			responses: &EndpointStatus{
				Available:      true,
				SupportsStream: true,
				LatencyMs:      50,
			},
			want: string(db.EndpointTypeResponses),
		},
		{
			name: "Chat without streaming",
			chat: &EndpointStatus{
				Available:      true,
				SupportsStream: false,
				LatencyMs:      100,
			},
			responses: &EndpointStatus{
				Available:      false,
				SupportsStream: false,
			},
			want: string(db.EndpointTypeChat),
		},
		{
			name: "Responses without streaming (last resort)",
			chat: &EndpointStatus{
				Available: false,
			},
			responses: &EndpointStatus{
				Available:      true,
				SupportsStream: false,
				LatencyMs:      200,
			},
			want: string(db.EndpointTypeResponses),
		},
		{
			name:      "Neither available - default to chat",
			chat:      &EndpointStatus{Available: false},
			responses: &EndpointStatus{Available: false},
			want:      string(db.EndpointTypeChat),
		},
		{
			name:  "Chat streaming, Responses not available",
			chat: &EndpointStatus{
				Available:      true,
				SupportsStream: true,
			},
			responses: &EndpointStatus{
				Available: false,
			},
			want: string(db.EndpointTypeChat),
		},
		{
			name: "Responses streaming, Chat not available",
			chat: &EndpointStatus{
				Available: false,
			},
			responses: &EndpointStatus{
				Available:      true,
				SupportsStream: true,
			},
			want: string(db.EndpointTypeResponses),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ap.determinePreferredEndpoint(tt.chat, tt.responses)
			assert.Equal(t, tt.want, got, "determinePreferredEndpoint() should return correct endpoint")
		})
	}
}

// TestDeterminePreferredEndpoint_PriorityOrder verifies the complete
// priority order of endpoint selection.
func TestDeterminePreferredEndpoint_PriorityOrder(t *testing.T) {
	ap := &AdaptiveProbe{}

	// Priority 1: Chat with streaming (most stable)
	t.Run("Priority1_ChatWithStreaming", func(t *testing.T) {
		chat := &EndpointStatus{Available: true, SupportsStream: true}
		responses := &EndpointStatus{Available: true, SupportsStream: true}
		assert.Equal(t, "chat", ap.determinePreferredEndpoint(chat, responses))
	})

	// Priority 2: Responses with streaming
	t.Run("Priority2_ResponsesWithStreaming", func(t *testing.T) {
		chat := &EndpointStatus{Available: true, SupportsStream: false}
		responses := &EndpointStatus{Available: true, SupportsStream: true}
		assert.Equal(t, "responses", ap.determinePreferredEndpoint(chat, responses))
	})

	// Priority 3: Chat without streaming
	t.Run("Priority3_ChatWithoutStreaming", func(t *testing.T) {
		chat := &EndpointStatus{Available: true, SupportsStream: false}
		responses := &EndpointStatus{Available: true, SupportsStream: false}
		assert.Equal(t, "chat", ap.determinePreferredEndpoint(chat, responses))
	})

	// Priority 4: Responses without streaming
	t.Run("Priority4_ResponsesWithoutStreaming", func(t *testing.T) {
		chat := &EndpointStatus{Available: false}
		responses := &EndpointStatus{Available: true, SupportsStream: false}
		assert.Equal(t, "responses", ap.determinePreferredEndpoint(chat, responses))
	})

	// Priority 5: Default to chat
	t.Run("Priority5_DefaultToChat", func(t *testing.T) {
		chat := &EndpointStatus{Available: false}
		responses := &EndpointStatus{Available: false}
		assert.Equal(t, "chat", ap.determinePreferredEndpoint(chat, responses))
	})
}

// TestEndpointStatus_IsAvailable tests the helper methods
func TestEndpointStatus_IsAvailable(t *testing.T) {
	t.Run("Available endpoint", func(t *testing.T) {
		status := EndpointStatus{
			Available:      true,
			SupportsStream: true,
			LatencyMs:      100,
			LastChecked:    time.Now(),
		}
		assert.True(t, status.Available)
		assert.True(t, status.SupportsStream)
	})

	t.Run("Unavailable endpoint", func(t *testing.T) {
		status := EndpointStatus{
			Available:      false,
			SupportsStream: false,
			ErrorMessage:   "Connection refused",
			LastChecked:    time.Now(),
		}
		assert.False(t, status.Available)
		assert.False(t, status.SupportsStream)
		assert.NotEmpty(t, status.ErrorMessage)
	})
}
