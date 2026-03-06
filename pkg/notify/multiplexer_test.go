package notify

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockProvider is a test provider that records calls
type mockProvider struct {
	name      string
	sendFunc  func(ctx context.Context, n *Notification) (*Result, error)
	closeFunc func() error
	mu        sync.Mutex
	calls     int
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Send(ctx context.Context, n *Notification) (*Result, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	if m.sendFunc != nil {
		return m.sendFunc(ctx, n)
	}
	return &Result{Provider: m.name, Success: true}, nil
}

func (m *mockProvider) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockProvider) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// TestNewMultiplexer tests multiplexer creation
func TestNewMultiplexer(t *testing.T) {
	m := NewMultiplexer(
		WithMinLevel(LevelWarning),
		WithDefaultRetry(3),
	)

	if m.minLevel != LevelWarning {
		t.Errorf("expected minLevel Warning, got %v", m.minLevel)
	}
	if m.defaultRetry != 3 {
		t.Errorf("expected defaultRetry 3, got %d", m.defaultRetry)
	}
	if len(m.providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(m.providers))
	}
}

// TestMultiplexerAddProvider tests adding providers
func TestMultiplexerAddProvider(t *testing.T) {
	m := NewMultiplexer()
	p := &mockProvider{name: "test"}

	m.AddProvider(p)

	if len(m.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.providers))
	}
	if m.providers["test"] != p {
		t.Error("provider not stored correctly")
	}
	if m.configs["test"] == nil {
		t.Error("config should be created for provider")
	}
}

// TestMultiplexerAddProviderWithConfig tests adding provider with config
func TestMultiplexerAddProviderWithConfig(t *testing.T) {
	m := NewMultiplexer()
	p := &mockProvider{name: "test"}
	cfg := &ProviderConfig{
		RetryCount: 5,
		Timeout:    10 * time.Second,
	}

	m.AddProviderWithConfig(p, cfg)

	if len(m.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.providers))
	}
	if m.configs["test"] == nil {
		t.Error("config should be set")
	}
	if m.configs["test"].RetryCount != 5 {
		t.Errorf("expected RetryCount 5, got %d", m.configs["test"].RetryCount)
	}
}

// TestMultiplexerRemoveProvider tests removing providers
func TestMultiplexerRemoveProvider(t *testing.T) {
	m := NewMultiplexer()
	p := &mockProvider{name: "test"}
	m.AddProvider(p)

	if !m.RemoveProvider("test") {
		t.Error("expected RemoveProvider to return true")
	}
	if len(m.providers) != 0 {
		t.Errorf("expected 0 providers after removal")
	}

	if m.RemoveProvider("nonexistent") {
		t.Error("expected RemoveProvider to return false for nonexistent provider")
	}
}

// TestMultiplexerListProviders tests listing providers
func TestMultiplexerListProviders(t *testing.T) {
	m := NewMultiplexer()
	m.AddProvider(&mockProvider{name: "provider1"})
	m.AddProvider(&mockProvider{name: "provider2"})
	m.AddProvider(&mockProvider{name: "provider3"})

	names := m.ListProviders()
	if len(names) != 3 {
		t.Errorf("expected 3 providers, got %d", len(names))
	}
}

// TestMultiplexerGetProvider tests getting a provider
func TestMultiplexerGetProvider(t *testing.T) {
	m := NewMultiplexer()
	p := &mockProvider{name: "test"}
	m.AddProvider(p)

	got := m.GetProvider("test")
	if got != p {
		t.Error("expected same provider instance")
	}

	got = m.GetProvider("nonexistent")
	if got != nil {
		t.Error("expected nil for nonexistent provider")
	}
}

// TestMultiplexerSend tests sending notifications
func TestMultiplexerSend(t *testing.T) {
	m := NewMultiplexer()

	var sendCount int
	p := &mockProvider{
		name: "test",
		sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
			sendCount++
			return &Result{Provider: "test", Success: true}, nil
		},
	}
	m.AddProvider(p)

	ctx := context.Background()
	results, err := m.Send(ctx, &Notification{Message: "test"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("expected successful result")
	}
	if p.getCalls() != 1 {
		t.Errorf("expected 1 call, got %d", p.getCalls())
	}
}

