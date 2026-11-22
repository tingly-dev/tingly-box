package project

import (
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// coordinatorToolTurn is the state of the coordinator's MCP tools for one turn.
// The caller supplies the settings and trigger for the active turn; they must
// not be reused for another turn. The project ID is never accepted from Claude.
type coordinatorToolTurn struct {
	mu            sync.Mutex
	snapshot      *db.Project
	userTriggered bool
	quiet         atomic.Bool
	// gen counts the turns begun, so a turn that ends after the next one
	// began (the follow-up Claude Code starts by itself) leaves it open;
	// quietGen is the turn that ended itself with no_update.
	gen, quietGen uint64
}

func cloneProject(p *db.Project) *db.Project {
	c := *p
	c.Repos = append([]db.Repo(nil), p.Repos...)
	return &c
}

// begin opens the tools for a turn and returns the turn's generation, which
// end takes.
func (t *coordinatorToolTurn) begin(p *db.Project, userTriggered bool) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.snapshot, t.userTriggered = cloneProject(p), userTriggered
	t.quiet.Store(false)
	t.gen++
	return t.gen
}

// adoptUser makes the turn one that must answer the user: a message of
// theirs was inserted into it. False when the turn has already ended or ended
// itself with no_update, so the message cannot be answered by it and must be
// queued instead.
func (t *coordinatorToolTurn) adoptUser() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.snapshot == nil || t.quiet.Load() {
		return false
	}
	t.userTriggered = true
	return true
}

// quietIn reports whether the turn begin numbered gen ended itself with
// no_update, also once a later turn has begun.
func (t *coordinatorToolTurn) quietIn(gen uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return gen != 0 && t.quietGen == gen
}

// end closes the tools of the turn begin numbered gen; a later turn's stay
// open.
func (t *coordinatorToolTurn) end(gen uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.gen == gen {
		t.snapshot = nil
	}
}

// run is the gate every coordinator-scope tool call passes. Calls are
// serialised with turn completion and no_update, because Claude may submit
// several at once. fn gets the project as it is now and runs only while the
// turn is active, not ended with no_update, the project is neither paused nor
// archived, and its settings are what the turn began with.
//
// A tool that edits the project (editsProject) has changed the settings the turn
// began with itself, so after it succeeds the turn adopts the project as the tool
// left it. Without that, the guard would refuse every later call of the turn. A
// change from anywhere else still trips it. The tool must apply its change to the
// project it is given and save that same value.
func (t *coordinatorToolTurn) run(store *db.Store, tool string, editsProject bool, fn func(current *db.Project) (*mcp.CallToolResult, error)) (*mcp.CallToolResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot, userTriggered := t.snapshot, t.userTriggered
	if snapshot == nil {
		return textResult("No coordinator turn is active.", true), nil
	}
	if t.quiet.Load() {
		return textResult("This coordinator turn has already ended with no_update.", true), nil
	}
	current, err := store.GetProject(snapshot.ID)
	if err != nil {
		return textResult(err.Error(), true), nil
	}
	if current.Paused || current.ArchivedAt != nil {
		return textResult("project is paused or archived", true), nil
	}
	if len(configurationChanges(snapshot, current)) > 0 {
		return textResult("Project settings changed while this response was running. Re-evaluate using the current settings.", true), nil
	}
	if tool == toolNoUpdate && userTriggered {
		return textResult("A message from the user needs a reply. Answer it, or start or message a thread.", true), nil
	}
	res, err := fn(current)
	if ok := err == nil && res != nil && !res.IsError; ok {
		if editsProject {
			t.snapshot = cloneProject(current)
		}
		if tool == toolNoUpdate {
			t.quiet.Store(true)
			t.quietGen = t.gen
		}
	}
	return res, err
}
