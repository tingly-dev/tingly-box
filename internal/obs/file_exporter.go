package obs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// FileExporter implements RecordExporter. On each Export call it:
//  1. Slim-ifies every Record (content-addressed dedup via in-memory blob set).
//  2. Writes new blobs atomically (tmp+rename).
//  3. Appends SlimRecords to per-session JSONL files.
//
// FileExporter is NOT goroutine-safe; it must be driven by a single goroutine
// (the BatchProcessor worker).
type FileExporter struct {
	baseDir  string
	blobSet  map[string]struct{} // hashes known to be on disk (or written this run)
	fdLRU    *fileLRU
}

// NewFileExporter creates a FileExporter rooted at baseDir and populates the
// in-memory blob set by scanning existing blobs.
func NewFileExporter(baseDir string) (*FileExporter, error) {
	set, err := scanBlobSet(baseDir)
	if err != nil {
		logrus.Warnf("obs: blob set scan failed, starting fresh: %v", err)
		set = make(map[string]struct{})
	}
	return &FileExporter{
		baseDir: baseDir,
		blobSet: set,
		fdLRU:   newFileLRU(256),
	}, nil
}

// Export processes a batch of Records.
func (e *FileExporter) Export(_ context.Context, records []*Record) error {
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
func (e *FileExporter) Shutdown(_ context.Context) error {
	e.fdLRU.closeAll()
	return nil
}

// sessionPath returns the JSONL path for a record.
// Layout: {baseDir}/{scenario}/sessions/{YYYY-MM-DD}/{session}.jsonl
// Falls back to _unknown/{YYYY-MM-DD}.jsonl when the session is empty.
func (e *FileExporter) sessionPath(r *Record) string {
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
func (e *FileExporter) appendLines(path string, lines []*SlimRecord) error {
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

// writeIndexEntry appends a one-line index entry the first time a session
// is seen in a date partition. The index lives at sessions/{date}/_index.jsonl.
// This is best-effort; errors are logged but not propagated.
func (e *FileExporter) writeIndexEntry(baseDir, scenario, date, sessionID, source string) {
	indexPath := filepath.Join(baseDir, scenario, "sessions", date, "_index.jsonl")
	entry := map[string]interface{}{
		"ts":     time.Now().UTC().Format(time.RFC3339),
		"sid":    sessionID,
		"source": source,
	}
	data, _ := json.Marshal(entry)
	data = append(data, '\n')
	f, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logrus.Debugf("obs: failed to open index %s: %v", indexPath, err)
		return
	}
	_, _ = f.Write(data)
	_ = f.Sync()
	_ = f.Close()
}
