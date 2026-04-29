package obs

import "time"

// Record is the canonical data model for one LLM request/response cycle.
// Construct it on the hot path and pass to Sink.Emit; the only cost is a
// non-blocking channel send.
type Record struct {
	Timestamp  time.Time
	RequestID  string
	SessionID  string // sha256(raw session value)[:16], empty when unknown
	SessionSrc string // "user" | "hdr" | "ip" | ""
	Provider   string
	Scenario   string
	Model      string

	OriginalRequest    *RecordRequest
	TransformedRequest *RecordRequest
	ProviderResponse   *RecordResponse
	FinalResponse      *RecordResponse

	Duration time.Duration
	Err      string
	Steps    []string
}
