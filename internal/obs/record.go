package obs

import (
	"time"

	requestrecord "github.com/tingly-dev/tingly-box/internal/record"
)

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

	// Provider connection details for debugging
	APIStyle string `json:"api_style,omitempty"` // Provider API style (e.g., "openai", "anthropic")
	BaseURL  string `json:"base_url,omitempty"`  // Provider base URL

	OriginalRequest    *RecordRequest
	TransformedRequest *RecordRequest
	ProviderResponse   *RecordResponse
	FinalResponse      *RecordResponse

	// RequestRecord is the additive Protocol Stage recording envelope. Legacy
	// request/response fields remain unchanged while the new recorder is
	// canaried behind the Stage and scenario recording switches.
	RequestRecord *requestrecord.RequestRecord

	Duration time.Duration
	Err      string
	Steps    []string
}
