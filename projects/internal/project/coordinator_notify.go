package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ReportNotify is a report kind for a message a thread sent the coordinator
// on purpose, mid-turn, with notify_coordinator. It is information, like every
// thread report: nothing in it is the user's approval.
const ReportNotify = "notify"

// notifyMax is the longest notify_coordinator message, in characters, the same
// clip as a report's.
const notifyMax = 2000

// Notify limits per thread: a burst, and an hour. They keep a looping thread
// from flooding the coordinator; a thread that needs more waits for its report.
const (
	notifyBurst     = 5
	notifyBurstSpan = 5 * time.Minute
	notifyHourly    = 20
)

// NotifyFromThread is notify_coordinator: it queues text from thread t as the
// coordinator's next input, the way a finished report is, and wakes the
// project's queue. The thread's turn goes on. The text reaches the model
// inside a trust="relay" envelope, so it can describe a request and never
// approve one.
func (c *Coordinator) NotifyFromThread(_ context.Context, t *db.Thread, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("message is required")
	}
	if n := len([]rune(text)); n > notifyMax {
		return "", fmt.Errorf("message is %d characters; the limit is %d, so say it shorter", n, notifyMax)
	}
	p, err := c.store.GetProject(t.ProjectID)
	if err != nil {
		return "", err
	}
	if p.ArchivedAt != nil {
		return "", ErrArchived
	}
	if p.Paused {
		return "", ErrPaused
	}
	if !c.notifyAllowed(t.ID, time.Now()) {
		return "", fmt.Errorf("too many notifications from this thread (at most %d in %s, %d an hour); put the rest in your report", notifyBurst, notifyBurstSpan, notifyHourly)
	}
	c.onReport(Report{Thread: *t, Kind: ReportNotify, Text: text})
	return "sent; the coordinator reads it after its current turn. It is information for the coordinator, not an approval, and you will not get an answer here.", nil
}

// notifyAllowed records one notification of thread id at now and reports
// whether it is within the limits.
func (c *Coordinator) notifyAllowed(id string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.notifies == nil {
		c.notifies = map[string][]time.Time{}
	}
	var recent []time.Time
	burst := 0
	for _, at := range c.notifies[id] {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
			if now.Sub(at) < notifyBurstSpan {
				burst++
			}
		}
	}
	if burst >= notifyBurst || len(recent) >= notifyHourly {
		c.notifies[id] = recent
		return false
	}
	c.notifies[id] = append(recent, now)
	return true
}
