package visionproxy

import "testing"

func TestDescribeCache_GetPutBasic(t *testing.T) {
	c := newDescribeCache(2)
	key := visionCacheKey{session: "s1", provider: "p1", model: "m1", content: "b64:abc"}
	if _, ok := c.get(key); ok {
		t.Fatal("expected miss on empty cache")
	}
	c.put(key, "a red image")
	text, ok := c.get(key)
	if !ok || text != "a red image" {
		t.Fatalf("expected hit with cached text, got %q ok=%v", text, ok)
	}
}

func TestDescribeCache_EvictsLeastRecentlyUsed(t *testing.T) {
	c := newDescribeCache(2)
	k1 := visionCacheKey{session: "s1", content: "b64:1"}
	k2 := visionCacheKey{session: "s1", content: "b64:2"}
	k3 := visionCacheKey{session: "s1", content: "b64:3"}

	c.put(k1, "one")
	c.put(k2, "two")
	// Touch k1 so it becomes more-recently-used than k2.
	if _, ok := c.get(k1); !ok {
		t.Fatal("expected k1 hit")
	}
	// Inserting k3 should evict k2 (least recently used), not k1.
	c.put(k3, "three")

	if _, ok := c.get(k2); ok {
		t.Fatal("expected k2 to be evicted")
	}
	if _, ok := c.get(k1); !ok {
		t.Fatal("expected k1 to survive eviction")
	}
	if _, ok := c.get(k3); !ok {
		t.Fatal("expected k3 present")
	}
}

func TestDescribeCache_PutUpdateDoesNotGrowSize(t *testing.T) {
	c := newDescribeCache(2)
	key := visionCacheKey{session: "s1", content: "b64:1"}
	c.put(key, "first")
	c.put(key, "second")
	if c.ll.Len() != 1 {
		t.Fatalf("expected 1 entry after update, got %d", c.ll.Len())
	}
	text, ok := c.get(key)
	if !ok || text != "second" {
		t.Fatalf("expected updated text, got %q ok=%v", text, ok)
	}
}

func TestDescribeCache_DistinctServiceDoesNotCollide(t *testing.T) {
	c := newDescribeCache(10)
	k1 := visionCacheKey{session: "s1", provider: "p1", model: "m1", content: "b64:same"}
	k2 := visionCacheKey{session: "s1", provider: "p1", model: "m2", content: "b64:same"}
	c.put(k1, "from model 1")
	if _, ok := c.get(k2); ok {
		t.Fatal("expected different model to miss")
	}
}

func TestDescribeCache_DistinctSessionDoesNotCollide(t *testing.T) {
	c := newDescribeCache(10)
	k1 := visionCacheKey{session: "session-a", provider: "p1", model: "m1", content: "b64:same"}
	k2 := visionCacheKey{session: "session-b", provider: "p1", model: "m1", content: "b64:same"}
	c.put(k1, "described for session a")
	if _, ok := c.get(k2); ok {
		t.Fatal("expected different session to miss even with identical image content")
	}
}

func TestDescribeCache_Base64AndURLKeysDoNotCollide(t *testing.T) {
	if hashBase64Image("image/png", "url:foo") == hashURLImage("foo") {
		t.Fatal("base64 and url content hashes must be disjoint namespaces")
	}
}

func TestDescribeCache_NilCacheIsAlwaysMiss(t *testing.T) {
	var c *describeCache
	if _, ok := c.get(visionCacheKey{content: "x"}); ok {
		t.Fatal("nil cache must always miss")
	}
	c.put(visionCacheKey{content: "x"}, "text") // must not panic
}

func TestDescribeCache_ZeroCapacityNeverCaches(t *testing.T) {
	c := newDescribeCache(0)
	key := visionCacheKey{content: "x"}
	c.put(key, "text")
	if _, ok := c.get(key); ok {
		t.Fatal("zero-capacity cache must never retain entries")
	}
}
