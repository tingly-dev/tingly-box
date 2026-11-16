package config

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestMCPRuntimeConfigConcurrentEditsAndSaveRollback(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	require.NoError(t, err)
	require.NoError(t, cfg.SetToolConfig(ToolTypeMCPRuntime, &typ.MCPRuntimeConfig{RequestTimeout: 30, Sources: []typ.MCPSourceConfig{}}))
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := cfg.UpdateMCPRuntimeConfig(func(current *typ.MCPRuntimeConfig) error {
				current.Sources = append(current.Sources, typ.MCPSourceConfig{ID: fmt.Sprintf("source%d", i), Command: "node"})
				return nil
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	before := cfg.GetMCPRuntimeConfig()
	require.Len(t, before.Sources, 8)
	reloaded, err := NewConfig(WithConfigDir(cfg.ConfigDir))
	require.NoError(t, err)
	require.Equal(t, before, reloaded.GetMCPRuntimeConfig())
	cfg.ConfigFile = t.TempDir() // A directory cannot be replaced by the config writer.
	_, err = cfg.UpdateMCPRuntimeConfig(func(current *typ.MCPRuntimeConfig) error {
		current.RequestTimeout = 99
		current.Sources = nil
		return nil
	})
	require.Error(t, err)
	require.Equal(t, before, cfg.GetMCPRuntimeConfig(), "failed writes must retain the live config")
}
