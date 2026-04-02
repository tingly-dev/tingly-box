package fetcher

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/quota"
)

// RegisterAll registers all built-in quota fetchers into the given registrar.
func RegisterAll(r quota.FetcherRegistrar, logger *logrus.Logger) {
	fetchers := []quota.Fetcher{
		NewAnthropicFetcher(logger),
		NewOpenAIFetcher(logger),
		NewGeminiFetcher(logger),
		NewCursorFetcher(logger),
		NewCopilotFetcher(logger),
		NewVertexAIFetcher(logger),
		NewZaiFetcher(logger),
		NewGLMFetcher(logger),
		NewKimiK2Fetcher(logger),
		NewOpenRouterFetcher(logger),
		NewMiniMaxFetcher(logger),
		NewMiniMaxCNFetcher(logger),
		NewCodexFetcher(logger),
	}
	for _, f := range fetchers {
		if err := r.RegisterFetcher(f); err != nil {
			logger.WithError(err).Debugf("Failed to register fetcher: %s", f.Name())
		}
	}
}
