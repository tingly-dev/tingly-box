# AgentBoot Package

AgentBoot is a unified bootstrapping and runtime layer for AI coding agents. It hides per-agent process management, protocol parsing, and permission/ask routing behind a small, agent-agnostic API.

## Features

- **Unified Agent interface** — one `Execute` entry point for every agent type.
- **Streaming execution handle** — consume a totally-ordered event channel and respond to permission / ask requests inline.
- **Driver + Transport + Runner split** — process setup, protocol parsing, and execution are independent and individually testable.
- **Pluggable process factory** — swap real `os/exec` for a scripted fake in tests without touching the driver or transport.
- **Session store integration** — list and resume Claude Code sessions read from `~/.claude/projects`.

## Supported Agents

| Agent    | Status         | Description       |
|----------|----------------|-------------------|
| `claude` | Implemented    | Claude Code CLI   |
| `mock`   | Test-only      | In-memory agent for tests |
| `codex`  | Planned        | OpenAI Codex      |
| `gemini` | Planned        | Google Gemini     |
| `cursor` | Planned        | Cursor AI         |

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
            log.Printf("non-fatal: %v", e.Err)
        }
    }

    result, err := handle.Wait()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result.TextOutput())
}
```

### Through the AgentBoot registry

```go
ab, err := agentboot.New(agentboot.Config{
    DefaultAgent:     agentboot.AgentTypeClaude,
    DefaultFormat:    agentboot.OutputFormatStreamJSON,
    EnableStreamJSON: true,
})
if err != nil {
    log.Fatal(err)
}

ab.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgent(ab.GetConfig()))

agent, err := ab.GetDefaultAgent()
if err != nil {
    log.Fatal(err)
}

handle, err := agent.Execute(ctx, "List files", agentboot.ExecutionOptions{
    ProjectPath: "/tmp",
})
// ...consume handle.Events(), then handle.Wait()
```

### Resuming a Claude session

```go
ab, _ := agentboot.New(agentboot.Config{
    DefaultAgent:      agentboot.AgentTypeClaude,
    ClaudeProjectsDir: "", // empty = ~/.claude/projects
})

sessions, err := ab.ListRecentSessions(ctx, "/abs/project/path", 10)
if err != nil {
    log.Fatal(err)
}

opts := ab.ResumeSession(sessions[0].SessionID)
opts.ProjectPath = "/abs/project/path"

