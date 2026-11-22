package project

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Approval is a request a tool makes of the user from inside its own call: the
// action is outside what a thread may do on its own (another thread's branch, a
// force over commits it has not seen, a PR it did not open), so it is put to the
// user as an ordinary approval card, in the web and in IM, and runs only if they
// allow it. Permission rules name a tool and cannot see its arguments, which is
// why the tool asks itself. See .design/github-tools.md ("Beyond the boundary").
type Approval struct {
	// Tool is the full name of the tool asking; Summary is the card's one line.
	Tool, Summary string
	// Reason says why this call needs the user (one clause per boundary crossed).
	Reason string
	// Input is shown on the card.
	Input map[string]any
	// Key, when set, lets "allow for this thread" remember the answer for this
	// exact request; empty means the answer is for this call only.
	Key string
}

// Approver asks the user about a. denied is why not, when the user said no.
type Approver func(ctx context.Context, threadID string, a Approval) (allowed bool, denied string, err error)

// askApproval is the Approver of a running service: a card in the thread's
// transcript that the turn waits on, answered like any other approval.
func (m *Threads) askApproval(ctx context.Context, threadID string, a Approval) (bool, string, error) {
	if a.Key != "" && m.isAllowed(threadID, a.Key) {
		return true, "", nil
	}
	ans, err := m.prompter(threadID).wait(ctx, db.EvApproval, a.Tool, a.Summary, requestPayload{
		RequestID: uuid.NewString(), Tool: a.Tool, Input: a.Input, Reason: a.Reason, State: "pending",
	})
	if err != nil {
		return false, "", err
	}
	if !ans.Allow {
		return false, strings.TrimSpace(ans.Answer), nil
	}
	if ans.Scope == "thread" && a.Key != "" {
		m.allow(threadID, a.Key)
	}
	return true, "", nil
}

// authorize puts a to the user and returns nil only when they allow it. A
// refusal is an error the model reads as the user's decision, not a failure to
// retry; nothing has been changed when it returns one.
func (l *Links) authorize(ctx context.Context, t *db.Thread, a Approval) error {
	if l.approve == nil {
		return errors.New("this needs the user's approval, and there is no way to ask for it here: " + a.Reason)
	}
	ok, why, err := l.approve(ctx, t.ID, a)
	if err != nil {
		return errors.New("the user was not asked: " + err.Error())
	}
	if !ok {
		msg := "the user did not approve this (" + a.Reason + "), so nothing was changed"
		if why != "" {
			msg += ": " + why
		}
		return errors.New(msg)
	}
	return nil
}
