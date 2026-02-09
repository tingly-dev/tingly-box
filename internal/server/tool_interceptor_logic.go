package server

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// resolveToolInterceptor determines interception and tool stripping behavior for a provider.
func (s *Server) resolveToolInterceptor(provider *typ.Provider, hasBuiltInWebSearch bool) (shouldIntercept bool, shouldStripTools bool, interceptorConfig *typ.ToolInterceptorConfig) {
	if s.toolInterceptor != nil {
		interceptorConfig = s.toolInterceptor.GetConfigForProvider(provider)
	}

	shouldIntercept = interceptorConfig != nil && (interceptorConfig.PreferLocalSearch || !hasBuiltInWebSearch)
	shouldStripTools = interceptorConfig == nil && !hasBuiltInWebSearch

	if interceptorConfig != nil && interceptorConfig.PreferLocalSearch {
		logrus.Debugf("Tool interceptor active for provider %s (prefer_local_search enabled)", provider.Name)
	} else if shouldIntercept {
		logrus.Debugf("Tool interceptor active for provider %s (no built-in web_search)", provider.Name)
	} else if shouldStripTools {
		logrus.Debugf("Tool interceptor disabled and provider %s has no built-in web_search; stripping search/fetch tools", provider.Name)
	} else if hasBuiltInWebSearch {
		logrus.Debugf("Provider %s has built-in web_search, using native implementation", provider.Name)
	}

	return shouldIntercept, shouldStripTools, interceptorConfig
}
