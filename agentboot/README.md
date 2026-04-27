# AgentBoot Package

AgentBoot is a unified agent bootstrapping and management package that provides a clean abstraction for running different AI coding agents.

## Features

- **Unified Agent Interface**: Common interface for all agent types
- **Stream-JSON Support**: Real-time event streaming for rich output
- **Permission Management**: Flexible permission handling with multiple modes
- **Extensible Design**: Easy to add new agent types
- **Configuration**: Environment-based configuration

## Supported Agents

| Agent | Status | Description |
|-------|--------|-------------|
| `claude` | ✅ Implemented | Claude Code CLI |
| `codex` | 🚧 Planned | OpenAI Codex |
| `gemini` | 🚧 Planned | Google Gemini |
| `cursor` | 🚧 Planned | Cursor AI |

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/tingly-dev/tingly-box/agentboot"
    _ "github.com/tingly-dev/tingly-box/agentboot/claude" // Import to register Claude agent
)

func main() {
    // Create AgentBoot instance
    ab, err := agentboot.New(agentboot.Config{
        DefaultAgent:     agentboot.AgentTypeClaude,
        DefaultFormat:    agentboot.OutputFormatStreamJSON,
        EnableStreamJSON: true,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Register Claude agent (or use agents that have auto-registered via init())
    claudeAgent := claude.NewAgent(ab.GetConfig())
    ab.RegisterAgent(agentboot.AgentTypeClaude, claudeAgent)

    // Get Claude agent
    agent, err := ab.GetAgent(agentboot.AgentTypeClaude)
    if err != nil {
        log.Fatal(err)
    }

    // Execute prompt
    result, err := agent.Execute(context.Background(), "Say hello", agentboot.ExecutionOptions{
        OutputFormat: agentboot.OutputFormatStreamJSON,
    })

    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.TextOutput())
}
```

### Alternative: Using Claude Agent Directly

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
    // Create config
    config := agentboot.Config{
        DefaultFormat:    agentboot.OutputFormatStreamJSON,
        EnableStreamJSON: true,
    }

    // Create Claude agent directly
    agent := claude.NewAgent(config)

    // Execute prompt
    result, err := agent.Execute(context.Background(), "Say hello", agentboot.ExecutionOptions{
        OutputFormat: agentboot.OutputFormatStreamJSON,
    })

    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.TextOutput())
}
```

### With Permission Handler

```go
package main

import (
    "context"
    "log"

    "github.com/tingly-dev/tingly-box/agentboot"
    "github.com/tingly-dev/tingly-box/agentboot/claude"
    "github.com/tingly-dev/tingly-box/agentboot/permission"
)

func main() {
    // Create configuration
    config := agentboot.Config{
        DefaultFormat:    agentboot.OutputFormatStreamJSON,
        EnableStreamJSON: true,
    }

    // Create permission handler
    permHandler := permission.NewDefaultHandler(permission.Config{
        DefaultMode: agentboot.PermissionModeManual,
        Timeout:     300, // 5 minutes
    })

    // Create agent with permission handler
    agent := claude.NewAgentWithPermissionHandler(config, permHandler)

    // Set permission mode for a session
    permHandler.SetMode("session-123", agentboot.PermissionModeAuto)

    // Execute prompt
    result, err := agent.Execute(context.Background(), "List files", agentboot.ExecutionOptions{
        OutputFormat: agentboot.OutputFormatStreamJSON,
    })

    if err != nil {
        log.Fatal(err)
    }

    log.Println(result.TextOutput())
}
```

## Output Formats

### Text Format
Simple text output (default):
```bash
claude --print --output-format text "Say hello"
```

### Stream-JSON Format
Rich event streaming:
```bash
claude --output-format stream-json --verbose --print "Say hello"
```

Event types:
- `text_delta` - Incremental text output
- `tool_call_start` - Tool invocation begins
- `tool_call_end` - Tool completes
- `permission_request` - Permission needed
- `status` - Agent status change
- `thinking` - Reasoning state

## Permission Modes

| Mode | Description |
|------|-------------|
| `auto` | Auto-approve all requests |
| `manual` | Require user approval |
| `skip` | Skip permission prompts |

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AGENTBOOT_DEFAULT_AGENT` | Default agent type | `claude` |
| `AGENTBOOT_DEFAULT_FORMAT` | Default output format | `text` |
| `AGENTBOOT_ENABLE_STREAM_JSON` | Enable stream-json | `true` |
| `AGENTBOOT_STREAM_BUFFER_SIZE` | Event buffer size | `100` |
| `RCC_PERMISSION_MODE` | Permission mode | `auto` |
| `RCC_PERMISSION_TIMEOUT` | Approval timeout | `5m` |
| `RCC_WHITELIST` | Whitelisted tools | |
| `RCC_BLACKLIST` | Blacklisted tools | |

## Package Structure

```
agentboot/
├── agentboot.go      # Core package and factory
├── types.go          # Common types and interfaces
├── config.go         # Configuration
├── events/           # Event streaming
│   ├── events.go     # Event interfaces
│   ├── parser.go     # Stream parser
│   └── bus.go        # Event bus
├── permission/       # Permission management
│   ├── handler.go    # Permission handler interface
│   ├── handler_impl.go # Default implementation
│   └── store.go      # Permission history storage
└── claude/           # Claude Code agent
    ├── agent.go      # Claude agent
    ├── launcher.go   # Process launcher
    ├── config.go     # Claude config
    └── events.go     # Claude event types
```

## Adding a New Agent

1. Create agent package: `agentboot/youragent/`

2. Implement `agent.Agent` interface:
```go
type Agent struct {
    launcher *Launcher
}

func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error)
func (a *Agent) IsAvailable() bool
func (a *Agent) Type() agentboot.AgentType
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat)
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat
```

3. Register in `agentboot.go`:
```go
ab.agents[AgentTypeYourAgent] = youragent.NewAgent(ab.config)
```

## License

MIT
