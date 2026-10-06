// Package tracestore persists capture.Snapshots with request-body dedup for
// long-running sessions (.design/recording.md §3.2).
//
// Long agent sessions resend the whole history every turn, so storing each
// body verbatim grows quadratically. Bodies are therefore split along their
// JSON structure: every top-level array becomes a prefix chain of
// content-addressed elements, other large top-level values become single
// blocks, and each block is written once per partition. A trace line holds
// only a constant-size skeleton of references.
//
// On-disk layout, one partition per scenario × day × session:
//
//	{root}/{scenario}/{YYYY-MM-DD}/{session}.jsonl.gz    blocks and traces, gzip members appended per batch
//	{root}/{scenario}/{YYYY-MM-DD}/{session}.index.jsonl one metadata line per trace, for listing
//
// A block line always precedes the first trace that references it, so one
// sequential read of the data file restores every trace. Partitions never
// reference each other, so retention is deleting directories.
package tracestore

import (
	"encoding/json"
	"time"
)

// SchemaVersion is written on every trace line.
const SchemaVersion = 1

// line is one row of the data file: a block ("blob") or a trace.
type line struct {
	Kind string          `json:"k"`
	Hash string          `json:"h,omitempty"`
	Data json.RawMessage `json:"d,omitempty"`
	*TraceRecord
}

// chainNode is the block written for one position of a prefix chain: the
// element at this position plus the node for everything before it.
type chainNode struct {
	Prev string `json:"p,omitempty"`
	Elem string `json:"e"`
}

// TraceRecord is the stored form of one Trace.
type TraceRecord struct {
	V          int               `json:"v"`
	RequestID  string            `json:"rid"`
	Timestamp  time.Time         `json:"ts"`
	DurationMs int64             `json:"duration_ms"`
	Scenario   string            `json:"scenario,omitempty"`
	Rule       string            `json:"rule,omitempty"`
	Session    string            `json:"sid,omitempty"`
	SessionSrc string            `json:"sid_src,omitempty"`
	Points     string            `json:"points,omitempty"`
	Error      string            `json:"error,omitempty"`
	Inbound    *MessageRecord    `json:"inbound,omitempty"`
	Exchanges  []*ExchangeRecord `json:"exchanges,omitempty"`
}

// ExchangeRecord is the stored form of one upstream round trip.
type ExchangeRecord struct {
	Seq       int            `json:"seq"`
	Provider  string         `json:"provider,omitempty"`
	UUID      string         `json:"provider_uuid,omitempty"`
	APIStyle  string         `json:"api_style,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	TTFBMs    int64          `json:"ttfb_ms,omitempty"`
	DurMs     int64          `json:"duration_ms"`
	Error     string         `json:"error,omitempty"`
	Request   *MessageRecord `json:"request,omitempty"`
	Response  *MessageRecord `json:"response,omitempty"`
}

// MessageRecord is the stored form of one HTTP request or response.
type MessageRecord struct {
	Method      string            `json:"method,omitempty"`
	URL         string            `json:"url,omitempty"`
	Status      int               `json:"status,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	Size        int64             `json:"size,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	Complete    bool              `json:"complete,omitempty"`
	Body        *Body             `json:"body,omitempty"`
}

// Body is a stored body in exactly one of its encodings.
type Body struct {
	// Fields is a JSON object split into its top-level members, key order
	// kept. Each member is inline (V), one block (Ref) or a prefix chain
	// (Chain, N elements).
	Fields []Field `json:"fields,omitempty"`
	// Raw is any other JSON value, inline; Ref is one stored as a block.
	Raw json.RawMessage `json:"raw,omitempty"`
	Ref string          `json:"ref,omitempty"`
	// Text is a non-JSON UTF-8 body (SSE streams); B64 is anything else.
	Text *string `json:"text,omitempty"`
	B64  string  `json:"b64,omitempty"`
}

// Field is one top-level member of a JSON object body.
type Field struct {
	Key   string          `json:"k"`
	V     json.RawMessage `json:"v,omitempty"`
	Ref   string          `json:"ref,omitempty"`
	Chain string          `json:"chain,omitempty"`
	N     int             `json:"n,omitempty"`
}

// IndexEntry is one line of a partition's index file: the metadata a listing
// needs without opening the data file.
type IndexEntry struct {
	RequestID  string    `json:"rid"`
	Timestamp  time.Time `json:"ts"`
	DurationMs int64     `json:"duration_ms"`
	Scenario   string    `json:"scenario,omitempty"`
	Rule       string    `json:"rule,omitempty"`
	Session    string    `json:"sid,omitempty"`
	Points     string    `json:"points,omitempty"`
	Exchanges  int       `json:"exchanges"`
	Provider   string    `json:"provider,omitempty"` // last exchange's provider
	Status     int       `json:"status,omitempty"`   // last exchange's status
	Error      string    `json:"error,omitempty"`
}
