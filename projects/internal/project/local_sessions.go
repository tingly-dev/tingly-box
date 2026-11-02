package project

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude/session"
	"github.com/tingly-dev/tingly-box/agentboot/history"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// LocalSessions lists Claude Code histories for this folder.
func (m *Threads) LocalSessions(ctx context.Context, id string) ([]history.SessionMetadata, error) {
	p, err := m.store.GetProject(id)
	if err != nil {
		return nil, err
	}
	if p.Type != db.ProjectFolder {
		return nil, ErrNotFolderProject
	}
	folder, err := CleanFolder(p.Path)
	if err != nil {
		return nil, err
	}
	reader, err := session.NewStore("")
	if err != nil {
		return nil, err
	}
	return reader.ListSessions(ctx, folder)
}

// LoadLocalSession attaches an existing Claude history to a folder thread.
func (m *Threads) LoadLocalSession(ctx context.Context, id, sid string) (*db.Thread, error) {
	p, err := m.store.GetProject(id)
	if err != nil {
		return nil, err
	}
	if p.Type != db.ProjectFolder {
		return nil, ErrNotFolderProject
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if p.Paused {
		return nil, ErrPaused
	}
	folder, err := CleanFolder(p.Path)
	if err != nil {
		return nil, err
	}
	// Reopening the same session should select its existing thread.
	threads, err := m.store.ListThreads(id)
	if err != nil {
		return nil, err
	}
	for i := range threads {
		if threads[i].SessionID == sid {
			return &threads[i], nil
		}
	}
	reader, err := session.NewStore("")
	if err != nil {
		return nil, err
	}
	data, err := reader.ReadLocalSession(folder, sid)
	if err != nil {
		return nil, err
	}
	events, title, err := localTranscript(data)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = sid
	}
	t := &db.Thread{ProjectID: id, Agent: db.AgentClaudeCode, Title: firstLine(title, 60), Task: title, Status: db.ThreadIdle, Origin: "user", Cwd: folder, SessionID: sid, Turns: 1, HeadBranch: HeadBranch(ctx, folder)}
	if err = m.store.CreateImportedThread(t, events); err != nil {
		return nil, err
	}
	_ = m.store.TouchProject(id)
	m.publish(t)
	return t, nil
}

// localTranscript keeps visible messages and tool calls from Claude's JSONL.
func localTranscript(data []byte) ([]db.ThreadEvent, string, error) {
	var events []db.ThreadEvent
	title := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 32<<20)
	for scanner.Scan() {
		var row struct {
			Type        string    `json:"type"`
			Timestamp   time.Time `json:"timestamp"`
			IsSidechain bool      `json:"isSidechain"`
			Parent      string    `json:"parentToolUseID"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.IsSidechain || row.Parent != "" || (row.Type != "user" && row.Type != "assistant") {
			continue
		}
		add := func(kind, content, payload, request string) {
			if content != "" || payload != "" {
				events = append(events, db.ThreadEvent{Kind: kind, Content: content, Payload: payload, RequestID: request, CreatedAt: row.Timestamp})
			}
		}
		var plain string
		if json.Unmarshal(row.Message.Content, &plain) == nil {
			if row.Type == "user" && strings.TrimSpace(plain) != "" {
				add(db.EvUser, plain, "", "")
				if title == "" {
					title = plain
				}
			}
			continue
		}
		var blocks []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
		}
		if json.Unmarshal(row.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if row.Type == "user" {
					add(db.EvUser, b.Text, "", "")
					if title == "" {
						title = b.Text
					}
				} else {
					add(db.EvAssistant, b.Text, "", "")
				}
			case "tool_use":
				add(db.EvToolUse, b.Name, string(b.Input), b.ID)
			case "tool_result":
				var text string
				if json.Unmarshal(b.Content, &text) != nil {
					text = string(b.Content)
				}
				add(db.EvToolResult, text, "", b.ToolUseID)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, "", err
	}
	if len(events) == 0 {
		return nil, "", errors.New("session contains no readable messages")
	}
	return events, title, nil
}
