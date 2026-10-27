package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// LLMSource gives the coordinator a model client. model may be empty for
// the connection's default; the resolved model is returned.
type LLMSource interface {
	ClientFor(ctx context.Context, model string) (llm.Client, string, error)
}

const (
	// historyLimit is how many recent conversation entries a coordinator
	// turn sees. Like the official coordinator, it works from recent
	// messages, the thread table and memory rather than the full history.
	historyLimit = 40
	// maxToolRounds bounds tool use within one coordinator turn.
	maxToolRounds = 8
	noUpdate      = "NO_UPDATE"
)

// Coordinator runs the project conversation: one serial queue per project.
type Coordinator struct {
	store   *db.Store
	layout  Layout
	hub     *Hub
	threads *Threads
	llm     LLMSource
	// OnFinal is called with each finished coordinator reply, thread report
	// and error, once, in order. It must not block.
	OnFinal func(db.ThreadEvent)

	mu     sync.Mutex
	queues map[string]*queue
}

type queue struct {
	running bool
	user    bool
	config  bool
	reports []Report
}

// NewCoordinator wires the coordinator to the thread manager's reports.
func NewCoordinator(store *db.Store, layout Layout, hub *Hub, threads *Threads, src LLMSource) *Coordinator {
	c := &Coordinator{store: store, layout: layout, hub: hub, threads: threads, llm: src, queues: map[string]*queue{}}
	threads.OnReport = c.onReport
	return c
}

func (c *Coordinator) publishEvent(projectID string, e *db.ThreadEvent) {
	c.hub.Publish(Event{Type: EventThreadEvent, ProjectID: projectID, Data: e})
}

// addEvent appends to the project's conversation, which is its main thread's
// events.
func (c *Coordinator) addEvent(projectID string, e *db.ThreadEvent) *db.ThreadEvent {
	p, err := c.store.GetProject(projectID)
	if err != nil || p.MainThreadID == "" {
		logrus.WithError(err).Warn("project has no main thread")
		return e
	}
	e.ThreadID = p.MainThreadID
	if err := c.store.AddThreadEvent(e); err != nil {
		logrus.WithError(err).Warn("save conversation event")
		return e
	}
	c.publishEvent(projectID, e)
	return e
}

