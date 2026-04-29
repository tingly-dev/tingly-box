package obs

import (
	"encoding/json"
	"time"
)

const inlineThreshold = 256 // bytes; values smaller than this stay inline

// SlimRecord is the JSON-serializable slim form stored in session JSONL files.
// Large values are replaced by {"$ref":"sha256:<hex>"} pointers into the blob store.
type SlimRecord struct {
	V         int    `json:"v"` // schema version = 3
	Timestamp string `json:"ts"`
	RequestID string `json:"rid"`

	SessionID  string `json:"sid,omitempty"`
	SessionSrc string `json:"sid_src,omitempty"`

	Provider string `json:"provider,omitempty"`
	Scenario string `json:"scenario,omitempty"`
	Model    string `json:"model,omitempty"`

	OriginalRequest    *SlimHTTPData `json:"original_request,omitempty"`
	TransformedRequest *SlimHTTPData `json:"transformed_request,omitempty"`
	ProviderResponse   *SlimHTTPData `json:"provider_response,omitempty"`
	FinalResponse      *SlimHTTPData `json:"final_response,omitempty"`

	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`

	Steps []string `json:"transform_steps,omitempty"`
}

// SlimHTTPData mirrors RecordRequest / RecordResponse with a body that may
// contain {"$ref":"sha256:<hex>"} markers instead of large inline values.
type SlimHTTPData struct {
	Method     string            `json:"method,omitempty"`
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	StatusCode int               `json:"status_code,omitempty"`
	Body       interface{}       `json:"body,omitempty"`
	IsStreaming bool             `json:"is_streaming,omitempty"`
}

// SlimifyRecord converts a Record to a SlimRecord by replacing large JSON
// sub-values with content-addressed $ref pointers.
//
// knownBlobs is the set of hashes already on disk; only new blobs are returned
// in the second return value (hash → serialised JSON bytes).
func SlimifyRecord(r *Record, knownBlobs map[string]struct{}) (*SlimRecord, map[string][]byte) {
	newBlobs := make(map[string][]byte)
	slim := &SlimRecord{
		V:          3,
		Timestamp:  r.Timestamp.UTC().Format(time.RFC3339),
		RequestID:  r.RequestID,
		SessionID:  r.SessionID,
		SessionSrc: r.SessionSrc,
		Provider:   r.Provider,
		Scenario:   r.Scenario,
		Model:      r.Model,
		DurationMs: r.Duration.Milliseconds(),
		Error:      r.Err,
		Steps:      r.Steps,
	}

	if r.OriginalRequest != nil {
		slim.OriginalRequest = slimRequest(r.OriginalRequest, knownBlobs, newBlobs)
	}
	if r.TransformedRequest != nil {
		slim.TransformedRequest = slimRequest(r.TransformedRequest, knownBlobs, newBlobs)
	}
	if r.ProviderResponse != nil {
		slim.ProviderResponse = slimResponse(r.ProviderResponse, knownBlobs, newBlobs)
	}
	if r.FinalResponse != nil {
		slim.FinalResponse = slimResponse(r.FinalResponse, knownBlobs, newBlobs)
	}

	return slim, newBlobs
}

func slimRequest(req *RecordRequest, known map[string]struct{}, out map[string][]byte) *SlimHTTPData {
	d := &SlimHTTPData{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
	}
	if req.Body != nil {
		d.Body = slimBody(req.Body, known, out)
	}
	return d
}

func slimResponse(resp *RecordResponse, known map[string]struct{}, out map[string][]byte) *SlimHTTPData {
	d := &SlimHTTPData{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Headers,
		IsStreaming: resp.IsStreaming,
	}
	if resp.Body != nil {
		// Responses are hashed as a whole unit.
		d.Body = maybeRef(resp.Body, known, out)
	}
	return d
}

// slimBody replaces large fields within a request body map with $ref pointers.
// Arrays under the keys "system", "tools", and "messages" are slimmed per-element
// to maximise deduplication across conversation turns.
func slimBody(body map[string]interface{}, known map[string]struct{}, out map[string][]byte) map[string]interface{} {
	result := make(map[string]interface{}, len(body))
	for k, v := range body {
		switch k {
		case "system", "tools", "messages":
			result[k] = slimArray(v, known, out)
		default:
			// Inline small scalars; hash large blobs.
			result[k] = maybeRef(v, known, out)
		}
	}
	return result
}

// slimArray hashes each element of an array individually.
// If v is not a slice (e.g. a string system prompt), it is treated as a single value.
func slimArray(v interface{}, known map[string]struct{}, out map[string][]byte) interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		return maybeRef(v, known, out)
	}
	result := make([]interface{}, len(arr))
	for i, elem := range arr {
		result[i] = maybeRef(elem, known, out)
	}
	return result
}

// maybeRef serialises v to JSON; if the result exceeds inlineThreshold it
// stores it as a blob and returns a {"$ref":"sha256:<hash>"} marker.
func maybeRef(v interface{}, known map[string]struct{}, out map[string][]byte) interface{} {
	data, err := json.Marshal(v)
	if err != nil || len(data) < inlineThreshold {
		return v // keep inline
	}
	hash := hashBytes(data)
	if _, exists := known[hash]; !exists {
		if _, pending := out[hash]; !pending {
			out[hash] = data
		}
		known[hash] = struct{}{}
	}
	return map[string]string{"$ref": "sha256:" + hash}
}
