package tracestore

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/recording/capture"
)

const (
	queueSize     = 256
	batchSize     = 64
	flushInterval = time.Second
	// maxPartitions bounds the per-partition dedup sets kept in memory. An
	// evicted partition that is written again re-stores its blocks once.
	maxPartitions = 128
)

// Writer persists Snapshots asynchronously. Emit never blocks the request
// path: when the queue is full the snapshot is dropped and counted.
// All encoding, hashing and file I/O happen on the single worker goroutine.
type Writer struct {
	root    string
	queue   chan *capture.Snapshot
	flushes chan chan struct{}
	done    chan struct{}
	closed  atomic.Bool
	wg      sync.WaitGroup
	dropped atomic.Uint64

	// worker-owned
	seen  map[string]map[string]struct{} // partition path -> block hashes on disk
	order []string                       // partition LRU, oldest first
}

// NewWriter starts a Writer rooted at root (typically <configDir>/record/traces).
func NewWriter(root string) *Writer {
	w := &Writer{
		root:    root,
		queue:   make(chan *capture.Snapshot, queueSize),
		flushes: make(chan chan struct{}),
		done:    make(chan struct{}),
		seen:    map[string]map[string]struct{}{},
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Root returns the directory traces are written under.
func (w *Writer) Root() string { return w.root }

// Emit enqueues s for writing. Nil-safe and non-blocking.
func (w *Writer) Emit(s *capture.Snapshot) {
	if w == nil || s == nil || w.closed.Load() {
		return
	}
	select {
	case w.queue <- s:
	default:
		if n := w.dropped.Add(1); n == 1 || n%100 == 0 {
			logrus.Warnf("recording: trace queue full, dropped %d trace(s)", n)
		}
	}
}

// Dropped returns how many snapshots were dropped because the queue was full.
func (w *Writer) Dropped() uint64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

// Flush blocks until everything emitted before the call is on disk.
func (w *Writer) Flush(ctx context.Context) error {
	if w == nil || w.closed.Load() {
		return nil
	}
	ack := make(chan struct{})
	select {
	case w.flushes <- ack:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close drains the queue and stops the worker.
func (w *Writer) Close(ctx context.Context) error {
	if w == nil || !w.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(w.done)
	stopped := make(chan struct{})
	go func() { w.wg.Wait(); close(stopped) }()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Writer) run() {
	defer w.wg.Done()
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	var pending []*capture.Snapshot
	drain := func() {
		for {
			select {
			case s := <-w.queue:
				pending = append(pending, s)
			default:
				return
			}
		}
	}
	write := func() {
		if len(pending) > 0 {
			w.writeBatch(pending)
			clear(pending)
			pending = pending[:0]
		}
	}
	for {
		select {
		case s := <-w.queue:
			pending = append(pending, s)
			if len(pending) >= batchSize {
				write()
			}
		case <-ticker.C:
			write()
		case ack := <-w.flushes:
			drain()
			write()
			close(ack)
		case <-w.done:
			drain()
			write()
			return
		}
	}
}

// partitionBlocks collects the blocks one batch adds to a partition.
// Hashes become "seen" only after the batch is on disk, so a failed write
// never leaves references to blocks that were not written.
type partitionBlocks struct {
	seen    map[string]struct{}
	pending map[string]struct{}
	lines   []line
}

func (p *partitionBlocks) put(data []byte) string {
	h := hashOf(data)
	if _, ok := p.seen[h]; ok {
		return h
	}
	if _, ok := p.pending[h]; ok {
		return h
	}
	p.pending[h] = struct{}{}
	p.lines = append(p.lines, line{Kind: "blob", Hash: h, Data: append(json.RawMessage(nil), data...)})
	return h
}

func (w *Writer) writeBatch(batch []*capture.Snapshot) {
	type part struct {
		blocks *partitionBlocks
		index  []IndexEntry
	}
	parts := map[string]*part{}
	var order []string
	for _, s := range batch {
		path := w.partitionPath(s)
		p, ok := parts[path]
		if !ok {
			p = &part{blocks: &partitionBlocks{seen: w.seenFor(path), pending: map[string]struct{}{}}}
			parts[path] = p
			order = append(order, path)
		}
		rec := toRecord(s, p.blocks)
		p.blocks.lines = append(p.blocks.lines, line{Kind: "trace", TraceRecord: rec})
		p.index = append(p.index, indexEntry(rec))
	}
	for _, path := range order {
		p := parts[path]
		if err := appendData(path, p.blocks.lines); err != nil {
			logrus.Warnf("recording: write %s: %v", path, err)
			continue
		}
		for h := range p.blocks.pending {
			p.blocks.seen[h] = struct{}{}
		}
		if err := appendIndex(indexPath(path), p.index); err != nil {
			logrus.Warnf("recording: write index for %s: %v", path, err)
		}
	}
}

// seenFor returns the dedup set of a partition, tracking LRU order.
func (w *Writer) seenFor(path string) map[string]struct{} {
	if s, ok := w.seen[path]; ok {
		for i, p := range w.order {
			if p == path {
				w.order = append(append(w.order[:i:i], w.order[i+1:]...), path)
				break
			}
		}
		return s
	}
	if len(w.order) >= maxPartitions {
		delete(w.seen, w.order[0])
		w.order = w.order[1:]
	}
	s := map[string]struct{}{}
	w.seen[path] = s
	w.order = append(w.order, path)
	return s
}

func (w *Writer) partitionPath(s *capture.Snapshot) string {
	scenario := sanitize(s.Scenario)
	if scenario == "" {
		scenario = "_"
	}
	session, _ := obs.SessionShort(s.Session)
	if session == "" {
		session = "_unknown"
	}
	date := s.Timestamp.UTC().Format("2006-01-02")
	return filepath.Join(w.root, scenario, date, session+".jsonl.gz")
}

func indexPath(dataPath string) string {
	return strings.TrimSuffix(dataPath, ".jsonl.gz") + ".index.jsonl"
}

// sanitize makes a scenario id safe as a directory name on every OS
// (profile ids carry a colon, e.g. "claude_code:p1").
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		return r
	}, s)
}

func appendData(path string, lines []line) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewWriter(f)
	gz, err := gzip.NewWriterLevel(buf, gzip.BestSpeed)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(gz)
	enc.SetEscapeHTML(false)
	for i := range lines {
		if err := enc.Encode(&lines[i]); err != nil {
			_ = gz.Close()
			return err
		}
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return buf.Flush()
}

func appendIndex(path string, entries []IndexEntry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for i := range entries {
		if err := enc.Encode(&entries[i]); err != nil {
			return err
		}
	}
	return nil
}

func toRecord(s *capture.Snapshot, blocks blockSink) *TraceRecord {
	short, src := obs.SessionShort(s.Session)
	rec := &TraceRecord{
		V:          SchemaVersion,
		RequestID:  s.RequestID,
		Timestamp:  s.Timestamp,
		DurationMs: s.Duration.Milliseconds(),
		Scenario:   s.Scenario,
		Rule:       s.Rule,
		Session:    short,
		SessionSrc: src,
		Points:     string(s.Mode),
		Error:      s.Error,
		Inbound:    toMessage(s.Inbound, blocks),
	}
	for _, ex := range s.Exchanges {
		rec.Exchanges = append(rec.Exchanges, &ExchangeRecord{
			Seq:       ex.Seq,
			Provider:  ex.Provider.Name,
			UUID:      ex.Provider.UUID,
			APIStyle:  ex.Provider.APIStyle,
			StartedAt: ex.StartedAt,
			TTFBMs:    ex.TTFB.Milliseconds(),
			DurMs:     ex.Duration.Milliseconds(),
			Error:     ex.Error,
			Request:   toMessage(ex.Request, blocks),
			Response:  toMessage(ex.Response, blocks),
		})
	}
	return rec
}

func toMessage(m *capture.Message, blocks blockSink) *MessageRecord {
	if m == nil {
		return nil
	}
	return &MessageRecord{
		Method:      m.Method,
		URL:         m.URL,
		Status:      m.Status,
		Headers:     m.Headers,
		ContentType: m.ContentType,
		Stream:      m.Stream,
		Size:        m.Size,
		Truncated:   m.Truncated,
		End:         m.End,
		Body:        encodeBody(m.Body, blocks),
	}
}

func indexEntry(rec *TraceRecord) IndexEntry {
	e := IndexEntry{
		RequestID:  rec.RequestID,
		Timestamp:  rec.Timestamp,
		DurationMs: rec.DurationMs,
		Scenario:   rec.Scenario,
		Rule:       rec.Rule,
		Session:    rec.Session,
		Points:     rec.Points,
		Exchanges:  len(rec.Exchanges),
		Error:      rec.Error,
	}
	if n := len(rec.Exchanges); n > 0 {
		last := rec.Exchanges[n-1]
		e.Provider = last.Provider
		if last.Response != nil {
			e.Status = last.Response.Status
		}
		if e.Error == "" {
			e.Error = last.Error
		}
	}
	return e
}