// TestMultiplexerSendEmpty tests sending with no providers
func TestMultiplexerSendEmpty(t *testing.T) {
	m := NewMultiplexer()

	ctx := context.Background()
	results, err := m.Send(ctx, &Notification{Message: "test"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestMultiplexerSendLevelFilter tests level filtering
func TestMultiplexerSendLevelFilter(t *testing.T) {
	m := NewMultiplexer(WithMinLevel(LevelWarning))

	sendCalled := false
	p := &mockProvider{
		name: "test",
		sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
			sendCalled = true
			return &Result{Provider: "test", Success: true}, nil
		},
	}
	m.AddProvider(p)

	ctx := context.Background()
	results, err := m.Send(ctx, &Notification{Message: "test", Level: LevelDebug})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if sendCalled {
		t.Error("provider should not be called for debug level")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestMultiplexerSendTo tests sending to specific provider
func TestMultiplexerSendTo(t *testing.T) {
	m := NewMultiplexer()

	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	m.AddProvider(p1)
	m.AddProvider(p2)

	ctx := context.Background()
	result, err := m.SendTo(ctx, "p1", &Notification{Message: "test"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Provider != "p1" {
		t.Errorf("expected provider p1, got %v", result.Provider)
	}
	if p1.getCalls() != 1 {
		t.Errorf("expected p1 called once, got %d", p1.getCalls())
	}
	if p2.getCalls() != 0 {
		t.Errorf("expected p2 not called, got %d", p2.getCalls())
	}
}

// TestMultiplexerSendToNotFound tests SendTo with nonexistent provider
func TestMultiplexerSendToNotFound(t *testing.T) {
	m := NewMultiplexer()
	m.AddProvider(&mockProvider{name: "test"})

	ctx := context.Background()
	result, err := m.SendTo(ctx, "nonexistent", &Notification{Message: "test"})

	if err != ErrProviderNotFound {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
	if result != nil {
		t.Error("expected nil result")
	}
}

// TestMultiplexerSendWithError tests handling of provider errors
func TestMultiplexerSendWithError(t *testing.T) {
	m := NewMultiplexer()

	p := &mockProvider{
		name: "test",
		sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
			return &Result{Provider: "test", Success: false, Error: context.DeadlineExceeded}, context.DeadlineExceeded
		},
	}
	m.AddProvider(p)

	ctx := context.Background()
	results, err := m.Send(ctx, &Notification{Message: "test"})

	if err != ErrSendFailed {
		t.Errorf("expected ErrSendFailed, got %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected failed result")
	}
}

// TestMultiplexerConcurrentSend tests concurrent sends
func TestMultiplexerConcurrentSend(t *testing.T) {
	m := NewMultiplexer()

	// Add multiple providers with different latencies
	latencies := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	for i, lat := range latencies {
		p := &mockProvider{
			name: "p" + string(rune('0'+i)),
			sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
				time.Sleep(lat)
				return &Result{Provider: "test", Success: true}, nil
			},
		}
		m.AddProvider(p)
	}

	ctx := context.Background()
	start := time.Now()
	results, err := m.Send(ctx, &Notification{Message: "test"})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	// Concurrent sends should complete in ~30ms (max latency), not 60ms (sum)
	if elapsed > 100*time.Millisecond {
		t.Errorf("sends took too long: %v, expected concurrent execution", elapsed)
	}
}

// TestMultiplexerClose tests closing the multiplexer
func TestMultiplexerClose(t *testing.T) {
	m := NewMultiplexer()

	closed := false
	p := &mockProvider{
		name: "test",
		closeFunc: func() error {
			closed = true
			return nil
		},
	}
	m.AddProvider(p)

	m.Close()

	if !closed {
		t.Error("expected provider to be closed")
	}
}

// TestMultiplexerRetry tests retry functionality
func TestMultiplexerRetry(t *testing.T) {
	m := NewMultiplexer(WithDefaultRetry(2))

	attempts := 0
	p := &mockProvider{
		name: "test",
		sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
			attempts++
			if attempts < 2 {
				return &Result{Provider: "test", Success: false}, context.DeadlineExceeded
			}
			return &Result{Provider: "test", Success: true}, nil
		},
	}
	m.AddProviderWithConfig(p, &ProviderConfig{
		RetryCount: 3,
		RetryDelay: 10 * time.Millisecond,
	})

	ctx := context.Background()
	results, err := m.Send(ctx, &Notification{Message: "test"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 2 { // Initial fails, second succeeds
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("expected successful result")
	}
}

// TestMultiplexerContextCancellation tests context cancellation during retry
func TestMultiplexerContextCancellation(t *testing.T) {
	m := NewMultiplexer(WithDefaultRetry(5))

	p := &mockProvider{
		name: "test",
		sendFunc: func(ctx context.Context, n *Notification) (*Result, error) {
			return &Result{Provider: "test", Success: false}, context.DeadlineExceeded
		},
	}
	m.AddProviderWithConfig(p, &ProviderConfig{
		RetryCount: 5,
		RetryDelay: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _ = m.Send(ctx, &Notification{Message: "test"})
	elapsed := time.Since(start)

	// Should cancel before all retries complete
	if elapsed > 200*time.Millisecond {
		t.Errorf("context cancellation took too long: %v", elapsed)
	}
}
