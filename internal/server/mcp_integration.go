package server

func (s *Server) advisorMaxUses() int {
	if s.mcpRuntime == nil {
		return 0
	}
	cfg := s.mcpRuntime.GetConfig()
	if cfg == nil {
		return 0
	}
	for _, source := range cfg.Sources {
		if source.Advisor != nil && source.Advisor.MaxUsesPerRequest > 0 {
			return source.Advisor.MaxUsesPerRequest
		}
	}
	return 0
}
