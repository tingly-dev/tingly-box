package lock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A first launch reaches TryLock before anything has created the config
// directory (the GUI takes the lock first, so a second launch never opens the
// running instance's database). Opening the lock file in a missing directory
// used to fail with "no such file or directory" and the app never started.
func TestFileLock_TryLockCreatesMissingConfigDir(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "home", ".tingly-box") // two levels missing
	fl := NewFileLock(configDir)

	if err := fl.TryLock(); err != nil {
		t.Fatalf("TryLock in a missing config dir failed: %v", err)
	}
	defer fl.Unlock()

	info, err := os.Stat(configDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("config dir was not created: %v", err)
	}
	// The holder can publish its runtime files right away, as a server does.
	if err := fl.WritePort(23456); err != nil {
		t.Fatalf("WritePort failed: %v", err)
	}
	if got, err := fl.ReadPort(); err != nil || got != 23456 {
		t.Errorf("ReadPort = %d, %v; want 23456", got, err)
	}
}

// ErrLocked is how callers tell "an instance is already running" (focus it, or
// offer to take it over) from every other failure (nothing to take over).
func TestFileLock_TryLockErrorKinds(t *testing.T) {
	t.Run("held lock is ErrLocked", func(t *testing.T) {
		configDir := t.TempDir()
		holder := NewFileLock(configDir)
		if err := holder.TryLock(); err != nil {
			t.Fatalf("holder TryLock failed: %v", err)
		}
		defer holder.Unlock()

		err := NewFileLock(configDir).TryLock()
		if !errors.Is(err, ErrLocked) {
			t.Fatalf("second TryLock error = %v; want ErrLocked", err)
		}
	})

	t.Run("unusable config dir is not ErrLocked", func(t *testing.T) {
		// A regular file where the config dir should be: MkdirAll cannot succeed.
		blocker := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		err := NewFileLock(filepath.Join(blocker, ".tingly-box")).TryLock()
		if err == nil {
			t.Fatal("TryLock should fail when the config dir cannot be created")
		}
		if errors.Is(err, ErrLocked) {
			t.Fatalf("an unusable config dir must not look like a running instance: %v", err)
		}
	})
}
