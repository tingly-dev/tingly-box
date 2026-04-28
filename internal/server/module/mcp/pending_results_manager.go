package mcp

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// ServerPendingResultsManager implements PendingResultsManager by wrapping server's stash/inject logic
type ServerPendingResultsManager struct {
	stasher   PendingResultsStasher
	injectorV1 PendingResultsInjectorV1
	injectorBeta PendingResultsInjectorBeta
	injectorOpenAI PendingResultsInjectorOpenAI
}

// VirtualToolExecutionResult matches the server's internal type
type VirtualToolExecutionResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// PendingResultsStasher defines the stash functionality
type PendingResultsStasher interface {
	StashPendingVirtualToolResults(anchorIDs []string, results []VirtualToolExecutionResult)
}

// PendingResultsInjectorV1 defines injection for Anthropic V1
type PendingResultsInjectorV1 interface {
	InjectPendingVirtualResultsAnthropicV1(req *anthropic.MessageNewParams)
}

// PendingResultsInjectorBeta defines injection for Anthropic Beta
type PendingResultsInjectorBeta interface {
	InjectPendingVirtualResultsAnthropicBeta(req *anthropic.BetaMessageNewParams)
}

// PendingResultsInjectorOpenAI defines injection for OpenAI
type PendingResultsInjectorOpenAI interface {
	InjectPendingVirtualResultsOpenAI(req *openai.ChatCompletionNewParams)
}

func NewServerPendingResultsManager(
	stasher PendingResultsStasher,
	injectorV1 PendingResultsInjectorV1,
	injectorBeta PendingResultsInjectorBeta,
	injectorOpenAI PendingResultsInjectorOpenAI,
) *ServerPendingResultsManager {
	return &ServerPendingResultsManager{
		stasher:        stasher,
		injectorV1:      injectorV1,
		injectorBeta:    injectorBeta,
		injectorOpenAI:  injectorOpenAI,
	}
}

func (m *ServerPendingResultsManager) Stash(
	anchorIDs []string,
	results []ToolExecutionResult,
) error {
	// Convert to server's internal format
	internalResults := make([]VirtualToolExecutionResult, len(results))
	for i, r := range results {
		internalResults[i] = VirtualToolExecutionResult{
			ToolUseID: r.ToolUseID,
			Content:   r.Content,
			IsError:   r.IsError,
		}
	}

	m.stasher.StashPendingVirtualToolResults(anchorIDs, internalResults)
	return nil
}

func (m *ServerPendingResultsManager) Inject(req any) (any, error) {
	// Type switch based on request format
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		m.injectorV1.InjectPendingVirtualResultsAnthropicV1(r)
		return r, nil

	case *anthropic.BetaMessageNewParams:
		m.injectorBeta.InjectPendingVirtualResultsAnthropicBeta(r)
		return r, nil

	case *openai.ChatCompletionNewParams:
		m.injectorOpenAI.InjectPendingVirtualResultsOpenAI(r)
		return r, nil

	default:
		return nil, fmt.Errorf("unknown request type: %T", req)
	}
}

func (m *ServerPendingResultsManager) Clear(anchorID string) error {
	// Clear pending results for a specific anchor ID
	// Currently not actively used in the flow - the server handles
	// result lifecycle internally. This method is provided for
	// interface completeness and future manual cleanup scenarios.
	//
	// Implementation would involve removing the anchor ID from the
	// server's pending results stash, but this is currently handled
	// automatically by the server's request lifecycle management.
	return nil
}
