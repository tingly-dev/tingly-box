package obs

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMemoryLogHook verifies hook initialization.
func TestNewMemoryLogHook(t *testing.T) {
	hook := NewMemoryLogHook(100)

	assert.Equal(t, 100, cap(hook.entries))
	assert.Equal(t, 0, hook.Size())
	assert.Equal(t, 0, hook.writeIdx)
}

// TestFire verifies log entry storage.
func TestFire(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Info("test message")

	assert.Equal(t, 1, hook.Size())
	entries := hook.GetEntries()
	require.Len(t, entries, 1)
	assert.Equal(t, "test message", entries[0].Message)
	assert.Equal(t, logrus.InfoLevel, entries[0].Level)
}

// TestRotate verifies circular buffer overwrites oldest entries.
func TestRotate(t *testing.T) {
	hook := NewMemoryLogHook(3)
	logger := logrus.New()
	logger.AddHook(hook)

	// Fill buffer
	for i := 0; i < 5; i++ {
		logger.Info("message", i)
	}

	assert.Equal(t, 3, hook.Size())
	entries := hook.GetEntries()
	require.Len(t, entries, 3)
	// Oldest entries should be overwritten
	assert.Equal(t, "message2", entries[0].Message)
	assert.Equal(t, "message4", entries[2].Message)
}

// TestTee verifies simultaneous output to multiple writers.
func TestTee(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress default output
	logger.AddHook(hook)

	var buf1, buf2 bytes.Buffer
	hook.AddWriter(&buf1)
	hook.AddWriter(&buf2)

	logger.Info("test message")

	// Both writers should receive the log
	assert.Contains(t, buf1.String(), "test message")
	assert.Contains(t, buf2.String(), "test message")

	// Memory should also store the log
	assert.Equal(t, 1, hook.Size())
}

// TestGetEntries verifies retrieving all entries in chronological order.
func TestGetEntries(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Warn("warn1")
	logger.Info("info1")
	logger.Error("error1")

	entries := hook.GetEntries()
	require.Len(t, entries, 3)
	assert.Equal(t, "warn1", entries[0].Message)
	assert.Equal(t, "info1", entries[1].Message)
	assert.Equal(t, "error1", entries[2].Message)
}

// TestGetEntriesSince verifies time-based filtering.
func TestGetEntriesSince(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Info("old")
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	logger.Info("new")

	entries := hook.GetEntriesSince(cutoff)
	require.Len(t, entries, 1)
	assert.Equal(t, "new", entries[0].Message)
}

// TestGetEntriesByLevel verifies level filtering.
func TestGetEntriesByLevel(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	infos := hook.GetEntriesByLevel(logrus.InfoLevel)
	require.Len(t, infos, 1)
	assert.Equal(t, "info", infos[0].Message)

	errors := hook.GetEntriesByLevel(logrus.ErrorLevel)
	require.Len(t, errors, 1)
	assert.Equal(t, "error", errors[0].Message)
}

// TestGetEntriesByLevelRange verifies level range filtering.
func TestGetEntriesByLevelRange(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	// Get Warn, Error (<= Warn, >= Error)
	// logrus levels: Debug=5, Info=4, Warn=3, Error=2
	entries := hook.GetEntriesByLevelRange(logrus.ErrorLevel, logrus.WarnLevel)
	require.Len(t, entries, 2)
	assert.Equal(t, "warn", entries[0].Message)
	assert.Equal(t, "error", entries[1].Message)
}

// TestGetLatest verifies retrieving newest N entries.
func TestGetLatest(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	for i := 0; i < 5; i++ {
		logger.Info(i)
	}

	latest := hook.GetLatest(2)
	require.Len(t, latest, 2)
	assert.Equal(t, "3", latest[0].Message)
	assert.Equal(t, "4", latest[1].Message)
}

// TestGetLatestExceedCount verifies requesting more than available.
func TestGetLatestExceedCount(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Info("only one")

	latest := hook.GetLatest(100)
	require.Len(t, latest, 1)
	assert.Equal(t, "only one", latest[0].Message)
}

// TestClear verifies clearing all entries.
func TestClear(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.Info("before")
	hook.Clear()

	assert.Equal(t, 0, hook.Size())
	assert.Empty(t, hook.GetEntries())
}

// TestDeepCopy verifies stored entries are independent from originals.
func TestDeepCopy(t *testing.T) {
	hook := NewMemoryLogHook(10)
	logger := logrus.New()
	logger.AddHook(hook)

	logger.WithField("key", "value1").Info("msg")

	entries := hook.GetEntries()
	require.Len(t, entries, 1)

	// Modify original field data
	logger.WithField("key", "value2").Info("msg2")

	// First entry should be unchanged
	assert.Equal(t, "value1", entries[0].Data["key"])
}

// TestConcurrentAccess verifies thread safety.
func TestConcurrentAccess(t *testing.T) {
	hook := NewMemoryLogHook(1000)
	logger := logrus.New()
	logger.AddHook(hook)

	done := make(chan bool)

	// Writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logger.Info("writer", id, "log", j)
			}
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				hook.GetEntries()
				hook.Size()
				hook.GetLatest(10)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}

	// Should complete without panic/race
	assert.Equal(t, 1000, hook.Size())
}

// TestEmptyHook verifies behavior with no entries.
func TestEmptyHook(t *testing.T) {
	hook := NewMemoryLogHook(10)

	assert.Equal(t, 0, hook.Size())
	assert.Empty(t, hook.GetEntries())
	assert.Empty(t, hook.GetLatest(5))
	assert.Empty(t, hook.GetEntriesSince(time.Now()))
	assert.Empty(t, hook.GetEntriesByLevel(logrus.InfoLevel))
}
