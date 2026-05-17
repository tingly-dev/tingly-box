package probe

import (
	"testing"
	"time"
)

func makeCapability(providerUUID, modelID string) *ModelEndpointCapability {
	return &ModelEndpointCapability{
		ProviderUUID: providerUUID,
		ModelID:      modelID,
		SupportsChat: true,
		LastVerified: time.Now(),
	}
}

func TestProbeCache_SetGet(t *testing.T) {
	c := NewProbeCache(time.Minute)
	c.Set("p1", "m1", makeCapability("p1", "m1"))

	got := c.Get("p1", "m1")
	if got == nil {
		t.Fatal("Get returned nil for known key")
	}
	if got.ProviderUUID != "p1" || got.ModelID != "m1" {
		t.Errorf("Get returned wrong identity: %+v", got)
	}
	if !got.SupportsChat {
		t.Errorf("Get did not preserve SupportsChat")
	}

	if c.Get("p1", "missing") != nil {
		t.Error("Get should return nil for unknown model")
	}
	if c.Get("missing", "m1") != nil {
		t.Error("Get should return nil for unknown provider")
	}
}

func TestProbeCache_Expiry(t *testing.T) {
	c := NewProbeCache(10 * time.Millisecond)
	c.Set("p1", "m1", makeCapability("p1", "m1"))

	if c.Get("p1", "m1") == nil {
		t.Fatal("entry should be present immediately after Set")
	}

	time.Sleep(20 * time.Millisecond)
	if c.Get("p1", "m1") != nil {
		t.Error("entry should be expired and Get should return nil")
	}
}

func TestProbeCache_TTL(t *testing.T) {
	c := NewProbeCache(42 * time.Second)
	if got := c.TTL(); got != 42*time.Second {
		t.Errorf("TTL() = %v, want 42s", got)
	}
}

func TestProbeCache_Invalidate(t *testing.T) {
	c := NewProbeCache(time.Minute)
	c.Set("p1", "m1", makeCapability("p1", "m1"))
	c.Set("p1", "m2", makeCapability("p1", "m2"))

	c.Invalidate("p1", "m1")
	if c.Get("p1", "m1") != nil {
		t.Error("Invalidate did not remove the target entry")
	}
	if c.Get("p1", "m2") == nil {
		t.Error("Invalidate removed an unrelated entry")
	}
}

func TestProbeCache_InvalidateProvider(t *testing.T) {
	c := NewProbeCache(time.Minute)
	c.Set("p1", "m1", makeCapability("p1", "m1"))
	c.Set("p1", "m2", makeCapability("p1", "m2"))
	c.Set("p2", "m1", makeCapability("p2", "m1"))

	c.InvalidateProvider("p1")
	if c.Get("p1", "m1") != nil || c.Get("p1", "m2") != nil {
		t.Error("InvalidateProvider should remove all entries for the provider")
	}
	if c.Get("p2", "m1") == nil {
		t.Error("InvalidateProvider must not touch other providers")
	}
}

func TestProbeCache_Clear(t *testing.T) {
	c := NewProbeCache(time.Minute)
	c.Set("p1", "m1", makeCapability("p1", "m1"))
	c.Set("p2", "m2", makeCapability("p2", "m2"))

	c.Clear()
	if c.Get("p1", "m1") != nil || c.Get("p2", "m2") != nil {
		t.Error("Clear should remove every entry")
	}
}

func TestProbeCache_CleanupExpired(t *testing.T) {
	c := NewProbeCache(10 * time.Millisecond)
	c.Set("p1", "m1", makeCapability("p1", "m1"))

	// Confirm the entry is internally present before cleanup.
	if got := c.Get("p1", "m1"); got == nil {
		t.Fatal("entry should be present before sleep")
	}

	time.Sleep(20 * time.Millisecond)

	// CleanupExpired removes expired entries from the internal map. Use the
	// internal cache length to verify since Get already returns nil for
	// expired-but-not-yet-cleaned entries.
	c.CleanupExpired()
	c.mu.RLock()
	remaining := len(c.cache)
	c.mu.RUnlock()
	if remaining != 0 {
		t.Errorf("CleanupExpired left %d entries, want 0", remaining)
	}
}

func TestProbeCache_SetFromProbeResult(t *testing.T) {
	c := NewProbeCache(time.Minute)
	now := time.Now()
	c.SetFromProbeResult(&ProbeResult{
		ProviderUUID: "p1",
		ModelID:      "m1",
		ChatEndpoint: EndpointStatus{
			Available:      true,
			SupportsStream: true,
			LatencyMs:      120,
		},
		ResponsesEndpoint: EndpointStatus{
			Available:    false,
			ErrorMessage: "404",
			LatencyMs:    80,
		},
		PreferredEndpoint: "chat",
		LastUpdated:       now,
	})

	got := c.Get("p1", "m1")
	if got == nil {
		t.Fatal("entry missing after SetFromProbeResult")
	}
	if !got.SupportsChat || !got.ChatSupportsStream || got.ChatLatencyMs != 120 {
		t.Errorf("chat endpoint not projected correctly: %+v", got)
	}
	if got.SupportsResponses || got.ResponsesError != "404" || got.ResponsesLatencyMs != 80 {
		t.Errorf("responses endpoint not projected correctly: %+v", got)
	}
	if got.PreferredEndpoint != "chat" {
		t.Errorf("PreferredEndpoint = %q, want chat", got.PreferredEndpoint)
	}
	if !got.LastVerified.Equal(now) {
		t.Errorf("LastVerified not preserved: got %v want %v", got.LastVerified, now)
	}
}
