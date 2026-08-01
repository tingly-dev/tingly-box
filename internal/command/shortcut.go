package command

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/shortcut"
)

// LaunchSource is how the running tingly-box process was invoked (binary,
// npx, npx-bundle) — see internal/shortcut for the source constants. It comes
// from the global --source flag and is bound into Kong's Run() calls, so any
// subcommand that needs it (shortcut, start, restart) can read it directly
// without persisting it anywhere.
type LaunchSource string

// resolveShortcutSpec resolves this process's own executable path (following
// symlinks) and turns it, together with how this invocation was launched,
// into a LaunchSpec — the one piece shared by refreshShortcut and
// ShortcutCmdKong.Run.
func resolveShortcutSpec(source LaunchSource) (shortcut.LaunchSpec, error) {
	exePath, err := os.Executable()
	if err != nil {
		return shortcut.LaunchSpec{}, fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}
	return shortcut.ResolveLaunch(exePath, string(source), BuildVersion), nil
}

// refreshShortcut best-effort (re)writes the desktop/menu shortcut so it
// keeps launching through whatever way tingly-box is currently being run.
// Failures are logged, not surfaced — a shortcut write should never block
// starting the server.
func refreshShortcut(source LaunchSource) {
	spec, err := resolveShortcutSpec(source)
	if err != nil {
		return
	}
	if _, err := shortcut.Create(shortcut.Options{Name: "Tingly Box"}, spec); err != nil {
		logrus.WithError(err).Debug("failed to refresh shortcut")
	}
}

// ShortcutCmdKong creates a desktop / start-menu shortcut that launches
// Tingly Box (restart in daemon mode and open the web UI) with a double-click,
// so users don't have to remember and type the startup command — especially on
// Windows.
type ShortcutCmdKong struct {
	Name      string `kong:"flag,name='name',default='Tingly Box',help='Shortcut name'"`
	NoDesktop bool   `kong:"flag,name='no-desktop',help='Do not create a desktop shortcut'"`
	NoMenu    bool   `kong:"flag,name='no-menu',help='Do not create a Start Menu / application menu entry'"`
}

func (s *ShortcutCmdKong) Run(source LaunchSource) error {
	spec, err := resolveShortcutSpec(source)
	if err != nil {
		return err
	}

	created, err := shortcut.Create(shortcut.Options{
		Name:      s.Name,
		NoDesktop: s.NoDesktop,
		NoMenu:    s.NoMenu,
	}, spec)
	if err != nil {
		return err
	}

	if len(created) == 0 {
		fmt.Println("Nothing to do (both --no-desktop and --no-menu were set).")
		return nil
	}

	fmt.Println("Created shortcut(s):")
	for _, p := range created {
		fmt.Printf("  - %s\n", p)
	}
	fmt.Println("\nDouble-click it to start Tingly Box and open the web UI.")
	return nil
}
