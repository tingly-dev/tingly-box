package agentboot

import (
	"io"
	"time"
)

// SessionCloseTimeout bounds how long callers wait for a process or
// [PersistentSession] to close gracefully before giving up (the underlying
// Close is still best-effort past this point, not aborted). Shared so every
// close-with-a-deadline call site uses the same budget instead of an
// independent copy of the same literal.
const SessionCloseTimeout = 15 * time.Second

// ResolveTimeout applies [ExecutionOptions.Timeout]'s documented semantics —
// zero uses defaultTimeout, a negative value disables the timeout, a positive
// value is used as-is — and returns the resolved duration. A returned value
// of zero or less means "no timeout"; callers only wrap ctx in
// context.WithTimeout when it is positive.
func ResolveTimeout(timeout, defaultTimeout time.Duration) time.Duration {
	if timeout == 0 {
		return defaultTimeout
	}
	return timeout
}

// OutputFormat defines agent output format.
type OutputFormat string

const (
	OutputFormatText       OutputFormat = "text"
	OutputFormatStreamJSON OutputFormat = "stream-json"
)

// String returns the string representation of OutputFormat.
func (f OutputFormat) String() string {
	return string(f)
}

// ExecutionContext carries provider-neutral metadata for one execution.
// Transports may use Metadata to annotate interactive control events.
type ExecutionContext struct {
	SessionID string
	Metadata  map[string]string
}

// ExecutionOptions controls agent execution.
type ExecutionOptions struct {
	ProjectPath  string
	OutputFormat OutputFormat
	// Timeout overrides the Runner default. Zero uses the configured default;
	// a negative value explicitly disables the default timeout.
	Timeout time.Duration
	Env     []string
	// SessionID is the session ID to use or resume
	// If Resume is true, --resume <session_id> is used to continue an existing session
	// If Resume is false, --session-id <session_id> is used to create a new session with specific ID
	SessionID string
	// Resume indicates whether to resume an existing session (true) or create a new one (false)
	Resume bool
	// ControlMetadata is passed to the per-execution transport through
	// ExecutionContext. Keys and values are provider/integration-defined.
	ControlMetadata map[string]string

	// Model selection (per-execution override)
	Model         string
	FallbackModel string

	// Execution control
	MaxTurns int

	// Tool filtering (per-execution override)
	AllowedTools    []string
	DisallowedTools []string

	// MCP servers (per-execution override)
	MCPServers      map[string]interface{}
	StrictMcpConfig bool

	// System prompts (per-execution override)
	CustomSystemPrompt string
	AppendSystemPrompt string

	// Permission mode (per-execution override)
	PermissionMode string

	// Settings path (per-execution override)
	SettingsPath string

	// PermissionPromptTool specifies the tool for permission prompts (e.g., "stdio")
	// When set to "stdio", permission requests are sent via stdin/stdout for callback handling
	PermissionPromptTool string

	// Stderr, if set, receives the agent process's stderr for this execution
	// (see process.LaunchSpec.Stderr). nil keeps the factory default.
	Stderr io.Writer

	// Store, if set, receives session lifecycle events driven by the runner.
	// When non-nil and SessionID is non-empty the runner calls:
	//   SetRunning  — after the process starts successfully
	//   SetFailed   — if the process fails to start or Wait returns an error
	//   SetCompleted — if Wait returns without error
	Store LifecycleStore
}
