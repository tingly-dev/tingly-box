package recording

import (
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// streamTap accumulates the bounded, privacy-safe shape of an SSE stream.
// It never keeps chunk bodies: only counts, event names, timing and the
// identity/usage scalars needed for passive model fingerprinting
// (.research/model-fingerprinting.md §5 Tier 0).
type streamTap struct {
	start        time.Time
	firstChunkAt time.Time
	firstTokenAt time.Time
	lastChunkAt  time.Time

	chunkCount int
	eventTypes []string
	eventSeen  map[string]bool

	// interMs holds bounded gaps between consecutive chunks. Only the first
	// maxInterSamples are kept; the summary fields are all we persist.
	interMs []int64

	usage       *obs.UsageFingerprint
	usageSeenAt int

	finishReasons []string
	finishSeen    map[string]bool

	systemFingerprint string
	serviceTier       string
	model             string
	id                string
	doneMarker        bool
}

const maxInterSamples = 4096

func newStreamTap(start time.Time) *streamTap {
	return &streamTap{
		start:      start,
		eventSeen:  map[string]bool{},
		finishSeen: map[string]bool{},
	}
}

func (t *streamTap) record(eventType string, m map[string]interface{}) {
	now := time.Now()
	if t.firstChunkAt.IsZero() {
		t.firstChunkAt = now
	} else if len(t.interMs) < maxInterSamples {
		t.interMs = append(t.interMs, now.Sub(t.lastChunkAt).Milliseconds())
	}
	t.lastChunkAt = now
	t.chunkCount++

	et := eventType
	if et == "" {
		et = firstString(m, "type", "object")
	}
	if et != "" && !t.eventSeen[et] {
		t.eventSeen[et] = true
		t.eventTypes = append(t.eventTypes, et)
	}

	if t.firstTokenAt.IsZero() && chunkHasContent(m) {
		t.firstTokenAt = now
	}

	t.captureSignals(m, 0)

	switch asString(m["type"]) {
	case "message_stop", "response.completed":
		t.doneMarker = true
	}
}

// captureSignals pulls the fingerprint-relevant scalars out of a chunk. It
// recurses once into a nested "response" object (Responses API events carry
// the completed Response there).
func (t *streamTap) captureSignals(m map[string]interface{}, depth int) {
	if s := asString(m["system_fingerprint"]); s != "" {
		t.systemFingerprint = s
	}
	if s := asString(m["service_tier"]); s != "" {
		t.serviceTier = s
	}
	if s := asString(m["model"]); s != "" {
		t.model = s
	}
	if s := asString(m["id"]); s != "" {
		t.id = s
	}
	if u := obs.ExtractUsage(m); u != nil {
		// SDK unions serialise a zero-valued usage on every event; never let
		// such an all-zero block overwrite a non-zero one (order matters:
		// message_start zeros → message_delta real numbers → message_stop
		// zeros again).
		if usageHasValues(u) || t.usage == nil || !usageHasValues(t.usage) {
			t.usage = u
			t.usageSeenAt = t.chunkCount
		}
	}
	if choices, ok := m["choices"].([]interface{}); ok {
		for _, c := range choices {
			if cm, ok := c.(map[string]interface{}); ok {
				t.addFinish(asString(cm["finish_reason"]))
			}
		}
	}
	t.addFinish(asString(m["stop_reason"]))
	if d, ok := m["delta"].(map[string]interface{}); ok {
		t.addFinish(asString(d["stop_reason"]))
	}
	if depth == 0 {
		if resp, ok := m["response"].(map[string]interface{}); ok {
			t.captureSignals(resp, depth+1)
		}
	}
}

func (t *streamTap) addFinish(reason string) {
	if reason == "" || t.finishSeen[reason] {
		return
	}
	t.finishSeen[reason] = true
	t.finishReasons = append(t.finishReasons, reason)
}

// snapshot renders the tap as the persisted StreamFingerprint. total is the
// elapsed request time.
func (t *streamTap) snapshot(total time.Duration) *obs.StreamFingerprint {
	if t == nil || t.chunkCount == 0 {
		return nil
	}
	fp := &obs.StreamFingerprint{
		ChunkCount:      t.chunkCount,
		EventTypes:      append([]string(nil), t.eventTypes...),
		UsageLast:       t.usageSeenAt > 0 && t.usageSeenAt == t.chunkCount,
		DoneMarker:      t.doneMarker,
		TotalMs:         total.Milliseconds(),
		InterChunkCount: len(t.interMs),
	}
	if !t.firstChunkAt.IsZero() {
		fp.FirstChunkMs = t.firstChunkAt.Sub(t.start).Milliseconds()
	}
	if !t.firstTokenAt.IsZero() {
		fp.TTFTMs = t.firstTokenAt.Sub(t.start).Milliseconds()
	}
	if len(t.interMs) > 0 {
		fp.InterChunkMinMs = t.interMs[0]
		fp.InterChunkMaxMs = t.interMs[0]
		for _, v := range t.interMs[1:] {
			if v < fp.InterChunkMinMs {
				fp.InterChunkMinMs = v
			}
			if v > fp.InterChunkMaxMs {
				fp.InterChunkMaxMs = v
			}
		}
	}
	return fp
}

// chunkHasContent reports whether a chunk carries generated text (as opposed
// to role/usage/control frames). Used for TTFT.
func chunkHasContent(m map[string]interface{}) bool {
	if choices, ok := m["choices"].([]interface{}); ok {
		for _, c := range choices {
			cm, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if d, ok := cm["delta"].(map[string]interface{}); ok {
				if asString(d["content"]) != "" {
					return true
				}
			}
		}
	}
	if d, ok := m["delta"].(map[string]interface{}); ok {
		if asString(d["text"]) != "" || asString(d["thinking"]) != "" {
			return true
		}
	}
	if s := asString(m["delta"]); s != "" {
		typ := asString(m["type"])
		if strings.Contains(typ, "output_text") || strings.Contains(typ, "reasoning_text") {
			return true
		}
	}
	return false
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s := asString(m[k]); s != "" {
			return s
		}
	}
	return ""
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func usageHasValues(u *obs.UsageFingerprint) bool {
	if u == nil {
		return false
	}
	return u.InputTokens+u.OutputTokens+u.CacheReadTokens+
		u.CacheWriteTokens+u.ReasoningTokens+u.TotalTokens > 0
}
