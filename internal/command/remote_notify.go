package command

import (
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/pkg/lock"
)

// notifyServerBotReload pokes the running server's bot-reload endpoint
// (POST /api/v1/imbot-admin/reload) so bot settings written by this CLI
// process — a separate process sharing the SQLite store — take effect
// immediately instead of waiting for the server's low-frequency reconcile
// loop.
//
// Best-effort by design: when the server is not running there is nothing to
// notify, and the initial sync on next startup picks the change up. The live
// port is discovered via the runtime port file (gated on the PID lock, see
// .design/runtime-port-file.md). Returns true only when the server
// acknowledged the reload.
func notifyServerBotReload(appManager *AppManager) bool {
	if appManager == nil || appManager.AppConfig() == nil {
		return false
	}

	fileLock := lock.NewFileLock(appManager.AppConfig().ConfigDir())
	if !fileLock.IsLocked() {
		return false
	}

	url := fmt.Sprintf("http://localhost:%d/api/v1/imbot-admin/reload", appManager.GetRuntimeServerPort())
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return false
	}
	if token := appManager.GetUserToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logrus.WithError(err).Debug("Failed to notify running server to reload bots")
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logrus.WithField("status", resp.StatusCode).Debug("Server rejected bot reload notification")
		return false
	}
	return true
}
