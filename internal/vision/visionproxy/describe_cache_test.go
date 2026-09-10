package visionproxy

import "testing"

func TestDescribeCache_GetPutBasic(t *testing.T) {
	c := newDescribeCache(nil)
	key := visionCacheKey{session: "s1", provider: "p1", model: "m1", content: "b64:abc"}
	if _, ok := c.get(key); ok {
		t.Fatal("expected miss on empty cache")
	}
	c.put(key, "a red image")
	text, ok := c.get(key)
	if !ok || text != "a red image" {
		t.Fatalf("expected hit with cached text, got %q ok=%v", text, ok)
	}
	c.put(key, "second")
	if text, _ := c.get(key); text != "second" {
		t.Fatalf("expected updated text, got %q", text)
	}
}

func TestDescribeCache_DistinctServiceDoesNotCollide(t *testing.T) {
	c := newDescribeCache(nil)
	k1 := visionCacheKey{session: "s1", provider: "p1", model: "m1", content: "b64:same"}
	k2 := visionCacheKey{session: "s1", provider: "p1", model: "m2", content: "b64:same"}
	c.put(k1, "from model 1")
	if _, ok := c.get(k2); ok {
		t.Fatal("expected different model to miss")
	}
}

func TestDescribeCache_DistinctSessionDoesNotCollide(t *testing.T) {
	c := newDescribeCache(nil)
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
	if _, ok := c.get(visionCacheKey{provider: "p", model: "m", content: "x"}); ok {
		t.Fatal("nil cache must always miss")
	}
	c.put(visionCacheKey{content: "x"}, "text") // must not panic
	c.markFailed(visionCacheKey{content: "x"})  // must not panic
}

func TestMemoryDescribeStore_ResetsPastCapacity(t *testing.T) {
	s := newMemoryDescribeStore()
	for i := 0; i < memoryDescribeStoreCapacity; i++ {
		s.Put(visionCacheKey{content: string(rune(i))}, "x")
	}
	first := visionCacheKey{content: string(rune(0))}
	if _, ok := s.Get(first); !ok {
		t.Fatal("entries under capacity must be retained")
	}
	s.Put(visionCacheKey{content: "overflow"}, "x")
	if _, ok := s.Get(first); ok {
		t.Fatal("past capacity the fallback map resets")
	}
	if _, ok := s.Get(visionCacheKey{content: "overflow"}); !ok {
		t.Fatal("the entry that triggered the reset is kept")
	}
}
