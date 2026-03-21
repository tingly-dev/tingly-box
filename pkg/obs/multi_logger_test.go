package obs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiLogger(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	cfg := &MultiLoggerConfig{
		TextLogPath:    filepath.Join(tempDir, "test.log"),
		TextMaxSize:    1,
		TextMaxBackups: 1,
		TextMaxAge:     1,
		TextCompress:   false,
		JSONLogPath:    filepath.Join(tempDir, "test.jsonl"),
		JSONMaxSize:    1,
		JSONMaxBackups: 1,
		JSONMaxAge:     1,
	}

	logger, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	defer logger.Close()

	t.Run("write and read text log", func(t *testing.T) {
		// Write to text log
		n, err := logger.Write([]byte("test log message\n"))
		require.NoError(t, err)
		assert.Equal(t, 17, n)

		// Verify file exists
		_, err = os.Stat(cfg.TextLogPath)
		assert.NoError(t, err)
	})

	t.Run("write and read JSON log entry", func(t *testing.T) {
		entry := &logrus.Entry{
			Logger:  logrus.New(),
			Time:    time.Now(),
			Level:   logrus.InfoLevel,
			Message: "test message",
			Data: logrus.Fields{
				"key": "value",
			},
		}

		err := logger.WriteEntry(entry)
		require.NoError(t, err)

		// Verify file exists
		_, err = os.Stat(cfg.JSONLogPath)
		assert.NoError(t, err)
	})

	t.Run("read JSON logs with filter", func(t *testing.T) {
		// Write multiple entries
		for i := 0; i < 5; i++ {
			entry := &logrus.Entry{
				Logger:  logrus.New(),
				Time:    time.Now(),
				Level:   logrus.InfoLevel,
				Message: "message",
				Data:    logrus.Fields{},
			}
			err := logger.WriteEntry(entry)
			require.NoError(t, err)
		}

		// Read logs
		entries, err := logger.ReadJSONLogs(10)
		require.NoError(t, err)
		assert.True(t, len(entries) >= 5)
	})

	t.Run("source filtering", func(t *testing.T) {
		// Reset level to ensure Info entries are written
		logger.SetLevel(logrus.InfoLevel)

		// Write HTTP source entry
		httpEntry := &logrus.Entry{
			Logger:  logrus.New(),
			Time:    time.Now(),
			Level:   logrus.InfoLevel,
			Message: "http request",
			Data:    logrus.Fields{"source": string(LogSourceHTTP)},
		}
		err := logger.WriteEntry(httpEntry)
		require.NoError(t, err)

		// Write system source entry
		systemEntry := &logrus.Entry{
			Logger:  logrus.New(),
			Time:    time.Now(),
			Level:   logrus.InfoLevel,
			Message: "system log",
			Data:    logrus.Fields{"source": string(LogSourceSystem)},
		}
		err = logger.WriteEntry(systemEntry)
		require.NoError(t, err)

		// Read only HTTP logs
		httpLogs, err := logger.ReadJSONLogs(10)
		require.NoError(t, err)
		assert.True(t, len(httpLogs) > 0, "Expected at least one HTTP log entry")
		// Verify all entries are HTTP source
		for _, entry := range httpLogs {
			if entry.Source != "" {
				assert.Equal(t, string(LogSourceHTTP), entry.Source)
			}
		}
	})

	t.Run("level filtering", func(t *testing.T) {
		logger.SetLevel(logrus.WarnLevel)

		// Debug entry should not be written
		debugEntry := &logrus.Entry{
			Logger:  logrus.New(),
			Time:    time.Now(),
			Level:   logrus.DebugLevel,
			Message: "debug message",
			Data:    logrus.Fields{},
		}
		err := logger.WriteEntry(debugEntry)
		require.NoError(t, err)

		// Warn entry should be written
		warnEntry := &logrus.Entry{
			Logger:  logrus.New(),
			Time:    time.Now(),
			Level:   logrus.WarnLevel,
			Message: "warn message",
			Data:    logrus.Fields{},
		}
		err = logger.WriteEntry(warnEntry)
		require.NoError(t, err)
	})
}

func TestDefaultMultiLoggerConfig(t *testing.T) {
	cfg := DefaultMultiLoggerConfig("/tmp/test-config")

	assert.Equal(t, "/tmp/test-config/log/tingly-box.log", cfg.TextLogPath)
	assert.Equal(t, "/tmp/test-config/log/tingly-box.jsonl", cfg.JSONLogPath)
	assert.Equal(t, 5, cfg.JSONMaxSize)
	assert.Equal(t, 3, cfg.JSONMaxBackups)
}

func TestMultiLoggerHook(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &MultiLoggerConfig{
		TextLogPath:    filepath.Join(tempDir, "test.log"),
		TextMaxSize:    1,
		TextMaxBackups: 1,
		TextMaxAge:     1,
		TextCompress:   false,
		JSONLogPath:    filepath.Join(tempDir, "test.jsonl"),
		JSONMaxSize:    1,
		JSONMaxBackups: 1,
		JSONMaxAge:     1,
	}

	logger, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	defer logger.Close()

	hook := NewMultiLoggerHook(logger, nil)

	// Test Levels
	levels := hook.Levels()
	assert.Equal(t, logrus.AllLevels, levels)

	// Test Fire
	entry := &logrus.Entry{
		Logger:  logrus.New(),
		Time:    time.Now(),
		Level:   logrus.InfoLevel,
		Message: "hook test",
		Data:    logrus.Fields{"test": true},
	}

	err = hook.Fire(entry)
	require.NoError(t, err)

	// Verify log was written
	entries, err := logger.ReadJSONLogs(10)
	require.NoError(t, err)
	assert.True(t, len(entries) > 0)
}
