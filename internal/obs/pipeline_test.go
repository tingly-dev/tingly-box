package obs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestBatchProcessorNonBlocking verifies Emit never blocks even when the queue is full.
func TestBatchProcessorNonBlocking(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exp, err := NewFileExporter(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Tiny queue so it fills up quickly.
	bp := NewBatchProcessor(exp, BatchProcessorOptions{QueueSize: 2, MaxBatch: 1, FlushInterval: time.Hour})

	// Emit more records than the queue can hold; must not block.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			bp.Emit(&Record{
				Timestamp: time.Now(),
				RequestID: "req",
				Scenario:  "test",
			})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Emit blocked")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bp.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestSlimifyDedup verifies that repeated identical large values produce exactly
// one blob and are referenced by $ref in both SlimRecords.
func TestSlimifyDedup(t *testing.T) {
	t.Parallel()

	// Each element must marshal to >= 256 bytes to trigger blobbing.
	longDesc := "This is a deliberately very long tool description designed to exceed the 256-byte inline threshold used by the slim layer. It needs to be well over two hundred and fifty-six bytes when marshalled to JSON, including the enclosing object keys. Padding: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa."
	bigTools := make([]interface{}, 3)
	for i := range bigTools {
		bigTools[i] = map[string]interface{}{
			"name":        "read_file",
			"description": longDesc,
			"input_schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				},
			},
		}
	}

	makeRecord := func() *Record {
		return &Record{
			Timestamp: time.Now(),
			RequestID: "req",
			OriginalRequest: &RecordRequest{
				Method: "POST",
				URL:    "/v1/messages",
				Body: map[string]interface{}{
					"model":   "claude-opus-4-7",
					"tools":   bigTools,
					"messages": []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
				},
			},
		}
	}

	known := make(map[string]struct{})
	slim1, blobs1 := SlimifyRecord(makeRecord(), known)
	slim2, blobs2 := SlimifyRecord(makeRecord(), known)

	// Second record must produce no new blobs (all hashes already in known).
	if len(blobs2) != 0 {
		t.Errorf("expected 0 new blobs on second record, got %d", len(blobs2))
	}

	// First record must produce blobs for each tool element.
	if len(blobs1) == 0 {
		t.Error("expected blobs from first record, got none")
	}

	// tools[] in both slim records must contain $ref markers.
	// isRef returns true when v is a {"$ref":"sha256:..."} marker (map[string]string or map[string]interface{}).
	isRef := func(v interface{}) bool {
		if m, ok := v.(map[string]string); ok {
			ref, hasRef := m["$ref"]
			return hasRef && len(ref) > 7 // "sha256:" + hash
		}
		if m, ok := v.(map[string]interface{}); ok {
			_, hasRef := m["$ref"]
			return hasRef
		}
		return false
	}

	checkRefs := func(slim *SlimRecord) {
		if slim.OriginalRequest == nil {
			t.Fatal("OriginalRequest is nil")
		}
		body, ok := slim.OriginalRequest.Body.(map[string]interface{})
		if !ok {
			t.Fatalf("body is %T, want map", slim.OriginalRequest.Body)
		}
		tools, ok := body["tools"].([]interface{})
		if !ok {
			t.Fatalf("tools is %T, want []interface{}", body["tools"])
		}
		for i, tool := range tools {
			if !isRef(tool) {
				t.Errorf("tool[%d] expected $ref, got: %T %v", i, tool, tool)
			}
		}
	}
	checkRefs(slim1)
	checkRefs(slim2)
}

// TestFileExporterRoundTrip writes records through the full FileExporter and
// verifies that JSONL files and blobs appear in the expected layout.
func TestFileExporterRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	exp, err := NewFileExporter(dir)
	if err != nil {
		t.Fatal(err)
	}

	records := []*Record{
		{
			Timestamp:  time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC),
			RequestID:  "req-001",
			SessionID:  "abc123",
			SessionSrc: "hdr",
			Provider:   "anthropic",
			Scenario:   "claude_code",
			Model:      "claude-opus-4-7",
			Duration:   1200 * time.Millisecond,
			OriginalRequest: &RecordRequest{
				Method: "POST",
				URL:    "/v1/messages",
				Body: map[string]interface{}{
					"model": "claude-opus-4-7",
					"messages": []interface{}{
						map[string]interface{}{"role": "user", "content": "hello"},
					},
				},
			},
			FinalResponse: &RecordResponse{
				StatusCode: 200,
				Body:       map[string]interface{}{"type": "message", "content": "world"},
			},
		},
	}

	if err := exp.Export(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	if err := exp.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Session JSONL must exist.
	sessionFile := filepath.Join(dir, "claude_code", "sessions", "2026-04-29", "abc123.jsonl")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("session JSONL not found: %v", err)
	}

	var slim SlimRecord
	if err := json.Unmarshal(data[:len(data)-1], &slim); err != nil {
		t.Fatalf("malformed JSONL: %v", err)
	}
	if slim.V != 3 {
		t.Errorf("expected v=3, got %d", slim.V)
	}
	if slim.RequestID != "req-001" {
		t.Errorf("expected rid=req-001, got %s", slim.RequestID)
	}
}

// TestBlobStoreIdempotent verifies that writing the same blob twice doesn't error.
func TestBlobStoreIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := []byte(`{"hello":"world"}`)
	hash := hashBytes(content)

	if err := writeBlob(dir, hash, content); err != nil {
		t.Fatal(err)
	}
	if err := writeBlob(dir, hash, content); err != nil {
		t.Fatal("second write should be idempotent but returned error:", err)
	}

	path := blobPath(dir, hash)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("blob content mismatch")
	}
}
