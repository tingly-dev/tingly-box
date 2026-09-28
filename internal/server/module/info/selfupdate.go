package info

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/shortcut"
	"github.com/tingly-dev/tingly-box/pkg/daemon"
)

// Self-update lets the web UI apply an available update instead of only
// showing the commands to copy. It is deliberately narrow: only a global npm
// install that this process may write to is updated in place — npx, Docker and
// standalone binaries keep their copy-paste instructions, since the running
// server cannot replace them itself.
//
// After installing, an unsupervised server restarts itself by spawning the
// freshly installed launcher's `restart` detached. Under a service manager
// (systemd, pm2) it does not: the manager owns the process, so the UI tells
// the user to restart the service instead.

const (
	selfUpdateInstallTimeout = 5 * time.Minute
	// restartDelay lets the HTTP response reach the browser before the
	// restart stops this server.
	restartDelay = time.Second
)

// SelfUpdateSupport reports whether POST /info/version/update can update this
// install, and why not when it cannot.
type SelfUpdateSupport struct {
	Supported bool   `json:"supported" example:"true"`
	Reason    string `json:"reason,omitempty" example:"started via npx; run the npx command with the new version"`
	// Supervised means a service manager owns the process: the update is
	// installed but the service must be restarted to apply it.
	Supervised bool `json:"supervised" example:"false"`
}

// SelfUpdateResult is the payload of a successful POST /info/version/update.
type SelfUpdateResult struct {
	Version string `json:"version" example:"0.261001.1"`
	// Restarting is true when the server is restarting itself into Version.
	Restarting bool `json:"restarting" example:"true"`
	// RestartRequired is true when the update is installed but the service
	// manager must restart the server to apply it.
	RestartRequired bool `json:"restart_required" example:"false"`
}

// SelfUpdateResponse is the JSON envelope for POST /info/version/update.
type SelfUpdateResponse struct {
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
	Output  string           `json:"output,omitempty"`
	Data    SelfUpdateResult `json:"data,omitempty"`
}

// updater holds the system hooks self-update needs, so tests can fake them.
type updater struct {
	launchSource string
	args         []string // this process's command line, without argv[0]

	getenv     func(string) string
	fileExists func(string) bool
	runCmd     func(ctx context.Context, name string, args ...string) ([]byte, error)
	writable   func(dir string) bool
	spawn      func(name string, args ...string) error
	after      func(d time.Duration, f func())

	mu      sync.Mutex
	running bool
}

func newUpdater(launchSource string) *updater {
	return &updater{
		launchSource: launchSource,
		args:         os.Args[1:],
		getenv:       os.Getenv,
		fileExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		runCmd: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		writable: dirWritable,
		spawn:    daemon.SpawnDetached,
		after: func(d time.Duration, f func()) {
			time.AfterFunc(d, f)
		},
	}
}

// support decides whether this install can update itself. It returns the npm
// global prefix so update does not have to resolve it again.
func (u *updater) support(ctx context.Context) (SelfUpdateSupport, string) {
	switch u.launchSource {
	case shortcut.SourceNpm:
	case shortcut.SourceNpx, shortcut.SourceNpxBundle:
		return SelfUpdateSupport{Reason: "started via npx; run the npx command with the new version"}, ""
	case shortcut.SourceNpmBundle:
		return SelfUpdateSupport{Reason: "tingly-box-bundle is retired; install tingly-box with npm instead"}, ""
	case shortcut.SourceBinary:
		return SelfUpdateSupport{Reason: "standalone binary; download the new release"}, ""
	default:
		return SelfUpdateSupport{Reason: "unknown install method"}, ""
	}

	if u.fileExists("/.dockerenv") || u.fileExists("/run/.containerenv") {
		return SelfUpdateSupport{Reason: "running in a container; pull the new image instead"}, ""
	}

	prefix, err := u.npm(ctx, "prefix", "-g")
	if err != nil {
		return SelfUpdateSupport{Reason: "npm is not available to this process"}, ""
	}
	root, err := u.npm(ctx, "root", "-g")
	if err != nil {
		return SelfUpdateSupport{Reason: "npm is not available to this process"}, ""
	}
	for _, dir := range []string{root, npmBinDir(prefix)} {
		if !u.writable(dir) {
			return SelfUpdateSupport{Reason: fmt.Sprintf("the global npm directory %s is not writable by this process; update from a shell", dir)}, ""
		}
	}

	return SelfUpdateSupport{Supported: true, Supervised: u.supervised()}, prefix
}

// supervised reports whether a service manager owns this process.
func (u *updater) supervised() bool {
	return u.getenv("INVOCATION_ID") != "" || // systemd
		u.getenv("pm_id") != "" // pm2
}

// refusedError is returned when an update is not attempted at all (as
// opposed to attempted and failed).
type refusedError struct{ reason string }

func (e *refusedError) Error() string { return e.reason }

// update installs version with npm and, when unsupervised, schedules a
// restart into it. It returns npm's output alongside any error. The install
// is detached from ctx's cancellation — a closed browser tab must not kill
// npm halfway through replacing the package.
func (u *updater) update(ctx context.Context, version string) (SelfUpdateResult, string, error) {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return SelfUpdateResult{}, "", &refusedError{"an update is already in progress"}
	}
	u.running = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.running = false
		u.mu.Unlock()
	}()

	ctx = context.WithoutCancel(ctx)
	sup, prefix := u.support(ctx)
	if !sup.Supported {
		return SelfUpdateResult{}, "", &refusedError{sup.Reason}
	}

	installCtx, cancel := context.WithTimeout(ctx, selfUpdateInstallTimeout)
	defer cancel()
	out, err := u.runCmd(installCtx, npmExecutable(), "install", "-g", tinglyBoxNPM+"@"+version)
	if err != nil {
		return SelfUpdateResult{}, string(out), fmt.Errorf("npm install failed: %w", err)
	}

	result := SelfUpdateResult{Version: version}
	if sup.Supervised {
		result.RestartRequired = true
		return result, string(out), nil
	}

	// Restart through the newly installed launcher: this process's own
	// executable is the previous version's binary.
	launcher := npmLauncher(prefix)
	restartArgs := restartArgsFrom(u.args)
	u.after(restartDelay, func() {
		_ = u.spawn(launcher, restartArgs...)
	})
	result.Restarting = true
	return result, string(out), nil
}

func (u *updater) npm(ctx context.Context, args ...string) (string, error) {
	out, err := u.runCmd(ctx, npmExecutable(), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// restartArgsFrom turns this process's start command line into the matching
// restart command, keeping every flag (host, port, source, ...).
func restartArgsFrom(args []string) []string {
	out := append([]string(nil), args...)
	for i, arg := range out {
		if arg == "start" {
			out[i] = "restart"
			return out
		}
	}
	return append(out, "restart")
}

func npmExecutable() string {
	if runtime.GOOS == "windows" {
		return "npm.cmd"
	}
	return "npm"
}

// npmBinDir is where npm links global package executables.
func npmBinDir(prefix string) string {
	if runtime.GOOS == "windows" {
		return prefix
	}
	return filepath.Join(prefix, "bin")
}

// npmLauncher is the tingly-box launcher npm installs globally.
func npmLauncher(prefix string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(prefix, "tingly-box.cmd")
	}
	return filepath.Join(npmBinDir(prefix), "tingly-box")
}

// dirWritable reports whether this process can create files in dir.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".tingly-box-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}
