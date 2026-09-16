package protocolserver

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/clock"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// endpointMemoryTTL is how long a learned service → endpoint mapping is
// trusted. Long enough that a busy gateway learns each model once a day
// rather than once an hour; short enough that a vendor moving a model between
// formats corrects itself without a restart.
const endpointMemoryTTL = 8 * time.Hour

// endpointRetryCooldown throttles learning retries that did NOT work. A retry
// that succeeds is remembered for endpointMemoryTTL and clears the cooldown,
// and later requests then use the learned endpoint directly — there is no
// failure left to retry. So this answers one question only: after we spent a
// round-trip on the other endpoint and that failed too, how soon may we spend
// another? A minute bounds the waste at one extra request per service per
// minute while an upstream is sick, and is deliberately far shorter than the
// TTL — a learned mapping is a verified fact worth keeping for hours, while a
// failed attempt usually just means the upstream was down.
const endpointRetryCooldown = time.Minute

// endpointMemorySweepAt is the entry count past which a write also drops dead
// entries. Model names come from the request, so without a sweep a client
// sending varied names could grow the map without bound.
const endpointMemorySweepAt = 1024

// endpointMemory remembers which OpenAI endpoint a specific model on a
// specific provider actually answers on, for upstreams whose format varies by
// model (ai.EndpointModePerModel).
//
// Deliberately in-memory and TTL'd, never persisted: this is a runtime
// observation, not user configuration. Writing it into config.json would make
// a transient upstream state look like a setting the user chose — the mistake
// that made the old AdaptiveProbe (PR #976) impossible to reason about. A
// restart simply re-learns, at the cost of one extra round-trip per model.
//
// Only successful outcomes are stored. A failed attempt records nothing but
// its timestamp, so a sick upstream cannot poison routing.
type endpointMemory struct {
	mu sync.RWMutex
	// entries is keyed by loadbalance.FormatServiceID — the same service key
	// the breaker, usage tracking and the failover loop use, so a learned
	// endpoint can be correlated with the rest of a service's state in logs.
	entries map[string]endpointMemoryEntry
}

// endpointMemoryEntry holds both halves of what is known about one service:
// the verified endpoint (if any) and when the last unsuccessful retry ran.
// One entry rather than two maps keeps "a success ends the cooldown"
// structural instead of an invariant between two containers.
type endpointMemoryEntry struct {
	endpoint  protocol.APIType // "" = nothing learned
	expires   time.Time
	lastRetry time.Time // zero = no cooldown running
}

func (e endpointMemoryEntry) dead(now time.Time) bool {
	return (e.endpoint == "" || now.After(e.expires)) &&
		(e.lastRetry.IsZero() || now.Sub(e.lastRetry) >= endpointRetryCooldown)
}

func newEndpointMemory() *endpointMemory {
	return &endpointMemory{entries: map[string]endpointMemoryEntry{}}
}

// defaultEndpointMemory is the process-wide store. One per process is right:
// the fact it holds is a property of the upstream, not of a request or a rule.
var defaultEndpointMemory = newEndpointMemory()

// Lookup returns the endpoint learned for this service, or "" when nothing is
// known (or what was known has expired).
func (m *endpointMemory) Lookup(serviceID string) protocol.APIType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry := m.entries[serviceID]
	if entry.endpoint == "" || clock.Now().After(entry.expires) {
		return ""
	}
	return entry.endpoint
}

// Remember records a verified outcome: this service answered on this endpoint.
func (m *endpointMemory) Remember(serviceID string, endpoint protocol.APIType) {
	if endpoint == "" {
		return
	}
	now := clock.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	// lastRetry is left zero: success ends the cooldown.
	m.entries[serviceID] = endpointMemoryEntry{endpoint: endpoint, expires: now.Add(endpointMemoryTTL)}
	m.sweepLocked(now)
}

// Forget drops a learned mapping. Called when the learned endpoint stops
// working, so the next request re-learns instead of failing for the rest of
// the TTL.
func (m *endpointMemory) Forget(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, serviceID)
}

// ShouldRetry reports whether a learning retry (one extra round-trip on the
// other endpoint) may run now. False only while the cooldown from a previous
// unsuccessful retry is still running.
func (m *endpointMemory) ShouldRetry(serviceID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	last := m.entries[serviceID].lastRetry
	return last.IsZero() || clock.Now().Sub(last) >= endpointRetryCooldown
}

// MarkRetry starts the cooldown. Called *before* the extra round-trip, not
// after: a retry that hangs or panics must still hold the gate shut, or a
// broken upstream turns into a loop.
func (m *endpointMemory) MarkRetry(serviceID string) {
	now := clock.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.entries[serviceID]
	entry.lastRetry = now
	m.entries[serviceID] = entry
	m.sweepLocked(now)
}

// sweepLocked drops entries that hold nothing live. Opportunistic and
// amortized — the same shape internal/probe's endpoint cache uses — so no
// background goroutine is needed.
func (m *endpointMemory) sweepLocked(now time.Time) {
	if len(m.entries) <= endpointMemorySweepAt {
		return
	}
	for key, entry := range m.entries {
		if entry.dead(now) {
			delete(m.entries, key)
		}
	}
}

// alternateEndpoint returns the other OpenAI endpoint, or "" when the input is
// not one of the two.
func alternateEndpoint(endpoint protocol.APIType) protocol.APIType {
	switch endpoint {
	case protocol.TypeOpenAIChat:
		return protocol.TypeOpenAIResponses
	case protocol.TypeOpenAIResponses:
		return protocol.TypeOpenAIChat
	default:
		return ""
	}
}

// endpointMemoryKey is the service key this store uses, shared with the
// breaker and usage tracking.
func endpointMemoryKey(providerUUID, model string) string {
	return loadbalance.FormatServiceID(providerUUID, model)
}
