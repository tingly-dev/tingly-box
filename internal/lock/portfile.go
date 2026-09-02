package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	portFileName    = "tingly-server.port"
	versionFileName = "tingly-server.version"
)

// runtimeFile is a single-value runtime artifact in the config directory —
// not configuration: the server writes it right after acquiring the file
// lock and removes it on shutdown. Readers must gate on FileLock.IsLocked():
// a stale file can survive a crashed server, but the flock is always
// released by the OS.
type runtimeFile struct {
	path string
}

// write publishes the value atomically (temp file + rename) so a concurrent
// reader never observes a partial value.
func (rf *runtimeFile) write(value string) error {
	tmp := rf.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(value+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath.Base(rf.path), err)
	}
	if err := os.Rename(tmp, rf.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to publish %s: %w", filepath.Base(rf.path), err)
	}
	return nil
}

func (rf *runtimeFile) read() (string, error) {
	data, err := os.ReadFile(rf.path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filepath.Base(rf.path), err)
	}
	return strings.TrimSpace(string(data)), nil
}

// remove deletes the file. A missing file is not an error.
func (rf *runtimeFile) remove() error {
	if err := os.Remove(rf.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove %s: %w", filepath.Base(rf.path), err)
	}
	return nil
}

// PortFile records the port the running server is actually listening on.
// Other CLI processes (cc/profile/log/status/open) read it to discover the
// live port, since the server port is intentionally not persisted in the
// config file. See .design/runtime-port-file.md.
type PortFile struct {
	runtimeFile
}

// NewPortFile creates a port file handle for the given config directory.
func NewPortFile(configDir string) *PortFile {
	return &PortFile{runtimeFile{path: filepath.Join(configDir, portFileName)}}
}

// Write records the listening port.
func (pf *PortFile) Write(port int) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port %d", port)
	}
	return pf.write(strconv.Itoa(port))
}

// Read returns the recorded port. Callers should treat any error as
// "port unknown" and fall back to the configured port.
func (pf *PortFile) Read() (int, error) {
	s, err := pf.read()
	if err != nil {
		return 0, err
	}
	port, err := strconv.Atoi(s)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q in %s", s, pf.path)
	}
	return port, nil
}

// Remove deletes the port file. A missing file is not an error.
func (pf *PortFile) Remove() error {
	return pf.remove()
}
