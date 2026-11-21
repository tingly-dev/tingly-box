package config

import "path/filepath"

// DefaultProjectsDir is the Projects data directory of the instance whose
// config directory is configDir: a sibling named "<config dir>-project"
// (~/.tingly-box-project by default), so instances never share a database
// and a thread's working directory never sits next to the config.
func DefaultProjectsDir(configDir string) string {
	return filepath.Clean(configDir) + "-project"
}
