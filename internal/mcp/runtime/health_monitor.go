package runtime

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// HealthMonitor provides health monitoring for persistent connections.
type HealthMonitor struct {
	source       ToolSource
	interval     time.Duration
	stopCh       chan struct{}
	stoppedCh    chan struct{}
	errorClassifier ErrorClassifier
	mu           sync.RWMutex
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(source ToolSource, errorClassifier ErrorClassifier) *HealthMonitor {
	return &HealthMonitor{
		source:          source,
		stopCh:          make(chan struct{}),
		stoppedCh:       make(chan struct{}),
		errorClassifier: errorClassifier,
	}
}

// Start begins health monitoring with the specified interval.
func (h *HealthMonitor) Start(ctx context.Context, interval time.Duration) {
	h.mu.Lock()
	if h.interval > 0 {
		h.mu.Unlock()
		return // Already running
	}
	h.interval = interval
	h.mu.Unlock()

	go h.monitor(ctx)
}

// Stop stops health monitoring.
func (h *HealthMonitor) Stop(ctx context.Context) {
	h.mu.Lock()
	if h.interval == 0 {
		h.mu.Unlock()
		return // Not running
	}
	h.interval = 0
	h.mu.Unlock()

	select {
	case h.stopCh <- struct{}{}:
		// Signal stop
	case <-time.After(5 * time.Second):
		logrus.Warn("health monitor: timeout sending stop signal")
	}

	select {
	case <-h.stoppedCh:
		// Stopped
	case <-ctx.Done():
		logrus.Warn("health monitor: context cancelled while waiting for stop")
	case <-time.After(5 * time.Second):
		logrus.Warn("health monitor: timeout waiting for stop")
	}
}

// monitor runs the health check loop.
func (h *HealthMonitor) monitor(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	defer close(h.stoppedCh)

	logrus.Debugf("health monitor: started for source=%s interval=%v", h.source.GetSourceID(), h.interval)

	for {
		select {
		case <-ticker.C:
			if err := h.source.HealthCheck(ctx); err != nil {
				logrus.WithFields(logrus.Fields{
					"source": h.source.GetSourceID(),
					"error":  err.Error(),
				}).Warn("health monitor: check failed")

				// Check if error is transient or permanent
				if h.errorClassifier != nil {
					if h.errorClassifier.IsPermanent(err) {
						logrus.WithFields(logrus.Fields{
							"source": h.source.GetSourceID(),
							"error":  err.Error(),
						}).Error("health monitor: permanent error detected, stopping monitoring")
						return
					}
				}

				// For transient errors, trigger reconnection
				if reconnectableSource, ok := h.source.(ReconnectableSource); ok {
					go reconnectableSource.Reconnect(ctx)
				}
			}

		case <-h.stopCh:
			logrus.Debugf("health monitor: stopped for source=%s", h.source.GetSourceID())
			return

		case <-ctx.Done():
			logrus.Debugf("health monitor: context cancelled for source=%s", h.source.GetSourceID())
			return
		}
	}
}

// ErrorClassifier classifies errors as transient or permanent.
type ErrorClassifier interface {
	IsTransient(err error) bool
	IsPermanent(err error) bool
}

// DefaultErrorClassifier provides default error classification.
type DefaultErrorClassifier struct{}

// IsTransient checks if an error is transient (temporary network issues, timeouts, etc.).
func (c *DefaultErrorClassifier) IsTransient(err error) bool {
	if err == nil {
		return false
	}

	// Network errors
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}

	// HTTP 5xx errors
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 && httpErr.StatusCode < 600
	}

	// Context errors (timeout, cancelled)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}

	return false
}

// IsPermanent checks if an error is permanent (auth failures, not found, etc.).
func (c *DefaultErrorClassifier) IsPermanent(err error) bool {
	if err == nil {
		return false
	}

	// HTTP 4xx errors (except 429 Too Many Requests)
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 && httpErr.StatusCode != 429
	}

	return false
}

// HTTPError represents an HTTP error with status code.
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// ReconnectableSource extends ToolSource with reconnection capability.
type ReconnectableSource interface {
	ToolSource
	Reconnect(ctx context.Context) error
}

// ExponentialBackoffStrategy implements exponential backoff with jitter.
type ExponentialBackoffStrategy struct {
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxRetries      int
}

// NewExponentialBackoffStrategy creates a new exponential backoff strategy.
func NewExponentialBackoffStrategy() *ExponentialBackoffStrategy {
	return &ExponentialBackoffStrategy{
		InitialInterval: 5 * time.Second,
		MaxInterval:     60 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      10,
	}
}

// NextRetry returns the duration to wait before the next retry.
func (s *ExponentialBackoffStrategy) NextRetry(retryCount int) time.Duration {
	if retryCount <= 0 {
		return s.InitialInterval
	}

	// Calculate next interval with exponential backoff: 2^(retryCount-1)
	// Use integer math for the shift operation
	multiplier := 1 << uint(retryCount-1)
	interval := time.Duration(float64(s.InitialInterval) * float64(multiplier))

	// Apply additional multiplier
	interval = time.Duration(float64(interval) * s.Multiplier)

	// Cap at max interval
	if interval > s.MaxInterval {
		interval = s.MaxInterval
	}

	// Add jitter (±25%)
	jitter := time.Duration(float64(interval) * 0.25 * (2.0*randFloat64() - 1.0))
	interval += jitter

	return interval
}

// ShouldRetry determines if another retry should be attempted.
func (s *ExponentialBackoffStrategy) ShouldRetry(retryCount int) bool {
	return retryCount < s.MaxRetries
}

func randFloat64() float64 {
	// Simple random float between 0 and 1
	return float64(time.Now().UnixNano()%1000) / 1000.0
}