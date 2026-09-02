package visionproxy

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// defaultDescribeCacheCapacity bounds the number of cached descriptions kept
// in memory across all sessions. Values are small (a formatted description
// string), so this bounds entry count rather than raw bytes — see
// .sdlc/docs/vision-vision-proxy-description-cache-20260902.spec.md §3.2 for
// the sizing rationale. No env override (YAGNI): raise this const directly
// if a real workload needs more headroom.
const defaultDescribeCacheCapacity = 2000

// visionCacheKey identifies one (session, vision service, image content)
// triple. All three dimensions matter independently:
//   - session: the same bytes are only treated as "the same image" within one
//     session — a different session (different user/conversation, or even a
//     coincidental byte match) must never reuse another session's
//     description. See the spec's §1 for why this beats a pure
//     content-addressed global cache.
//   - provider+model: switching the configured vision service must silently
//     invalidate old descriptions rather than serve a different model's
//     answer under the new model's name. provider is the provider UUID
//     (loadbalance.Service.Provider / providerResolver.GetProviderByUUID),
//     not the provider's display name.
//   - content: which image, identified by hashBase64Image (base64 sources)
//     or the remote URL itself (URL sources).
type visionCacheKey struct {
	session  string
	provider string
	model    string
	content  string
}

// hashBase64Image derives the content component of a cache key for a
// base64-encoded image. It hashes the base64 text directly (no decode) —
// cheap, and two occurrences of the same base64-encoded image always hash
// identically regardless of what else changes around them. mediaType is
// folded into the hash so a (byte-identical-but-differently-labeled) source
// cannot collide with a different declared media type.
func hashBase64Image(mediaType, b64 string) string {
	h := sha256.Sum256([]byte(mediaType + "\x00" + b64))
	return "b64:" + hex.EncodeToString(h[:])
}

// hashURLImage derives the content component of a cache key for a
// remote-URL image. The URL is already a compact, stable content identifier,
// so it is used as-is (prefixed to keep the b64/url namespaces disjoint).
func hashURLImage(remoteURL string) string {
	return "url:" + remoteURL
}

// describeCache is a fixed-capacity, in-memory LRU cache from
// visionCacheKey to the already-formatted replacement text. It is
// process-local, not persisted, and not shared across instances — the cache
// exists purely to avoid re-describing byte-identical images within one
// session during this process's lifetime.
type describeCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List // front = most recently used
	items    map[visionCacheKey]*list.Element
}

type describeCacheEntry struct {
	key  visionCacheKey
	text string
}

// newDescribeCache builds an LRU cache bounded to capacity entries. A
// non-positive capacity makes get always miss and put a no-op (degrades to
// "no cache" rather than panicking or growing unbounded).
func newDescribeCache(capacity int) *describeCache {
	return &describeCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[visionCacheKey]*list.Element),
	}
}

// get looks up key, marking it most-recently-used on a hit.
func (c *describeCache) get(key visionCacheKey) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*describeCacheEntry).text, true
}

// put inserts or updates key, evicting the least-recently-used entry if the
// cache is over capacity afterward. A nil cache or non-positive capacity
// makes this a no-op.
func (c *describeCache) put(key visionCacheKey, text string) {
	if c == nil || c.capacity <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*describeCacheEntry).text = text
		c.ll.MoveToFront(el)
		return
	}
	el := c.ll.PushFront(&describeCacheEntry{key: key, text: text})
	c.items[key] = el
	if c.ll.Len() > c.capacity {
		oldest := c.ll.Back()
		if oldest != nil {
			c.ll.Remove(oldest)
			delete(c.items, oldest.Value.(*describeCacheEntry).key)
		}
	}
}
