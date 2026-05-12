package obs

import (
	"bufio"
	"container/list"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// CASFileExporter implements RecordExporter with content-addressed storage:
//  1. Slim-ifies every Record (large sub-values replaced by $ref pointers).
//  2. Writes new blobs atomically (tmp+rename) under {baseDir}/blobs/.
//  3. Appends SlimRecords to per-session JSONL files.
//
// CASFileExporter is NOT goroutine-safe; it must be driven by a single
// goroutine (the BatchProcessor worker).
type CASFileExporter struct {
	baseDir string
	blobSet map[string]struct{} // hashes known to be on disk (or written this run)
	fdLRU   *fileLRU
}

// NewCASFileExporter creates a CASFileExporter rooted at baseDir and populates
// the in-memory blob set by scanning existing blobs.
func NewCASFileExporter(baseDir string) (*CASFileExporter, error) {
	set, err := scanBlobSet(baseDir)
	if err != nil {
		logrus.Warnf("obs: blob set scan failed, starting fresh: %v", err)
		set = make(map[string]struct{})
	}
	return &CASFileExporter{
		baseDir: baseDir,
		blobSet: set,
		fdLRU:   newFileLRU(256),
	}, nil
}

// Export processes a batch of Records.
func (e *CASFileExporter) Export(_ context.Context, records []*Record) error {
	if len(records) == 0 {
		return nil
	}

	// Slim all records; accumulate new blobs and per-session slim records.
	newBlobs := make(map[string][]byte)
	type sessionEntry struct {
		path  string
		lines []*SlimRecord
	}
	sessions := make(map[string]*sessionEntry)

	for _, r := range records {
		slim, blobs := SlimifyRecord(r, e.blobSet)
		for h, content := range blobs {
			newBlobs[h] = content
			e.blobSet[h] = struct{}{}
		}
		path := e.sessionPath(r)
		if _, ok := sessions[path]; !ok {
			sessions[path] = &sessionEntry{path: path}
		}
		sessions[path].lines = append(sessions[path].lines, slim)
	}

	// Write new blobs (idempotent, atomic).
	for hash, content := range newBlobs {
		if err := writeBlob(e.baseDir, hash, content); err != nil {
			logrus.Warnf("obs: failed to write blob %s: %v", hash[:8], err)
		}
	}

	// Append slim records to session JSONL files.
	for _, se := range sessions {
		if err := e.appendLines(se.path, se.lines); err != nil {
			logrus.Warnf("obs: failed to append session %s: %v", se.path, err)
		}
	}

	return nil
}

// Shutdown flushes and closes all open session files.
func (e *CASFileExporter) Shutdown(_ context.Context) error {
	e.fdLRU.closeAll()
	return nil
}

// sessionPath returns the JSONL path for a record.
// Layout: {baseDir}/{scenario}/sessions/{YYYY-MM-DD}/{session}.jsonl
// Falls back to _unknown/{YYYY-MM-DD}.jsonl when the session is empty.
func (e *CASFileExporter) sessionPath(r *Record) string {
	date := r.Timestamp.UTC().Format("2006-01-02")
	scenario := r.Scenario
	if scenario == "" {
		scenario = r.Provider
	}
	if scenario == "" {
		scenario = "_"
	}
	sessionsDir := filepath.Join(e.baseDir, scenario, "sessions", date)

	if r.SessionID == "" {
		return filepath.Join(sessionsDir, "_unknown.jsonl")
	}
	switch r.SessionSrc {
	case "ip":
		return filepath.Join(sessionsDir, "_ip", r.SessionID+".jsonl")
	default:
		return filepath.Join(sessionsDir, r.SessionID+".jsonl")
	}
}

// appendLines JSON-encodes each slim record and appends them to path,
// using the fd LRU cache to avoid repeated open/close.
func (e *CASFileExporter) appendLines(path string, lines []*SlimRecord) error {
	f := e.fdLRU.get(path)
	if f == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		e.fdLRU.put(path, f)
	}

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			return err
		}
	}
	return w.Flush()
}

// fileLRU is a bounded cache of open *os.File handles keyed by path.
// The least-recently-used entry is evicted (with fsync+close) when the
// capacity is exceeded. Not goroutine-safe; call only from a single goroutine.
type fileLRU struct {
	cap   int
	index map[string]*list.Element
	order *list.List // front = most recent
}

type lruEntry struct {
	path string
	file *os.File
}

func newFileLRU(cap int) *fileLRU {
	if cap <= 0 {
		cap = 256
	}
	return &fileLRU{
		cap:   cap,
		index: make(map[string]*list.Element),
		order: list.New(),
	}
}

func (l *fileLRU) get(path string) *os.File {
	elem, ok := l.index[path]
	if !ok {
		return nil
	}
	l.order.MoveToFront(elem)
	return elem.Value.(*lruEntry).file
}

func (l *fileLRU) put(path string, f *os.File) {
	if elem, ok := l.index[path]; ok {
		l.order.MoveToFront(elem)
		old := elem.Value.(*lruEntry)
		if old.file != f {
			syncAndClose(old.file)
			old.file = f
		}
		return
	}
	for l.order.Len() >= l.cap {
		l.evictLRU()
	}
	entry := &lruEntry{path: path, file: f}
	elem := l.order.PushFront(entry)
	l.index[path] = elem
}

func (l *fileLRU) closeAll() {
	for l.order.Len() > 0 {
		l.evictLRU()
	}
}

func (l *fileLRU) evictLRU() {
	back := l.order.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*lruEntry)
	syncAndClose(entry.file)
	delete(l.index, entry.path)
	l.order.Remove(back)
}

func syncAndClose(f *os.File) {
	if f != nil {
		_ = f.Sync()
		_ = f.Close()
	}
}
