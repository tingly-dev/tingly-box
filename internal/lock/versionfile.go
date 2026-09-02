package lock

import (
	"fmt"
	"path/filepath"
	"strings"
)

// VersionFile records the build version of the running server. Launchers
// read it to tell whether the running server matches their own version —
// a missing or unreadable file means "version unknown" (e.g. a server
// started by a build predating version recording). Same lifecycle and
// reader rules as PortFile.
type VersionFile struct {
	runtimeFile
}

// NewVersionFile creates a version file handle for the given config directory.
func NewVersionFile(configDir string) *VersionFile {
	return &VersionFile{runtimeFile{path: filepath.Join(configDir, versionFileName)}}
}

// Write records the running server's version.
func (vf *VersionFile) Write(version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return fmt.Errorf("empty version")
	}
	return vf.write(version)
}

// Read returns the recorded version. Callers should treat any error as
// "version unknown".
func (vf *VersionFile) Read() (string, error) {
	s, err := vf.read()
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", fmt.Errorf("empty version in %s", vf.path)
	}
	return s, nil
}

// Remove deletes the version file. A missing file is not an error.
func (vf *VersionFile) Remove() error {
	return vf.remove()
}
