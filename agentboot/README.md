# AgentBoot Package

AgentBoot is a unified bootstrapping and runtime layer for AI coding agents. It
hides per-agent process management, protocol parsing, and permission/ask
routing behind a small, agent-agnostic API.

The production adapter launches the **Claude Code CLI** as a subprocess and
communicates with its stream-JSON/control protocol. AgentBoot does not embed the
Python `claude-agent-sdk` and does not use Claude Desktop as an execution
backend.

## Features

- **Unified Agent interface** — one `Execute` entry point for every agent type.
- **Streaming execution handle** — consume a totally-ordered event channel and respond to permission / ask requests inline.
- **Per-execution protocol isolation** — every run owns its transport, accumulator, routing context, process, and control state.
- **Driver + Transport + Runner split** — process setup, protocol parsing, and execution are independent and individually testable.
- **Pluggable process factory** — swap real `os/exec` for a scripted fake in tests without touching the driver or transport.
- **Session store integration** — list and resume Claude Code sessions read from `~/.claude/projects`.

## Supported Agents

| Agent | Status | Backend |
| --- | --- | --- |
| `claude` | Implemented | Claude Code CLI |

The generic driver/transport/runner seams support future providers. Test
fixtures substitute the process factory; they are not registered production
agents.

## Usage

### Direct Claude agent

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/tingly-dev/tingly-box/agentboot"
    "github.com/tingly-dev/tingly-box/agentboot/claude"
)

func main() {
    agent := claude.NewAgentWithConfig(claude.DefaultConfig())

    handle, err := agent.Execute(context.Background(), "Say hello", agentboot.ExecutionOptions{
        ProjectPath:  "/tmp",
        OutputFormat: agentboot.OutputFormatStreamJSON,
    })
    if err != nil {
        log.Fatal(err)
    }

    for ev := range handle.Events() {
        switch e := ev.(type) {
        case agentboot.MessageEvent:
            fmt.Printf("message: %T\n", e.Raw)
        case agentboot.ApprovalRequestEvent:
            _ = handle.Respond(e.ID, agentboot.ApprovalResponse{Approved: true})
        case agentboot.AskRequestEvent:
            _ = handle.Respond(e.ID, agentboot.AskResponse{Approved: true})
        case agentboot.ErrorEvent:
            log.Printf("stream error: %v", e.Err)
        }
    }

    result, err := handle.Wait()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result.TextOutput())
}
```

### Through AgentService

```go
config := agentboot.Config{
    DefaultAgent:     agentboot.AgentTypeClaude,
    DefaultFormat:    agentboot.OutputFormatStreamJSON,
    EnableStreamJSON: true,
}
service, err := claude.NewService(config)
if err != nil {
    log.Fatal(err)
}

handle, err := service.Execute(
    ctx,
    agentboot.AgentTypeClaude,
    "/tmp",
    "List files",
    agentboot.ExecutionOptions{},
)
// ...consume handle.Events(), then handle.Wait()
```

### Resuming a Claude session

```go
service, _ := claude.NewService(agentboot.DefaultConfig())

// To read history from a non-default location:
// service, _ = claude.NewService(
//     agentboot.DefaultConfig(),
//     claude.WithProjectsDir("/path/to/claude-projects"),
// )

sessions, err := service.ListSessions(ctx, "/abs/project/path", 10)
if err != nil {
    log.Fatal(err)
}

