package lock

import (
	"os"
	"path/filepath"
	"testing"
)

// The shared atomic write/read/remove behavior is covered by portfile_test.go;
// this only exercises VersionFile's own validation.
func TestVersionFile_RoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	vf := NewVersionFile(dir)

	if _, err := vf.Read(); err == nil {
		t.Error("Read should fail when version file does not exist")
	}

	if err := vf.Write("0.260828.1000"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if v, err := vf.Read(); err != nil || v != "0.260828.1000" {
		t.Fatalf("Read = %q, %v; want 0.260828.1000", v, err)
	}

	for _, v := range []string{"", "   ", "\n"} {
		if err := vf.Write(v); err == nil {
			t.Errorf("Write(%q) should fail", v)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, versionFileName), []byte("\n"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := vf.Read(); err == nil {
		t.Error("Read should fail on empty content")
	}

	if err := vf.Remove(); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if err := vf.Remove(); err != nil {
		t.Errorf("Remove on missing file should not fail, got: %v", err)
	}
}
