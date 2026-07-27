package security

import "github.com/sirupsen/logrus"

// LogAuditor implements PairingAuditor by writing pairing events through the
// regular application logger. Pairing events don't need a separate,
// unpersisted audit trail — they're security-relevant log lines like any
// other, and belong in the same stream operators already watch.
type LogAuditor struct{}

// NewLogAuditor returns a PairingAuditor backed by logrus.
func NewLogAuditor() PairingAuditor { return LogAuditor{} }

// Info logs a successful pairing event at info level.
func (LogAuditor) Info(action, userID, clientIP, message string, details map[string]interface{}) {
	auditFields(action, userID, clientIP, details).Info(message)
}

// Warn logs a rejected or otherwise noteworthy pairing event at warn level.
func (LogAuditor) Warn(action, userID, clientIP, message string, details map[string]interface{}) {
	auditFields(action, userID, clientIP, details).Warn(message)
}

func auditFields(action, userID, clientIP string, details map[string]interface{}) *logrus.Entry {
	fields := logrus.Fields{"action": action}
	if userID != "" {
		fields["user_id"] = userID
	}
	if clientIP != "" {
		fields["client_ip"] = clientIP
	}
	for k, v := range details {
		fields[k] = v
	}
	return logrus.WithFields(fields)
}