handle, err := service.ExecuteSession(
    ctx,
    sessions[0].SessionID,
    "Continue",
    agentboot.ExecutionOptions{},
)
```

## Execution Lifecycle

`Agent.Execute` returns an `ExecutionHandle` that owns the in-flight execution:

1. Iterate `handle.Events()` from a single goroutine. Events are delivered in totally-ordered form.
2. For `ApprovalRequestEvent` and `AskRequestEvent`, call `handle.Respond(req.ID, response)` with the matching `ControlResponse` — `ApprovalResponse` or `AskResponse`. The response is forwarded to the agent process's stdin.
3. On a terminal result, the runner closes stdin so the CLI can flush session state; it escalates to `Kill` only after a bounded grace period.
4. The events channel closes after the process has exited *and* every decoded event has been delivered.
5. After the channel closes, `handle.Wait()` returns the aggregated `*Result` and the authoritative terminal error.
6. `handle.Cancel()` requests cooperative shutdown; the underlying context can be cancelled to the same effect.

`Respond`, `Cancel`, and `Wait` are safe to call from any goroutine.
Different handles from the same Agent may run concurrently; their mutable
protocol state is isolated.

## Stream Events

| Event                   | Description                                                  |
|-------------------------|--------------------------------------------------------------|
| `MessageEvent`          | Agent message (assistant text, tool use, tool result, etc.) |
| `ApprovalRequestEvent`  | Tool permission request — caller must `Respond`              |
| `AskRequestEvent`       | Interactive question (e.g. AskUserQuestion)                  |
| `ErrorEvent`            | Stream error; may mirror the fatal error returned by `Wait`   |

Fatal errors are surfaced via the error returned from `handle.Wait()`.
Use `errors.As` with `*ResultError`, `*ProcessError`, or `*ProtocolError`
when callers need structured handling.

## Output Formats

| Format                        | Description                              |
|-------------------------------|------------------------------------------|
| `OutputFormatText`            | Plain text output                        |
| `OutputFormatStreamJSON`      | Streaming JSON events (default)          |

## Claude Permission Modes

`ExecutionOptions.PermissionMode` is forwarded to Claude Code CLI. Prefer the
constants in the `claude` package:

| Mode | Claude Code behavior |
| --- | --- |
| `claude.PermissionModeDefault` | Use the CLI's default permission flow |
| `claude.PermissionModeAuto` | Delegate decisions to Claude Code's rule classifier |
| `claude.PermissionModeManual` | Require interactive approval |
| `claude.PermissionModeDontAsk` | Do not ask for denied operations |
| `claude.PermissionModeAcceptEdits` | Automatically accept edit operations |
| `claude.PermissionModePlan` | Run in plan mode |
| `claude.PermissionModeBypassPermissions` | Bypass permission checks |

`Agent.SetSkipPermissions(true)` maps to Claude Code's separate
`--dangerously-skip-permissions` flag. It is an explicit bypass and is not
equivalent to `PermissionModeAuto`.

## Configuration

Root configuration is plain Go and its defaults are provided by
`agentboot.DefaultConfig()`:

| Field                       | Default                          |
|-----------------------------|----------------------------------|
| `DefaultAgent`              | `AgentTypeClaude`                |
| `DefaultFormat`             | `OutputFormatStreamJSON`         |
| `EnableStreamJSON`          | `true`                           |
| `StreamBufferSize`          | `100`                            |
| `DefaultExecutionTimeout`   | `0` (no timeout)                 |

`ExecutionOptions` carries per-call overrides: project path, output format,
timeout, env, session ID + resume flag, model and fallback model, max turns,
allowed/disallowed tools, MCP servers, custom/append system prompts, permission
mode, settings path, permission prompt tool, provider-defined control metadata,
and a lifecycle store. A zero timeout uses the configured default; a negative
timeout explicitly disables it.

The root `agentboot.NewAgentService` constructor is provider-neutral. It does
not import Claude or assume a session format. `claude.NewService` is the
composition helper for the production Claude Code path: it registers the Claude
agent and injects the read-only Claude session history reader. Other providers
can inject their own reader with `agentboot.WithSessionReader`.
`claude.WithProjectsDir` overrides Claude Code's default
`~/.claude/projects` history location.

Claude Code CLI discovery also recognizes `CLAUDE_CLI_PATH`,
`CLAUDE_USE_BUNDLED`, and `CLAUDE_USE_GLOBAL`. Every selected executable is
validated with `--version`; application-local bundled candidates mean a
packaged Claude Code executable, never Claude Desktop.

## Package Structure

```
agentboot/
├── agentboot.go          # Agent registry and root Config
├── agent.go              # AgentType and Agent interface
├── options.go            # OutputFormat and ExecutionOptions
├── result.go             # Aggregated Result read API
├── config.go             # Root defaults
├── errors.go             # Structured result/process/protocol errors
├── handle.go             # ExecutionHandle + ControlResponse types
├── events.go             # Typed StreamEvent sum
├── driver.go             # AgentDriver + process.LaunchSpec compatibility alias
├── transport.go          # AgentTransport + per-execution factory
├── runner.go             # Runner configuration and construction
├── runner_execute.go     # Generic per-execution lifecycle
├── runner_state.go       # Run state and terminal error conversion
├── run.go                # High-level Prompter/MessageSink consumer
├── service.go            # Public AgentService façade
├── ask/                  # Ask/permission prompter implementations
├── common/               # Canonical Event, SessionReader, history types
├── process/              # LaunchSpec + process abstraction (OS/fake)
├── protocol/             # Stream-JSON encoder / decoder
├── session/              # Runtime LifecycleStore interface
└── claude/               # Claude Code agent implementation
    ├── agent.go          # claude.Agent (wraps Runner)
    ├── service.go        # Agent + Claude session-reader composition
    ├── driver.go         # Launch preparation and CLI selection
    ├── transport.go      # Stream-JSON parsing
    ├── cli_builder.go    # Argv construction
    ├── discovery.go      # Verified Claude Code CLI discovery
    ├── environment.go    # Clean/merged CLI environment
    ├── accumulator.go    # Per-message accumulation
    ├── message.go        # Claude wire message types
    ├── content.go        # Content block decoding
    ├── tool_renderer.go  # Tool-use rendering
    ├── formatter.go      # Output formatting helpers
    ├── prompt_builder.go # Prompt assembly
    ├── session/          # Claude-specific session store (~/.claude/projects)
    └── examples/session/ # Session query example
