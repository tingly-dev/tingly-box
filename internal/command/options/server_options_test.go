package options

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/config"
)

func TestResolveStartOptionsProtocolStage(t *testing.T) {
	t.Parallel()

	appConfig, err := config.NewAppConfig(config.WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewAppConfig() error = %v", err)
	}
	t.Cleanup(func() { _ = appConfig.GetGlobalConfig().CloseStores() })

	var flags StartFlags
	cmd := &cobra.Command{Use: "start"}
	AddStartFlags(cmd, &flags)
	if err := cmd.ParseFlags([]string{"--stage"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	resolved := ResolveStartOptions(cmd, flags, appConfig)
	if !resolved.EnableProtocolStage {
		t.Fatal("EnableProtocolStage = false, want true")
	}
}
