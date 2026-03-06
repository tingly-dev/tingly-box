package notify

import (
	"context"
	"sync"
	"time"
)

// Multiplexer sends notifications to multiple providers
type Multiplexer struct {
	mu           sync.RWMutex
	providers    map[string]Provider
	configs      map[string]*ProviderConfig
	minLevel     Level
	defaultRetry int
}

// MultiplexerOption configures a Multiplexer
type MultiplexerOption func(*Multiplexer)

// WithMinLevel sets the minimum notification level to send
func WithMinLevel(level Level) MultiplexerOption {
	return func(m *Multiplexer) {
		m.minLevel = level
	}
}

// WithDefaultRetry sets the default retry count for all providers
func WithDefaultRetry(count int) MultiplexerOption {
	return func(m *Multiplexer) {
		m.defaultRetry = count
	}
}

// NewMultiplexer creates a new notification multiplexer
func NewMultiplexer(opts ...MultiplexerOption) *Multiplexer {
	m := &Multiplexer{
		providers:    make(map[string]Provider),
		configs:      make(map[string]*ProviderConfig),
		minLevel:     LevelDebug,
		defaultRetry: 0,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Send sends a notification to all configured providers concurrently
func (m *Multiplexer) Send(ctx context.Context, notification *Notification) ([]*Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, err
	}

	// Filter by level
	if !IsLevelAtLeast(notification.Level, m.minLevel) {
		return []*Result{}, nil
	}

	m.mu.RLock()
	providers := make([]Provider, 0, len(m.providers))
	configs := make([]*ProviderConfig, 0, len(m.providers))
	for name, p := range m.providers {
		providers = append(providers, p)
		configs = append(configs, m.getOrDefaultConfig(name))
	}
	m.mu.RUnlock()

	if len(providers) == 0 {
		return []*Result{}, nil
	}

	// Send to all providers concurrently
	results := make([]*Result, len(providers))
	var wg sync.WaitGroup
	wg.Add(len(providers))

	for i, p := range providers {
		go func(idx int, provider Provider, config *ProviderConfig) {
			defer wg.Done()
			result, err := m.sendWithRetry(ctx, provider, config, notification)
			if err != nil {
				result = &Result{
					Provider: provider.Name(),
					Success:  false,
					Error:    err,
				}
			}
			results[idx] = result
		}(i, p, configs[i])
	}

	wg.Wait()

	// Check for errors
	var hasError bool
	for _, r := range results {
		if r.Error != nil {
			hasError = true
			break
		}
	}

	if hasError {
		return results, ErrSendFailed
	}
	return results, nil
}

// sendWithRetry sends a notification with retry logic
func (m *Multiplexer) sendWithRetry(ctx context.Context, provider Provider, config *ProviderConfig, notification *Notification) (*Result, error) {
	var lastResult *Result
	var lastErr error

	retryCount := config.RetryCount
	if retryCount == 0 {
		retryCount = m.defaultRetry
	}

	retryDelay := config.RetryDelay
	if retryDelay == 0 {
		retryDelay = time.Second
	}

	for attempt := 0; attempt <= retryCount; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		start := time.Now()
		result, err := provider.Send(ctx, notification)
		latency := time.Since(start)

		if err == nil && result != nil {
			result.Latency = latency
			return result, nil
		}

		lastErr = err
		if result != nil {
			result.Latency = latency
		}
		lastResult = result

		// Wait before retry (if not last attempt)
		if attempt < retryCount {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}
	}

	if lastResult != nil {
		return lastResult, lastErr
	}
	return nil, lastErr
}

// getOrDefaultConfig returns provider config or default
func (m *Multiplexer) getOrDefaultConfig(name string) *ProviderConfig {
	if cfg, ok := m.configs[name]; ok {
		return cfg
	}
	return &ProviderConfig{Timeout: 30 * time.Second}
}

// SendTo sends a notification to a specific provider
func (m *Multiplexer) SendTo(ctx context.Context, providerName string, notification *Notification) (*Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, err
	}

	m.mu.RLock()
	provider, exists := m.providers[providerName]
	m.mu.RUnlock()

	if !exists {
		return nil, ErrProviderNotFound
	}

	start := time.Now()
	result, err := provider.Send(ctx, notification)
	if err != nil {
		if result == nil {
			result = &Result{
				Provider: providerName,
				Success:  false,
				Error:    err,
			}
		}
		result.Latency = time.Since(start)
		return result, err
	}

	result.Latency = time.Since(start)
	return result, nil
}

// AddProvider adds a notification provider
func (m *Multiplexer) AddProvider(provider Provider) {
	m.AddProviderWithConfig(provider, nil)
}

// AddProviderWithConfig adds a notification provider with specific configuration
func (m *Multiplexer) AddProviderWithConfig(provider Provider, config *ProviderConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name := provider.Name()
	m.providers[name] = provider

	if config == nil {
		config = &ProviderConfig{Timeout: 30 * time.Second}
	}
	if config.Name == "" {
		config.Name = name
	}
	m.configs[name] = config
}

// SetProviderConfig sets configuration for a provider
func (m *Multiplexer) SetProviderConfig(name string, config ProviderConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := config
	if cfg.Name == "" {
		cfg.Name = name
	}
	m.configs[name] = &cfg
}

// RemoveProvider removes a provider by name
func (m *Multiplexer) RemoveProvider(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; !exists {
		return false
	}

	provider := m.providers[name]
	provider.Close()
	delete(m.providers, name)
	delete(m.configs, name)
	return true
}

// GetProvider returns a provider by name
func (m *Multiplexer) GetProvider(name string) Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[name]
}

// ListProviders returns all registered provider names
func (m *Multiplexer) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// Close closes all providers
func (m *Multiplexer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for _, provider := range m.providers {
		if err := provider.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
