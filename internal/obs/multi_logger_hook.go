package obs

import (
	"github.com/sirupsen/logrus"
)

// MultiLoggerHook is a logrus hook that writes to the multi-mode logger
// This is used for global logrus integration (e.g., main application logs)
type MultiLoggerHook struct {
	logger *MultiLogger
	levels []logrus.Level
}

// NewMultiLoggerHook creates a new multi logger hook for global use
// Use this for adding the MultiLogger to the global logrus instance
func NewMultiLoggerHook(logger *MultiLogger, levels []logrus.Level) *MultiLoggerHook {
	if levels == nil {
		levels = logrus.AllLevels
	}
	return &MultiLoggerHook{
		logger: logger,
		levels: levels,
	}
}

// Levels returns the log levels this hook processes
func (h *MultiLoggerHook) Levels() []logrus.Level {
	return h.levels
}

// Fire processes each log entry, writing it to the JSON log
func (h *MultiLoggerHook) Fire(entry *logrus.Entry) error {
	return h.logger.WriteEntry(entry)
}
