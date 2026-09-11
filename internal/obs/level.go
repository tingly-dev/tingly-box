package obs

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// LevelForStatus maps an HTTP status code to the log severity it deserves:
// a 5xx is a server-side failure (Error), a 4xx is a client/provider-side
// rejection worth noticing but not alarming on (Warn), and everything else
// is routine (Info). Shared by anything that logs one line per HTTP
// response — the access log and the upstream-call log — so status-to-level
// thresholds live in one place instead of being redefined per call site.
func LevelForStatus(statusCode int) logrus.Level {
	switch {
	case statusCode >= http.StatusInternalServerError:
		return logrus.ErrorLevel
	case statusCode >= http.StatusBadRequest:
		return logrus.WarnLevel
	default:
		return logrus.InfoLevel
	}
}
