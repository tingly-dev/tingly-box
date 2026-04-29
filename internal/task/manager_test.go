package task_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/task"
)

// ---- test helpers ----

const testPoll = 10 * time.Millisecond

// newManager returns a started Manager backed by a fresh MemoryStore.
func newManager(t *testing.T, opts ...task.ManagerOption) (task.Manager, *task.MemoryStore) {
	t.Helper()
	store := task.NewMemoryStore()
	opts = append([]task.ManagerOption{
		task.WithPollInterval(testPoll),
		task.WithWaitInterval(testPoll),
	}, opts...)
	mgr := task.NewManager(store, opts...)
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop(context.Background()) })
	return mgr, store
}

// waitDone blocks until taskID is terminal or the test times out.
func waitDone(t *testing.T, mgr task.Manager, taskID string) *task.Task {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tk, err := mgr.Wait(ctx, taskID)
	if err != nil {
		t.Fatalf("Wait(%s): %v", taskID, err)
	}
	return tk
}

// funcHandler is a Handler that delegates Run to a closure.
type funcHandler struct {
	typ string
	fn  func(ctx context.Context, t *task.Task, ctl task.Controller) (*task.TaskResult, error)
}

func (h *funcHandler) Type() string { return h.typ }
func (h *funcHandler) Run(ctx context.Context, t *task.Task, ctl task.Controller) (*task.TaskResult, error) {
	return h.fn(ctx, t, ctl)
}

func mustRegister(t *testing.T, mgr task.Manager, h task.Handler) {
	t.Helper()
	if err := mgr.Register(h); err != nil {
		t.Fatalf("Register(%s): %v", h.Type(), err)
	}
}

func mustSubmit(t *testing.T, mgr task.Manager, req task.SubmitRequest) *task.Task {
	t.Helper()
	tk, err := mgr.Submit(context.Background(), req)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return tk
}

// ---- tests ----

func TestSubmit_BasicFields(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "noop", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	tk := mustSubmit(t, mgr, task.SubmitRequest{
		Type:      "noop",
		OwnerType: "user",
		OwnerID:   "u1",
		Source:    "test",
	})

	if tk.ID == "" {
		t.Error("ID is empty")
	}
	if tk.Status != task.StatusPending {
		t.Errorf("want pending, got %s", tk.Status)
	}
	if tk.OwnerType != "user" || tk.OwnerID != "u1" {
		t.Error("owner fields not set")
	}
}

func TestSubmit_MaxAttemptsDefault(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "noop", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "noop"})
	if tk.MaxAttempts != 1 {
		t.Errorf("default MaxAttempts want 1, got %d", tk.MaxAttempts)
	}
}

func TestRun_Success(t *testing.T) {
	mgr, _ := newManager(t)

	resultPayload := json.RawMessage(`{"ok":true}`)
	mustRegister(t, mgr, &funcHandler{
		typ: "echo",
		fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			return &task.TaskResult{Result: resultPayload}, nil
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "echo"})
	done := waitDone(t, mgr, tk.ID)

	if done.Status != task.StatusSucceeded {
		t.Errorf("want succeeded, got %s", done.Status)
	}
	if string(done.Result) != string(resultPayload) {
		t.Errorf("result mismatch: got %s", done.Result)
	}
	if done.FinishedAt == nil {
		t.Error("FinishedAt not set")
	}
	if done.Attempt != 1 {
		t.Errorf("want attempt=1, got %d", done.Attempt)
	}
}

func TestRun_Fail(t *testing.T) {
	mgr, _ := newManager(t)

	mustRegister(t, mgr, &funcHandler{
		typ: "bad",
		fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			return nil, errors.New("something broke")
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "bad"})
	done := waitDone(t, mgr, tk.ID)

	if done.Status != task.StatusFailed {
		t.Errorf("want failed, got %s", done.Status)
	}
	if done.Error == "" {
		t.Error("Error field not set")
	}
}

