package command

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/command/options"
)

// ServerFlagsKong.Resolve is the one flag resolver behind both the CLI's
// start/restart/open and the GUI binary's root command.
func TestServerFlagsKongResolve(t *testing.T) {
	appManager, err := app.NewAppManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}
	appConfig := appManager.AppConfig()
	if err := appConfig.SetServerPort(23456); err != nil {
		t.Fatalf("Failed to set server port: %v", err)
	}

	t.Run("zero port falls back to config, caller flags pass through", func(t *testing.T) {
		flags := ServerFlagsKong{Host: "127.0.0.1", EnableUI: true}
		opts := flags.Resolve(appConfig, options.StartFlags{Daemon: true, LogFile: "x.log"})
		if opts.Port != 23456 {
			t.Errorf("Port = %d, want config port 23456", opts.Port)
		}
		if opts.Host != "127.0.0.1" || !opts.EnableUI || !opts.Daemon || opts.LogFile != "x.log" {
			t.Errorf("unexpected options: %+v", opts)
		}
	})

	t.Run("explicit port wins and is persisted", func(t *testing.T) {
		opts := ServerFlagsKong{Port: 34567}.Resolve(appConfig, options.StartFlags{})
		if opts.Port != 34567 {
			t.Errorf("Port = %d, want 34567", opts.Port)
		}
		if got := appConfig.GetServerPort(); got != 34567 {
			t.Errorf("config port = %d, want 34567 persisted", got)
		}
	})

	t.Run("explicit --debug wins over config", func(t *testing.T) {
		opts := ServerFlagsKong{EnableDebug: true}.Resolve(appConfig, options.StartFlags{})
		if !opts.EnableDebug {
			t.Error("EnableDebug = false, want true from --debug")
		}
	})
}
