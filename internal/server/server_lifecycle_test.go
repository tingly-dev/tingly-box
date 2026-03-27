package server

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/server/config"
)

func TestNewServerKeepsModuleContextAlive(t *testing.T) {
	cfg, err := config.NewConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	s := NewServer(cfg, WithOpenBrowser(false))
	if s.ctx == nil {
		t.Fatal("expected context to be initialized")
	}
	if err := s.ctx.Err(); err != nil {
		t.Fatalf("expected context to remain active after NewServer, got %v", err)
	}

	if s.cancel == nil {
		t.Fatal("expected cancel to be initialized")
	}
	s.cancel()

	select {
	case <-s.ctx.Done():
	default:
		t.Fatal("expected context to be canceled by cancel")
	}
}
