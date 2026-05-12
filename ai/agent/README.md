# ai/agent

Generic agent configuration types and utilities for AI coding assistants.

## Overview

This package provides reusable type definitions and per-agent implementations for configuring AI coding assistants like Claude Code, OpenCode, and Codex. It is designed to be dependency-free (except for config file operations) and usable by any Go project.

## Architecture

```
ai/agent/
├── types.go          # AgentType, request/response types
├── info.go           # AgentInfo and helpers
├── parse.go          # ParseAgentType with alias support
├── interface.go      # AgentConfig interface and registry
├── claude_code.go    # Claude Code specific implementation
├── opencode.go       # OpenCode specific implementation
├── codex.go          # Codex specific implementation
├── restore.go        # Common restore logic
└── utils.go          # Utility functions
```

## Design

Each agent has an **independent implementation** in its own file, all implementing a common `AgentConfig` interface:

```go
type AgentConfig interface {
    Apply(params interface{}) (*ApplyAgentResult, error)
    Restore() (*RestoreAgentResult, error)
}
```

### No Generic Interfaces Needed

While the interface exists for registry purposes, each agent file implements it independently. There's no shared "apply" logic that tries to be generic - each agent knows exactly how to configure itself.

## Agent Types

The package supports three agent types:

- **Claude Code** (`claude-code`): Claude Code CLI agent (@cc)
- **OpenCode** (`opencode`): OpenCode IDE extension
- **Codex** (`codex`): OpenAI Codex CLI (@codex)

## Usage

### Using the Registry

```go
import "github.com/tingly-dev/tingly-box/ai/agent"

// Get the config for an agent type
config, ok := agent.DefaultRegistry.Get(agent.AgentTypeClaudeCode)
if !ok {
    // handle error
}

// Apply configuration
result, err := config.Apply(&agent.ClaudeCodeParams{
    BaseURL:           "http://localhost:12580",
    APIKey:            "your-token",
    Unified:           true,
    InstallStatusLine: false,
})
```

### Direct Agent Usage

```go
// Claude Code
ccConfig := &agent.ClaudeCodeConfig{}
result, err := ccConfig.Apply(&agent.ClaudeCodeParams{...})

// OpenCode
ocConfig := &agent.OpenCodeConfig{}
result, err := ocConfig.Apply(&agent.OpenCodeParams{...})

// Codex
cxConfig := &agent.CodexConfig{}
result, err := cxConfig.Apply(&agent.CodexParams{...})
```

### Parsing Agent Types

```go
// Parse with alias support
agentType, err := agent.ParseAgentType("cc")
if err != nil {
    // handle error
}
// agentType == agent.AgentTypeClaudeCode
```

Supported aliases:
- `cc`, `claude`, `claude-code`, `claudecode` → `AgentTypeClaudeCode`
- `oc`, `opencode`, `open-code` → `AgentTypeOpenCode`
- `cx`, `codex` → `AgentTypeCodex`

### Getting Agent Information

```go
// List all supported agents
infos := agent.ListAgentInfo()
for _, info := range infos {
    fmt.Printf("%s: %s\n", info.Name, info.Description)
    fmt.Printf("  Config files: %v\n", info.ConfigFiles)
    fmt.Printf("  Scenario: %s\n", info.Scenario)
}

// Get specific agent info
info, ok := agent.GetAgentInfo(agent.AgentTypeClaudeCode)
if ok {
    fmt.Printf("Name: %s\n", info.Name)
}
```

### Helper Functions

```go
// Generate Claude Code environment variables
env := agent.GenerateClaudeCodeEnv(baseURL, apiKey, unified)

// Generate OpenCode config payload
payload := agent.GenerateOpenCodePayload(configBaseURL, apiKey, models)

// Generate OpenCode setup script
script := agent.GenerateOpenCodeScript(configBaseURL, apiKey, modelsJSON, "unix")

// Collect and deduplicate Codex models
models := agent.CollectCodexModels(rawModels)
```

## Types

### AgentType

```go
type AgentType string

const (
    AgentTypeClaudeCode AgentType = "claude-code"
    AgentTypeOpenCode  AgentType = "opencode"
    AgentTypeCodex     AgentType = "codex"
)
```

### ApplyAgentRequest

```go
type ApplyAgentRequest struct {
    AgentType         AgentType
    Provider          string  // Provider UUID
    Model             string  // Model name
    Unified           bool    // For Claude Code: single config for all models
    Force             bool    // Skip confirmation prompts
    Preview           bool    // Show what would be applied without applying
    InstallStatusLine bool    // Install status line script (Claude Code only)
}
```

### ApplyAgentResult

```go
type ApplyAgentResult struct {
    Success      bool
    AgentType    AgentType
    ProviderName string
    ProviderUUID string
    Model        string
    ConfigFiles  []string
    BackupPaths  []string
    RulesCreated int
    RulesUpdated int
    Warnings     []string
    Message      string
}
```

## Per-Agent Parameters

### Claude Code

```go
type ClaudeCodeParams struct {
    BaseURL           string
    APIKey            string
    Unified           bool
    InstallStatusLine bool
}
```

### OpenCode

```go
type OpenCodeParams struct {
    ConfigBaseURL string
    APIKey        string
    Models        map[string]interface{}
}
```

### Codex

```go
type CodexParams struct {
    CodexBaseURL string
    APIKey       string
    Models       []string
}
```

## Implementation Details

### What's Included

- **Pure type definitions**: AgentType, request/response structs
- **Per-agent implementations**: Each agent in its own file
- **Helper functions**: Env generation, config payload builders
- **Registry system**: Get agent configs by type
- **Restore logic**: Backup restoration for all agents

### What's NOT Included

- **Routing rules**: Handled by `internal/agent` in Tingly-Box
- **Provider management**: Each project manages its own providers
- **Server config operations**: Uses `internal/server/config` for file operations

## Design Philosophy

1. **Independent files**: Each agent (`claude_code.go`, `opencode.go`, `codex.go`) is self-contained
2. **No forced abstraction**: Each agent implements the interface its own way
3. **Reusable types**: Generic types can be used by any project
4. **Tingly-Box integration**: `internal/agent/rule_bridge.go` adds routing rules on top

## Related Packages

- `internal/agent`: Tingly-Box specific agent configuration (routing rules, full apply with providers)
- `internal/server/config`: Configuration file operations (ApplyClaudeSettings, etc.)

## Migration Note

If you were previously using `internal/agent`, your code will continue to work due to re-exports in `internal/agent/rule_bridge.go`. No changes are required.

For new code or external projects, import `github.com/tingly-dev/tingly-box/ai/agent` directly.

## Example: Adding a New Agent

To add a new agent type:

1. Create `ai/agent/newagent.go`:
```go
package agent

type NewAgentConfig struct{}

func (n *NewAgentConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
    params := paramsInterface.(*NewAgentParams)
    // Implementation
}

func (n *NewAgentConfig) Restore() (*RestoreAgentResult, error) {
    return RestoreAgent(AgentTypeNewAgent)
}
```

2. Register in `interface.go`:
```go
DefaultRegistry.Register(AgentTypeNewAgent, &NewAgentConfig{})
```

3. Add agent type constant and info to `info.go`