func TestRun_RetrySuccess(t *testing.T) {
	mgr, _ := newManager(t)

	var calls int
	var mu sync.Mutex

	mustRegister(t, mgr, &funcHandler{
		typ: "flaky",
		fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n < 2 {
				return nil, &task.RetryableError{Cause: errors.New("transient"), Backoff: testPoll}
			}
			return &task.TaskResult{}, nil
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "flaky", MaxAttempts: 3})
	done := waitDone(t, mgr, tk.ID)

	if done.Status != task.StatusSucceeded {
		t.Errorf("want succeeded, got %s (error: %s)", done.Status, done.Error)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("want 2 calls, got %d", calls)
	}
}

func TestRun_RetryExhausted(t *testing.T) {
	mgr, _ := newManager(t)

	mustRegister(t, mgr, &funcHandler{
		typ: "always-fail",
		fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			return nil, &task.RetryableError{
				Cause:   errors.New("transient"),
				Backoff: testPoll, // very short backoff so test doesn't hang
			}
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "always-fail", MaxAttempts: 2})
	done := waitDone(t, mgr, tk.ID)

	if done.Status != task.StatusFailed {
		t.Errorf("want failed after exhausting retries, got %s", done.Status)
	}
	if done.Attempt != 2 {
		t.Errorf("want attempt=2, got %d", done.Attempt)
	}
}

func TestCancel_Pending(t *testing.T) {
	// Submit a task, cancel before scheduler dispatches it.
	// We use a far-future ScheduledAt so the scheduler won't touch it.
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "delayed", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	future := time.Now().Add(1 * time.Hour)
	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "delayed", ScheduledAt: &future})

	if err := mgr.Cancel(context.Background(), tk.ID, "not needed"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	got, _ := mgr.Get(context.Background(), tk.ID)
	if got.Status != task.StatusCancelled {
		t.Errorf("want cancelled, got %s", got.Status)
	}
	if got.CancelledAt == nil {
		t.Error("CancelledAt not set")
	}
	if got.Error != "not needed" {
		t.Errorf("reason not stored, got %q", got.Error)
	}
}

func TestCancel_Running(t *testing.T) {
	mgr, _ := newManager(t)

	started := make(chan struct{})

	mustRegister(t, mgr, &funcHandler{
		typ: "blocker",
		fn: func(ctx context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "blocker"})
	<-started // handler is running

	if err := mgr.Cancel(context.Background(), tk.ID, "stop it"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	done := waitDone(t, mgr, tk.ID)
	if done.Status != task.StatusCancelled {
		t.Errorf("want cancelled, got %s", done.Status)
	}
}

func TestCancel_Terminal(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "ok", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "ok"})
	waitDone(t, mgr, tk.ID)

	err := mgr.Cancel(context.Background(), tk.ID, "too late")
	if !errors.Is(err, task.ErrNotCancellable) {
		t.Errorf("want ErrNotCancellable, got %v", err)
	}
}

func TestGet_NotFound(t *testing.T) {
	mgr, _ := newManager(t)
	_, err := mgr.Get(context.Background(), uuid.New().String())
	if !errors.Is(err, task.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestHandlerNotFound(t *testing.T) {
	mgr, _ := newManager(t)
	// No handler registered for "ghost".
	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "ghost"})
	done := waitDone(t, mgr, tk.ID)
	if done.Status != task.StatusFailed {
		t.Errorf("want failed, got %s", done.Status)
	}
}

func TestDuplicateRegister(t *testing.T) {
	mgr, _ := newManager(t)
	h := &funcHandler{typ: "dup", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}}
	mustRegister(t, mgr, h)
	if err := mgr.Register(h); !errors.Is(err, task.ErrDuplicateHandler) {
		t.Errorf("want ErrDuplicateHandler, got %v", err)
	}
}

