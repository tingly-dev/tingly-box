package bot

import (
	"context"
	"testing"
)

// TestFinishRunning reports the intentional-stop flag and deregisters the bot.
// This is the single decision input for auto-restart: a bot that was stopped
// intentionally (stopped=true) must not be restarted.
func TestFinishRunning(t *testing.T) {
	m := &Manager{
		running: map[string]*runningBot{
			"crashed": {stopped: false},
			"stopped": {stopped: true},
		},
		baseCtx: context.Background(),
	}

	if intentional := m.finishRunning("crashed"); intentional {
		t.Fatalf("crashed bot: intentional = true, want false")
	}
	if intentional := m.finishRunning("stopped"); !intentional {
		t.Fatalf("stopped bot: intentional = false, want true")
	}

	if _, ok := m.running["crashed"]; ok {
		t.Fatalf("crashed bot was not removed from running map")
	}
	if _, ok := m.running["stopped"]; ok {
		t.Fatalf("stopped bot was not removed from running map")
	}

	// Unknown UUID is a safe no-op reporting "not intentional".
	if intentional := m.finishRunning("missing"); intentional {
		t.Fatalf("missing bot: intentional = true, want false")
	}
}

// TestScheduleRestart_SkipsWhenShuttingDown ensures a canceled base context
// (server shutdown) suppresses the restart so bots are not resurrected after
// stop.
func TestScheduleRestart_SkipsWhenShuttingDown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := &Manager{running: map[string]*runningBot{}, baseCtx: ctx}

	// With a canceled context scheduleRestart must return before touching the
	// store (which is nil here — a panic would mean it didn't short-circuit).
	m.scheduleRestart("any", BotSetting{UUID: "any"})
}
