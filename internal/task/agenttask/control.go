package agenttask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

var (
	ErrControlNotActive       = errors.New("task control is not active")
	ErrControlAnswered        = errors.New("task control was already answered")
	ErrInvalidControlDecision = errors.New("invalid task control decision")
)

type ControlDecision struct {
	Action string
	Answer string
	Reason string
}

type controlSubmission struct {
	decision ControlDecision
	ack      chan error
}

type controlWaiter struct {
	runID   string
	kind    task.ControlKind
	claimed bool
	submit  chan controlSubmission
}

// ControlBroker bridges durable TaskRun controls to a live ExecutionHandle.
// It intentionally keeps only process-local delivery state; persisted controls
// become expired after restart because the original stdin handle is gone.
type ControlBroker struct {
	mu      sync.Mutex
	pending map[string]*controlWaiter
	timeout time.Duration
}

func NewControlBroker(timeout time.Duration) *ControlBroker {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &ControlBroker{pending: make(map[string]*controlWaiter), timeout: timeout}
}

func (b *ControlBroker) Respond(ctx context.Context, runID, controlID string, decision ControlDecision) error {
	b.mu.Lock()
	waiter, ok := b.pending[controlID]
	if !ok || waiter.runID != runID {
		b.mu.Unlock()
		return ErrControlNotActive
	}
	if waiter.claimed {
		b.mu.Unlock()
		return ErrControlAnswered
	}
	if err := validateDecision(waiter.kind, decision); err != nil {
		b.mu.Unlock()
		return err
	}
	waiter.claimed = true
	b.mu.Unlock()

	submission := controlSubmission{decision: decision, ack: make(chan error, 1)}
	// The channel is buffered and this waiter is exclusively claimed. Always
	// enqueue after claiming so a client disconnect cannot consume the one-shot
	// right without delivering the answer to the agent.
	waiter.submit <- submission
	select {
	case err := <-submission.ack:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func validateDecision(kind task.ControlKind, decision ControlDecision) error {
	switch kind {
	case task.ControlKindApproval:
		if decision.Action != "approve" && decision.Action != "deny" {
			return fmt.Errorf("%w: approval action must be approve or deny", ErrInvalidControlDecision)
		}
	case task.ControlKindQuestion:
		if decision.Action != "answer" && decision.Action != "deny" {
			return fmt.Errorf("%w: question action must be answer or deny", ErrInvalidControlDecision)
		}
		if decision.Action == "answer" && strings.TrimSpace(decision.Answer) == "" {
			return fmt.Errorf("%w: question answer is required", ErrInvalidControlDecision)
		}
	default:
		return fmt.Errorf("%w: unsupported control kind", ErrInvalidControlDecision)
	}
	return nil
}

func (b *ControlBroker) AwaitApproval(ctx context.Context, runID string, handle agentboot.ExecutionHandle, ctl task.RunController, req agentboot.ApprovalRequestEvent) error {
	control := task.PendingControl{
		ID: uuid.NewString(), Kind: task.ControlKindApproval, ToolName: req.ToolName,
		Input: safeControlInput(req.Input), Reason: req.Reason,
	}
	return b.await(ctx, runID, handle, ctl, req.ID, control, func(decision ControlDecision) agentboot.ControlResponse {
		return agentboot.ApprovalResponse{Approved: decision.Action == "approve", Reason: decision.Reason}
	})
}

func (b *ControlBroker) AwaitAsk(ctx context.Context, runID string, handle agentboot.ExecutionHandle, ctl task.RunController, req agentboot.AskRequestEvent) error {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = string(safeControlInput(req.Input))
	}
	control := task.PendingControl{
		ID: uuid.NewString(), Kind: task.ControlKindQuestion, ToolName: req.ToolName,
		Input: safeControlInput(req.Input), Message: message, Reason: req.Reason,
	}
	return b.await(ctx, runID, handle, ctl, req.ID, control, func(decision ControlDecision) agentboot.ControlResponse {
		return agentboot.AskResponse{
			Approved: decision.Action == "answer", Response: strings.TrimSpace(decision.Answer), Reason: decision.Reason,
		}
	})
}

func (b *ControlBroker) await(
	ctx context.Context,
	runID string,
	handle agentboot.ExecutionHandle,
	ctl task.RunController,
	nativeRequestID string,
	control task.PendingControl,
	response func(ControlDecision) agentboot.ControlResponse,
) error {
	now := time.Now()
	control.CreatedAt = now
	control.ExpiresAt = now.Add(b.timeout)
	waiter := &controlWaiter{runID: runID, kind: control.Kind, submit: make(chan controlSubmission, 1)}
	b.mu.Lock()
	b.pending[control.ID] = waiter
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, control.ID)
		b.mu.Unlock()
	}()

	status := task.RunStatusWaitingApproval
	if control.Kind == task.ControlKindQuestion {
		status = task.RunStatusWaitingInput
	}
	if err := ctl.SetPendingControl(ctx, status, &control); err != nil {
		_ = handle.Respond(nativeRequestID, response(ControlDecision{Action: "deny", Reason: "could not persist control request"}))
		return err
	}
	_ = ctl.UpdateProgress(ctx, controlSummary(control))
	_ = ctl.AppendRunEvent(ctx, task.RunEvent{
		ID: uuid.NewString(), Kind: "control_requested", Summary: controlSummary(control),
		CreatedAt: now,
	})

	timer := time.NewTimer(b.timeout)
	defer timer.Stop()
	deliver := func(submission controlSubmission) error {
		err := handle.Respond(nativeRequestID, response(submission.decision))
		eventKind := "control_answered"
		if err != nil {
			eventKind = "control_delivery_failed"
		}
		resolveErr := ctl.ResolvePendingControl(context.Background(), task.RunEvent{
			ID: uuid.NewString(), Kind: eventKind,
			Summary: decisionSummary(control, submission.decision, err), CreatedAt: time.Now(),
		})
		if err == nil {
			err = resolveErr
		}
		submission.ack <- err
		return err
	}
	claimSystemDecision := func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		if waiter.claimed {
			return false
		}
		waiter.claimed = true
		return true
	}
	select {
	case submission := <-waiter.submit:
		return deliver(submission)

	case <-timer.C:
		if !claimSystemDecision() {
			return deliver(<-waiter.submit)
		}
		denial := ControlDecision{Action: "deny", Reason: "task control request timed out"}
		err := handle.Respond(nativeRequestID, response(denial))
		_ = ctl.ResolvePendingControl(context.Background(), task.RunEvent{
			ID: uuid.NewString(), Kind: "control_expired", Summary: "Request expired and was denied", CreatedAt: time.Now(),
		})
		return err

	case <-ctx.Done():
		if !claimSystemDecision() {
			return deliver(<-waiter.submit)
		}
		_ = handle.Respond(nativeRequestID, response(ControlDecision{Action: "deny", Reason: "task stopped before the request was answered"}))
		_ = ctl.ResolvePendingControl(context.Background(), task.RunEvent{
			ID: uuid.NewString(), Kind: "control_cancelled", Summary: "Request cancelled because the task stopped", CreatedAt: time.Now(),
		})
		return ctx.Err()
	}
}