func TestList_StatusFilter(t *testing.T) {
	mgr, _ := newManager(t)

	mustRegister(t, mgr, &funcHandler{typ: "quick", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	future := time.Now().Add(1 * time.Hour)
	t1 := mustSubmit(t, mgr, task.SubmitRequest{Type: "quick", ScheduledAt: &future}) // stays pending
	t2 := mustSubmit(t, mgr, task.SubmitRequest{Type: "quick"})
	waitDone(t, mgr, t2.ID)

	pending, err := mgr.List(context.Background(), task.ListFilter{Status: []task.TaskStatus{task.StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != t1.ID {
		t.Errorf("pending filter wrong: got %v", ids(pending))
	}

	succeeded, err := mgr.List(context.Background(), task.ListFilter{Status: []task.TaskStatus{task.StatusSucceeded}})
	if err != nil {
		t.Fatal(err)
	}
	if len(succeeded) != 1 || succeeded[0].ID != t2.ID {
		t.Errorf("succeeded filter wrong: got %v", ids(succeeded))
	}
}

func TestList_OwnerFilter(t *testing.T) {
	mgr, _ := newManager(t)

	future := time.Now().Add(1 * time.Hour)
	mustSubmit(t, mgr, task.SubmitRequest{Type: "x", OwnerType: "bot", OwnerID: "b1", ScheduledAt: &future})
	mustSubmit(t, mgr, task.SubmitRequest{Type: "x", OwnerType: "bot", OwnerID: "b2", ScheduledAt: &future})
	mustSubmit(t, mgr, task.SubmitRequest{Type: "x", OwnerType: "bot", OwnerID: "b1", ScheduledAt: &future})

	got, err := mgr.List(context.Background(), task.ListFilter{OwnerType: "bot", OwnerID: "b1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 tasks for b1, got %d", len(got))
	}
}

func TestList_LimitOffset(t *testing.T) {
	mgr, _ := newManager(t)

	future := time.Now().Add(1 * time.Hour)
	for i := 0; i < 5; i++ {
		mustSubmit(t, mgr, task.SubmitRequest{Type: "x", ScheduledAt: &future})
	}

	page1, _ := mgr.List(context.Background(), task.ListFilter{Limit: 2, Offset: 0})
	page2, _ := mgr.List(context.Background(), task.ListFilter{Limit: 2, Offset: 2})

	if len(page1) != 2 {
		t.Errorf("page1: want 2, got %d", len(page1))
	}
	if len(page2) != 2 {
		t.Errorf("page2: want 2, got %d", len(page2))
	}
	// Pages must not overlap.
	seen := make(map[string]bool)
	for _, tk := range append(page1, page2...) {
		if seen[tk.ID] {
			t.Errorf("duplicate task %s across pages", tk.ID)
		}
		seen[tk.ID] = true
	}
}

func TestSerializationKey_Sequential(t *testing.T) {
	mgr, _ := newManager(t)

	firstRunning := make(chan struct{})
	releaseFirst := make(chan struct{})

	var mu sync.Mutex
	var order []int
	call := 0

	mustRegister(t, mgr, &funcHandler{
		typ: "seq",
		fn: func(ctx context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
			mu.Lock()
			call++
			n := call
			mu.Unlock()

			if n == 1 {
				close(firstRunning)
				<-releaseFirst
			}

			mu.Lock()
			order = append(order, n)
			mu.Unlock()
			return &task.TaskResult{}, nil
		},
	})

	ctx := context.Background()
	const key = "serialize-me"

	t1 := mustSubmit(t, mgr, task.SubmitRequest{Type: "seq", SerializationKey: key})
	<-firstRunning // t1 is running

	t2 := mustSubmit(t, mgr, task.SubmitRequest{Type: "seq", SerializationKey: key})

	// Give the scheduler several cycles to try dispatching t2.
	time.Sleep(5 * testPoll)

	tk2, err := mgr.Get(ctx, t2.ID)
	if err != nil {
		t.Fatal(err)
	}
	// t2 must not be running — t1 still holds the key.
	if tk2.Status == task.StatusRunning {
		t.Errorf("t2 started while t1 still holds the serialization key")
	}

	close(releaseFirst) // unblock t1

	waitDone(t, mgr, t1.ID)
	waitDone(t, mgr, t2.ID)

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("wrong execution order: %v", order)
	}
}

func TestScheduledAt_FutureNotRun(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "timed", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	future := time.Now().Add(1 * time.Hour)
	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "timed", ScheduledAt: &future})

	time.Sleep(5 * testPoll) // several scheduler ticks

	got, _ := mgr.Get(context.Background(), tk.ID)
	if got.Status != task.StatusPending {
		t.Errorf("future task should remain pending, got %s", got.Status)
	}
}

func TestScheduledAt_PastRunsImmediately(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "past", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	past := time.Now().Add(-1 * time.Second)
	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "past", ScheduledAt: &past})
	done := waitDone(t, mgr, tk.ID)
	if done.Status != task.StatusSucceeded {
		t.Errorf("past-scheduled task should succeed, got %s", done.Status)
	}
}

