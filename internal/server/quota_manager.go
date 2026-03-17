package server

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
	"github.com/tingly-dev/tingly-box/internal/quota/fetcher"
	"github.com/tingly-dev/tingly-box/internal/server/config"
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

	// Register fetchers
	if err := quotaMgr.RegisterFetcher(fetcher.NewAnthropicFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Anthropic fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewOpenAIFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register OpenAI fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewGeminiFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Gemini fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewCursorFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Cursor fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewCopilotFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Copilot fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewVertexAIFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register VertexAI fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewZaiFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Zai fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewKimiK2Fetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register KimiK2 fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewOpenRouterFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register OpenRouter fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewMiniMaxFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register MiniMax fetcher")
	}
	if err := quotaMgr.RegisterFetcher(fetcher.NewCodexFetcher(logrus.StandardLogger())); err != nil {
		logrus.WithError(err).Debug("Failed to register Codex fetcher")
	}

	s.quotaManager = quotaMgr
	logrus.Info("Provider quota manager initialized")
	return nil
}
