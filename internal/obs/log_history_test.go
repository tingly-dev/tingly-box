package obs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadLogsAcrossBlocksAndRotation(t *testing.T) {
	cfg := DefaultMultiLoggerConfig(t.TempDir())
	ml, err := NewMultiLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ml.Close() })
	l := ml.GetLogrusLogger(LogSourceModelRequest)
	long := strings.Repeat("报错 details\n", 2000)
	l.WithError(errors.New(long)).Error(long)
	require.NoError(t, ml.jsonLogger.Rotate())
	l.Info("newest")
	entries, err := ml.ReadJSONLogsBySource(10, LogSourceModelRequest)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "newest", entries[0].Message)
	require.Equal(t, long, entries[1].Message)
	require.Equal(t, long, entries[1].Fields["error"])
	entries, err = ml.ReadJSONLogsBySource(1, LogSourceModelRequest)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestReadLogsWithoutTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	var data []byte
	for _, msg := range []string{strings.Repeat("a", 9000), strings.Repeat("b", 5000), "last"} {
		b, err := json.Marshal(LogEntry{Message: msg})
		require.NoError(t, err)
		if len(data) > 0 {
			data = append(data, '\n')
		}
		data = append(data, b...)
	}
	require.NoError(t, os.WriteFile(path, data, 0600))
	entries, err := readLogEntriesBackwards(path, 10, nil)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	require.Equal(t, "last", entries[0].Message)
	require.Equal(t, strings.Repeat("a", 9000), entries[2].Message)
}
