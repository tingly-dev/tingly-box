package imbot

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// periodicBotSync periodically syncs bot states with database settings.
// This ensures that bots created via CLI (with Enabled: true) are automatically started
// without requiring web UI interaction.
func (m *BotManager) periodicBotSync(ctx context.Context) {
	// Initial sync immediately after startup
	if err := m.Sync(ctx); err != nil {
		logrus.WithError(err).Warn("Initial bot sync failed")
	} else {
		logrus.Debug("Initial bot sync completed")
	}

	// Periodic sync every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.Sync(ctx); err != nil {
				logrus.WithError(err).Debug("Periodic bot sync failed")
			} else {
				logrus.Debug("Periodic bot sync completed")
			}
		case <-ctx.Done():
			logrus.Debug("Periodic bot sync stopped")
			return
		}
	}
}
