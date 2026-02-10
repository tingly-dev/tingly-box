package config

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

func TestLoadFromAppConfigDefaults(t *testing.T) {
	appCfg := &serverconfig.Config{
		ConfigDir: t.TempDir(),
		JWTSecret: "jwt-secret",
		UserToken: "user-token",
	}

	cfg, err := LoadFromAppConfig(appCfg, Options{})
	require.NoError(t, err)
	require.Equal(t, 18080, cfg.Port)
	require.Equal(t, filepath.Join(appCfg.ConfigDir, "remote-cc.db"), cfg.DBPath)
	require.Equal(t, 30*time.Minute, cfg.SessionTimeout)
	require.Equal(t, 7*24*time.Hour, cfg.MessageRetention)
	require.Equal(t, 5, cfg.RateLimitMax)
	require.Equal(t, 5*time.Minute, cfg.RateLimitWindow)
	require.Equal(t, 5*time.Minute, cfg.RateLimitBlock)
}

func TestLoadFromAppConfigOverrides(t *testing.T) {
	appCfg := &serverconfig.Config{
		ConfigDir: t.TempDir(),
		JWTSecret: "jwt-secret",
		UserToken: "user-token",
		RemoteCoder: serverconfig.RemoteCoderConfig{
			Port:                 18081,
			DBPath:               "/tmp/rc.db",
			SessionTimeout:       "10m",
			MessageRetentionDays: 3,
			RateLimitMax:         9,
			RateLimitWindow:      "2m",
			RateLimitBlock:       "4m",
		},
	}

	overridePort := 19000
	overrideRetention := 1
	overrideWindow := 30 * time.Second

	cfg, err := LoadFromAppConfig(appCfg, Options{
		Port:                 &overridePort,
		MessageRetentionDays: &overrideRetention,
		RateLimitWindow:      &overrideWindow,
	})
	require.NoError(t, err)
	require.Equal(t, overridePort, cfg.Port)
	require.Equal(t, "/tmp/rc.db", cfg.DBPath)
	require.Equal(t, 10*time.Minute, cfg.SessionTimeout)
	require.Equal(t, 1*24*time.Hour, cfg.MessageRetention)
	require.Equal(t, 9, cfg.RateLimitMax)
	require.Equal(t, 30*time.Second, cfg.RateLimitWindow)
	require.Equal(t, 4*time.Minute, cfg.RateLimitBlock)
}
