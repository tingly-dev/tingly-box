package obs

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

// GzipFileExporter implements RecordExporter by appending one gzip member per
// batch to a per-session .jsonl.gz file. Records are emitted with all bodies
// inlined (no $ref / blob extraction).
//
// On-disk layout:
//
//	{baseDir}/{scenario}/sessions/{YYYY-MM-DD}/{session}.jsonl.gz
//
// Multiple gzip members concatenated in the same file remain a valid gzip
// stream; standard tools (zcat, gzip -d, gzip.Reader with Multistream(true))
// decompress them transparently.
type GzipFileExporter struct {
	baseDir string
	level   int
}

// NewGzipFileExporter creates an exporter rooted at baseDir.
func NewGzipFileExporter(baseDir string) *GzipFileExporter {
	return &GzipFileExporter{baseDir: baseDir, level: gzip.BestSpeed}
}

// Export groups records by session path and appends one gzip member per session.
func (e *GzipFileExporter) Export(_ context.Context, records []*Record) error {
	if len(records) == 0 {
		return nil
	}
	bySession := make(map[string][]*Record)
	for _, r := range records {
		path := e.sessionPath(r)
		bySession[path] = append(bySession[path], r)
	}
	var firstErr error
	for path, recs := range bySession {
		if err := e.appendMember(path, recs); err != nil {
			logrus.Warnf("obs: gzip append %s: %v", path, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Shutdown is a no-op; GzipFileExporter holds no persistent state.
func (e *GzipFileExporter) Shutdown(_ context.Context) error { return nil }

// sessionPath mirrors CASFileExporter.sessionPath but with a .jsonl.gz suffix.
func (e *GzipFileExporter) sessionPath(r *Record) string {
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
		return filepath.Join(sessionsDir, "_unknown.jsonl.gz")
	}
	switch r.SessionSrc {
	case "ip":
		return filepath.Join(sessionsDir, "_ip", r.SessionID+".jsonl.gz")
	default:
		return filepath.Join(sessionsDir, r.SessionID+".jsonl.gz")
	}
}

// appendMember writes a single gzip member containing one JSON line per record.
func (e *GzipFileExporter) appendMember(path string, recs []*Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := bufio.NewWriter(f)
	gw, err := gzip.NewWriterLevel(buf, e.level)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(gw)
	enc.SetEscapeHTML(false)
	for _, r := range recs {
		if err := enc.Encode(FullRecord(r)); err != nil {
			_ = gw.Close()
			return err
		}
	}
	if err := gw.Close(); err != nil {
		return err
	}
	if err := buf.Flush(); err != nil {
		return err
	}
	return f.Sync()
}
