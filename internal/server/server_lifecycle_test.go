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
	if s.moduleCtx == nil {
		t.Fatal("expected module context to be initialized")
	}
	if err := s.moduleCtx.Err(); err != nil {
		t.Fatalf("expected module context to remain active after NewServer, got %v", err)
	}

	if s.moduleCancel == nil {
		t.Fatal("expected module cancel to be initialized")
	}
	s.moduleCancel()

	select {
	case <-s.moduleCtx.Done():
	default:
		t.Fatal("expected module context to be canceled by moduleCancel")
	}
}