func controlSummary(control task.PendingControl) string {
	if control.Kind == task.ControlKindApproval {
		return fmt.Sprintf("Approval requested for %s", control.ToolName)
	}
	if control.Message != "" {
		return control.Message
	}
	return "Agent requested input"
}

func decisionSummary(control task.PendingControl, decision ControlDecision, deliveryErr error) string {
	if deliveryErr != nil {
		return "Response could not be delivered to the agent"
	}
	if decision.Action == "approve" {
		return fmt.Sprintf("Approved %s once", control.ToolName)
	}
	if decision.Action == "answer" {
		return "Answered agent question"
	}
	return "Denied agent request"
}

func safeControlInput(input map[string]any) json.RawMessage {
	value := sanitizeControlValue(input)
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	const maxInput = 16 * 1024
	if len(data) <= maxInput {
		return data
	}
	preview := string(data[:maxInput])
	truncated, _ := json.Marshal(map[string]any{"truncated": true, "preview": preview})
	return truncated
}

func sanitizeControlValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api_key") || strings.Contains(lower, "authorization") || strings.Contains(lower, "credential") || strings.Contains(lower, "cookie") {
				out[key] = "[redacted]"
			} else {
				out[key] = sanitizeControlValue(item)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitizeControlValue(typed[i])
		}
		return out
	default:
		return value
	}
}