// Post adds a user message to the project's conversation (its main thread)
// and runs a coordinator turn. src says where the message came from.
func (c *Coordinator) Post(projectID, text string, atts []Attachment, src Source) (*db.ThreadEvent, error) {
	p, err := c.store.GetProject(projectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	if p.Paused {
		return nil, ErrPaused
	}
	m := &db.ThreadEvent{Kind: db.EvUser, Content: text, Payload: AttachmentsPayload(atts)}
	src.stamp(m)
	c.addEvent(projectID, m)
	_ = c.store.TouchProject(projectID)
	c.schedule(projectID, func(q *queue) { q.user = true })
	return m, nil
}

// Busy reports whether a coordinator turn is running or queued.
func (c *Coordinator) Busy(projectID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	q, ok := c.queues[projectID]
	return ok && (q.running || q.user || q.config || len(q.reports) > 0)
}

// final hands a message the coordinator is done with (a finished reply, a
// thread's report, an error) to OnFinal, where the host's IM chats hear it.
func (c *Coordinator) final(m *db.ThreadEvent) {
	if c.OnFinal != nil && m != nil {
		c.OnFinal(*m)
	}
}

func (c *Coordinator) onReport(r Report) {
	c.final(c.addEvent(r.Thread.ProjectID, &db.ThreadEvent{
		Kind:    db.EvThreadUpdate,
		Content: r.Text,
		Payload: jsonString(map[string]any{"thread_id": r.Thread.ID, "seq": r.Thread.Seq, "title": r.Thread.Title, "kind": r.Kind, "status": r.Thread.Status}),
	}))
	p, err := c.store.GetProject(r.Thread.ProjectID)
	if err != nil || p.Paused || p.ArchivedAt != nil {
		return
	}
	c.schedule(r.Thread.ProjectID, func(q *queue) { q.reports = append(q.reports, r) })
}

func (c *Coordinator) schedule(projectID string, add func(*queue)) {
	c.mu.Lock()
	q, ok := c.queues[projectID]
	if !ok {
		q = &queue{}
		c.queues[projectID] = q
	}
	add(q)
	start := !q.running
	q.running = true
	c.mu.Unlock()
	if start {
		go c.work(projectID, q)
	}
}

// work drains a project's queue. Reports that arrive while a turn runs are
// coalesced into the next turn.
func (c *Coordinator) work(projectID string, q *queue) {
	c.hub.Publish(Event{Type: EventCoordinator, ProjectID: projectID, Data: map[string]bool{"busy": true}})
	for {
		c.mu.Lock()
		user, config, reports := q.user, q.config, q.reports
		q.user, q.config, q.reports = false, false, nil
		if !user && !config && len(reports) == 0 {
			q.running = false
			c.mu.Unlock()
			c.hub.Publish(Event{Type: EventCoordinator, ProjectID: projectID, Data: map[string]bool{"busy": false}})
			return
		}
		c.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		if err := c.turn(ctx, projectID, user, config); err != nil {
			c.final(c.addEvent(projectID, &db.ThreadEvent{Kind: db.EvError, Content: err.Error()}))
		}
		cancel()
	}
}

// turn runs one coordinator turn: the model answers, calls tools, and the
// loop continues until it stops calling tools.
func (c *Coordinator) turn(ctx context.Context, projectID string, userTriggered, configChanged bool) error {
	p, err := c.store.GetProject(projectID)
	if err != nil {
		return err
	}
	if p.Paused || p.ArchivedAt != nil {
		return nil
	}
	history, err := c.store.ListRecentThreadEvents(p.MainThreadID, historyLimit)
	if err != nil {
		return err
	}
	msgs := renderHistory(history, c.layout.LibraryDir(projectID))
	if len(msgs) == 0 {
		return nil
	}
	if configChanged {
		msgs = appendUser(msgs, "(Project settings changed. Re-evaluate pending work using the current settings. Continue previously requested work if the change unblocks it; do not start unrelated work or repeat completed work. Existing threads keep their worktrees. If no action or useful update is needed, reply exactly "+noUpdate+".)")
	} else if !userTriggered {
		// The thread updates are already in the history; nudge the model
		// to stay quiet unless they matter.
		msgs = appendUser(msgs, "(Coordinator check-in: the thread updates above just arrived. Post a brief update for the user only if it matters, otherwise reply exactly "+noUpdate+".)")
	}
	tools := coordinatorTools()

	for round := 0; round < maxToolRounds; round++ {
		p, err = c.store.GetProject(projectID)
		if err != nil {
			return err
		}
		if p.Paused || p.ArchivedAt != nil {
			return nil
		}
		client, model, err := c.llm.ClientFor(ctx, p.CoordinatorModel)
		if err != nil {
			return fmt.Errorf("coordinator unavailable: %w", err)
		}
		threads, _ := c.store.ListThreads(projectID)
		system := coordinatorSystemPrompt(c.layout, p, threads, time.Now())

		var live *db.ThreadEvent
		var streamed strings.Builder
		onDelta := func(d string) {
			if !userTriggered {
				return // report turns post only their final text (it may be NO_UPDATE)
			}
			streamed.WriteString(d)
			if live == nil {
				live = c.addEvent(projectID, &db.ThreadEvent{Kind: db.EvAssistant, Content: streamed.String()})
				return
			}
			c.hub.Publish(Event{Type: EventMessageDelta, ProjectID: projectID, Data: map[string]string{"id": live.ID, "text": d}})
		}
		res, err := client.Stream(ctx, llm.Request{Model: model, System: system, Messages: msgs, Tools: tools, MaxTokens: 4096}, onDelta)
		text := strings.TrimSpace(res.Text)
		switch {
		case live != nil:
			live.Content = text
			if text == "" {
				live.Content = streamed.String()
			}
			_ = c.store.UpdateThreadEvent(live)
			c.publishEvent(projectID, live)
			c.final(live)
		case text != "" && text != noUpdate && !strings.HasPrefix(text, noUpdate):
			c.final(c.addEvent(projectID, &db.ThreadEvent{Kind: db.EvAssistant, Content: text}))
		}
		if err != nil {
			return fmt.Errorf("coordinator (%s): %w", model, err)
		}
		if len(res.ToolCalls) == 0 {
			return nil
		}
		results := make([]llm.ToolResult, 0, len(res.ToolCalls))
		var started []string
		for _, call := range res.ToolCalls {
			current, err := c.store.GetProject(projectID)
			if err != nil {
				return err
			}
			if current.Paused || current.ArchivedAt != nil {
				return nil
			}
			// An in-flight answer may still name a removed repository or use
			// old instructions. Let the next round plan again before acting.
			if len(configurationChanges(p, current)) > 0 {
				results = append(results, llm.ToolResult{CallID: call.ID, IsError: true,
					Content: "Project settings changed while this response was running. This tool was not executed. Re-evaluate it using the current project settings."})
				continue
			}
			out, isErr, ids := c.runTool(ctx, current, call)
			started = append(started, ids...)
			results = append(results, llm.ToolResult{CallID: call.ID, Content: out, IsError: isErr})
		}
		if len(started) > 0 {
			c.addEvent(projectID, &db.ThreadEvent{Kind: db.EvThreadStarted,
				Payload: jsonString(map[string]any{"thread_ids": started})})
		}
		msgs = append(msgs,
			llm.Message{Role: "assistant", Content: res.Text, ToolCalls: res.ToolCalls},
			llm.Message{Role: "user", ToolResults: results},
		)
	}
	return nil
}

// appendUser adds text as a user turn, merging with a trailing user turn.
func appendUser(msgs []llm.Message, text string) []llm.Message {
	if n := len(msgs); n > 0 && msgs[n-1].Role == "user" && len(msgs[n-1].ToolResults) == 0 {
		msgs[n-1].Content += "\n\n" + text
		return msgs
	}
	return append(msgs, llm.Message{Role: "user", Content: text})
}

// renderHistory turns the stored conversation into alternating model turns.
func renderHistory(history []db.ThreadEvent, libraryDir string) []llm.Message {
	var out []llm.Message
	add := func(role, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content += "\n\n" + text
			return
		}
		out = append(out, llm.Message{Role: role, Content: text})
	}
	for _, m := range history {
		switch m.Kind {
		case db.EvSystem:
			add("user", "[Project update] "+m.Content)
		case db.EvUser:
			add("user", withAttachments(m.Content, libraryDir, ParseAttachments(m.Payload)))
		case db.EvAssistant:
			add("assistant", m.Content)
		case db.EvThreadStarted:
			var pl struct {
				ThreadIDs []string `json:"thread_ids"`
			}
			_ = json.Unmarshal([]byte(m.Payload), &pl)
			add("assistant", fmt.Sprintf("(I started %d thread(s): %s)", len(pl.ThreadIDs), shortIDs(pl.ThreadIDs)))
		case db.EvSuggestions:
			var pl suggestionsPayload
			_ = json.Unmarshal([]byte(m.Payload), &pl)
			var parts []string
			for i, s := range pl.Suggestions {
				state := "not started"
				if s.ThreadID != "" {
					state = "started as " + short(s.ThreadID)
				}
				parts = append(parts, fmt.Sprintf("%d. %s (%s)", i+1, s.Title, state))
			}
			add("assistant", "(I proposed threads: "+strings.Join(parts, "; ")+")")
		case db.EvThreadUpdate:
			var pl struct {
				ThreadID string `json:"thread_id"`
				Title    string `json:"title"`
				Kind     string `json:"kind"`
				Status   string `json:"status"`
			}
			_ = json.Unmarshal([]byte(m.Payload), &pl)
			add("user", fmt.Sprintf("[Thread update] %q (%s) %s, now %s. Report:\n%s", pl.Title, short(pl.ThreadID), pl.Kind, pl.Status, clip(m.Content, 2000)))
		}
	}
	for len(out) > 0 && out[0].Role != "user" {
		out = out[1:]
	}
	return out
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func shortIDs(ids []string) string {
	var s []string
	for _, id := range ids {
		s = append(s, short(id))
	}
	return strings.Join(s, ", ")
}

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func coordinatorSystemPrompt(l Layout, p *db.Project, threads []db.Thread, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, `You are the coordinator of the project %q — the user's chief of staff for it. The user sends all work for this project into this one conversation. Threads do the work; you direct them.

How to route each message:
- Quick questions: answer here yourself, briefly.
- New work: start a thread with start_thread. One thread per independent task; a message with several unrelated tasks becomes several threads. A thread cannot see this conversation, so its task must be self-contained: the goal, relevant context and decisions, acceptance criteria, and how to verify.
- Follow-ups in an area a thread already owns: send them to that thread with message_thread, and tell the user which thread got it. Prefer this over starting a new thread for the same area.
- When the user authorizes a push, merge, deployment, or publication, include that authorization and its scope (action, repository/branch or destination, and any conditions the user specified) in start_thread or message_thread. Relay only authorization the user actually gave; do not infer publication permission from a general request to work autonomously. If a thread asks for authorization already given, relay the existing authorization and tell it to proceed within that scope instead of asking the user again. Repository threads allow these actions with user authorization; no higher-priority instruction change or replacement thread is needed for that default restriction.
- If the user asks to see a plan first or to confirm before starting, use propose_threads instead of start_thread.
- When the user states how the project should run (how many threads at once, how much to post, when to propose vs start, models, review rules), follow it and save it with remember so it persists. Respect saved preferences from memory.
- When the user asks you to remember or forget something, use remember / forget. Use update_instructions only when the user asks to change the project instructions.
- Threads report back as [Thread update] messages. After an update, tell the user only what matters: what is ready for review, what a thread needs from them (quote its question), what failed and why. Answering a thread's question for the user is not your call unless they told you how; relay it.
- Never claim a thread did something it did not report. Use get_thread for details.
- Keep messages short and concrete. Refer to threads by title. Do not repeat the thread cards the UI already shows.

Current date: %s
`, p.Name, now.Format("2006-01-02 15:04 MST"))
	if g := strings.TrimSpace(p.Goal); g != "" {
		fmt.Fprintf(&b, "\nProject goal: %s\n", g)
	}
	if len(p.Repos) > 0 {
		b.WriteString("\nRepositories (each thread gets its own branch and worktree in one of them; pass repo to start_thread to choose, the first is the default):\n")
		for _, r := range p.Repos {
			fmt.Fprintf(&b, "- %s (base branch %s)\n", r.Path, r.BaseBranch)
		}
	} else {
		b.WriteString("\nThis project has no repositories: threads work in a scratch directory and deliver files to the project Library.\n")
	}
	if ins := strings.TrimSpace(p.Instructions); ins != "" {
		fmt.Fprintf(&b, "\n<project_instructions>\n%s\n</project_instructions>\n", ins)
	}
	if mem := l.memoryIndexText(p.ID); mem != "" {
		fmt.Fprintf(&b, "\n<project_memory_index>\n%s\n</project_memory_index>\n", mem)
	}
	b.WriteString("\n<threads>\n")
	if len(threads) == 0 {
		b.WriteString("(none yet)\n")
	}
	for i, t := range threads {
		if i >= 30 {
			fmt.Fprintf(&b, "(%d older threads omitted)\n", len(threads)-i)
			break
		}
		fmt.Fprintf(&b, "- %s %q status=%s", short(t.ID), t.Title, t.Status)
		if t.Branch != "" {
			fmt.Fprintf(&b, " branch=%s commits=%d", t.Branch, t.CommitsAhead)
		}
		if t.WaitReason != "" {
			fmt.Fprintf(&b, " waiting_on_user=%q", clip(t.WaitReason, 200))
		}
		if t.LastReport != "" {
			fmt.Fprintf(&b, "\n  last report: %s", clip(strings.ReplaceAll(t.LastReport, "\n", " "), 300))
		}
		b.WriteString("\n")
	}
	b.WriteString("</threads>\n")
	return b.String()
}

