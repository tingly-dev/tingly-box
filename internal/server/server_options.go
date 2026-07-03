package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/typ"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
)

// ServerOption defines a functional option for Server configuration
type ServerOption func(*Server)

// WithDefault applies all default server options
func WithDefault() ServerOption {
	return func(s *Server) {
		s.enableUI = true      // Default: UI enabled
		s.enableAdaptor = true // Default: adapter enabled
		s.openBrowser = true   // Default: open browser enabled
		s.host = ""            // Default: empty host (resolves to localhost)
	}
}

func WithVersion(version string) ServerOption {
	return func(s *Server) {
		s.version = version
	}
}

// WithUI enables or disables the UI for the server
func WithUI(enabled bool) ServerOption {
	return func(s *Server) {
		s.enableUI = enabled
	}
}

func WithHost(host string) ServerOption {
	return func(s *Server) {
		s.host = host
	}
}

// WithAdaptor enables or disables the adaptor for the server
func WithAdaptor(enabled bool) ServerOption {
	return func(s *Server) {
		s.enableAdaptor = enabled
	}
}

// WithOpenBrowser enables or disables automatic browser opening
func WithOpenBrowser(enabled bool) ServerOption {
	return func(s *Server) {
		s.openBrowser = enabled
	}
}

// WithRecordMode sets the record mode for request/response recording
// mode: empty string = disabled, "all" = record all, "response" = response only, "scenario" = record scenario only
func WithRecordMode(mode obs.RecordMode) ServerOption {
	return func(s *Server) {
		s.recordMode = mode
	}
}

// WithRecordDir sets the scenario-level record directory
func WithRecordDir(dir string) ServerOption {
	return func(s *Server) {
		s.recordDir = dir
	}
}

// WithRecordingCAS toggles content-addressed dedup alongside the default
// gzip recording. When enabled, each session is written twice: once as a
// gzip JSONL.gz (default), and once as content-addressed slim JSONL plus a
// per-record blob tree. Useful for cross-session prompt analysis and replay.
func WithRecordingCAS(enabled bool) ServerOption {
	return func(s *Server) {
		s.recordCAS = enabled
	}
}

// WithRecording enables dual-stage recording for protocol conversion scenarios
func WithRecording(enabled bool) ServerOption {
	return func(s *Server) {
		s.enableRecording = enabled
	}
}

// WithGuardrails sets a guardrails runtime for stream evaluation.
func WithGuardrails(runtime *guardrails.Guardrails) ServerOption {
	return func(s *Server) {
		s.setGuardrailsRuntimeRef(runtime)
	}
}

// WithDebug enables or disables debug mode for the server
func WithDebug(enabled bool) ServerOption {
	return func(s *Server) {
		s.debug = enabled
	}
}

// WithMultiLogger sets the multi-mode logger for the server
func WithMultiLogger(logger *pkgobs.MultiLogger) ServerOption {
	return func(s *Server) {
		s.multiLogger = logger
	}
}

// WithTemplateManager allows TBE to inject a custom TemplateManager.
// This follows the same pattern as WithAuthMiddleware for consistency.
func WithTemplateManager(tm *data.TemplateManager) ServerOption {
	return func(s *Server) {
		s.templateManager = tm
		if s.config != nil {
			s.config.SetTemplateManager(tm)
		}
	}
}

// WithAuthMiddleware sets custom auth middlewares for WebUI and Model API endpoints
// This allows TBE to inject its own JWT auth middleware instead of using
// tingly-box's default UserAuthMiddleware and ModelAuthMiddleware
//
// Usage in TBE:
//
//	server := NewServer(cfg,
//	    WithAuthMiddleware(tbeUserAuth, tbeModelAuth),
//	)
func WithAuthMiddleware(userAuth, modelAuth gin.HandlerFunc) ServerOption {
	return func(s *Server) {
		s.customUserAuthMiddleware = userAuth
		s.customModelAuthMiddleware = modelAuth
	}
}

// WithUserAuthMiddleware sets a custom user auth middleware for WebUI endpoints
// Use this if you only want to replace UserAuthMiddleware but keep ModelAuthMiddleware
func WithUserAuthMiddleware(userAuth gin.HandlerFunc) ServerOption {
	return func(s *Server) {
		s.customUserAuthMiddleware = userAuth
	}
}

// WithModelAuthMiddleware sets a custom model auth middleware for Model API endpoints
// Use this if you only want to replace ModelAuthMiddleware but keep UserAuthMiddleware
func WithModelAuthMiddleware(modelAuth gin.HandlerFunc) ServerOption {
	return func(s *Server) {
		s.customModelAuthMiddleware = modelAuth
	}
}

// sinkOpts derives obs.SinkOption values from the server's recording config.
func (s *Server) sinkOpts() []obs.SinkOption {
	var opts []obs.SinkOption
	if s.recordCAS {
		opts = append(opts, obs.WithCASExporter())
	}
	return opts
}

// GetOrCreateScenarioSink gets or creates a recording sink for the specified scenario
// The sink is created on-demand and cached for subsequent use
func (s *Server) GetOrCreateScenarioSink(scenario typ.RuleScenario) *obs.Sink {
	s.scenarioRecordSinksMu.Lock()
	defer s.scenarioRecordSinksMu.Unlock()

	// Return existing sink if already created
	if sink, exists := s.scenarioRecordSinks[scenario]; exists {
		return sink
	}

	mode := s.GetScenarioRecordMode(scenario)
	if mode == "" {
		return nil
	}

	// Create new sink for this scenario using the scenario-effective recording mode.
	// This allows `recording_v2` on individual scenarios such as `claude_code`
	// even when global CLI record mode is unset.
	sink := obs.NewSink(s.recordDir, mode, s.sinkOpts()...)
	if sink == nil {
		// Sink creation failed or recording is disabled (empty recordDir)
		// This is expected when no record directory is configured
		logrus.Debugf("Failed to create scenario recording sink for %s", scenario)
		return nil
	}

	s.scenarioRecordSinks[scenario] = sink
	logrus.Debugf("Created scenario recording sink for %s, mode: %s, directory: %s", scenario, mode, s.recordDir)
	return sink
}

func (s *Server) GetScenarioRecordMode(scenario typ.RuleScenario) obs.RecordMode {
	if s == nil || s.config == nil {
		return s.recordMode
	}

	if mode := s.config.GetScenarioRecordingMode(scenario); mode != typ.RecordingModeDisabled {
		return obs.RecordMode(mode)
	}

	return s.recordMode
}

// EnsureProtocolRecorder delegates to the AI Model API handler, which owns
// the ProtocolRecorder type. Kept as a thin root wrapper since callers
// (anthropic_message.go and its tests) have not moved to aimodel yet.
func (s *Server) EnsureProtocolRecorder(c *gin.Context, scenario string, provider *typ.Provider, model string, mode obs.RecordMode, bs []byte) *recording.ProtocolRecorder {
	return s.aiHandler.EnsureProtocolRecorder(c, scenario, provider, model, mode, bs)
}
