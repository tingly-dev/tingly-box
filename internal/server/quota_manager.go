package server

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/quota/fetcher"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/provider_quota"
)

// initQuotaManager initializes the provider quota manager
func (s *Server) initQuotaManager(cfg *config.Config) error {
	// Create quota store
	store, err := quota.NewGormStore(cfg.ConfigDir, logrus.StandardLogger())
	if err != nil {
		return err
	}

	// Create quota manager with default config
	qConfig := quota.DefaultConfig()
	quotaMgr := quota.NewManager(qConfig, store, cfg, logrus.StandardLogger())

	// Register all built-in fetchers
	fetcher.RegisterAll(quotaMgr, logrus.StandardLogger())

	s.quotaManager = quotaMgr
	logrus.Info("Provider quota manager initialized")
	return nil
}

// GetQuotaManager returns the quota manager (returns interface type for module access)
func (s *Server) GetQuotaManager() providerQuotaModule.Manager {
	return s.quotaManager
}
