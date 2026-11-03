package engine

import (
	"testing"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// The CLI's in-process API retries must surface as system entries, so the
// transcript shows the turn is alive (and why it is pausing) instead of
// silence until the attempt lands. Field names match the real event
// (attempt / max_retries / retry_delay_ms / error).
func TestConverter_APIRetryBecomesSystemEntry(t *testing.T) {
	c := newConverter()
	entries := c.messages(&claude.SystemMessage{
		SubType: claude.SystemSubtypeAPIRetry,
		Raw: map[string]interface{}{
			"attempt":        2,
			"max_retries":    10,
			"retry_delay_ms": 4000,
			"error":          "rate_limit",
		},
	})
	if len(entries) != 1 {
		t.Fatalf("api_retry produced %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Kind != "system" {
		t.Fatalf("entry kind = %q, want system", e.Kind)
	}
	want := "api retry attempt 2 (rate_limit), next in 4.0s"
	if e.Content != want {
		t.Fatalf("entry content = %q, want %q", e.Content, want)
	}
	if e.Payload != nil {
		t.Fatalf("entry payload = %s, want none", string(e.Payload))
	}
}

// A retry notice without reason/delay still renders its attempt number.
func TestConverter_APIRetryMinimal(t *testing.T) {
	c := newConverter()
	entries := c.messages(&claude.SystemMessage{
		SubType: claude.SystemSubtypeAPIRetry,
		Raw:     map[string]interface{}{"attempt": 1},
	})
	if len(entries) != 1 || entries[0].Content != "api retry attempt 1" {
		t.Fatalf("minimal retry entry = %+v", entries)
	}
}
