package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// TestLazyAppManager_NoDatabaseUntilAppConfig pins the process-model rule
// behind #1912: constructing the manager and reading the things a
// lifecycle command needs (config dir, runtime port) must not create the
// config tree or open tingly.db. Only AppConfig / GetGlobalConfig may.
func TestLazyAppManager_NoDatabaseUntilAppConfig(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "conf")
	am := NewLazyAppManager(dir, func(err error) { t.Fatalf("init failed: %v", err) })

	if got := am.ConfigDir(); got != dir {
		t.Fatalf("ConfigDir = %q, want %q", got, dir)
	}
	if got := am.GetRuntimeServerPort(); got != constant.DefaultServerPort {
		t.Fatalf("GetRuntimeServerPort = %d, want default %d", got, constant.DefaultServerPort)
	}
	if am.Initialized() {
		t.Fatal("AppConfig built before anyone asked for it")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("config dir created without AppConfig being built (stat err=%v)", err)
	}

	cfg := am.AppConfig()
	if cfg == nil || !am.Initialized() {
		t.Fatal("AppConfig did not build")
	}
	if cfg.ConfigDir() != dir {
		t.Fatalf("AppConfig.ConfigDir = %q, want %q", cfg.ConfigDir(), dir)
	}
	if _, err := os.Stat(constant.GetDBFile(dir)); err != nil {
		t.Fatalf("AppConfig built but database missing: %v", err)
	}
	if am.AppConfig() != cfg {
		t.Fatal("AppConfig must build once and return the same instance")
	}
}

func TestLazyAppManager_InitErrorHandler(t *testing.T) {
	// A file where the config directory should be makes MkdirAll fail.
	parent := t.TempDir()
	blocker := filepath.Join(parent, "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	var got error
	am := NewLazyAppManager(filepath.Join(blocker, "conf"), func(err error) { got = err })
	if cfg := am.AppConfig(); cfg != nil {
		t.Fatal("expected nil AppConfig on init failure")
	}
	if got == nil {
		t.Fatal("init error handler not called")
	}
	if am.GetGlobalConfig() != nil {
		t.Fatal("GetGlobalConfig must be nil when AppConfig failed to build")
	}
}

func TestNewAppManager_IsEager(t *testing.T) {
	dir := t.TempDir()
	am, err := NewAppManager(dir)
	if err != nil {
		t.Fatalf("NewAppManager: %v", err)
	}
	if !am.Initialized() {
		t.Fatal("NewAppManager must build AppConfig immediately (the GUI relies on it)")
	}
}
