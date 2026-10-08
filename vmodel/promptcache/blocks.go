package promptcache

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// The extractors split a raw request body into blocks in the order a provider
// lays the prompt out: tool definitions, then the system prompt, then the
// conversation. Each block's key is its canonical JSON with every prompt-cache
// hint removed (protocol.PromptCacheHintFields) — a hint describes the prefix,
// it is not part of it, so moving a breakpoint must not move the key.

// AnthropicBlocks splits an Anthropic Messages request. Each tool, system block
// and message content block is one block; a string system prompt or message
// content is one block. A top-level cache_control (automatic caching) places
// the breakpoint on the last block.
func AnthropicBlocks(raw []byte) ([]Block, error) {
	var req struct {
		Tools        []json.RawMessage `json:"tools"`
		System       json.RawMessage   `json:"system"`
		Messages     []json.RawMessage `json:"messages"`
		CacheControl json.RawMessage   `json:"cache_control"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var blocks []Block
	for i, tool := range req.Tools {
		blocks = append(blocks, newBlock(fmt.Sprintf("tools[%d]", i), tool))
	}
	blocks = append(blocks, listOrStringBlocks("system", req.System, nil)...)
	for i, msg := range req.Messages {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(msg, &m); err != nil {
			return nil, err
		}
		// The role is part of every block's key: the same text under another
		// role is a different prompt.
		blocks = append(blocks, listOrStringBlocks(fmt.Sprintf("messages[%d]", i), m.Content, map[string]any{"role": m.Role})...)
	}
	if len(blocks) > 0 && !isNull(req.CacheControl) {
		blocks[len(blocks)-1].Breakpoint = true
	}
	return blocks, nil
}

// ChatBlocks splits an OpenAI Chat Completions request: each tool and each
// message (role, content, tool calls and all) is one block.
func ChatBlocks(raw []byte) ([]Block, error) {
	var req struct {
		Tools    []json.RawMessage `json:"tools"`
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var blocks []Block
	for i, tool := range req.Tools {
		blocks = append(blocks, newBlock(fmt.Sprintf("tools[%d]", i), tool))
	}
	for i, msg := range req.Messages {
		blocks = append(blocks, newBlock(fmt.Sprintf("messages[%d]", i), msg))
	}
	return blocks, nil
}

// ResponsesBlocks splits an OpenAI Responses request: each tool, the
// instructions, and each input item is one block; a string input is one block.
func ResponsesBlocks(raw []byte) ([]Block, error) {
	var req struct {
		Tools        []json.RawMessage `json:"tools"`
		Instructions json.RawMessage   `json:"instructions"`
		Input        json.RawMessage   `json:"input"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	var blocks []Block
	for i, tool := range req.Tools {
		blocks = append(blocks, newBlock(fmt.Sprintf("tools[%d]", i), tool))
	}
	if !isNull(req.Instructions) {
		blocks = append(blocks, newBlock("instructions", req.Instructions))
	}
	blocks = append(blocks, listOrStringBlocks("input", req.Input, nil)...)
	return blocks, nil
}

// listOrStringBlocks turns a field that is either a string or a list into
// blocks: the string as one block labelled label, each list element as
// label[i]. extra fields (e.g. the message role) are folded into every key.
func listOrStringBlocks(label string, raw json.RawMessage, extra map[string]any) []Block {
	if isNull(raw) {
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return []Block{newBlockWith(label, raw, extra)}
	}
	blocks := make([]Block, 0, len(list))
	for i, el := range list {
		sub := fmt.Sprintf("%s[%d]", label, i)
		if extra != nil {
			sub = fmt.Sprintf("%s.content[%d]", label, i)
		}
		blocks = append(blocks, newBlockWith(sub, el, extra))
	}
	return blocks
}

func newBlock(label string, raw json.RawMessage) Block {
	return newBlockWith(label, raw, nil)
}

func newBlockWith(label string, raw json.RawMessage, extra map[string]any) Block {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		v = string(raw)
	}
	breakpoint := hasCacheHint(v)
	v = stripHints(v)
	if extra != nil {
		v = map[string]any{"block": v, "extra": extra}
	}
	key, _ := json.Marshal(v) // map keys sorted: canonical
	return Block{
		Label:      label,
		Tokens:     int64((len(key) + 3) / 4), // same len/4 rule as token.EstimateTokensString
		Breakpoint: breakpoint,
		key:        key,
	}
}

// hasCacheHint reports whether the block itself (not something nested inside
// a tool_result) carries a cache breakpoint — Anthropic attaches cache_control
// to the block it marks.
func hasCacheHint(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, k := range protocol.PromptCacheHintFields {
		if val, ok := m[k]; ok && val != nil {
			return true
		}
	}
	return false
}

func stripHints(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, child := range node {
			if slices.Contains(protocol.PromptCacheHintFields, k) {
				continue
			}
			out[k] = stripHints(child)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, child := range node {
			out[i] = stripHints(child)
		}
		return out
	default:
		return v
	}
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
