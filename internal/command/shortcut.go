package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/shortcut"
)

// PersistLaunchSource records how tingly-box was launched (best-effort) so that
// `shortcut --target=auto` can generate a matching shortcut later. It is a no-op
// for empty / unknown values (including "auto") and only writes when the value
// changes.
func PersistLaunchSource(appManager *AppManager, source string) {
	source = strings.TrimSpace(source)
	if !shortcut.IsKnownSource(source) {
		return
	}
	cfg := appManager.GetGlobalConfig()
	if cfg == nil || cfg.GetLaunchSource() == source {
		return
	}
	_ = cfg.SetLaunchSource(source)
}

// ShortcutCmdKong creates a desktop / start-menu shortcut that launches
// Tingly Box (restart in daemon mode and open the web UI) with a double-click,
// so users don't have to remember and type the startup command — especially on
// Windows.
type ShortcutCmdKong struct {
	Name      string `kong:"flag,name='name',default='Tingly Box',help='Shortcut name'"`
	Target    string `kong:"flag,name='target',enum='auto,binary,npx,npx-bundle',default='auto',help='What the shortcut runs: binary (this executable), npx, npx-bundle, or auto-detect from the recorded launch source'"`
	NoDesktop bool   `kong:"flag,name='no-desktop',help='Do not create a desktop shortcut'"`
	NoMenu    bool   `kong:"flag,name='no-menu',help='Do not create a Start Menu / application menu entry'"`
}

func (s *ShortcutCmdKong) Run(appManager *AppManager) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if resolved, rerr := filepath.EvalSymlinks(exePath); rerr == nil {
		exePath = resolved
	}

	var persisted string
	if cfg := appManager.GetGlobalConfig(); cfg != nil {
		persisted = cfg.GetLaunchSource()
	}

	spec := shortcut.ResolveLaunch(exePath, s.Target, persisted)
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