func coordinatorTools() []llm.Tool {
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	threadSpec := map[string]any{
		"title": str("Short title shown on the thread card (a few words)."),
		"task":  str("Self-contained instructions for the thread: goal, context, acceptance criteria, how to verify."),
		"repo":  str("Optional. Path of the project repository to work in; \"none\" for no repository."),
	}
	return []llm.Tool{
		{Name: "start_thread", Description: "Start a new thread (a Claude Code session on its own branch) for one task.", Properties: threadSpec, Required: []string{"title", "task"}},
		{Name: "propose_threads", Description: "Propose threads for the user to start, instead of starting them. Use when the user wants to review a plan first.",
			Properties: map[string]any{"threads": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": threadSpec, "required": []string{"title", "task"}}}},
			Required:   []string{"threads"}},
		{Name: "message_thread", Description: "Send a message to an existing thread (follow-up work, an answer to its question). It runs after the thread's current turn.",
			Properties: map[string]any{"thread_id": str("Thread id (the 8-character prefix is enough)."), "message": str("What to tell the thread.")}, Required: []string{"thread_id", "message"}},
		{Name: "get_thread", Description: "Read a thread's status, branch, last report and recent transcript.",
			Properties: map[string]any{"thread_id": str("Thread id or its 8-character prefix.")}, Required: []string{"thread_id"}},
		{Name: "resolve_thread", Description: "Mark a thread resolved (done). Only when the user says so or its work is merged.",
			Properties: map[string]any{"thread_id": str("Thread id or prefix.")}, Required: []string{"thread_id"}},
		{Name: "remember", Description: "Save a requirement, decision, pitfall or coordination preference to project memory. Every new thread and coordinator turn sees the memory index.",
			Properties: map[string]any{"topic": str("Short topic; becomes the file name."), "content": str("The note."), "summary": str("One line for the MEMORY.md index.")}, Required: []string{"topic", "content", "summary"}},
		{Name: "forget", Description: "Delete a memory file.", Properties: map[string]any{"file": str("Memory file name, e.g. release-date.md.")}, Required: []string{"file"}},
		{Name: "update_instructions", Description: "Replace the project instructions (only when the user asks).",
			Properties: map[string]any{"instructions": str("The full new instructions (max 16000 characters).")}, Required: []string{"instructions"}},
	}
}

// suggestionsPayload is the payload of a suggestions message.
type suggestionsPayload struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion is a proposed thread; ThreadID is set once started.
type Suggestion struct {
	Title    string `json:"title"`
	Task     string `json:"task"`
	Repo     string `json:"repo,omitempty"`
	ThreadID string `json:"thread_id,omitempty"`
}

func (c *Coordinator) findThread(projectID, ref string) (*db.Thread, error) {
	ref = strings.TrimSpace(ref)
	threads, err := c.store.ListThreads(projectID)
	if err != nil {
		return nil, err
	}
	for i := range threads {
		if ref != "" && strings.HasPrefix(threads[i].ID, ref) {
			return &threads[i], nil
		}
	}
	return nil, fmt.Errorf("no thread %q in this project", ref)
}

// runTool executes one coordinator tool call. It returns the tool output,
// whether it is an error, and any thread ids it started.
func (c *Coordinator) runTool(ctx context.Context, p *db.Project, call llm.ToolCall) (string, bool, []string) {
	var in struct {
		Title, Task, Repo string
		Threads           []Suggestion
		ThreadID          string `json:"thread_id"`
		Message           string
		Topic, Content    string
		Summary, File     string
		Instructions      string
	}
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return "invalid input: " + err.Error(), true, nil
	}
	fail := func(err error) (string, bool, []string) { return err.Error(), true, nil }
	switch call.Name {
	case "start_thread":
		if strings.TrimSpace(in.Task) == "" {
			return "task is required", true, nil
		}
		t, err := c.threads.Start(ctx, p, StartSpec{Title: in.Title, Task: in.Task, Repo: in.Repo, Origin: "coordinator"})
		if err != nil {
			return fail(err)
		}
		return fmt.Sprintf("started thread %s %q (branch %s)", short(t.ID), t.Title, orNone(t.Branch)), false, []string{t.ID}
	case "propose_threads":
		if len(in.Threads) == 0 {
			return "threads is empty", true, nil
		}
		c.addEvent(p.ID, &db.ThreadEvent{Kind: db.EvSuggestions,
			Payload: jsonString(suggestionsPayload{Suggestions: in.Threads})})
		return fmt.Sprintf("proposed %d threads; the user can start them from the card", len(in.Threads)), false, nil
	case "message_thread":
		t, err := c.findThread(p.ID, in.ThreadID)
		if err != nil {
			return fail(err)
		}
		busy := c.threads.Running(t.ID)
		if _, err := c.threads.Send(ctx, t.ID, in.Message, fromThread(p.MainThreadID), nil); err != nil {
			return fail(err)
		}
		if busy {
			return "queued; the thread will read it after its current turn", false, nil
		}
		return "sent; the thread is working on it", false, nil
	case "get_thread":
		t, err := c.findThread(p.ID, in.ThreadID)
		if err != nil {
			return fail(err)
		}
		// The thread's own conversation: a subagent's steps stay out.
		events, _, _ := c.store.PageThreadEvents(t.ID, db.EventPage{Limit: 15})
		var lines []string
		for _, e := range events {
			if e.Kind == db.EvUsage || e.Kind == db.EvTask {
				continue
			}
			lines = append(lines, fmt.Sprintf("[%s] %s", e.Kind, clip(e.Content, 300)))
		}
		return fmt.Sprintf("thread %s %q status=%s branch=%s base=%s commits_ahead=%d files_changed=%d waiting_on=%q\ntask: %s\nlast report: %s\nrecent transcript:\n%s",
			short(t.ID), t.Title, t.Status, orNone(t.Branch), orNone(t.BaseBranch), t.CommitsAhead, t.FilesChanged, t.WaitReason,
			clip(t.Task, 1500), clip(t.LastReport, 2000), strings.Join(lines, "\n")), false, nil
	case "resolve_thread":
		t, err := c.findThread(p.ID, in.ThreadID)
		if err != nil {
			return fail(err)
		}
		if _, err := c.threads.Resolve(t.ID); err != nil {
			return fail(err)
		}
		return "resolved", false, nil
	case "remember":
		name, err := c.layout.Remember(p.ID, in.Topic, in.Content, in.Summary)
		if err != nil {
			return fail(err)
		}
		c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: map[string]string{"memory": name}})
		return "saved to memory as " + name, false, nil
	case "forget":
		if err := c.layout.DeleteMemory(p.ID, in.File); err != nil {
			return fail(err)
		}
		c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: map[string]string{"memory": in.File}})
		return "deleted " + in.File, false, nil
	case "update_instructions":
		if len([]rune(in.Instructions)) > MaxInstructions {
			return fail(fmt.Errorf("instructions exceed %d characters", MaxInstructions))
		}
		p.Instructions = in.Instructions
		if err := c.store.SaveProject(p); err != nil {
			return fail(err)
		}
		c.hub.Publish(Event{Type: EventProject, ProjectID: p.ID, Data: p})
		return "instructions updated", false, nil
	}
	return fail(errors.New("unknown tool " + call.Name))
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// StartSuggestions starts proposed threads from a suggestions event of a main
// thread. indexes selects which; empty starts every one not started yet.
func (c *Coordinator) StartSuggestions(ctx context.Context, mainThreadID, eventID string, indexes []int) (*db.ThreadEvent, error) {
	main, err := c.store.GetThread(mainThreadID)
	if err != nil {
		return nil, err
	}
	p, err := c.store.GetProject(main.ProjectID)
	if err != nil {
		return nil, err
	}
	projectID := p.ID
	m, err := c.store.GetThreadEvent(eventID)
	if err != nil {
		return nil, err
	}
	if m.ThreadID != main.ID || m.Kind != db.EvSuggestions {
		return nil, db.ErrNotFound
	}
	var pl suggestionsPayload
	if err := json.Unmarshal([]byte(m.Payload), &pl); err != nil {
		return nil, err
	}
	want := map[int]bool{}
	for _, i := range indexes {
		want[i] = true
	}
	for i := range pl.Suggestions {
		s := &pl.Suggestions[i]
		if s.ThreadID != "" || (len(want) > 0 && !want[i]) {
			continue
		}
		t, err := c.threads.Start(ctx, p, StartSpec{Title: s.Title, Task: s.Task, Repo: s.Repo, Origin: "suggestion"})
		if err != nil {
			return nil, err
		}
		s.ThreadID = t.ID
	}
	m.Payload = jsonString(pl)
	if err := c.store.UpdateThreadEvent(m); err != nil {
		return nil, err
	}
	c.publishEvent(projectID, m)
	return m, nil
}
