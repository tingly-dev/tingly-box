// Package capture records, as faithfully as possible, what a real client sends
// to a local fake upstream.
//
// The ground truth is the raw bytes of every accepted TCP connection: a tee
// below net/http writes everything read from the client to raw/conn-N.in and
// (optionally) everything written back to raw/conn-N.out. Nothing is parsed,
// canonicalized, redacted or re-ordered in those files, so header-name casing,
// header order, duplicate headers, the request line, chunked framing and the
// exact body bytes survive. index.jsonl only locates each request inside them.
//
// See .design/vmodel-capture.md.
package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Options configure a Tap.
type Options struct {
	// Out also records the bytes written back to the client
	// (raw/conn-N.out). Callers normally want this on.
	Out bool
	// RequestID makes the capture handler add a `request-id` response header
	// (req_vmodel_<seq>) to responses that do not set one.
	RequestID bool
	// Warn receives one-line diagnostics (an unwritable capture file, a
	// credential that does not look fake). It must not be nil.
	Warn func(string)
}

// Tap owns one capture directory.
type Tap struct {
	dir  string
	opts Options

	connSeq atomic.Int64
	reqSeq  atomic.Int64

	indexMu sync.Mutex
	index   *os.File

	warnOnce sync.Map // message -> struct{}
}

// New prepares dir for capture. The directory must not exist or must be empty:
// a capture is evidence, so it is never mixed with or written over another.
func New(dir string, opts Options) (*Tap, error) {
	if dir == "" {
		return nil, fmt.Errorf("capture: directory is required")
	}
	if opts.Warn == nil {
		opts.Warn = func(string) {}
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("capture: %s is not empty; use a new directory", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o700); err != nil {
		return nil, fmt.Errorf("capture: create %s: %w", dir, err)
	}
	index, err := os.OpenFile(filepath.Join(dir, "index.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("capture: create index: %w", err)
	}
	return &Tap{dir: dir, opts: opts, index: index}, nil
}

// Dir returns the capture directory.
func (t *Tap) Dir() string { return t.dir }

// SetMeta writes meta.json (replacing any previous one) with the caller's
// description of the run plus the creation time. Safe to call once the
// listener address is known.
func (t *Tap) SetMeta(meta map[string]any) error {
	out := map[string]any{"created_at": time.Now().UTC().Format(time.RFC3339Nano)}
	for k, v := range meta {
		out[k] = v
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(t.dir, "meta.json"), append(data, '\n'), 0o600)
}

// Close closes the index. Raw connection files are closed with their
// connections.
func (t *Tap) Close() error {
	t.indexMu.Lock()
	defer t.indexMu.Unlock()
	return t.index.Close()
}

func (t *Tap) warn(msg string) {
	if _, dup := t.warnOnce.LoadOrStore(msg, struct{}{}); !dup {
		t.opts.Warn(msg)
	}
}

// entry is one index.jsonl line. It locates a request inside the raw files and
// carries nothing the raw bytes do not.
type entry struct {
	Seq        int64     `json:"seq,omitempty"`
	T          time.Time `json:"t"`
	Conn       int64     `json:"conn"`
	In         [2]int64  `json:"in"`
	Out        *[2]int64 `json:"out,omitempty"`
	Method     string    `json:"method,omitempty"`
	Target     string    `json:"target,omitempty"`
	Status     int       `json:"status,omitempty"`
	Incomplete bool      `json:"incomplete,omitempty"`
	Unparsed   bool      `json:"unparsed,omitempty"`
}

func (t *Tap) write(e entry) {
	data, err := json.Marshal(e)
	if err != nil {
		t.warn("capture: marshal index entry: " + err.Error())
		return
	}
	t.indexMu.Lock()
	defer t.indexMu.Unlock()
	if _, err := t.index.Write(append(data, '\n')); err != nil {
		t.warn("capture: write index: " + err.Error())
	}
}
