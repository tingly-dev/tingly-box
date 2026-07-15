package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const portFileName = "tingly-server.port"

// PortFile records the port the running server is actually listening on.
// Like the PID lock file, it is a runtime artifact in the config directory,
// not configuration: the server writes it right after acquiring the file
// lock and removes it on shutdown. Other CLI processes (cc/profile/log/
// status/open) read it to discover the live port, since the server port is
// intentionally not persisted in the config file.
//
// Readers must gate on FileLock.IsLocked(): a stale port file can survive a
// crashed server, but the flock is always released by the OS.
type PortFile struct {
	path string
}

// NewPortFile creates a port file handle for the given config directory.
func NewPortFile(configDir string) *PortFile {
	return &PortFile{path: filepath.Join(configDir, portFileName)}
}

// Write records the listening port. The write is atomic (temp file + rename)
// so a concurrent reader never observes a partial value.
func (pf *PortFile) Write(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	tmp := pf.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(port)+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write port file: %w", err)
	}
	if err := os.Rename(tmp, pf.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to publish port file: %w", err)
	}
	return nil
}

// Read returns the recorded port. Callers should treat any error as
// "port unknown" and fall back to the configured port.
func (pf *PortFile) Read() (int, error) {
	data, err := os.ReadFile(pf.path)
	if err != nil {
		return 0, fmt.Errorf("failed to read port file: %w", err)
	}
	s := strings.TrimSpace(string(data))
	port, err := strconv.Atoi(s)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q in %s", s, pf.path)
	}
	return port, nil
}

// Remove deletes the port file. A missing file is not an error.
func (pf *PortFile) Remove() error {
	if err := os.Remove(pf.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove port file: %w", err)
	}
	return nil
}