func TestWait_AlreadyTerminal(t *testing.T) {
	mgr, _ := newManager(t)
	mustRegister(t, mgr, &funcHandler{typ: "instant", fn: func(_ context.Context, _ *task.Task, _ task.Controller) (*task.TaskResult, error) {
		return &task.TaskResult{}, nil
	}})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "instant"})
	waitDone(t, mgr, tk.ID) // first wait

	// Second Wait should return immediately without error.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := mgr.Wait(ctx, tk.ID); err != nil {
		t.Errorf("second Wait on terminal task: %v", err)
	}
}

func TestWait_ContextCancelled(t *testing.T) {
	mgr, _ := newManager(t)
	// Submit a future-dated task that will never run within the test.
	future := time.Now().Add(1 * time.Hour)
	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "never", ScheduledAt: &future})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := mgr.Wait(ctx, tk.ID)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want DeadlineExceeded, got %v", err)
	}
}

func TestUpdateProgress(t *testing.T) {
	mgr, _ := newManager(t)

	ready := make(chan struct{})

	mustRegister(t, mgr, &funcHandler{
		typ: "progress",
		fn: func(ctx context.Context, _ *task.Task, ctl task.Controller) (*task.TaskResult, error) {
			if err := ctl.UpdateProgress(ctx, "halfway"); err != nil {
				return nil, err
			}
			close(ready)
			return &task.TaskResult{}, nil
		},
	})

	tk := mustSubmit(t, mgr, task.SubmitRequest{Type: "progress"})
	waitDone(t, mgr, tk.ID)

	got, _ := mgr.Get(context.Background(), tk.ID)
	// Progress is overwritten by the final status update, so we just verify the
	// task succeeded (the store write in UpdateProgress didn't break anything).
	if got.Status != task.StatusSucceeded {
		t.Errorf("want succeeded, got %s", got.Status)
	}
}

