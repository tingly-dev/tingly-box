package visionproxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

// visionCacheKey identifies one (session, vision service, image content)
// triple. All three dimensions matter independently:
//   - session: the same bytes are only treated as "the same image" within one
//     session — a different session (different user/conversation, or even a
//     coincidental byte match) must never reuse another session's
//     description. See .design/vision-proxy.md §10.2 for why this beats a
//     pure content-addressed global cache.
//   - provider+model: switching the configured vision service must silently
//     invalidate old descriptions rather than serve a different model's
//     answer under the new model's name. provider is the provider UUID
//     (loadbalance.Service.Provider / providerResolver.GetProviderByUUID),
//     not the provider's display name.
//   - content: which image, identified by hashBase64Image (base64 sources)
//     or hashURLImage (URL sources).
type visionCacheKey struct {
	session  string
	provider string
	model    string
	content  string
}

// hashBase64Image derives the content component of a cache key for a
// base64-encoded image. It hashes the base64 text directly (no decode) —
// two occurrences of the same base64-encoded image always hash identically
// regardless of what else changes around them. mediaType is folded in so a
// byte-identical-but-differently-labeled source cannot collide with a
// different declared media type.
//
// The hash is xxhash64 plus the length, not a cryptographic digest. Every
// image a conversation carries is hashed on every request, and a 2 MB
// screenshot costs ~8 ms under SHA-256 versus well under 1 ms here — a
// 30-screenshot session was paying a quarter second per turn just to look
// itself up. Keys are scoped per session and per service, so the space a
// collision would have to happen in is a few dozen images; 64 bits plus
// the exact length is ample for that, and no adversary gains anything by
// forging a collision against their own session.
func hashBase64Image(mediaType, b64 string) string {
	h := xxhash.New()
	_, _ = h.WriteString(mediaType)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(b64)
	return fmt.Sprintf("b64:%016x-%d", h.Sum64(), len(b64))
}

// hashURLImage derives the content component of a cache key for a
// remote-URL image. The URL is hashed rather than stored verbatim so the
// key column stays fixed-width whatever the URL's length (a presigned URL
// can run to kilobytes), with a prefix keeping the b64/url namespaces
// disjoint. The URL text is the identity: a URL that changes per request
// (rotating signature or expiry in the query string) is a new image every
// time, and is re-described — see .design/vision-proxy.md §10.4.
func hashURLImage(remoteURL string) string {
	return fmt.Sprintf("url:%016x-%d", xxhash.Sum64String(remoteURL), len(remoteURL))
}

// describeCache maps visionCacheKey to the already-formatted replacement
// text. It has a single positive tier, a DescribeStore — SQLite in
// production (describe_store.go), a bounded map for tests and for the
// no-database fallback — plus a small memory-only negative cache.
//
// There is deliberately no in-memory LRU in front of the store. A request
// looks up every image its conversation carries, but a SQLite point
// lookup on the unique index costs tens of microseconds, less than hashing
// one image's base64, and far below the downstream model's latency. What
// a second tier bought in speed it cost in two sources of truth, promotion
// and write-through logic, and a third capacity number to explain; the
// store is the one place a description lives.
type describeCache struct {
	store DescribeStore

	mu sync.Mutex
	// failed is the negative cache: keys whose last describe failed, with
	// the time it failed. Memory-only and short-lived (describeFailureTTL)
	// on purpose — see recentlyFailed.
	failed map[visionCacheKey]time.Time
	now    func() time.Time
}

// describeFailureTTL is how long a failed describe is remembered. Within
// it the image is fail-stripped without an upstream call and without
// taking a describe slot. The value is a compromise between two failure
// kinds this code cannot tell apart: a transient one (rate limit, network,
// upstream timeout) should retry soon — the TTL bounds how long the image
// stays stripped once the upstream recovers; a permanent one (dead URL,
// rejected bytes) would otherwise retry every turn and, worse, hold a
// describe slot every turn, starving every older image behind it. The
// negative cache is never persisted: after a restart everything gets a
// fresh attempt.
const describeFailureTTL = 10 * time.Minute

// describeFailureCapacity bounds the negative cache; when exceeded, expired
// entries are dropped and, if still over, the whole map is reset — losing
// negative entries only costs retries.
const describeFailureCapacity = 1000

// newDescribeCache builds a cache over store. A nil store falls back to a
// bounded in-process map (newMemoryDescribeStore): descriptions then last
// only for the process, which is what tests want and what production gets
// only if the database is unavailable at boot.
func newDescribeCache(store DescribeStore) *describeCache {
	if store == nil {
		store = newMemoryDescribeStore()
	}
	return &describeCache{
		store:  store,
		failed: make(map[visionCacheKey]time.Time),
		now:    time.Now,
	}
}

// get looks key up in the store. Nothing is ever written under an empty
// service key (no usable service means no describe, hence no put), so
// that case is a miss without a round trip.
func (c *describeCache) get(key visionCacheKey) (string, bool) {
	if c == nil || key.provider == "" && key.model == "" {
		return "", false
	}
	return c.store.Get(key)
}

// put writes key to the store and forgets any recorded failure for it.
func (c *describeCache) put(key visionCacheKey, text string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.failed, key)
	c.mu.Unlock()
	c.store.Put(key, text)
}

// recentlyFailed reports whether key failed to describe within
// describeFailureTTL. Expired entries are removed on the way.
func (c *describeCache) recentlyFailed(key visionCacheKey) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	at, ok := c.failed[key]
	if !ok {
		return false
	}
	if c.now().Sub(at) >= describeFailureTTL {
		delete(c.failed, key)
		return false
	}
	return true
}

// markFailed records a failed describe for key.
func (c *describeCache) markFailed(key visionCacheKey) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.failed) >= describeFailureCapacity {
		for k, at := range c.failed {
			if now.Sub(at) >= describeFailureTTL {
				delete(c.failed, k)
			}
		}
		if len(c.failed) >= describeFailureCapacity {
			c.failed = make(map[visionCacheKey]time.Time)
		}
	}
	c.failed[key] = now
}

// memoryDescribeStoreCapacity bounds the fallback map; past it the map is
// reset rather than evicted by recency — the fallback is not meant to
// carry a real workload, only to keep the proxy correct without a database.
const memoryDescribeStoreCapacity = 10000

// memoryDescribeStore is the in-process DescribeStore used by tests and as
// the fallback when no database is available.
type memoryDescribeStore struct {
	mu    sync.Mutex
	items map[visionCacheKey]string
}

func newMemoryDescribeStore() *memoryDescribeStore {
	return &memoryDescribeStore{items: make(map[visionCacheKey]string)}
}

func (s *memoryDescribeStore) Get(key visionCacheKey) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text, ok := s.items[key]
	return text, ok
}

func (s *memoryDescribeStore) Put(key visionCacheKey, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[key]; !exists && len(s.items) >= memoryDescribeStoreCapacity {
		s.items = make(map[visionCacheKey]string)
	}
	s.items[key] = text
}
