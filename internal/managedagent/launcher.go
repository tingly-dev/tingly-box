package managedagent

import "context"

// Launcher is the execution seam. The Service owns state and invariants; the
// Launcher owns processes. It is implemented over agentboot (local runtime
// today, a docker process factory next) in a later step; a nil Launcher is
// legal and leaves sessions queued, which is what lets the data model and API
// ship ahead of execution.
type Launcher interface {
	// Start provisions the workspace if needed and begins the session's first
	// turn asynchronously. State changes come back through the stores.
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
	Session     *Session
	Workspace   *Workspace
	Environment *Environment
	Source      *Source
}

// Response answers an approval_request or ask_request event.
type Response struct {
	RequestID string `json:"request_id"`
	Approved  bool   `json:"approved"`
	Answer    string `json:"answer,omitempty"`
}

// Git is the host-side repository seam: what the control plane needs from
// a workspace's checkout beyond running the agent. It runs on the host under
// every runtime, so credentials never enter the sandbox.
type Git interface {
	Diff(ctx context.Context, ws *Workspace) (*Diff, error)
	Push(ctx context.Context, ws *Workspace, log func(line string)) error
	// IsRepo reports whether the workspace directory is a git work tree.
	IsRepo(ctx context.Context, ws *Workspace) bool
	// HasWork reports whether the checkout holds anything that would be
	// lost by deleting it: uncommitted changes, untracked files, or commits
	// past BaseRef. Unknown (not a repo, git error) counts as work.
	HasWork(ctx context.Context, ws *Workspace) (bool, error)
}

// Diff is a workspace's change summary against its base ref.
type Diff struct {
	ChangedFiles int      `json:"changed_files"`
	Stat         string   `json:"stat"`
	Patch        string   `json:"patch"`
	Untracked    []string `json:"untracked,omitempty"`
	Truncated    bool     `json:"truncated"`
}
