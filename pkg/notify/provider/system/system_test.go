package system

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// TestSystemNew tests system provider creation
func TestSystemNew(t *testing.T) {
	p := New(Config{
		AppName: "TestApp",
	})

	if p.Name() != "system" {
		t.Errorf("expected name 'system', got %v", p.Name())
	}
	if p.config.AppName != "TestApp" {
		t.Errorf("expected AppName 'TestApp', got %v", p.config.AppName)
	}
}

// TestSystemNewDefaultAppName tests default app name
func TestSystemNewDefaultAppName(t *testing.T) {
	p := New(Config{})

	if p.config.AppName != "notify" {
		t.Errorf("expected default AppName 'notify', got %v", p.config.AppName)
	}
}

// TestSystemWithOptions tests system provider with options
func TestSystemWithOptions(t *testing.T) {
	p := New(Config{},
		WithName("custom-system"),
		WithSound("/path/to/sound.wav"),
		WithIcon("/path/to/icon.png"),
	)

	if p.Name() != "custom-system" {
		t.Errorf("expected name 'custom-system', got %v", p.Name())
	}
	if p.config.Sound != "/path/to/sound.wav" {
		t.Errorf("expected sound '/path/to/sound.wav', got %v", p.config.Sound)
	}
	if p.config.Icon != "/path/to/icon.png" {
		t.Errorf("expected icon '/path/to/icon.png', got %v", p.config.Icon)
	}
}

// TestSystemSendEmptyMessage tests validation
func TestSystemSendEmptyMessage(t *testing.T) {
	p := New(Config{})

	ctx := context.Background()
	_, err := p.Send(ctx, &notify.Notification{Message: ""})

	if err == nil {
		t.Error("expected error for empty message")
	}
}

// TestSystemSendWithTitle tests notification with title
func TestSystemSendWithTitle(t *testing.T) {
	p := New(Config{})

	ctx := context.Background()
	// This will fail in CI without a display, but tests the code path
	_, _ = p.Send(ctx, &notify.Notification{
		Title:   "Test Title",
		Message: "Test Message",
		Level:   notify.LevelInfo,
	})
	// We don't assert success/failure as it depends on the environment
}

// TestSystemSendWithLevel tests different notification levels
func TestSystemSendWithLevel(t *testing.T) {
	p := New(Config{})
	ctx := context.Background()

	levels := []notify.Level{
		notify.LevelDebug,
		notify.LevelInfo,
		notify.LevelWarning,
		notify.LevelError,
		notify.LevelCritical,
	}

	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			_, _ = p.Send(ctx, &notify.Notification{
				Message: "test",
				Level:   level,
			})
		})
	}
}

// TestSystemSendWithImageURL tests notification with image
func TestSystemSendWithImageURL(t *testing.T) {
	p := New(Config{})

	ctx := context.Background()
	_, _ = p.Send(ctx, &notify.Notification{
		Message:  "test",
		ImageURL: "https://example.com/image.png",
	})
	// Image URL should be passed to beeep as icon
}

// TestSystemSendWithSound tests notification with sound
func TestSystemSendWithSound(t *testing.T) {
	p := New(Config{
		Sound: "/path/to/sound.wav",
	})

	ctx := context.Background()
	_, _ = p.Send(ctx, &notify.Notification{
		Message: "test",
	})
	// Sound should trigger beep
}

// TestSystemSendWithMetadataSound tests sound from metadata
func TestSystemSendWithMetadataSound(t *testing.T) {
	p := New(Config{})

	ctx := context.Background()
	_, _ = p.Send(ctx, &notify.Notification{
		Message: "test",
		Metadata: map[string]interface{}{
			"sound": "/path/to/metadata-sound.wav",
		},
	})
}

// TestSystemClose tests Close method
func TestSystemClose(t *testing.T) {
	p := New(Config{})

	err := p.Close()
	if err != nil {
		t.Errorf("Close() should not return error, got %v", err)
	}
}

// TestIsSupported tests IsSupported function
func TestIsSupported(t *testing.T) {
	// beeep supports all major platforms, so this should always be true
	supported := IsSupported()
	if !supported {
		t.Error("expected system notifications to be supported")
	}
}

// TestSystemSendEmptyTitle tests notification without title
func TestSystemSendEmptyTitle(t *testing.T) {
	p := New(Config{})

	ctx := context.Background()
	_, _ = p.Send(ctx, &notify.Notification{
		Message: "test message",
		Level:   notify.LevelWarning,
	})
	// When title is empty, level should be used as title
}
