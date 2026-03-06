# Server & Client Examples

## Overview

This directory contains examples demonstrating simplified interaction with Claude using stream-json input format.

## Examples

### Server Example (`server/server.go`)

An interactive server that:
- Takes simple user input (just type your message)
- Automatically wraps it in the correct stream-json format
- Sends to Claude CLI using stream-json input
- Returns Claude's response in a simplified format

**Usage:**
```bash
cd agentboot/claude/examples/server
go run server.go
```

**Options:**
- `--model, -m <model>` - Set the model to use
- `--cwd, -c <directory>` - Set working directory
- `--allow-tools <tools>` - Comma-separated list of allowed tools
- `--debug, -d` - Enable debug output
- `--help, -h` - Show help message

**Interactive Commands:**
- `debug` - Toggle debug mode
- `new` - Start a new conversation
- `quit`, `exit`, `q` - Exit the server

**Example Session:**
```
Claude Interactive Server (stream-json mode)
Type your message and press Enter. Type 'quit' or 'exit' to quit.

You> What is 2+2?

Claude:
2+2 equals 4.

You> quit
Goodbye!
```

### Client Example (`client/client.go`)

A client example showing:
- Interactive mode for single queries
- Batch processing for multiple queries from a file
- Network client pattern (for future use)

**Usage:**
```bash
cd agentboot/claude/examples/client
go run client.go                    # Interactive mode
go run client.go --batch input.txt  # Batch mode
```

**Options:**
- `--batch, -b <file>` - Process multiple queries from a file
- `--debug, -d` - Enable debug output
- `--help, -h` - Show help message

## Input Format

The server automatically wraps your input in the correct stream-json format:

**You type:**
```
What is 2+2?
```

**Server sends to Claude (stream-json format):**
```json
{"type":"user","message":{"role":"user","content":"What is 2+2?"}}
```

## Stream-JSON Format

The server uses Claude's stream-json input format internally:

```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": "your message here"
  }
}
```

This format enables:
- Multi-turn conversations
- Tool use with callbacks
- Real-time streaming responses

## Features

### Server Features
- ✅ Simplified input (just type your message)
- ✅ Automatic stream-json format handling
- ✅ Multi-turn conversation support
- ✅ Tool permission handling
- ✅ Debug mode
- ✅ Working directory configuration

### Client Features
- ✅ Interactive mode
- ✅ Batch processing from file
- ✅ Network client pattern (extensible)
- ✅ Debug output

## Running Examples

### Quick Start - Interactive Server
```bash
cd agentboot/claude/examples/server
go run server.go
```

### With Debug Mode
```bash
go run server.go --debug
```

### With Specific Model
```bash
go run server.go --model claude-sonnet-4-6
```

### Batch Processing
```bash
cd agentboot/claude/examples/client
echo "What is 2+2?" > input.txt
echo "What is the capital of France?" >> input.txt
go run client.go --batch input.txt
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Server                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  User Input           ┌─────────────────────────────────┐  │
│  "What is 2+2?"  ───►│  StreamPromptBuilder            │  │
│                      │                                 │  │
│                      │  .AddUserMessage(userPrompt)   │  │
│                      │                                 │  │
│                      │  Creates stream-json format:  │  │
│                      │  {"type":"user",               │  │
│                      │   "message":{                  │  │
│                      │     "role":"user",              │  │
│                      │     "content":"..."             │  │
│                      │   }}                            │  │
│                      └─────────────────────────────────┘  │
│                             │                              │
│                             ▼                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              QueryLauncher.Query()                   │  │
│  │                                                       │  │
│  │  • Spawns claude --print "" --input-format          │  │
│  │    stream-json --output-format stream-json           │  │
│  │                                                       │  │
│  │  • Streams messages to stdin                        │  │
│  │                                                       │  │
│  │  • Reads responses from stdout                       │  │
│  └──────────────────────────────────────────────────────┘  │
│                             │                              │
│                             ▼                              │
│  Simplified Output    ─────────────────────────────►  User  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Comparison with Raw Stream-JSON

### Raw stream-json (complex):
```bash
echo '{"type":"user","message":{"role":"user","content":"What is 2+2?"}}' | \
  claude --print "" --input-format stream-json --output-format stream-json
```

### Using server (simplified):
```bash
go run server.go
# Just type: What is 2+2?
```