// ---- MemoryStore unit tests ----

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := task.NewMemoryStore()
	_, err := s.Get(context.Background(), "missing")
	if !errors.Is(err, task.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_CreateAndGet(t *testing.T) {
	s := task.NewMemoryStore()
	ctx := context.Background()
	now := time.Now()
	tk := &task.Task{ID: "t1", Type: "x", Status: task.StatusPending, CreatedAt: now, UpdatedAt: now}
	if err := s.Create(ctx, tk); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "t1" || got.Status != task.StatusPending {
		t.Errorf("unexpected task: %+v", got)
	}
}

func TestMemoryStore_MarkInterruptedOnStartup(t *testing.T) {
	s := task.NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	mkTask := func(id string, status task.TaskStatus) {
		tk := &task.Task{ID: id, Type: "x", Status: status, CreatedAt: now, UpdatedAt: now}
		if err := s.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	mkTask("running", task.StatusRunning)
	mkTask("queued", task.StatusQueued)
	mkTask("pending", task.StatusPending)
	mkTask("succeeded", task.StatusSucceeded)

	if err := s.MarkInterruptedOnStartup(ctx); err != nil {
		t.Fatal(err)
	}

	check := func(id string, want task.TaskStatus) {
		t.Helper()
		got, _ := s.Get(ctx, id)
		if got.Status != want {
			t.Errorf("%s: want %s, got %s", id, want, got.Status)
		}
	}
	check("running", task.StatusInterrupted)
	check("queued", task.StatusPending)
	check("pending", task.StatusPending)
	check("succeeded", task.StatusSucceeded)
}

func TestMemoryStore_FindDueTasks(t *testing.T) {
	s := task.NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	past := now.Add(-1 * time.Second)
	future := now.Add(1 * time.Hour)

	for _, td := range []struct {
		id          string
		scheduledAt *time.Time
		status      task.TaskStatus
	}{
		{"due-nil", nil, task.StatusPending},
		{"due-past", &past, task.StatusPending},
		{"future", &future, task.StatusPending},
		{"running", nil, task.StatusRunning},
	} {
		var sat *time.Time
		if td.scheduledAt != nil {
			v := *td.scheduledAt
			sat = &v
		}
		tk := &task.Task{ID: td.id, Type: "x", Status: td.status, ScheduledAt: sat, CreatedAt: now, UpdatedAt: now}
		if err := s.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.FindDueTasks(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("want 2 due tasks, got %d: %v", len(got), ids(got))
	}
}

func TestMemoryStore_FindDueTasks_Limit(t *testing.T) {
	s := task.NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 5; i++ {
		tk := &task.Task{ID: uuid.New().String(), Type: "x", Status: task.StatusPending, CreatedAt: now, UpdatedAt: now}
		if err := s.Create(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := s.FindDueTasks(ctx, now, 3)
	if len(got) != 3 {
		t.Errorf("want 3, got %d", len(got))
	}
}

func TestIsRetryable(t *testing.T) {
	base := errors.New("oops")
	re := &task.RetryableError{Cause: base}

	if !task.IsRetryable(re) {
		t.Error("IsRetryable should be true for RetryableError")
	}
	if task.IsRetryable(base) {
		t.Error("IsRetryable should be false for plain error")
	}
}

func TestBackoffFor(t *testing.T) {
	// RetryableError with explicit backoff should honour it.
	explicit := &task.RetryableError{Cause: errors.New("x"), Backoff: 7 * time.Second}
	if d := task.BackoffFor(explicit, 0); d != 7*time.Second {
		t.Errorf("explicit backoff: want 7s, got %v", d)
	}

	// Without explicit backoff, should use exponential formula.
	plain := &task.RetryableError{Cause: errors.New("x")}
	d0 := task.BackoffFor(plain, 0) // 2^0 * 5s = 5s
	if d0 != 5*time.Second {
		t.Errorf("attempt 0: want 5s, got %v", d0)
	}
	d1 := task.BackoffFor(plain, 1) // 2^1 * 5s = 10s
	if d1 != 10*time.Second {
		t.Errorf("attempt 1: want 10s, got %v", d1)
	}

	// Cap at 10 minutes.
	dBig := task.BackoffFor(plain, 20)
	if dBig != 10*time.Minute {
		t.Errorf("cap: want 10m, got %v", dBig)
	}
}

func TestTaskStatus_IsTerminal(t *testing.T) {
	terminal := []task.TaskStatus{
		task.StatusSucceeded, task.StatusFailed,
		task.StatusCancelled, task.StatusInterrupted,
	}
	nonTerminal := []task.TaskStatus{
		task.StatusPending, task.StatusQueued, task.StatusRunning,
	}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}

// ---- helpers ----

func ids(tasks []task.Task) []string {
	out := make([]string, len(tasks))
	for i, tk := range tasks {
		out[i] = tk.ID
	}
	return out
}
