# ai/agent

Generic agent configuration file writer for AI coding assistants.

## Overview

This package provides **pure configuration file writing** functionality for AI coding assistants. It contains **NO business logic** - all model names, profiles, and configuration strategies are controlled by the caller.

## Architecture

```
ai/agent/                          # Pure config file writers
├── types.go                       # AgentType, request/response types
├── info.go                        # AgentInfo and helpers
├── parse.go                       # ParseAgentType with alias support
├── interface.go                   # AgentConfig interface and registry
├── claude_code.go                 # Claude Code file writer
├── opencode.go                    # OpenCode file writer
├── codex.go                       # Codex file writer
├── restore.go                     # Common restore logic
└── utils.go                       # Utility functions

internal/agent/                    # Tingly-Box specific layer
├── builder.go                     # Business logic (model names, env building)
├── rule.go                        # Routing rule management
└── rule_bridge.go                 # Integration: builder + ai/agent + routing
```

## Design Philosophy

**`ai/agent` is a PURE config file writer**:
- No hardcoded model names
- No knowledge of "unified" vs "separate" mode
- No knowledge of profile structures
- Just writes what you tell it to write

**`internal/agent` provides business logic**:
- Model name strategies (unified/separate)
- Profile structures
- Routing rule management

## Usage

### Claude Code

```go
type ClaudeCodeParams struct {
    // BaseURL is the base URL for the Claude API
    BaseURL string

    // APIKey is the authentication token
    APIKey string

    // Model configuration - see ClaudeCodeModelConfig below
    ModelConfig ClaudeCodeModelConfig

    // InstallStatusLine installs the status line script
    InstallStatusLine bool

    // ExtraEnv contains additional environment variables
    ExtraEnv map[string]string

    // ExtraConfig contains additional config entries for settings.json
    ExtraConfig map[string]interface{}
}

type ClaudeCodeModelConfig struct {
    // Default is the default model to use
    Default string

    // Haiku is the model for Haiku requests (optional, uses Default if empty)
    Haiku string

    // Opus is the model for Opus requests (optional, uses Default if empty)
    Opus string

    // Sonnet is the model for Sonnet requests (optional, uses Default if empty)
    Sonnet string

    // SubAgent is the model for sub-agent tasks (optional, uses Default if empty)
    SubAgent string
}
```

**Usage examples:**

```go
// Unified mode - all slots use same model
result, err := agent.DefaultRegistry.Get(agent.AgentTypeClaudeCode).Apply(&agent.ClaudeCodeParams{
    BaseURL:  "http://localhost:12580/tingly/claude_code",
    APIKey:   "your-token",
    ModelConfig: agent.ClaudeCodeModelConfig{
        Default: "tingly/cc",  // All slots use this model
    },
})

// Separate mode - different models for different purposes
result, err := agent.DefaultRegistry.Get(agent.AgentTypeClaudeCode).Apply(&agent.ClaudeCodeParams{
    BaseURL:  "http://localhost:12580/tingly/claude_code",
    APIKey:   "your-token",
    ModelConfig: agent.ClaudeCodeModelConfig{
        Default:  "tingly/cc-default",
        Haiku:    "tingly/cc-haiku",
        Opus:     "tingly/cc-opus",
        Sonnet:   "tingly/cc-sonnet",
        SubAgent: "tingly/cc-subagent",
    },
})

// With custom env vars
result, err := agent.DefaultRegistry.Get(agent.AgentTypeClaudeCode).Apply(&agent.ClaudeCodeParams{
    BaseURL:  "http://localhost:12580/tingly/claude_code",
    APIKey:   "your-token",
    ModelConfig: agent.ClaudeCodeModelConfig{
        Default: "tingly/cc",
    },
    ExtraEnv: map[string]string{
        "CUSTOM_VAR": "value",
    },
})
```

### OpenCode

```go
// Build the complete config structure
config := map[string]interface{}{
    "$schema": "https://opencode.ai/config.json",
    "provider": map[string]interface{}{
        "tingly-box": map[string]interface{}{
            "name": "tingly-box",
            "npm":  "@ai-sdk/anthropic",
            "options": map[string]interface{}{
                "baseURL": "http://localhost:12580/tingly/opencode",
                "apiKey":  "your-token",
            },
            "models": map[string]interface{}{
                "tingly-opencode": map[string]interface{}{"name": "tingly-opencode"},
            },
        },
    },
}

result, err := agent.DefaultRegistry.Get(agent.AgentTypeOpenCode).Apply(&agent.OpenCodeParams{
    Config: config,
})
```

### Codex

```go
// Provide the list of models to configure
result, err := agent.DefaultRegistry.Get(agent.AgentTypeCodex).Apply(&agent.CodexParams{
    CodexBaseURL: "http://localhost:12580/tingly/codex",
    APIKey:       "your-token",
    Models:       []string{"tingly-codex", "custom-model"},
})
```

## Business Logic in Tingly-Box

`internal/agent/builder.go` contains all business logic:

```go
// BuildClaudeCodeEnv - decides model names based on unified mode
func BuildClaudeCodeEnv(baseURL, apiKey string, unified bool) map[string]string

// BuildOpenCodeConfig - constructs default OpenCode structure
func BuildOpenCodeConfig(configBaseURL, apiKey string, models map[string]interface{}) map[string]interface{}

// CollectCodexModels - deduplicates model names from routing rules
func CollectCodexModels(rules []string) []string
```

## Integration Pattern

```
User Request
    ↓
internal/agent/rule_bridge.ApplyAgent()
    ↓
internal/agent/builder.BuildXxx()  ← Business logic here
    ↓
ai/agent.Apply()                     ← Pure file writer
    ↓
Config files written
```

## Per-Agent Parameters

### Claude Code

```go
type ClaudeCodeParams struct {
    // Env is the COMPLETE environment variables map
    // Caller is responsible for all model names and settings
    Env map[string]string

    // InstallStatusLine installs the status line script
    InstallStatusLine bool

    // ExtraConfig contains additional config entries
    ExtraConfig map[string]interface{}
}
```

### OpenCode

```go
type OpenCodeParams struct {
    // Config is the COMPLETE OpenCode configuration object
    // Caller is responsible for the entire structure
    Config map[string]interface{}
}
```

### Codex

```go
type CodexParams struct {
    CodexBaseURL string
    APIKey       string
    // Models is a list collected by the caller
    Models       []string
}
```

## Key Changes from Original Design

### Before (Business Logic in ai/agent)

```go
// ai/agent had hardcoded knowledge
env["ANTHROPIC_MODEL"] = "tingly/cc"  // Hardcoded!
env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"  // Hardcoded!
```

### After (Pure Writer)

```go
// ai/agent just writes what it's given
type ClaudeCodeParams struct {
    Env map[string]string  // Caller provides everything
}
```

## Benefits

1. ✅ **Zero hardcoded logic** in `ai/agent`
2. ✅ **Reusable** by any project with different model naming schemes
3. ✅ **Clear separation**: File writing vs. business logic
4. ✅ **Flexible**: Caller controls all aspects of configuration
5. ✅ **Testable**: Pure functions without side effects
