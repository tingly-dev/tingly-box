package bot_test

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// fakeSettingsStore is the minimum SettingsStore implementation needed to
// drive bot.Manager.Start in a test. Only the methods bot.Manager actually
// touches are implemented.
type fakeSettingsStore struct {
	mu       sync.Mutex
	settings map[string]db.Settings
}

func (s *fakeSettingsStore) GetSettingsByUUIDInterface(uuid string) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.settings[uuid]
	if !ok {
		return nil, fmt.Errorf("settings not found: %s", uuid)
	}
	return rec, nil
}

func (s *fakeSettingsStore) ListEnabledSettingsInterface() (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]db.Settings, 0, len(s.settings))
	for _, rec := range s.settings {
		if rec.Enabled {
			out = append(out, rec)
		}
	}
	return out, nil
}

// newLifecycleManager spins up a bot.Manager with a fakeSettingsStore
// containing a single enabled tingly bot, and registers an InProcessTransport
// for that UUID so the bot can actually serve traffic. Returns the manager,
// the bot UUID, and the transport for inspection.
func newLifecycleManager(t *testing.T) (*bot.Manager, string, *tingly.InProcessTransport) {
	t.Helper()

	uuid := fmt.Sprintf("lifecycle-bot-%d", time.Now().UnixNano())

	tr := tingly.NewInProcessTransport()
	tingly.Register(uuid, tr)
	t.Cleanup(func() { tingly.Unregister(uuid) })

	store := &fakeSettingsStore{
		settings: map[string]db.Settings{
			uuid: {
				UUID:     uuid,
				Name:     "lifecycle-test",
				Platform: "tingly",
				AuthType: "none",
				Auth:     map[string]string{},
				Enabled:  true,
			},
		},
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          10 * time.Minute,
		MessageRetention: time.Hour,
	}, nil)

	svc, err := agentboot.NewAgentService(agentboot.Config{ClaudeProjectsDir: t.TempDir()})
	require.NoError(t, err)

	m := bot.NewManager(store, sessionMgr, svc)
	m.SetDataPath(t.TempDir() + "/chats.json")

	return m, uuid, tr
}

// TestManager_RestartBot_Tingly exercises the full stop/start cycle that the
// new POST /api/v1/imbot-admin/restart/:uuid endpoint relies on. It asserts
// that ctx cancellation reliably stops the bot within WaitForStop's timeout
// and that no goroutines leak across multiple cycles.
func TestManager_RestartBot_Tingly(t *testing.T) {
	m, uuid, _ := newLifecycleManager(t)

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, m.Start(parentCtx, uuid))
	require.True(t, m.IsRunning(uuid))

	// Let the imbot.Manager's Connect path complete so all transient
	// startup goroutines have settled before we sample the baseline.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	baseline := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		m.Stop(uuid)
		require.True(t,
			m.WaitForStop(uuid, 5*time.Second),
			"iter %d: WaitForStop timed out — goroutine likely leaked", i)
		require.False(t, m.IsRunning(uuid))

		require.NoError(t, m.Start(parentCtx, uuid))
		time.Sleep(50 * time.Millisecond)
		require.True(t, m.IsRunning(uuid))
	}

	m.Stop(uuid)
	require.True(t, m.WaitForStop(uuid, 5*time.Second))

	// Allow shutdown machinery to settle before sampling.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	final := runtime.NumGoroutine()

	// A small slack accounts for non-bot goroutines (logrus rotation,
	// pprof, etc.). The point is to catch a per-iteration leak — 3
	// restarts would inflate the count well beyond +4 if any goroutine
	// per cycle escaped ctx cancel.
	require.LessOrEqualf(t, final, baseline+4,
		"goroutine leak: baseline=%d final=%d (after 3 restarts)", baseline, final)
}

// TestManager_StopOneBotDoesNotAffectOthers asserts the core "independent
// restart" guarantee: stopping bot A must leave bot B running. This is the
// in-process equivalent of the subprocess isolation we deferred.
func TestManager_StopOneBotDoesNotAffectOthers(t *testing.T) {
	uuidA := fmt.Sprintf("indep-bot-A-%d", time.Now().UnixNano())
	uuidB := fmt.Sprintf("indep-bot-B-%d", time.Now().UnixNano())

	trA := tingly.NewInProcessTransport()
	trB := tingly.NewInProcessTransport()
	tingly.Register(uuidA, trA)
	tingly.Register(uuidB, trB)
	t.Cleanup(func() {
		tingly.Unregister(uuidA)
		tingly.Unregister(uuidB)
	})

	store := &fakeSettingsStore{
		settings: map[string]db.Settings{
			uuidA: {UUID: uuidA, Name: "A", Platform: "tingly", AuthType: "none", Auth: map[string]string{}, Enabled: true},
			uuidB: {UUID: uuidB, Name: "B", Platform: "tingly", AuthType: "none", Auth: map[string]string{}, Enabled: true},
		},
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          10 * time.Minute,
		MessageRetention: time.Hour,
	}, nil)
	svc, err := agentboot.NewAgentService(agentboot.Config{ClaudeProjectsDir: t.TempDir()})
	require.NoError(t, err)

	m := bot.NewManager(store, sessionMgr, svc)
	m.SetDataPath(t.TempDir() + "/chats.json")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, m.Start(ctx, uuidA))
	require.NoError(t, m.Start(ctx, uuidB))
	require.True(t, m.IsRunning(uuidA))
	require.True(t, m.IsRunning(uuidB))

	// Stop A; B must remain running.
	m.Stop(uuidA)
	require.True(t, m.WaitForStop(uuidA, 5*time.Second))
	require.False(t, m.IsRunning(uuidA))
	require.True(t, m.IsRunning(uuidB), "stopping bot A must not affect bot B")

	// Restart A while B is still running; B must still be untouched.
	require.NoError(t, m.Start(ctx, uuidA))
	time.Sleep(50 * time.Millisecond)
	require.True(t, m.IsRunning(uuidA))
	require.True(t, m.IsRunning(uuidB))

	m.Stop(uuidA)
	m.Stop(uuidB)
	require.True(t, m.WaitForStop(uuidA, 5*time.Second))
	require.True(t, m.WaitForStop(uuidB, 5*time.Second))
}
