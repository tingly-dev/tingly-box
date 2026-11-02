package localsessions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"time"
)

// Preview uses file-version-bound byte cursors rather than event offsets.
// Even files with a multi-gigabyte single line advance within a fixed budget.
func (s *Store) Preview(ctx context.Context, id, cursor, direction string) (Page, error) {
	out := Page{Events: []Event{}}
	target, err := s.decode(id, "session")
	if err != nil {
		return out, err
	}
	root, err := s.openRoot()
	if err != nil {
		return out, ErrMissing
	}
	defer root.Close()
	f, err := safeOpen(root, target.Rel, false)
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return out, ErrMissing
	}
	var t token
	if cursor != "" {
		t, err = s.decode(cursor, "events")
		if err != nil || t.Rel != target.Rel {
			return out, ErrInvalid
		}
		s.mu.Lock()
		s.pruneLocked()
		snap, ok := s.snapshots[t.Key]
		s.mu.Unlock()
		if !ok || snap.rel != target.Rel || !os.SameFile(snap.info, info) || snap.info.Size() != info.Size() || !snap.info.ModTime().Equal(info.ModTime()) {
			return out, ErrChanged
		}
	} else {
		if direction != "forward" && direction != "backward" {
			return out, ErrInvalid
		}
		t = token{Kind: "events", Rel: target.Rel, Key: randomID(), Direction: direction}
		if direction == "backward" {
			t.Offset = info.Size()
		}
		s.mu.Lock()
		s.pruneLocked()
		if len(s.snapshots) >= 64 {
			delete(s.snapshots, oldestSnapshot(s.snapshots))
		}
		s.snapshots[t.Key] = snapshot{rel: target.Rel, info: info, expires: time.Now().Add(10 * time.Minute)}
		s.mu.Unlock()
	}
	if t.Offset < 0 || t.Offset > info.Size() || (t.Direction != "backward" && t.Direction != "forward") {
		return out, ErrInvalid
	}
	out.Direction = t.Direction
	out.FileSize = info.Size()
	start, end := t.Offset, min(info.Size(), t.Offset+MaxReadBytes)
	if t.Direction == "backward" {
		start, end = max(int64(0), t.Offset-MaxReadBytes+1), t.Offset
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	buf := make([]byte, end-start)
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return out, ErrMissing
	}
	buf = buf[:n]
	out.ScannedBytes = n
	pos := 0
	budget := 0
	consume := func(line []byte, offset int64) bool {
		if len(bytes.TrimSpace(line)) == 0 {
			return true
		}
		if len(line) > MaxRecordBytes {
			out.SkippedRecords++
			return true
		}
		event, ok := projectEvent(line, offset)
		if !ok {
			out.SkippedRecords++
			return true
		}
		encoded, _ := json.Marshal(event)
		if len(encoded) > MaxPageJSON {
			event.Blocks = []Block{{Kind: "notice", Label: "event_too_large", Truncated: true}}
			encoded, _ = json.Marshal(event)
		}
		if len(out.Events) >= MaxEvents || budget+len(encoded) > MaxPageJSON {
			return false
		}
		budget += len(encoded)
		out.Events = append(out.Events, event)
		return true
	}
	if t.Direction == "forward" {
		if t.Skip {
			if i := bytes.IndexByte(buf, '\n'); i >= 0 {
				pos = i + 1
				t.Skip = false
			} else {
				pos = len(buf)
			}
		}
		for pos < len(buf) && !t.Skip {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			i := bytes.IndexByte(buf[pos:], '\n')
			next := len(buf)
			if i >= 0 {
				next = pos + i + 1
			} else if end < info.Size() {
				t.Skip = true
				out.SkippedRecords++
				pos = len(buf)
				break
			}
			if !consume(bytes.TrimSuffix(buf[pos:next], []byte{'\n'}), start+int64(pos)) {
				break
			}
			pos = next
		}
		t.Offset = start + int64(pos)
	} else {
		pos = len(buf)
		if t.Skip {
			if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
				pos = i + 1
				t.Skip = false
			} else {
				pos = 0
			}
		}
		for pos > 0 && !t.Skip {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			lineEnd := pos
			if buf[lineEnd-1] == '\n' {
				lineEnd--
			}
			before := bytes.LastIndexByte(buf[:lineEnd], '\n')
			lineStart := before + 1
			if before < 0 && start > 0 {
				var prev [1]byte
				n, e := f.ReadAt(prev[:], start-1)
				out.ScannedBytes += n
				if e != nil || prev[0] != '\n' {
					t.Skip = true
					out.SkippedRecords++
					pos = 0
					break
				}
			}
			if !consume(buf[lineStart:lineEnd], start+int64(lineStart)) {
				break
			}
			pos = lineStart
		}
		t.Offset = start + int64(pos)
		slices.Reverse(out.Events)
	}
	// Refuse a torn page if the CLI appended, truncated or replaced the file
	// during this read. The next explicit refresh starts a fresh snapshot.
	after, err := f.Stat()
	if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Page{}, ErrChanged
	}
	if (t.Direction == "forward" && t.Offset < info.Size()) || (t.Direction == "backward" && t.Offset > 0) {
		out.NextCursor = s.encode(t)
	}
	return out, nil
}

