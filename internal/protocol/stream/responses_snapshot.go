package stream

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3/responses"
)

// Keep per-part builders so long delta streams do not repeatedly copy the
// entire preceding output while reconciling their final snapshots.
type responseTextTracker map[string]*strings.Builder

func (t responseTextTracker) retain(index, partIndex int, kind, text string, snapshot bool) string {
	if text == "" {
		return ""
	}
	key := fmt.Sprintf("%d/%d/%s", index, partIndex, kind)
	b := t[key]
	if b == nil {
		b = &strings.Builder{}
		t[key] = b
	}
	if snapshot {
		text = snapshotSuffix(b.String(), text)
	}
	b.WriteString(text)
	return text
}

func responseTextKind(eventType string) string {
	switch {
	case strings.Contains(eventType, "refusal"):
		return "refusal"
	case strings.Contains(eventType, "reasoning_summary"):
		return "summary"
	case strings.Contains(eventType, "reasoning_text"):
		return "reasoning"
	default:
		return "text"
	}
}

func snapshotSuffix(emitted, snapshot string) string {
	if strings.HasPrefix(snapshot, emitted) {
		return strings.TrimPrefix(snapshot, emitted)
	}
	return "" // A contradictory snapshot cannot retract bytes already sent.
}

func responsePartIndex(event responses.ResponseStreamEventUnion) int {
	if strings.Contains(event.Type, "reasoning_summary") {
		return int(event.SummaryIndex)
	}
	return int(event.ContentIndex)
}
