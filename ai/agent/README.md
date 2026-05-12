# ai/agent

Generic agent configuration types and utilities for AI coding assistants.

## Overview

This package provides reusable type definitions and helper functions for configuring AI coding assistants like Claude Code, OpenCode, and Codex. It is designed to be dependency-free and usable by any Go project.

## Agent Types

The package supports three agent types:

- **Claude Code** (`claude-code`): Claude Code CLI agent (@cc)
- **OpenCode** (`opencode`): OpenCode IDE extension
- **Codex** (`codex`): OpenAI Codex CLI (@codex)

## Usage

### Parsing Agent Types

```go
import "github.com/tingly-dev/tingly-box/ai/agent"

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

### Agent Configuration Requests

```go
req := &agent.ApplyAgentRequest{
    AgentType:         agent.AgentTypeClaudeCode,
    Provider:          "provider-uuid",
    Model:             "model-name",
    Unified:           true,
    InstallStatusLine: false,
}
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

## Design

This package is intentionally minimal and dependency-free:

- **No internal dependencies**: Can be used by any project
- **Pure data types**: No behavior tied to specific implementations
- **Re-exports in internal/agent**: Backward compatibility for Tingly-Box

## Related Packages

- `internal/agent`: Tingly-Box specific agent configuration (routing rules, apply logic)
- `internal/server/config`: Configuration file operations

## Migration Note

If you were previously using `internal/agent`, your code will continue to work due to re-exports in `internal/agent/types_bridge.go`. No changes are required.

For new code or external projects, import `github.com/tingly-dev/tingly-box/ai/agent` directly.
