package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultSummaryFile is the path the harness writes per-row results to when
// --summary is not specified. A fixed name in CWD makes --resume trivial
// ("just rerun with --resume") and matches the user-requested behavior.
const DefaultSummaryFile = "harness-summary.csv"

// summaryCSVColumns is the canonical column order of the summary CSV.
// The CSV contains only metadata; full prompt/output are written to
// separate markdown files in harness-output/ and referenced by output_id.
var summaryCSVColumns = []string{
	"timestamp",
	"agent",
	"entry",
	"model",
	"api_style",
	"request_model",
	"provider_baseurl",
	"status",
	"duration_ms",
	"exit_code",
	"output_id",      // reference to output file in harness-output/
	"prompt_summary", // first 50 chars of prompt
	"error",          // kept short for in-CSV viewing
}

// summaryWriter persists per-row results to a CSV file as soon as each row
// completes. The file is opened in append mode so a Ctrl-C / panic mid-run
// leaves the rows up to that point intact on disk.
type summaryWriter struct {
	path string
	f    *os.File
	w    *csv.Writer
	ow   *outputWriter // output writer for full content
}

// openSummaryWriter opens (or creates) the summary CSV at path in append mode.
// If the file is new or empty, the column header row is written first.
// Also initializes the output writer for full content files.
func openSummaryWriter(path string, outputDir string) (*summaryWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("open summary file %s: %w", path, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat summary file %s: %w", path, err)
	}
	w := csv.NewWriter(f)
	if st.Size() == 0 {
		if err := w.Write(summaryCSVColumns); err != nil {
			f.Close()
			return nil, fmt.Errorf("write summary header: %w", err)
		}
		w.Flush()
		if err := w.Error(); err != nil {
			f.Close()
			return nil, fmt.Errorf("flush summary header: %w", err)
		}
	}

	// Initialize output writer
	ow, err := openOutputWriter(outputDir)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("open output writer: %w", err)
	}

	return &summaryWriter{path: path, f: f, w: w, ow: ow}, nil
}

// Append writes one result row and flushes immediately so the row survives
// abrupt termination. Full content is written to a separate file; only
// metadata and a summary are written to the CSV.
func (s *summaryWriter) Append(r *RealAgentTestResult) error {
	if s == nil {
		return nil
	}

	// Write full output to file
	outputID := ""
	if s.ow != nil {
		id, err := s.ow.Write(r)
		if err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		outputID = id
	}

	status := "FAIL"
	switch {
	case r.Success:
		status = "PASS"
	case r.TimedOut:
		status = "TIMEOUT"
	}

	// Generate prompt summary (first 50 chars)
	promptSummary := truncateString(r.Prompt, 50)

	row := []string{
		time.Now().Format(time.RFC3339),
		r.Agent,
		r.EntryName,
		r.Model,
		r.APIStyle,
		r.RequestModel,
		r.BaseURL,
		status,
		strconv.FormatInt(r.Duration.Milliseconds(), 10),
		strconv.Itoa(r.ExitCode),
		outputID,
		promptSummary,
		r.Error,
	}
	if err := s.w.Write(row); err != nil {
		return fmt.Errorf("write summary row: %w", err)
	}
	s.w.Flush()
	if err := s.w.Error(); err != nil {
		return fmt.Errorf("flush summary row: %w", err)
	}
	return s.f.Sync()
}

// Close flushes pending output and closes the underlying file. Safe to call
// on a nil writer.
func (s *summaryWriter) Close() error {
	if s == nil {
		return nil
	}
	s.w.Flush()
	if err := s.w.Error(); err != nil {
		_ = s.f.Close()
		_ = s.ow.Close()
		return err
	}
	_ = s.ow.Close()
	return s.f.Close()
}

// resumeKey is the (agent, entry) tuple used to deduplicate previously-recorded
// runs when --resume is set. Per the chosen semantics ("skip every recorded
// row"), we treat any prior row as authoritative regardless of status.
type resumeKey struct {
	Agent string
	Entry string
}

// loadResumeKeys reads the summary CSV and returns the set of (agent, entry)
// pairs already recorded. A missing file returns an empty (non-nil) set so
// `--resume` is safe to specify before the first run.
func loadResumeKeys(path string) (map[resumeKey]struct{}, error) {
	keys := make(map[resumeKey]struct{})
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return keys, nil
		}
		return nil, fmt.Errorf("open summary file %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate older rows with different column counts

	header, err := r.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return keys, nil
		}
		return nil, fmt.Errorf("read summary header: %w", err)
	}
	agentIdx, entryIdx := indexOf(header, "agent"), indexOf(header, "entry")
	if agentIdx < 0 || entryIdx < 0 {
		return nil, fmt.Errorf("summary file %s missing required columns 'agent' and 'entry'", path)
	}

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read summary row: %w", err)
		}
		if agentIdx >= len(row) || entryIdx >= len(row) {
			continue
		}
		keys[resumeKey{
			Agent: strings.TrimSpace(row[agentIdx]),
			Entry: strings.TrimSpace(row[entryIdx]),
		}] = struct{}{}
	}
	return keys, nil
}

func indexOf(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}
