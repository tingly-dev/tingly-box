package server

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestEndpointCache_GetSet(t *testing.T) {
	c := NewEndpointCache(time.Hour)

	// Miss
	if _, ok := c.Get("p1", "m1"); ok {
		t.Fatal("expected miss on empty cache")
	}

	// Set and hit
	c.Set("p1", "m1", protocol.TypeOpenAIChat)
	got, ok := c.Get("p1", "m1")
	if !ok || got != protocol.TypeOpenAIChat {
		t.Fatalf("expected chat, got %v ok=%v", got, ok)
	}

	// Different model is independent
	if _, ok := c.Get("p1", "m2"); ok {
		t.Fatal("expected miss for different model")
	}
}

func TestEndpointCache_TTLExpiry(t *testing.T) {
	c := NewEndpointCache(10 * time.Millisecond)

	c.Set("p1", "m1", protocol.TypeOpenAIResponses)
	time.Sleep(20 * time.Millisecond)

	if _, ok := c.Get("p1", "m1"); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestEndpointCache_Overwrite(t *testing.T) {
	c := NewEndpointCache(time.Hour)

	c.Set("p1", "m1", protocol.TypeOpenAIChat)
	c.Set("p1", "m1", protocol.TypeOpenAIResponses)

	got, ok := c.Get("p1", "m1")
	if !ok || got != protocol.TypeOpenAIResponses {
		t.Fatalf("expected responses after overwrite, got %v", got)
	}
}
