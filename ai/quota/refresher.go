package quota

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Refresher runs periodic quota refreshes in the background.
type Refresher struct {
	manager  *Manager
	interval time.Duration
	stopCh   chan struct{}
	mu       sync.RWMutex
	running  bool
	logger   *logrus.Logger
}

// NewRefresher creates a background quota refresher.
func NewRefresher(manager *Manager, logger *logrus.Logger) *Refresher {
	return &Refresher{
		manager: manager,
		stopCh:  make(chan struct{}),
		logger:  logger,
	}
}

// Start starts the background refresh loop.
func (r *Refresher) Start(ctx context.Context, interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		r.logger.Warn("refresher already running")
		return
	}

	r.interval = interval
	r.running = true
	r.stopCh = make(chan struct{})

	r.logger.WithField("interval", interval).Info("starting quota refresher")

	go r.run(ctx)
}

// Stop stops the background refresh loop.
func (r *Refresher) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return
	}

	r.logger.Info("stopping quota refresher")
	close(r.stopCh)
	r.running = false
}

// IsRunning reports whether the refresher is running.
func (r *Refresher) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// run executes the refresh loop.
func (r *Refresher) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Refresh immediately before waiting for the first tick.
	r.refresh(ctx)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("context canceled, stopping refresher")
			return
		case <-r.stopCh:
			r.logger.Info("stop signal received, stopping refresher")
			return
		case <-ticker.C:
			r.refresh(ctx)
		}
	}
}

// refresh performs one scheduled refresh.
func (r *Refresher) refresh(ctx context.Context) {
	r.logger.Debug("running scheduled quota refresh")

	usages, err := r.manager.Refresh(ctx)
	if err != nil {
		r.logger.WithError(err).Error("scheduled refresh failed")
		return
	}

	successCount := 0
	errorCount := 0
	for _, usage := range usages {
		if usage.LastError != "" {
			errorCount++
		} else {
			successCount++
		}
	}

	r.logger.WithFields(logrus.Fields{
		"total":   len(usages),
		"success": successCount,
		"errors":  errorCount,
	}).Debug("scheduled refresh completed")
}

// SetInterval updates the refresh interval.
func (r *Refresher) SetInterval(interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interval = interval
}

// GetInterval returns the refresh interval.
func (r *Refresher) GetInterval() time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.interval
}
