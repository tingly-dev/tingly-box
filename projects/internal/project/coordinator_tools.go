package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// textTool adapts a handler that takes typed input and answers with text. A
// returned error becomes a failed tool result the model reads.
func textTool[T any](fn func(ctx context.Context, p *db.Project, in T) (string, error)) func(context.Context, ToolCall) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, call ToolCall) (*mcp.CallToolResult, error) {
		var in T
		if err := json.Unmarshal(call.Input, &in); err != nil {
			return textResult("invalid input: "+err.Error(), true), nil
		}
		out, err := fn(ctx, call.Project, in)
		if err != nil {
			return textResult(err.Error(), true), nil
		}
		return textResult(out, false), nil
	}
}

// coordinatorTools are the coordinator's tools: each with its schema, who may
// call it, and the method that runs it.
func (c *Coordinator) coordinatorTools() []Tool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	threadSpec := map[string]any{
		"title":  str("Thread card title: starts with a verb that names the goal, three to six words, no ids or paths (for example \"Fix Safari export button\")."),
		"task":   str("Self-contained instructions for the thread: goal, context, acceptance criteria, how to verify."),
		"repo":   str("Optional. Path of the project repository to work in; \"none\" for no repository."),
		"base":   str("Optional. Existing local branch the thread's branch starts from, and that its work is merged back into. Default: the repository's base branch. Use it for work that builds on a release or feature branch."),
		"branch": str("Optional. Name for the thread's new branch; it must not exist yet. Default: generated from the title."),
	}
	return []Tool{
		{Name: "start_thread", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Start a new thread (a Claude Code session) for one task, using the project worktree setting.", Properties: threadSpec, Required: []string{"title", "task"}, Handle: textTool(c.startThread)},
		{Name: "propose_threads", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Propose threads for the user to start, instead of starting them. Use when the user wants to review a plan first.",
			Properties: map[string]any{"threads": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": threadSpec, "required": []string{"title", "task"}}}},
			Required:   []string{"threads"}, Handle: textTool(c.proposeThreads)},
		{Name: "request_decision", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Ask the user to pick one option on a card, when a choice changes their goal or a step cannot be undone. Ask once, with two to four options and one recommended; do not repeat the question in text. Keep working on the recommended option while you wait, unless that step cannot be undone.",
			Properties: map[string]any{
				"question": str("The question, verb first, in about twelve words."),
				"context":  str("Background the user needs to choose. Optional."),
				"options": map[string]any{"type": "array", "minItems": 2, "maxItems": 4, "items": map[string]any{"type": "object", "properties": map[string]any{
					"label":       str("The option, a few words."),
					"consequence": str("What happens if chosen, one sentence."),
				}, "required": []string{"label"}}},
				"recommended": map[string]any{"type": "integer", "description": "Index of the recommended option, from 0."},
				"reason":      str("Why you recommend it, one sentence."),
			},
			Required: []string{"question", "options", "recommended"}, Handle: textTool(c.requestDecision)},
		{Name: "message_thread", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Send a message to an existing thread (follow-up work, an answer to its question). It runs after the thread's current turn.",
			Properties: map[string]any{"thread_id": str("Thread id (the 8-character prefix is enough)."), "message": str("What to tell the thread."),
				"cite": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Ids (the msg attribute of the user's event) of the user's own messages that approve or decide what you are telling the thread. The system quotes them to the thread word for word; without a citation the thread treats your message as not approved."}},
			Required: []string{"thread_id", "message"}, Handle: textTool(c.messageThread)},
		{Name: "get_thread", Scopes: ScopeCoordinator, Risk: RiskRead, Description: "Read a thread's status, branch, last report and recent transcript. Entries are cut and the result capped like read_conversation; page back with before, read entries whole with full.",
			Properties: map[string]any{"thread_id": str("Thread id or its 8-character prefix."),
				"before": str("Optional. 8-character id of a transcript entry to read back from."),
				"limit":  map[string]any{"type": "integer", "description": "How many entries, up to 40. Default 15."},
				"full":   map[string]any{"type": "boolean", "description": "Show each entry up to 4000 characters instead of 300."}}, Required: []string{"thread_id"}, Handle: textTool(c.getThread)},
		{Name: "read_conversation", Scopes: ScopeCoordinator, Risk: RiskRead, Description: "Read earlier messages of this conversation that are no longer shown to you: the user's words, your replies and thread reports. Newest first, back from the window you see or from a message id; or search by text. Results are capped like grep output: long messages are cut, and when the result is too big the oldest entries are hidden and you are told how to reach them. Use it when the user refers to something you cannot see; do not ask them to repeat.",
			Properties: map[string]any{"before": str("Optional. 8-character id of a message (shown in the results) to read back from; default is the oldest message you can see."),
				"contains": str("Optional. Only messages containing this text (case-insensitive)."),
				"id":       str("Optional. Read this one message whole instead (use the id of a cut-short entry)."),
				"limit":    map[string]any{"type": "integer", "description": "How many messages, up to 40. Default 20."},
				"full":     map[string]any{"type": "boolean", "description": "Show each message up to 4000 characters instead of 600 (fewer fit in one result)."}}, Handle: textTool(c.readConversationTool)},
		{Name: "list_threads", Scopes: ScopeCoordinator, Risk: RiskRead, Description: "List the project's threads, including old and archived ones the thread table in your prompt leaves out. Capped like ls output: when too many match, the oldest are hidden and you are told to narrow it.",
			Properties: map[string]any{"status": str("Optional. Only threads with this status, e.g. working, waiting, ready, resolved."),
				"contains": str("Optional. Only threads whose title, task or last report contains this text."),
				"limit":    map[string]any{"type": "integer", "description": "How many threads, up to 40. Default 20."}}, Handle: textTool(c.listThreadsTool)},
		{Name: "resolve_thread", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Mark a thread resolved (done). Only when the user says so or its work is merged.",
			Properties: map[string]any{"thread_id": str("Thread id or prefix.")}, Required: []string{"thread_id"}, Handle: textTool(c.resolveThread)},
		{Name: "rename_thread", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Change a thread's card title when its goal has changed. Does not touch its branch or history. Same title rules as start_thread.",
			Properties: map[string]any{"thread_id": str("Thread id or its 8-character prefix."), "title": str("New title: starts with a verb that names the goal, three to six words, no ids or paths.")}, Required: []string{"thread_id", "title"}, Handle: textTool(c.renameThread)},
		{Name: "no_update", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "End this turn without posting anything to the user. Use it when a thread update or settings change needs no message. Call it instead of replying with text.",
			Properties: map[string]any{"reason": str("Optional. Why nothing needs saying.")}, Handle: textTool(c.noUpdate)},
		{Name: "remember", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Save a requirement, decision, pitfall or coordination preference to project memory. Every new thread and coordinator turn sees the memory index. Reuse the topic of an existing file to update it instead of adding a duplicate; one fact per file; skip what the repository already shows.",
			Properties: map[string]any{"topic": str("Short topic; becomes the file name."), "content": str("The note."), "summary": str("One line for the MEMORY.md index.")}, Required: []string{"topic", "content", "summary"}, Handle: textTool(c.remember)},
		{Name: "forget", Scopes: ScopeCoordinator, Risk: RiskWrite, Description: "Delete a memory file.", Properties: map[string]any{"file": str("Memory file name, e.g. release-date.md.")}, Required: []string{"file"}, Handle: textTool(c.forget)},
		{Name: "update_instructions", Scopes: ScopeCoordinator, Risk: RiskWrite, EditsProject: true, Description: "Replace the project instructions (only when the user asks).",
			Properties: map[string]any{"instructions": str("The full new instructions (max 16000 characters).")}, Required: []string{"instructions"}, Handle: textTool(c.updateInstructions)},
	}
}

func (c *Coordinator) startThread(ctx context.Context, p *db.Project, in struct {
	Title, Task, Repo, Base, Branch string
}) (string, error) {
	if strings.TrimSpace(in.Task) == "" {
		return "", errors.New("task is required")
	}
	t, err := c.threads.Start(ctx, p, StartSpec{Title: in.Title, Task: in.Task, Repo: in.Repo, Base: in.Base, Branch: in.Branch, Origin: "coordinator"})
	if err != nil {
		return "", err
	}
	c.addEvent(p.ID, &db.ThreadEvent{Kind: db.EvThreadStarted, Payload: jsonString(map[string]any{"thread_ids": []string{t.ID}})})
	return fmt.Sprintf("started thread %s %q (branch %s, base %s)", short(t.ID), t.Title, orNone(t.Branch), orNone(t.BaseBranch)), nil
}

func (c *Coordinator) proposeThreads(_ context.Context, p *db.Project, in struct{ Threads []Suggestion }) (string, error) {
	if len(in.Threads) == 0 {
		return "", errors.New("threads is empty")
	}
	c.addEvent(p.ID, &db.ThreadEvent{Kind: db.EvSuggestions, Payload: jsonString(suggestionsPayload{Suggestions: in.Threads})})
	return fmt.Sprintf("proposed %d threads; the user can start them from the card", len(in.Threads)), nil
}

func (c *Coordinator) requestDecision(_ context.Context, p *db.Project, d Decision) (string, error) {
	if err := d.validate(); err != nil {
		return "", err
	}
	return c.ask(p.ID, d), nil
}

func (c *Coordinator) messageThread(ctx context.Context, p *db.Project, in struct {
	ThreadID string `json:"thread_id"`
	Message  string
	Cite     []string
}) (string, error) {
	t, err := c.findThread(p.ID, in.ThreadID)
	if err != nil {
		return "", err
	}
	busy := c.threads.Running(t.ID)
	msg := in.Message
	if strings.Contains(strings.ToLower(msg), quoteMarker) {
		return "", fmt.Errorf("the message may not contain %q: only the system writes the user's quoted words, pass the user's message ids in cite", quoteMarker)
	}
	if len(in.Cite) > 0 {
		quote, err := c.quoteUser(p, in.Cite)
		if err != nil {
			return "", err
		}
		msg += quote
	}
	if _, err := c.threads.Send(ctx, t.ID, msg, fromThread(p.MainThreadID), nil); err != nil {
		return "", err
	}
	if busy {
		return "queued; the thread will read it after its current turn", nil
	}
	return "sent; the thread is working on it", nil
}

func (c *Coordinator) getThread(_ context.Context, p *db.Project, in struct {
	ThreadID string `json:"thread_id"`
	Before   string
	Limit    int
	Full     bool
}) (string, error) {
	t, err := c.findThread(p.ID, in.ThreadID)
	if err != nil {
		return "", err
	}
	// The thread's own conversation: a subagent's steps stay out.
	page := db.EventPage{Limit: 15}
	if in.Limit > 0 {
		page.Limit = in.Limit
		if page.Limit > readItemsMax {
			page.Limit = readItemsMax
		}
	}
	if in.Before != "" {
		e, err := c.store.FindThreadEvent(t.ID, in.Before)
		if err != nil {
			return "", fmt.Errorf("no transcript entry %q in this thread", in.Before)
		}
		page.Before = e.ID
	}
	events, more, _ := c.store.PageThreadEvents(t.ID, page)
	var entries []readEntry
	for _, e := range events {
		if e.Kind == db.EvUsage || e.Kind == db.EvTask {
			continue
		}
		entries = append(entries, readEntry{short(e.ID), e.Kind, e.Content})
	}
	lines, oldest, hidden := fit(entries, in.Full, func(id string) string { return "get_thread full for the whole" })
	lines += footer(hidden, more, oldest, "get_thread")
	t.RunningTasks = c.threads.RunningTasks(t.ID)
	return envelope(trustRelay, fmt.Sprintf("thread %s %q status=%s%s branch=%s base=%s commits_ahead=%d files_changed=%d waiting_on=%q\ntask: %s\nlast report: %s\nrecent transcript:\n%s",
		short(t.ID), t.Title, t.Status, tasksNote(t.RunningTasks), orNone(t.Branch), orNone(t.BaseBranch), t.CommitsAhead, t.FilesChanged, t.WaitReason,
		clip(t.Task, 1500), clip(t.LastReport, 2000), lines), "source", "thread", "kind", "get_thread"), nil
}

func (c *Coordinator) readConversationTool(_ context.Context, p *db.Project, in struct {
	Before, Contains, ID string
	Limit                int
	Full                 bool
}) (string, error) {
	return c.readConversation(p, readArgs{Before: in.Before, Contains: in.Contains, ID: in.ID, Limit: in.Limit, Full: in.Full})
}

func (c *Coordinator) listThreadsTool(_ context.Context, p *db.Project, in struct {
	Status, Contains string
	Limit            int
}) (string, error) {
	return c.listThreads(p, in.Status, in.Contains, in.Limit)
}

func (c *Coordinator) renameThread(_ context.Context, p *db.Project, in struct {
	ThreadID string `json:"thread_id"`
	Title    string
}) (string, error) {
	t, err := c.findThread(p.ID, in.ThreadID)
	if err != nil {
		return "", err
	}
	if _, err := c.threads.SetTitle(t.ID, in.Title); err != nil {
		return "", err
	}
	return "renamed to " + strings.TrimSpace(in.Title), nil
}

func (c *Coordinator) resolveThread(_ context.Context, p *db.Project, in struct {
	ThreadID string `json:"thread_id"`
}) (string, error) {
	t, err := c.findThread(p.ID, in.ThreadID)
	if err != nil {
		return "", err
	}
	if _, err := c.threads.Resolve(t.ID); err != nil {
		return "", err
	}
	return "resolved", nil
}

// noUpdate ends the turn; the gate (coordinatorToolTurn.run) does the rest.
func (c *Coordinator) noUpdate(context.Context, *db.Project, struct{ Reason string }) (string, error) {
	return "ok", nil
}

func (c *Coordinator) remember(_ context.Context, p *db.Project, in struct{ Topic, Content, Summary string }) (string, error) {
	name, err := c.layout.Remember(p.ID, in.Topic, in.Content, in.Summary)
	if err != nil {
		return "", err
	}
	c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: map[string]string{"memory": name}})
	return "saved to memory as " + name, nil
}

func (c *Coordinator) forget(_ context.Context, p *db.Project, in struct{ File string }) (string, error) {
	if err := c.layout.DeleteMemory(p.ID, in.File); err != nil {
		return "", err
	}
	c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: map[string]string{"memory": in.File}})
	return "deleted " + in.File, nil
}

func (c *Coordinator) updateInstructions(_ context.Context, p *db.Project, in struct{ Instructions string }) (string, error) {
	if len([]rune(in.Instructions)) > MaxInstructions {
		return "", fmt.Errorf("instructions exceed %d characters", MaxInstructions)
	}
	p.Instructions = in.Instructions
	if err := c.store.SaveProject(p); err != nil {
		return "", err
	}
	c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: p})
	return "instructions updated", nil
}
