# Query Mode Usage Guide

## Overview

Query mode provides stdio-based communication with Claude CLI, matching the functionality of the happy SDK. It supports:
- Line-delimited JSON protocol over stdout/stdin
- Stream input for complex prompts
- Tool permission callbacks (canCallTool)
- Iterator-like message consumption

## Quick Start

### 1. Simple String Prompt

```go
launcher := claude.NewQueryLauncher(claude.Config{})

query, err := launcher.Query(ctx, claude.QueryConfig{
    Prompt: "Say hello",
    Options: &claude.QueryOptionsConfig{
        CWD:   "/tmp",
        Model: "claude-sonnet-4-6",
    },
})
defer query.Close()

for {
    msg, ok := query.Next()
    if !ok { break }
    fmt.Printf("Message: %s\n", msg.Type)
}
```

### 2. Using Channel Messages

```go
for {
    select {
    case msg := <-query.Messages():
        // Handle message
    case err := <-query.Errors():
        // Handle error
    case <-query.Done():
        // Query complete
        return
    }
}
```

### 3. Stream Prompt with Tool Callback

```go
builder := claude.NewStreamPromptBuilder()
builder.AddUserMessage("Help me with code")

canCallTool := func(ctx context.Context, toolName string, input map[string]interface{}, opts claude.CallToolOptions) (map[string]interface{}, error) {
    // Approve or deny tool use
    return map[string]interface{}{"approved": true}, nil
}

query, err := launcher.Query(ctx, claude.QueryConfig{
    Prompt: builder.Messages(),
    Options: &claude.QueryOptionsConfig{
        CanCallTool: canCallTool,
    },
})
```

### 4. Functional Options (Concise API)

```go
query, err := claude.QueryWithContext(ctx, "Your prompt",
    claude.WithModel("claude-sonnet-4-6"),
    claude.WithCWD("/project"),
    claude.WithContinue(),
)
```

## Running Examples

```bash
# From the tingly-box directory
cd agentboot/claude/examples

# Example 1: Simple query
go run query_example.go 1

# Example 2: Channel-based query
go run query_example.go 2

# Example 3: Stream prompt with tools
go run query_example.go 3

# Example 4: Resume conversation
go run query_example.go 4

# Example 5: Continue conversation
go run query_example.go 5

# Example 6: Functional options
go run query_example.go 6

# Example 7: Interrupt
go run query_example.go 7
```

## Running Tests

### Unit Tests (No CLI Required)

```bash
# Run all unit tests
go test ./agentboot/claude/ -v

# Run specific Query tests
go test ./agentboot/claude/ -v -run Query

# Run tests excluding integration
go test ./agentboot/claude/ -v -short
```

### Integration Tests (Requires Claude CLI)

```bash
# Run integration tests (will be skipped if CLI not available)
go test ./agentboot/claude/ -v -run Integration

# Run specific integration test
go test ./agentboot/claude/ -v -run TestQuerySimpleIntegration
```

## Message Types

Messages from Claude have the following structure:

```go
type SDKMessage struct {
    Type      string                 // "system", "assistant", "tool_use", "result", etc.
    RequestID string                 // For control messages
    Request   map[string]interface{} // Control request data
    Response  map[string]interface{} // Control response data
    SessionID string                 // Session identifier
    Message   map[string]interface{} // Message content (for assistant/user)
    SubType   string                 // Sub-type ("success", "error", etc.)
    Result    string                 // Result text (for result messages)
    UUID      string                 // Message UUID
    Timestamp string                 // ISO timestamp
    RawData   map[string]interface{} // Original JSON data
}
```

## Configuration Options

```go
type QueryOptionsConfig struct {
    // Working directory
    CWD string

    // Model selection
    Model         string
    FallbackModel string

    // System prompts
    CustomSystemPrompt  string
    AppendSystemPrompt  string

    // Conversation control
    ContinueConversation bool
    Resume              string

    // Tool filtering
    AllowedTools    []string
    DisallowedTools []string

    // Permission mode
    PermissionMode string

    // MCP servers
    MCPServers      map[string]interface{}
    StrictMcpConfig bool

    // Settings
    SettingsPath string
    MaxTurns     int

    // Callbacks
    CanCallTool CanCallToolCallback

    // Control
    AbortSignal <-chan struct{}
    CustomEnv   []string
}
```

## API Comparison with Happy (TypeScript)

| Happy (TS) | Go Equivalent |
|------------|---------------|
| `query({ prompt })` | `QueryWithContext(ctx, prompt)` |
| `for await (msg of query)` | `for msg, ok := query.Next(); ok; msg, ok = query.Next()` |
| `query.messages` | `<-query.Messages()` |
| `canCallTool: callback` | `CanCallTool: func(...) {...}` |
| `AsyncIterable<msg>` | `Query.Next()` or channel |
| `await query.interrupt()` | `query.Interrupt()` |
| `--input-format stream-json` | `StreamPrompt` channel |

## Error Handling

```go
query, err := launcher.Query(ctx, config)
if err != nil {
    log.Fatalf("Failed to start query: %v", err)
}
defer query.Close()

// Check for errors during processing
select {
case err := <-query.Errors():
    log.Printf("Query error: %v", err)
case <-query.Done():
    log.Println("Query completed successfully")
}
```

## Context Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

query, err := launcher.Query(ctx, config)
// ... after timeout or cancel, query will automatically close
```