```

## Adding a New Agent

The fastest path is to reuse the generic `Runner` by implementing `AgentDriver` and `AgentTransport`:

1. Create `agentboot/youragent/` with:
   - `Driver` — implements `agentboot.AgentDriver` (binary discovery, CLI flag construction, `LaunchSpec` assembly).
   - `Transport` — implements `agentboot.AgentTransport` (parse the agent's stdout into `common.Event`s and emit `StreamEvent`s).
2. Wrap them in an `Agent` type that satisfies `agentboot.Agent`:
   ```go
   func NewAgent(cfg agentboot.Config) *Agent {
       d := NewDriver(cfg)
       newTransport := func() agentboot.AgentTransport {
           return NewTransport()
       }
       return &Agent{runner: agentboot.NewRunner(d, newTransport), driver: d}
   }

   func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
       return a.runner.Execute(ctx, prompt, opts)
   }
   func (a *Agent) IsAvailable() bool                              { return a.driver.IsAvailable() }
   func (a *Agent) Type() agentboot.AgentType                      { return agentboot.AgentTypeYourAgent }
   func (a *Agent) SetDefaultFormat(f agentboot.OutputFormat)      { a.runner.SetDefaultFormat(f) }
   func (a *Agent) GetDefaultFormat() agentboot.OutputFormat       { return a.runner.GetDefaultFormat() }
   ```
   The factory must return a fresh transport; transports may own mutable
   per-run state.
3. Add the constant to `agent.go` and register the agent on an `AgentService`
   with `RegisterAgent`.

If the provider exposes historical sessions, implement `common.SessionReader`
and inject it with `agentboot.WithSessionReader`. Runtime lifecycle reporting
is a separate concern implemented through `session.LifecycleStore` in
`ExecutionOptions.Store`.

For agents that don't fit the process+protocol pipeline (in-process mocks, remote services), use `agentboot.NewControlledHandle` to drive an `ExecutionHandle` directly.

## Testing

- `process.NewFakeFactory` substitutes the binary with a scripted in-memory process.
- `claude.NewAgentWithFactory` wires a fake factory into the real Claude driver/transport so end-to-end stream-JSON parsing is exercised without spawning `claude`.
- `NewControlledHandle` lets tests build an `ExecutionHandle` from closures.

Run the deterministic module suite from `agentboot/`:

```bash
go test ./...
```

The opt-in E2E test exercises `AgentService → Runner → Claude Code CLI` and
requires a discoverable CLI plus valid credentials:

```bash
go test -tags e2e -run TestE2E_ClaudeRun .
```

The test skips when Claude Code CLI is unavailable.

See `agentboot/claude/fixture/fixture_test.go`,
`agentboot/claude/agent_concurrency_test.go`,
`agentboot/claude/accumulator_test.go`, and `agentboot/handle_test.go` for the
patterns.

## License

See repository `LICENSE.txt`.
