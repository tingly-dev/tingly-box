package command

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// reloadedBot is the per-bot status returned by the server's reload endpoint.
type reloadedBot struct {
	UUID    string `json:"uuid"`
	Running bool   `json:"running"`
}

// notifyServerBotReload pokes the running server's bot-reload endpoint
// (POST /api/v1/imbot-admin/reload) so bot settings written by this CLI
// process — a separate process sharing the SQLite store — take effect
// immediately instead of waiting for the server's low-frequency reconcile
// loop.
//
// Best-effort by design: when the server is not running there is nothing to
// notify, and the initial sync on next startup picks the change up. The live
// port is discovered via the runtime port file (gated on the PID lock, see
// .design/runtime-port-file.md). On acknowledgment it returns the per-bot
// statuses reported by the server, so callers can tell whether a specific
// bot actually came up — Sync swallows individual start failures by design.
func notifyServerBotReload(appManager *AppManager) ([]reloadedBot, bool) {
	if appManager == nil || appManager.AppConfig() == nil {
		return nil, false
	}

	if !lock.NewFileLock(appManager.AppConfig().ConfigDir()).IsLocked() {
		return nil, false
	}

	url := fmt.Sprintf("http://localhost:%d/api/v1/imbot-admin/reload", appManager.GetRuntimeServerPort())
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, false
	}
	if token := appManager.GetUserToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logrus.WithError(err).Debug("Failed to notify running server to reload bots")
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logrus.WithField("status", resp.StatusCode).Debug("Server rejected bot reload notification")
		return nil, false
	}

	var parsed struct {
		Success bool          `json:"success"`
		Bots    []reloadedBot `json:"bots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || !parsed.Success {
		return nil, false
	}
	return parsed.Bots, true
}

// botRunningAfterReload reports whether the given bot came up in a reload
// response.
func botRunningAfterReload(bots []reloadedBot, uuid string) bool {
	for _, b := range bots {
		if b.UUID == uuid {
			return b.Running
		}
	}
	return false
}
