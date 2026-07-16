package imbot

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// reconcileInterval paces the safety-net loop. State changes are applied at
// their source (web API handlers start/stop bots inline; the CLI pokes
// POST /api/v1/imbot-admin/reload after writing the shared settings store),
// so this loop only covers crashed bots and direct store edits that bypassed
// both paths. One minute keeps that worst-case recovery delay short; a no-op
// pass is one SQLite read plus an in-memory compare and stays silent, so the
// frequency is essentially free.
const reconcileInterval = time.Minute

// periodicBotSync reconciles bot runtime state with stored settings.
//
// It runs one immediate sync at startup (so bots enabled while the server
// was down come up without any UI interaction), then keeps a low-frequency
// reconcile loop as a self-healing backstop. It is intentionally NOT the
// primary propagation path — see reconcileInterval.
func (m *BotManager) periodicBotSync(ctx context.Context) {
	// Initial sync immediately after startup
	if err := m.Sync(ctx); err != nil {
		logrus.WithError(err).Warn("Initial bot sync failed")
	}

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Sync itself logs every bot it actually starts or stops, so a
			// no-op reconcile stays completely silent.
			if err := m.Sync(ctx); err != nil {
				logrus.WithError(err).Warn("Bot reconcile failed")
			}
		case <-ctx.Done():
			logrus.Debug("Bot reconcile loop stopped")
			return
		}
	}
}
