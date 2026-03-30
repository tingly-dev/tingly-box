package server

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/toolruntime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// resolveToolRuntime determines whether the generic runtime should be active for a provider.
func (s *Server) resolveToolRuntime(provider *typ.Provider) bool {
	if s.toolRuntime == nil {
		return false
	}
	enabled := s.toolRuntime.IsEnabledForProvider(provider)
	if enabled {
		logrus.Debugf("Tool runtime active for provider %s", provider.Name)
	}
	return enabled
}

func (s *Server) nativeToolSupport(provider *typ.Provider) toolruntime.NativeToolSupport {
	support := toolruntime.NativeToolSupport{}
	if s == nil || s.templateManager == nil || provider == nil {
		return support
	}
	for _, toolName := range []string{toolruntime.BuiltinToolSearch, toolruntime.BuiltinToolFetch} {
		if s.templateManager.ProviderSupportsNativeTool(provider, toolName) {
			support[toolName] = true
		}
	}
	return support
}
