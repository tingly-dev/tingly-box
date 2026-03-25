package config

import (
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherIsConfigEvent_IgnoresSiblingFiles(t *testing.T) {
	configDir := t.TempDir()
	cfg := &Config{
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, "config.json"),
	}

	cw, err := NewConfigWatcher(cfg)
	if err != nil {
		t.Fatalf("NewConfigWatcher failed: %v", err)
	}
	t.Cleanup(func() {
		_ = cw.Stop()
	})

	tests := []struct {
		name  string
		event fsnotify.Event
		want  bool
	}{
		{
			name: "config write",
			event: fsnotify.Event{
				Name: cfg.ConfigFile,
				Op:   fsnotify.Write,
			},
			want: true,
		},
		{
			name: "config rename",
			event: fsnotify.Event{
				Name: cfg.ConfigFile,
				Op:   fsnotify.Rename,
			},
			want: true,
		},
		{
			name: "bot sessions create",
			event: fsnotify.Event{
				Name: filepath.Join(configDir, "bot_sessions.json"),
				Op:   fsnotify.Create,
			},
			want: false,
		},
		{
			name: "bot chats rename",
			event: fsnotify.Event{
				Name: filepath.Join(configDir, "bot_chats.json"),
				Op:   fsnotify.Rename,
			},
			want: false,
		},
		{
			name: "skill locations write",
			event: fsnotify.Event{
				Name: filepath.Join(configDir, "skill_locations.json"),
				Op:   fsnotify.Write,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cw.isConfigEvent(tt.event); got != tt.want {
				t.Fatalf("isConfigEvent(%+v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}
