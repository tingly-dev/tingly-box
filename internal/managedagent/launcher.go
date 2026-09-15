package managedagent

import "context"

// Launcher is the execution seam. The Service owns state and invariants; the
// Launcher owns processes. It is implemented over agentboot in agentrun; a
// nil Launcher is legal and leaves sessions queued, which is what lets the
// data model and API be exercised without a CLI.
type Launcher interface {
	// Start begins the session's first turn asynchronously. State changes
	// come back through the stores.
	Start(ctx context.Context, run Run) error
	// Send queues a user message on a live session (steering).
	Send(ctx context.Context, sessionID, text string) error
	// Respond answers a pending approval / ask request.
	Respond(ctx context.Context, sessionID string, r Response) error
	// Interrupt stops the current turn; the session stays resumable.
	Interrupt(ctx context.Context, sessionID string) error
	// Stop tears the session's process down for good (archive).
	Stop(ctx context.Context, sessionID string) error
}

// Run is everything a Launcher needs to start a session, resolved once by
// the Service so the runtime never re-reads stores.
type Run struct {
	Session *Session
	Folder  *Folder
}

// Response answers an approval_request or ask_request event.
type Response struct {
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
	Answer    string `json:"answer,omitempty"`
}

// Git is the read-only view of a folder that happens to be a git work tree:
// what changed while the agent worked. Nothing here writes — committing and
// pushing stay with the person who owns the folder.
type Git interface {
	// IsRepo reports whether dir is a git work tree.
	IsRepo(ctx context.Context, dir string) bool
	// Head is the folder's current commit, recorded when a session starts
	// so its changes can be told apart from what was already there.
	Head(ctx context.Context, dir string) (string, error)
	// Diff summarises the working tree against base ("" = HEAD), untracked
	// files included.
	Diff(ctx context.Context, dir, base string) (*Diff, error)
	// ChangedFiles is Diff's count without the patch.
	ChangedFiles(ctx context.Context, dir, base string) (int, error)
}

// Diff is a folder's change summary since a session started.
type Diff struct {
	ChangedFiles int      `json:"changed_files"`
	Stat         string   `json:"stat"`
	Patch        string   `json:"patch"`
	Untracked    []string `json:"untracked,omitempty"`
	Truncated    bool     `json:"truncated"`
}
