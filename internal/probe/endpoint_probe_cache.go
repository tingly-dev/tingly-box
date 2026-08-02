package probe

import (
	"sync"
	"time"
)

// endpointProbeCacheTTL bounds how long a successful direct-endpoint probe
// (used by the Codex "native Responses" toggle) is trusted before the next
// check re-verifies against the real upstream.
const endpointProbeCacheTTL = time.Hour

// endpointProbeCache is a lightweight, best-effort cache for direct
// provider+model+endpoint capability probes (E2ETargetProvider + Direct +
// Endpoint). It exists to avoid burning a real, billed upstream call every
// time a user re-checks the same provider/model — e.g. flipping through
// candidate models while the Codex "native Responses" toggle re-validates on
// every swap (see UnifiedRoutingGraph's revalidation effect).
//
// Deliberately narrow and NOT a repeat of the Adaptive-era mistake
// (.design/openai-endpoint-routing.md §2.1): Adaptive cached probe results
// and consulted that cache on the HOT REQUEST PATH to make live routing
// decisions, so one stale/poisoned entry silently misrouted production
// traffic indefinitely. This cache only feeds a one-shot, user-initiated
// pre-flight check whose result becomes an explicit, visible rule flag —
// it never sits between a real request and its routing decision.
//
// Only successes are cached. A transient failure (rate limit, network blip)
// must never get remembered, or we'd reproduce Adaptive's "one failure marks
// it dead forever" bug — every failed check re-probes for real next time.
//
// Trade-off accepted for staying this simple: a cached success can go stale
// if the user edits the provider's credentials/base URL within the TTL
// window (no invalidation hook on provider update). Bounded by the TTL, and
// the provider would already be broken for every other request in that
// window too, not just Responses ones — not unique to this feature.
type endpointProbeCache struct {
	mu      sync.Mutex
	entries map[string]time.Time // key -> cached-at
}

func newEndpointProbeCache() *endpointProbeCache {
	return &endpointProbeCache{entries: make(map[string]time.Time)}
}

func endpointProbeCacheKey(providerUUID, model, endpoint string) string {
	return providerUUID + "\x00" + model + "\x00" + endpoint
}

// hit reports whether a cached success is still fresh for this key, evicting
// it if it has expired.
func (c *endpointProbeCache) hit(providerUUID, model, endpoint string) bool {
	key := endpointProbeCacheKey(providerUUID, model, endpoint)
	c.mu.Lock()
	defer c.mu.Unlock()
	cachedAt, ok := c.entries[key]
	if !ok {
		return false
	}
	if time.Since(cachedAt) > endpointProbeCacheTTL {
		delete(c.entries, key)
		return false
	}
	return true
}

// remember records a successful probe. Also opportunistically sweeps expired
// entries (the lock is already held, and this is the only other write path
// besides the lazy per-key eviction in hit) so a key that's checked once and
// never queried again doesn't sit in the map forever.
func (c *endpointProbeCache) remember(providerUUID, model, endpoint string) {
	key := endpointProbeCacheKey(providerUUID, model, endpoint)
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, cachedAt := range c.entries {
		if now.Sub(cachedAt) > endpointProbeCacheTTL {
			delete(c.entries, k)
		}
	}
	c.entries[key] = now
}
