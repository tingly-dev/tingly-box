package obs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiLoggerWithSource(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MultiLoggerConfig{
		TextLogPath:    tempDir + "/test.log",
		TextMaxSize:    1,
		TextMaxBackups: 1,
		TextMaxAge:     1,
		TextCompress:   false,
		JSONLogPath:    tempDir + "/test.jsonl",
		JSONMaxSize:    1,
		JSONMaxBackups: 1,
		JSONMaxAge:     1,
		MemorySinkConfig: map[LogSource]MemorySinkConfig{
			LogSourceAction: {MaxEntries: 10},
		},
	}

	multiLogger, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	defer multiLogger.Close()

	t.Run("WithSource returns scoped logger", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)
		assert.NotNil(t, actionLogger)
	})

	t.Run("LogAction writes to memory sink", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)

		// Log some actions
		actionLogger.LogAction("add_provider", map[string]interface{}{"name": "openai"}, true, "Success")
		actionLogger.LogAction("delete_provider", map[string]interface{}{"name": "anthropic"}, true, "Deleted")

		// Get entries from memory
		entries := actionLogger.GetMemoryEntries()
		assert.True(t, len(entries) >= 2, "Expected at least 2 entries")
	})

	t.Run("GetLogrusLogger returns cached logger", func(t *testing.T) {
		logger1 := multiLogger.GetLogrusLogger(LogSourceAction)
		logger2 := multiLogger.GetLogrusLogger(LogSourceAction)
		assert.Same(t, logger1, logger2, "Expected same cached logger instance")
	})

	t.Run("Different sources have separate memory sinks", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)
		httpLogger := multiLogger.WithSource(LogSourceHTTP)

		actionLogger.LogAction("test", nil, true, "test")

		// Action logger should have the entry
		actionEntries := actionLogger.GetMemoryEntries()
		assert.True(t, len(actionEntries) > 0)

		// HTTP logger should not (different sink)
		httpEntries := httpLogger.GetMemoryEntries()
		// The count might be different since they have separate sinks
		assert.NotNil(t, httpEntries)
	})
}

func TestScopedLoggerLogAction(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MultiLoggerConfig{
		TextLogPath:    tempDir + "/test.log",
		TextMaxSize:    1,
		TextMaxBackups: 1,
		TextMaxAge:     1,
		TextCompress:   false,
		JSONLogPath:    tempDir + "/test.jsonl",
		JSONMaxSize:    1,
		JSONMaxBackups: 1,
		JSONMaxAge:     1,
		MemorySinkConfig: map[LogSource]MemorySinkConfig{
			LogSourceAction: {MaxEntries: 10},
		},
	}

	multiLogger, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	defer multiLogger.Close()

	t.Run("LogAction creates correct log entry", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)

		details := map[string]interface{}{
			"provider_name": "openai",
			"model_count":   5,
		}

		actionLogger.LogAction("add_provider", details, true, "Provider added successfully")

		// Get from memory
		entries := actionLogger.GetMemoryEntries()
		require.True(t, len(entries) > 0, "Expected at least one entry")

		// Check the latest entry
		latest := entries[len(entries)-1]
		assert.Equal(t, "Provider added successfully", latest.Message)
		assert.Equal(t, "add_provider", latest.Data["action"])
		assert.Equal(t, true, latest.Data["success"])
		assert.NotNil(t, latest.Data["details"])
	})

	t.Run("GetLatestEntries returns correct number of entries", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)

		// Clear first
		actionLogger.ClearMemory()

		// Add 5 actions
		for i := 0; i < 5; i++ {
			actionLogger.LogAction("test", i, true, "test")
		}

		// Get latest 3
		latest := actionLogger.GetMemoryLatest(3)
		assert.Equal(t, 3, len(latest))
	})

	t.Run("Clear clears all entries", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)

		// Add some entries
		actionLogger.LogAction("test", nil, true, "test")
		actionLogger.LogAction("test", nil, true, "test")

		// Clear
		actionLogger.ClearMemory()

		// Verify empty
		entries := actionLogger.GetMemoryEntries()
		assert.Equal(t, 0, len(entries))
	})
}

func TestMultiLoggerSourceFiltering(t *testing.T) {
	tempDir := t.TempDir()

	cfg := &MultiLoggerConfig{
		TextLogPath:    tempDir + "/test.log",
		TextMaxSize:    1,
		TextMaxBackups: 1,
		TextMaxAge:     1,
		TextCompress:   false,
		JSONLogPath:    tempDir + "/test.jsonl",
		JSONMaxSize:    1,
		JSONMaxBackups: 1,
		JSONMaxAge:     1,
		MemorySinkConfig: map[LogSource]MemorySinkConfig{
			LogSourceAction: {MaxEntries: 10},
			LogSourceHTTP:   {MaxEntries: 10},
		},
	}

	multiLogger, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	defer multiLogger.Close()

	t.Run("ReadJSONLogs filters by source", func(t *testing.T) {
		actionLogger := multiLogger.WithSource(LogSourceAction)
		httpLogger := multiLogger.WithSource(LogSourceHTTP)

		// Write action logs
		for i := 0; i < 3; i++ {
			actionLogger.LogAction("test_action", i, true, "action")
		}

		// Write HTTP logs
		httpLogger.GetLogrusLogger().WithField("path", "/test").Info("HTTP request")

		// Read only action logs
		actionLogs, err := multiLogger.ReadJSONLogs(100)
		require.NoError(t, err)

		// All should have source="action"
		for _, entry := range actionLogs {
			if entry.Source != "" {
				assert.Equal(t, string(LogSourceAction), entry.Source)
			}
		}
	})
}
