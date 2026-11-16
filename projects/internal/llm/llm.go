// Package llm talks to the model endpoint configured in the standalone app's
// settings. It hides the two wire styles (Anthropic Messages, OpenAI Chat
// Completions) behind one streaming interface so the chat module does not care
// which one is in use.
//
// The default connection points at a local Tingly Box gateway, which speaks
// both styles and routes to whatever upstream the user configured there.
//
// The coordinator and threads do not use it: they run in Claude Code
// sessions, which reach the model through the host's environment.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// API styles.
const (
	StyleAnthropic = "anthropic"
	StyleOpenAI    = "openai"
)

// Connection is where and how to reach the model endpoint.
type Connection struct {
	APIStyle string `json:"api_style"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key"`
}

// Message is one turn of chat history.
type Message struct {
	Role    string // "user" or "assistant"
	Content string
	// ToolCalls are the tools an assistant turn called.
	ToolCalls []ToolCall
	// ToolResults answer the previous assistant turn's ToolCalls (user turn).
	ToolResults []ToolResult
}

// Tool is a function the model may call.
type Tool struct {
	Name        string
	Description string
	// Properties is the JSON Schema "properties" object of the input.
	Properties map[string]any
	Required   []string
}

// ToolCall is one call the model made.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult is the outcome of a ToolCall, sent back to the model.
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

// Request is a single generation.
type Request struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []Tool
	MaxTokens int
}

// Usage is the token accounting reported by the endpoint.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Result is the outcome of a completed generation.
type Result struct {
	Text       string
	ToolCalls  []ToolCall
	StopReason string
	Usage      Usage
}

// Model is one entry of the endpoint's model list.
type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	// ProviderModel / ProviderName are what the host routes ID to — the real
	// model an alias stands for, and the provider that serves it. Set by an
	// embedding host that can resolve its routing (Tingly Box does, the way
	// its status line does); empty when the endpoint says nothing.
	ProviderModel string `json:"provider_model,omitempty"`
	ProviderName  string `json:"provider_name,omitempty"`
}

// Client generates text against one connection.
type Client interface {
	// Stream runs req, calling onDelta for each text chunk as it arrives.
	// On error the returned Result still carries the text received so far.
	Stream(ctx context.Context, req Request, onDelta func(string)) (Result, error)
	// ListModels returns the models the endpoint offers.
	ListModels(ctx context.Context) ([]Model, error)
}

// DefaultMaxTokens is used when a request does not set MaxTokens.
const DefaultMaxTokens = 8192

// New builds a client for conn.
func New(conn Connection) (Client, error) {
	if strings.TrimSpace(conn.BaseURL) == "" {
		return nil, fmt.Errorf("no base URL configured")
	}
	switch conn.APIStyle {
	case StyleAnthropic, "":
		return newAnthropic(conn), nil
	case StyleOpenAI:
		return newOpenAI(conn), nil
	default:
		return nil, fmt.Errorf("unknown api style %q", conn.APIStyle)
	}
}

// Complete runs req without streaming deltas to a caller.
func Complete(ctx context.Context, c Client, req Request) (Result, error) {
	return c.Stream(ctx, req, func(string) {})
}

// Endpoint returns the concrete URL a chat request is sent to, so the UI can
// show the real value rather than a label.
func Endpoint(conn Connection) string {
	base := strings.TrimRight(conn.BaseURL, "/")
	if conn.APIStyle == StyleOpenAI {
		return base + "/chat/completions"
	}
	return base + "/v1/messages"
}
