// Package enginetest is a scripted stand-in for Claude Code, for testing a
// host of agentboot/engine without spawning the CLI. Each turn the engine
// runs calls the test's script with a Turn, which emits what the CLI would
// (text, approval requests, questions) and blocks where the CLI would.
//
// It drives one-shot turns only: build the engine without a pool.
package enginetest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// Agent is a scripted agentboot.Agent.
type Agent struct {
	// Script runs one turn. Returning nil completes the turn; an error
	// fails it.
	Script func(t *Turn) error

	mu    sync.Mutex
	turns []agentboot.ExecutionOptions
	n     atomic.Int64
}

// Service returns an AgentService whose Claude agent is a.
func (a *Agent) Service() *agentboot.AgentService {
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	if err != nil {
		panic(fmt.Sprintf("enginetest: NewAgentService: %v", err))
	}
	svc.RegisterAgent(agentboot.AgentTypeClaude, a)
	return svc
}

// Launches returns the options of every turn started so far, in order.
func (a *Agent) Launches() []agentboot.ExecutionOptions {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]agentboot.ExecutionOptions(nil), a.turns...)
}

func (a *Agent) Type() agentboot.AgentType { return agentboot.AgentTypeClaude }
func (a *Agent) IsAvailable() bool         { return true }

// Execute runs one turn of the script.
func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	a.mu.Lock()
	a.turns = append(a.turns, opts)
	a.mu.Unlock()
	t := &Turn{
		Ctx: ctx, Prompt: prompt, Opts: opts,
		agent:  a,
		events: make(chan agentboot.StreamEvent, 16),
		resp:   make(chan agentboot.ControlResponse, 1),
		done:   make(chan struct{}),
	}
	if opts.Store != nil {
		opts.Store.SetRunning(opts.SessionID)
	}
	go func() {
		defer close(t.done)
		defer close(t.events)
		err := a.Script(t)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			t.send(&claude.ResultMessage{Type: "result", Result: t.last})
		}
		t.err = err
		if opts.Store != nil {
			if err != nil {
				opts.Store.SetFailed(opts.SessionID, err.Error())
			} else {
				opts.Store.SetCompleted(opts.SessionID, t.last)
			}
		}
	}()
	return t, nil
}

// Turn is one running turn: the script's view of the CLI.
type Turn struct {
	Ctx    context.Context
	Prompt string
	Opts   agentboot.ExecutionOptions

	agent  *Agent
	events chan agentboot.StreamEvent
	resp   chan agentboot.ControlResponse
	done   chan struct{}
	last   string
	err    error
}

// Init reports the model the session runs on, as Claude Code's init does.
func (t *Turn) Init(model string) {
	t.send(&claude.SystemMessage{Type: "system", SubType: claude.SystemSubtypeInit, SessionID: t.Opts.SessionID, Raw: map[string]any{"model": model}})
}

// Say emits one assistant message with text, and the tokens the call used.
func (t *Turn) Say(text string, inputTokens, outputTokens int64) {
	t.last = text
	id := fmt.Sprintf("msg_%d", t.agent.n.Add(1))
	t.send(&claude.AssistantMessage{Type: "assistant", Message: anthropic.Message{
		ID:      id,
		Content: []anthropic.ContentBlockUnion{{Type: claude.ContentBlockTypeText, Text: text}},
		Usage:   anthropic.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
	}})
}

// ErrNoAnswer is returned by Approve and Ask when the turn ended first.
var ErrNoAnswer = errors.New("enginetest: the turn ended before an answer")

// Approve asks permission for a tool call and waits for the answer.
func (t *Turn) Approve(id, tool string, input map[string]any) (agentboot.ApprovalResponse, error) {
	t.send(agentboot.ApprovalRequestEvent{ID: id, AgentType: agentboot.AgentTypeClaude, ToolName: tool, Input: input})
	r, err := t.await()
	if err != nil {
		return agentboot.ApprovalResponse{}, err
	}
	resp, _ := r.(agentboot.ApprovalResponse)
	return resp, nil
}

// Ask asks the user a question and waits for the answer.
func (t *Turn) Ask(id, question string) (agentboot.AskResponse, error) {
	t.send(agentboot.AskRequestEvent{ID: id, AgentType: agentboot.AgentTypeClaude, ToolName: "AskUserQuestion", Message: question})
	r, err := t.await()
	if err != nil {
		return agentboot.AskResponse{}, err
	}
	resp, _ := r.(agentboot.AskResponse)
	return resp, nil
}

func (t *Turn) await() (agentboot.ControlResponse, error) {
	select {
	case r := <-t.resp:
		return r, nil
	case <-t.Ctx.Done():
		return nil, ErrNoAnswer
	}
}

func (t *Turn) send(ev any) {
	if se, ok := ev.(agentboot.StreamEvent); ok {
		t.events <- se
		return
	}
	t.events <- agentboot.MessageEvent{Raw: ev}
}

// Events implements agentboot.ExecutionHandle.
func (t *Turn) Events() <-chan agentboot.StreamEvent { return t.events }

// Respond implements agentboot.ExecutionHandle.
func (t *Turn) Respond(_ string, resp agentboot.ControlResponse) error {
	select {
	case t.resp <- resp:
		return nil
	default:
		return agentboot.ErrUnknownRequestID
	}
}

// Wait implements agentboot.ExecutionHandle.
func (t *Turn) Wait() (*agentboot.Result, error) {
	<-t.done
	return &agentboot.Result{Format: agentboot.OutputFormatText, Output: t.last}, t.err
}

// Cancel implements agentboot.ExecutionHandle.
func (t *Turn) Cancel() {}
