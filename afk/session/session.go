// Package session persists an agent conversation as an append-only log.
//
// The log is JSONL: one entry per line, written with O_APPEND and never
// rewritten. That is the whole point of the format — a conversation is a
// sequence of things that happened, and the previous whole-file-rewrite store
// had to serialize the entire history on every turn and could leave a truncated
// file if the process died mid-write. Appending a line cannot corrupt the lines
// before it.
//
// Entries are typed rather than bare messages so a record that is not itself
// conversation can be added later without changing the file format again.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/sirupsen/logrus"
)

var logger = logrus.WithField("package", "afk.session")

// EntryType discriminates the records in the log.
type EntryType string

const (
	// EntryMessage carries one conversation message.
	EntryMessage EntryType = "message"
)

// Entry is a single record in the log.
//
// Typed even though there is currently one type: the envelope is the file
// format, and adding a kind of record later should not mean rewriting every
// existing log. Compaction, notably, does not need one — the API returns a
// compaction block inside the assistant message, so it persists as part of that
// message like any other content block.
type Entry struct {
	Type    EntryType                   `json:"type"`
	Message *anthropic.BetaMessageParam `json:"message,omitempty"`
}

// Session is an append-only conversation log backed by one file.
//
// It is not safe for concurrent use by multiple goroutines; callers serialize
// per conversation, which is how chats behave anyway.
type Session struct {
	path    string
	entries []Entry
}

// Open reads the log at path, creating nothing until something is appended. A
// missing file is an empty session, not an error — a chat's first turn has no
// history to read.
func Open(path string) (*Session, error) {
	s := &Session{path: path}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("open session %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	// Conversation turns carry tool output, so lines are far larger than
	// bufio's 64KB default; without this a long turn stops the scan mid-file
	// and the rest of the conversation silently disappears.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			// A single unreadable line must not cost the user the rest of the
			// conversation, which is exactly what the whole-file store did.
			logger.WithError(err).WithFields(logrus.Fields{
				"path": path, "line": lineNo,
			}).Warn("skipping unreadable session entry")
			continue
		}
		s.entries = append(s.entries, e)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read session %s: %w", path, err)
	}
	return s, nil
}

// Messages projects the log to the conversation the model sees.
func (s *Session) Messages() []anthropic.BetaMessageParam {
	msgs := make([]anthropic.BetaMessageParam, 0, len(s.entries))
	for _, e := range s.entries {
		if e.Type == EntryMessage && e.Message != nil {
			msgs = append(msgs, *e.Message)
		}
	}
	return msgs
}

// Len returns the number of messages currently in the log. It counts rather
// than calling Messages, which would copy the whole conversation to throw it
// away.
func (s *Session) Len() int {
	n := 0
	for _, e := range s.entries {
		if e.Type == EntryMessage && e.Message != nil {
			n++
		}
	}
	return n
}

// Append writes messages to the end of the log and to the in-memory view. It is
// all-or-nothing per call only in the sense that a failed write stops the
// remaining messages; entries already flushed stay, because that is what an
// append-only log means.
func (s *Session) Append(msgs ...anthropic.BetaMessageParam) error {
	// A nil *Session stored in an afk.Log interface is not == nil, so every
	// consumer would otherwise need its own typed-nil guard before calling this.
	// Making the value harmless is cheaper than filtering it at each caller.
	if s == nil || len(msgs) == 0 {
		return nil
	}
	entries := make([]Entry, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		entries = append(entries, Entry{Type: EntryMessage, Message: &m})
	}
	return s.appendEntries(entries...)
}

// appendEntries writes entries to the end of the log and to the in-memory view.
func (s *Session) appendEntries(entries ...Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	// 0600: this file is the user's whole conversation with the model.
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open session for append: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("encode session entry: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("append session entry: %w", err)
		}
		s.entries = append(s.entries, e)
	}
	return f.Sync()
}

// AppendNew treats full as an extension of what is already logged and appends
// only the part that is not.
// ImportLegacy seeds an empty log from a whole-file JSON array of messages,
// the format the previous store wrote. It exists so upgrading does not silently
// drop every existing conversation; it is a no-op once the log has entries.
func (s *Session) ImportLegacy(legacyPath string) error {
	if len(s.entries) > 0 {
		return nil
	}
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var msgs []anthropic.BetaMessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		logger.WithError(err).WithField("path", legacyPath).
			Warn("legacy session unreadable, starting fresh")
		return nil
	}
	if len(msgs) == 0 {
		return nil
	}
	if err := s.Append(msgs...); err != nil {
		return err
	}
	logger.WithFields(logrus.Fields{
		"from": legacyPath, "to": s.path, "messages": len(msgs),
	}).Info("imported legacy session into append-only log")
	return nil
}
