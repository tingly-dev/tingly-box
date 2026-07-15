package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortFile_WriteReadRemove(t *testing.T) {
	dir := t.TempDir()
	pf := NewPortFile(dir)

	if _, err := pf.Read(); err == nil {
		t.Error("Read should fail when port file does not exist")
	}

	if err := pf.Write(19999); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	port, err := pf.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if port != 19999 {
		t.Errorf("Expected port 19999, got %d", port)
	}

	// Overwrite with a new port
	if err := pf.Write(12580); err != nil {
		t.Fatalf("Write (overwrite) failed: %v", err)
	}
	port, err = pf.Read()
	if err != nil {
		t.Fatalf("Read after overwrite failed: %v", err)
	}
	if port != 12580 {
		t.Errorf("Expected port 12580, got %d", port)
	}

	if err := pf.Remove(); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := pf.Read(); err == nil {
		t.Error("Read should fail after Remove")
	}

	// Removing a missing file is not an error
	if err := pf.Remove(); err != nil {
		t.Errorf("Remove on missing file should not fail, got: %v", err)
	}
}

func TestPortFile_WriteInvalidPort(t *testing.T) {
	pf := NewPortFile(t.TempDir())
	for _, p := range []int{0, -1, 65536} {
		if err := pf.Write(p); err == nil {
			t.Errorf("Write(%d) should fail", p)
		}
	}
}

func TestPortFile_ReadInvalidContent(t *testing.T) {
	dir := t.TempDir()
	pf := NewPortFile(dir)
	if err := os.WriteFile(filepath.Join(dir, portFileName), []byte("not-a-port\n"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := pf.Read(); err == nil {
		t.Error("Read should fail on non-numeric content")
	}
}