handle, err := agent.Execute(ctx, "Continue", opts)
```

## Execution Lifecycle

`Agent.Execute` returns an `ExecutionHandle` that owns the in-flight execution:

1. Iterate `handle.Events()` from a single goroutine. Events are delivered in totally-ordered form.
2. For `ApprovalRequestEvent` and `AskRequestEvent`, call `handle.Respond(req.ID, response)` with the matching `ControlResponse` — `ApprovalResponse` or `AskResponse`. The response is forwarded to the agent process's stdin.
3. The events channel closes after the process has exited *and* every decoded event has been delivered.
4. After the channel closes, `handle.Wait()` returns the aggregated `*Result`.
5. `handle.Cancel()` requests cooperative shutdown; the underlying context can be cancelled to the same effect.

`Respond`, `Cancel`, and `Wait` are safe to call from any goroutine.

## Stream Events

| Event                   | Description                                                  |
|-------------------------|--------------------------------------------------------------|
| `MessageEvent`          | Agent message (assistant text, tool use, tool result, etc.) |
| `ApprovalRequestEvent`  | Tool permission request — caller must `Respond`              |
| `AskRequestEvent`       | Interactive question (e.g. AskUserQuestion)                  |
| `ErrorEvent`            | Non-fatal error — execution continues                         |

Fatal errors are surfaced via the error returned from `handle.Wait()`.

## Output Formats

| Format                        | Description                              |
|-------------------------------|------------------------------------------|
| `OutputFormatText`            | Plain text output                        |
| `OutputFormatStreamJSON`      | Streaming JSON events (default)          |

## Permission Modes

| Mode                       | Description                            |
|----------------------------|----------------------------------------|
| `PermissionModeAuto`       | Auto-approve all requests              |
| `PermissionModeManual`     | Require caller approval per request    |
| `PermissionModeSkip`       | Skip permission prompts (CLI bypass)   |

Set per-execution via `ExecutionOptions.PermissionMode`, or globally on the Claude agent through `Agent.SetSkipPermissions`.

## Configuration

`Config` is plain Go — no environment variables are read by the package itself. Defaults are provided by `agentboot.DefaultConfig()`:

| Field                       | Default                          |
|-----------------------------|----------------------------------|
| `DefaultAgent`              | `AgentTypeClaude`                |
| `DefaultFormat`             | `OutputFormatStreamJSON`         |
| `EnableStreamJSON`          | `true`                           |
| `StreamBufferSize`          | `100`                            |
| `DefaultExecutionTimeout`   | `0` (no timeout)                 |
| `ClaudeProjectsDir`         | `""` (session store disabled)    |

`ExecutionOptions` carries per-call overrides: project path, output format, timeout, env, session ID + resume flag, model and fallback model, max turns, allowed/disallowed tools, MCP servers, custom/append system prompts, permission mode, settings path, permission prompt tool, and a session store sink.

## Package Structure

```
agentboot/
├── agentboot.go          # AgentBoot registry + session helpers
├── config.go             # DefaultConfig / DefaultPermissionConfig
├── types.go              # Agent interface, ExecutionOptions, Result, ...
├── handle.go             # ExecutionHandle + ControlResponse types
├── events.go             # StreamEvent sum type (Message/Approval/Ask/Error)
├── handler.go            # CompositeHandler for streamer/approval/ask/completion
├── builder.go            # Function-typed handler adapters
├── driver.go             # AgentDriver interface + LaunchSpec
├── transport.go          # AgentTransport interface
├── runner.go             # Generic Runner wiring driver+transport+process
├── run.go                # Run loop / event pump
├── message.go            # Internal message routing
├── session_bridge.go     # Session lifecycle bridging
├── ask/                  # Ask/permission prompter implementations
├── common/               # Shared event + session metadata types
├── process/              # Process abstraction (osexec + fake for tests)
├── prompt/               # Prompt utilities
├── protocol/             # Stream-JSON encoder / decoder
├── session/              # Generic session store interface
└── claude/               # Claude Code agent implementation
    ├── agent.go          # claude.Agent (wraps Runner)
    ├── driver.go         # CLI flag construction + binary discovery
    ├── transport.go      # Stream-JSON parsing
    ├── launcher.go       # Process supervision
    ├── cli_builder.go    # Argv construction
    ├── cli_discovery.go  # Locate the claude binary
    ├── accumulator.go    # Per-message accumulation
    ├── messages.go       # Claude message types
    ├── tool_renderer.go  # Tool-use rendering
    ├── formatter.go      # Output formatting helpers
    ├── prompt_builder.go # Prompt assembly
    ├── session/          # Claude-specific session store (~/.claude/projects)
    ├── examples/         # query, server, session sample programs
    └── ref/              # Reference docs
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
       t := NewTransport()
       return &Agent{runner: agentboot.NewRunner(d, t), driver: d}
   }

   func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
       return a.runner.Execute(ctx, prompt, opts)
   }
   func (a *Agent) IsAvailable() bool                              { return a.driver.IsAvailable() }
   func (a *Agent) Type() agentboot.AgentType                      { return agentboot.AgentTypeYourAgent }
   func (a *Agent) SetDefaultFormat(f agentboot.OutputFormat)      { a.runner.SetDefaultFormat(f) }
   func (a *Agent) GetDefaultFormat() agentboot.OutputFormat       { return a.runner.GetDefaultFormat() }
   ```
3. Add the constant to `types.go` and register the agent on an `AgentBoot` with `RegisterAgent`.

For agents that don't fit the process+protocol pipeline (in-process mocks, remote services), use `agentboot.NewControlledHandle` to drive an `ExecutionHandle` directly.

## Testing

- `process.NewFakeFactory` substitutes the binary with a scripted in-memory process.
- `claude.NewAgentWithFactory` wires a fake factory into the real Claude driver/transport so end-to-end stream-JSON parsing is exercised without spawning `claude`.
- `NewControlledHandle` lets tests build an `ExecutionHandle` from closures.

See `agentboot/claude/*_e2e_test.go` and `agentboot/handle_test.go` for the patterns.

## License

See repository `LICENSE.txt`.