type nativeBlock struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Thinking string          `json:"thinking"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Content  json.RawMessage `json:"content"`
}

type nativeEvent struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	UUID       string `json:"uuid"`
	ParentUUID string `json:"parentUuid"`
	Timestamp  string `json:"timestamp"`
	Sidechain  bool   `json:"isSidechain"`
	Summary    string `json:"summary"`
	Message    struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func projectEvent(line []byte, offset int64) (Event, bool) {
	var n nativeEvent
	if json.Unmarshal(line, &n) != nil {
		return Event{}, false
	}
	event := Event{ID: shorten(n.UUID, 160), Type: shorten(n.Type, 80), Role: n.Message.Role, Timestamp: shorten(n.Timestamp, 80), ParentID: shorten(n.ParentUUID, 160), Sidechain: n.Sidechain, Blocks: []Block{}}
	if event.ID == "" {
		event.ID = fmt.Sprintf("byte:%d", offset)
	}
	switch n.Type {
	case "user", "assistant":
		if n.Message.Role != "user" && n.Message.Role != "assistant" {
			event.Role = n.Type
		}
		var text string
		if json.Unmarshal(n.Message.Content, &text) == nil {
			event.Blocks = append(event.Blocks, textBlock("text", "", text))
		} else {
			blocks, _, err := boundedNativeBlocks(n.Message.Content, MaxBlocks+1)
			if err != nil {
				return Event{}, false
			}
			for _, b := range blocks {
				if len(event.Blocks) >= MaxBlocks {
					event.Blocks = append(event.Blocks, Block{Kind: "notice", Label: "additional_blocks_omitted", Truncated: true})
					break
				}
				switch b.Type {
				case "text":
					event.Blocks = append(event.Blocks, textBlock("text", "", b.Text))
				case "thinking":
					event.Blocks = append(event.Blocks, textBlock("thinking", "", b.Thinking))
				case "tool_use":
					event.Blocks = append(event.Blocks, textBlock("tool_use", shorten(b.Name, 120), string(b.Input)))
				case "tool_result":
					var result string
					moreParts := false
					if json.Unmarshal(b.Content, &result) != nil {
						parts, more, err := boundedNativeBlocks(b.Content, MaxBlocks)
						moreParts = more
						if err == nil {
							for _, p := range parts {
								if p.Type == "text" {
									result += shorten(p.Text, MaxBlockText) + "\n"
									if len(result) > MaxBlockText {
										break
									}
								}
							}
						}
					}
					resultBlock := textBlock("tool_result", "", result)
					resultBlock.Truncated = resultBlock.Truncated || moreParts
					event.Blocks = append(event.Blocks, resultBlock)
				case "image", "document":
					event.Blocks = append(event.Blocks, Block{Kind: "attachment", Label: shorten(b.Type, 80)})
				default:
					event.Blocks = append(event.Blocks, Block{Kind: "notice", Label: shorten(b.Type, 80)})
				}
			}
		}
	case "summary":
		event.Role = "system"
		event.Blocks = append(event.Blocks, textBlock("summary", "", n.Summary))
	case "system":
		event.Role = "system"
		event.Blocks = append(event.Blocks, Block{Kind: "notice", Label: shorten(n.Subtype, 120)})
	default:
		return Event{}, false
	}
	return event, len(event.Blocks) > 0
}
func textBlock(kind, label, text string) Block {
	return Block{Kind: kind, Label: label, Text: shorten(text, MaxBlockText), Truncated: len(text) > MaxBlockText}
}

// Decode only the blocks we can display. A small JSON line containing hundreds
// of thousands of tiny objects must not expand into a huge Go slice.
func boundedNativeBlocks(raw json.RawMessage, limit int) ([]nativeBlock, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('[') {
		return nil, false, fmt.Errorf("expected content blocks")
	}
	result := make([]nativeBlock, 0, limit)
	for decoder.More() {
		if len(result) == limit {
			return result, true, nil
		}
		var block nativeBlock
		if err := decoder.Decode(&block); err != nil {
			return nil, false, err
		}
		result = append(result, block)
	}
	_, err = decoder.Token()
	return result, false, err
}
