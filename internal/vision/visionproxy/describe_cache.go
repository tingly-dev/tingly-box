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
// .design/vision-proxy.md §10 for the sizing rationale. No env override
// (YAGNI): raise this const directly if a real workload needs more headroom.
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
// remote-URL image. The URL is hashed rather than stored verbatim so the
// key column stays fixed-width whatever the URL's length (a presigned URL
// can run to kilobytes), with a prefix keeping the b64/url namespaces
// disjoint. The URL text is the identity: a URL that changes per request
// (rotating signature or expiry in the query string) is a new image every
// time, and is re-described — see .design/vision-proxy.md §10.4.
func hashURLImage(remoteURL string) string {
	h := sha256.Sum256([]byte(remoteURL))
	return "url:" + hex.EncodeToString(h[:])
}

// describeCache is a two-tier cache from visionCacheKey to the
// already-formatted replacement text:
//
//   - a fixed-capacity in-memory LRU, the hot tier every lookup hits first;
//   - an optional DescribeStore (SQLite, see describe_store.go), the durable
//     tier consulted on a memory miss and written through on every put.
//
// The store is what makes prefix stability survive a restart: a session
// with a dozen screenshots must not re-describe all of them — and hand the
// downstream model a dozen freshly worded (hence different) descriptions —
// just because tingly-box was restarted or the LRU turned over. With no
// store the cache degrades to memory-only, which is what tests use.
type describeCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List // front = most recently used
	items    map[visionCacheKey]*list.Element
	store    DescribeStore // nil = memory-only
}

type describeCacheEntry struct {
	key  visionCacheKey
	text string
}

// newDescribeCache builds a memory-only LRU cache bounded to capacity
// entries. A non-positive capacity makes the memory tier always miss and
// never retain (degrades to "no cache" rather than panicking or growing
// unbounded).
func newDescribeCache(capacity int) *describeCache {
	return newDescribeCacheWithStore(capacity, nil)
}

// newDescribeCacheWithStore builds the two-tier cache: the memory LRU in
// front of store. A nil store is memory-only.
func newDescribeCacheWithStore(capacity int, store DescribeStore) *describeCache {
	return &describeCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[visionCacheKey]*list.Element),
		store:    store,
	}
}

// get looks up key: memory first (marking it most-recently-used on a hit),
// then the store. A store hit is promoted into memory so the next lookup —
// the same historical image on the next turn — never touches the database.
func (c *describeCache) get(key visionCacheKey) (string, bool) {
	if c == nil {
		return "", false
	}
	if text, ok := c.getMemory(key); ok {
		return text, true
	}
	// Nothing is ever written under an empty service key (no usable
	// service means no describe, hence no put), so skip the store round
	// trip rather than issue one guaranteed-miss SELECT per image.
	if c.store == nil || key.provider == "" && key.model == "" {
		return "", false
	}
	text, ok := c.store.Get(key)
	if !ok {
		return "", false
	}
	c.putMemory(key, text)
	return text, true
}

func (c *describeCache) getMemory(key visionCacheKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*describeCacheEntry).text, true
}

// put writes key through both tiers: memory (evicting the least-recently-
// used entry if over capacity) and, when configured, the store.
func (c *describeCache) put(key visionCacheKey, text string) {
	if c == nil {
		return
	}
	c.putMemory(key, text)
	if c.store != nil {
		c.store.Put(key, text)
	}
}

// putMemory inserts or updates key in the memory tier only. A non-positive
// capacity makes this a no-op.
func (c *describeCache) putMemory(key visionCacheKey, text string) {
	if c.capacity <= 0 {
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
