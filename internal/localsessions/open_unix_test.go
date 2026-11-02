//go:build !windows

package localsessions

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFIFOIsNotATranscript(t *testing.T) {
	s, _, _ := fixture(t, line("safe"))
	path := filepath.Join(s.root, "-work", "pipe.jsonl")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Skip(err)
	}
	id := s.encode(token{Kind: "session", Rel: "-work/pipe.jsonl"})
	if _, err := s.Preview(context.Background(), id, "", "backward"); err == nil {
		t.Fatal("FIFO accepted")
	}
}
