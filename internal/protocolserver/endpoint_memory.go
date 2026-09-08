package protocolserver

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// endpointMemoryTTL is how long a learned (provider, model) → endpoint
// mapping is trusted. Long enough that a busy gateway learns each model once
// a day rather than once an hour; short enough that a vendor moving a model
// between formats corrects itself without a restart.
const endpointMemoryTTL = 8 * time.Hour

// endpointRetryCooldown throttles learning retries that did NOT work. It has
// nothing to do with successful ones: a retry that succeeds is remembered for
// endpointMemoryTTL and clears the cooldown, and subsequent requests then use
// the learned endpoint directly — there is no failure left to retry.
//
// So the cooldown answers one question only: after we spent a round-trip on
// the other endpoint and that failed too, how soon may we spend another? A
// minute bounds the waste at one extra request per (provider, model) per
// minute while an upstream is sick, and is deliberately far shorter than the
// TTL — a learned mapping is a verified fact worth keeping for hours, while a
// failed attempt usually just means the upstream was down, and pinning that
// for eight hours would keep a genuinely Responses-only model broken long
// after the outage ended.
const endpointRetryCooldown = time.Minute

// endpointMemory remembers which OpenAI endpoint a specific model on a
// specific provider actually answers on, for providers that declare
// ai.EndpointModePerModel.
//
// Deliberately in-memory and TTL'd, never persisted: this is a runtime
// observation, not user configuration. Writing it into config.json would make
// a transient upstream state look like a setting the user chose — the mistake
// that made the old AdaptiveProbe (PR #976) impossible to reason about. A
// restart simply re-learns, at the cost of one extra round-trip per model.
//
// Only *successful* outcomes are stored. A failed attempt records nothing but
// its timestamp, so a sick upstream cannot poison routing.
type endpointMemory struct {
	mu      sync.RWMutex
	learned map[string]endpointMemoryEntry
	// retried holds when the last *unsuccessful* learning retry ran. A
	// successful one is deleted from here and lives in learned instead.
	retried map[string]time.Time
	now     func() time.Time // injectable for tests
}

type endpointMemoryEntry struct {
	endpoint protocol.APIType
	expires  time.Time
}

func newEndpointMemory() *endpointMemory {
	return &endpointMemory{
		learned: map[string]endpointMemoryEntry{},
		retried: map[string]time.Time{},
		now:     time.Now,
	}
}

// defaultEndpointMemory is the process-wide store. One per process is right:
// the fact it holds is a property of the upstream, not of a request or a rule.
var defaultEndpointMemory = newEndpointMemory()

func endpointMemoryKey(providerUUID, model string) string {
	return providerUUID + "\x00" + model
}

// Lookup returns the endpoint learned for this provider+model, or "" when
// nothing is known (or what was known has expired).
func (m *endpointMemory) Lookup(providerUUID, model string) protocol.APIType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.learned[endpointMemoryKey(providerUUID, model)]
	if !ok || m.now().After(entry.expires) {
		return ""
	}
	return entry.endpoint
}

// Remember records a verified outcome: this provider+model answered on this
// endpoint.
func (m *endpointMemory) Remember(providerUUID, model string, endpoint protocol.APIType) {
	if endpoint == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := endpointMemoryKey(providerUUID, model)
	m.learned[key] = endpointMemoryEntry{endpoint: endpoint, expires: m.now().Add(endpointMemoryTTL)}
	// Success ends the cooldown: there is nothing left to re-learn.
	delete(m.retried, key)
}

// ShouldRetry reports whether a learning retry (one extra round-trip on the
// other endpoint) may run now. False only while the cooldown from a previous
// *failed* retry is still running.
func (m *endpointMemory) ShouldRetry(providerUUID, model string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	last, ok := m.retried[endpointMemoryKey(providerUUID, model)]
	return !ok || m.now().Sub(last) >= endpointRetryCooldown
}

// MarkRetry starts the cooldown. Called *before* the extra round-trip, not
// after: a retry that hangs or panics must still hold the gate shut, or a
// broken upstream turns into a loop.
func (m *endpointMemory) MarkRetry(providerUUID, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.retried[endpointMemoryKey(providerUUID, model)] = m.now()
}

// alternateEndpoint returns the other OpenAI endpoint, or "" when the input
// is not one of the two.
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
