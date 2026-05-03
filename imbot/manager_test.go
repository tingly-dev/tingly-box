package imbot

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

func TestManagerRemoveBotDeletesEntry(t *testing.T) {
	manager := NewManager()
	cfg := &core.Config{
		UUID:     "bot-remove",
		Platform: core.PlatformTingly,
		Enabled:  false,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "test-token",
		},
	}

	if err := manager.AddBot(cfg); err != nil {
		t.Fatalf("AddBot failed: %v", err)
	}
	if got := manager.GetBotByUUID(cfg.UUID); got == nil {
		t.Fatalf("expected bot to exist before removal")
	}

	if err := manager.RemoveBot(cfg.UUID); err != nil {
		t.Fatalf("RemoveBot failed: %v", err)
	}
	if got := manager.GetBotByUUID(cfg.UUID); got != nil {
		t.Fatalf("expected bot to be removed, got %#v", got)
	}
}

func TestManagerAddBotRequiresUUID(t *testing.T) {
	manager := NewManager()
	cfg := &core.Config{
		Platform: core.PlatformTingly,
		Enabled:  false,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "test-token",
		},
	}

	if err := manager.AddBot(cfg); err == nil {
		t.Fatal("expected AddBot to fail when UUID is missing")
	}
}

func TestManagerStopDisconnectsBot(t *testing.T) {
	manager := NewManager()
	cfg := &core.Config{
		UUID:     "bot-stop",
		Platform: core.PlatformTingly,
		Enabled:  false,
		Auth: core.AuthConfig{
			Type:  "token",
			Token: "test-token",
		},
	}

	if err := manager.AddBot(cfg); err != nil {
		t.Fatalf("AddBot failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	bot := manager.GetBotByUUID(cfg.UUID)
	if bot == nil {
		t.Fatal("expected bot to exist")
	}
	if !bot.IsConnected() {
		t.Fatal("expected bot to be connected after Start")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := manager.Stop(stopCtx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if bot.IsConnected() {
		t.Fatal("expected bot to be disconnected after Stop")
	}
}
