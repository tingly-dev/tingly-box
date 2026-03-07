# Session Management Example

This example demonstrates how to use the session management features of agentboot to list and view Claude Code sessions.

## Building

```bash
go build -o session ./agentboot/examples/session/main.go
```

## Usage

```bash
# List recent sessions for a project (default: 10)
./session /path/to/project

# List recent sessions with a custom limit
./session /path/to/project 20

# Example with actual project path
./session /root/tingly-polish

# Output as JSON (pipe to jq for pretty printing)
./session /root/tingly-polish | jq .
```

## Output Format

The output is JSON formatted for easy parsing:

```json
[
  {
    "session_id": "947100b0-fcfe-4abc-af28-ace64aa23364",
    "project_path": "/root/tingly-polish",
    "status": "complete",
    "start_time": "2026-03-03T11:31:53Z",
    "end_time": "2026-03-03T11:37:23Z",
    "duration_ms": 330000,
    "first_message": "Implement session management feature",
    "last_result": "Successfully implemented all phases",
    "num_turns": 5,
    "input_tokens": 15234,
    "output_tokens": 3421,
    "cache_read_tokens": 8192,
    "total_cost_usd": 0.1234
  },
  {
    "session_id": "fbb2165f-742e-44df-9a48-631e815d0304",
    "project_path": "/root/tingly-polish",
    "status": "active",
    "start_time": "2026-03-03T10:15:22Z",
    "first_message": "Fix bug in JSON parser",
    "num_turns": 2,
    "total_cost_usd": 0.0567
  }
]
```

## Features

- **JSON Output**: Structured JSON output for easy parsing and integration
- **Path Resolution**: Automatically resolves project paths to Claude's encoded format
- **Session Metadata**: Includes session ID, status, timestamps, cost, and usage metrics
- **First Message Preview**: Shows the first user message for context

## Examples

```bash
# Get recent sessions
./session /root/tingly-polish

# Get last 20 sessions
./session /root/tingly-polish 20

# Pipe to jq for formatted output
./session /root/tingly-polish | jq '.[] | {id: .session_id, status: .status, cost: .total_cost_usd}'

# Filter by status
./session /root/tingly-polish | jq '.[] | select(.status == "complete")'

# Calculate total cost
./session /root/tingly-polish | jq '[.[] | .total_cost_usd] | add'
```

## Programmatic Usage

```go
package main

import (
    "context"
    "fmt"
    "github.com/tingly-dev/tingly-box/agentboot/session/claude"
)

func main() {
    // Create a session store
    store, _ := claude.NewStore("") // Uses default ~/.claude/projects

    // List recent sessions
    ctx := context.Background()
    sessions, _ := store.GetRecentSessions(ctx, "/root/project", 10)

    for _, sess := range sessions {
        fmt.Printf("%s: %s\n", sess.SessionID, sess.FirstMessage)
    }

    // Get session summary
    summary, _ := store.GetSessionSummary(ctx, sess.SessionID, 3, 2)
    fmt.Printf("First events: %d\n", len(summary.Head))
    fmt.Printf("Last events: %d\n", len(summary.Tail))
}
```

