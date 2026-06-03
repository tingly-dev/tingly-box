package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pkg/browser"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/module/codeximport"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/network"
)

// Start starts the HTTP server
func (s *Server) Start(port int) error {
	// Start token refresher background goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.oauthRefresher != nil {
		go s.oauthRefresher.Start(ctx)
		log.Println("OAuth token auto-refresh started")
	}

	// Start provider quota auto-refresh
	if s.quotaManager != nil {
		s.quotaManager.StartAutoRefresh(ctx)
		log.Println("Provider quota auto-refresh started")
	}

	// Start configuration watcher
	if s.watcher != nil {
		if err := s.watcher.Start(); err != nil {
			logrus.Debugf("Failed to start config watcher: %v", err)
		} else {
			log.Println("Configuration hot-reload enabled")
		}
	}

	// Start remote coder service (auto-start by default)
	if err := s.StartRemoteCoder(); err != nil {
		logrus.WithError(err).Warn("Failed to auto-start remote-coder")
	} else {
		logrus.Info("Remote-coder auto-start initiated")
	}

	addr := fmt.Sprintf("%s:%d", s.host, port)
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s.engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	resolvedHost := network.ResolveHost(s.host)

	// Store the resolved server host in config for TBClient to use
	// This ensures 0.0.0.0 resolves to actual local IP for client connections
	if resolvedHost != "" {
		if err := s.config.SetServerHost(resolvedHost); err != nil {
			logrus.WithError(err).Warn("Failed to store server host in config")
		}
	}

	scheme := "http"

	// CASE 1: Non-UI Mode ---
	if !s.enableUI {
		fmt.Printf("OpenAI v1 Chat API endpoint: %s://%s:%d/openai/v1/chat/completions\n", scheme, resolvedHost, port)
		fmt.Printf("Anthropic v1 Message API endpoint: %s://%s:%d/anthropic/v1/messages\n", scheme, resolvedHost, port)
		fmt.Printf("Embeddings API endpoint: %s://%s:%d/tingly/embed/v1/embeddings\n", scheme, resolvedHost, port)
		fmt.Printf("Image Generation API endpoint: %s://%s:%d/tingly/imagegen/v1/images/generations\n", scheme, resolvedHost, port)
		fmt.Printf("Image Generation (Responses API): %s://%s:%d/tingly/imagegen/v1/responses\n", scheme, resolvedHost, port)
		fmt.Printf("Virtual Model API (OpenAI): %s://%s:%d/virtual/openai/v1/chat/completions\n", scheme, resolvedHost, port)
		fmt.Printf("Virtual Model API (Anthropic): %s://%s:%d/virtual/anthropic/v1/messages\n", scheme, resolvedHost, port)
		fmt.Printf("Mode name: %s\n", constant.DefaultModeName)
		fmt.Printf("Model API key: %s\n", s.config.GetModelToken())
		return s.httpServer.ListenAndServe()
	}

	// CASE 2: Web UI Mode ---
	webUIURL := fmt.Sprintf("%s://%s:%d", scheme, resolvedHost, port)
	if s.config.HasUserToken() {
		webUIURL = fmt.Sprintf("%s/login/%s", webUIURL, s.config.GetUserToken())
	}

	fmt.Printf("Web UI: %s\n", webUIURL)
	if s.openBrowser {
		fmt.Printf("Starting server and opening browser...\n")
	} else {
		fmt.Printf("Starting server...\n")
	}

	// Use a channel to capture the immediate error if ListenAndServe fails
	serverError := make(chan error, 1)
	go func() {
		serverError <- s.httpServer.ListenAndServe()
	}()

	// Instead of a fixed 100ms sleep, we poll the port
	if err := waitForPort(addr, 2*time.Second); err != nil {
		// Check if the server goroutine already caught a "port in use" error
		select {
		case e := <-serverError:
			return e
		default:
			return fmt.Errorf("timeout: server did not start on %s: %v", addr, err)
		}
	}

	// Server is up, now open browser if enabled
	if s.openBrowser {
		browser.OpenURL(webUIURL)
	}

	// Block until server shuts down or errors out
	return <-serverError
}

// Helper: Polls the port to ensure it's open before browser opens
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("port %s not reachable", addr)
}

// GetRouter returns the Gin engine for testing purposes
func (s *Server) GetRouter() *gin.Engine {
	return s.engine
}

// GetLoadBalancer returns the load balancer instance
func (s *Server) GetLoadBalancer() *LoadBalancer {
	return s.loadBalancer
}

// HealthMonitor returns the server's health monitor
func (s *Server) HealthMonitor() *loadbalance.HealthMonitor {
	return s.healthMonitor
}

// StartRemoteCoder starts the remote control service if not already running
func (s *Server) StartRemoteCoder() error {
	s.remoteCoderMu.Lock()
	defer s.remoteCoderMu.Unlock()

	// Already running
	if s.remoteCoderCancel != nil {
		logrus.Debug("Remote control already running")
		return nil
	}

	// Check if imbotsettings handler is available
	if s.imbotSettingsHandler == nil {
		return fmt.Errorf("imbotsettings handler not available")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.remoteCoderCtx = ctx
	s.remoteCoderCancel = cancel

	logrus.Info("Starting remote control service...")

	// Start all enabled bots through the imbotsettings handler
	go func() {
		if err := s.imbotSettingsHandler.StartAllEnabled(ctx); err != nil && ctx.Err() == nil {
			logrus.WithError(err).Warn("Failed to start some enabled bots")
		}
		// Keep context alive until canceled
		<-ctx.Done()
		logrus.Info("Remote control service stopped")
	}()

	return nil
}

// StopRemoteCoder stops the remote control service if running
func (s *Server) StopRemoteCoder() {
	s.remoteCoderMu.Lock()
	defer s.remoteCoderMu.Unlock()

	if s.remoteCoderCancel == nil {
		logrus.Debug("Remote control not running")
		return
	}

	// Cancel the context first
	s.remoteCoderCancel()
	s.remoteCoderCancel = nil
	s.remoteCoderCtx = nil

	// Stop all bots through the imbotsettings handler
	if s.imbotSettingsHandler != nil {
		s.imbotSettingsHandler.StopAll()
		logrus.Info("All bots stopped via imbotsettings handler")
	}

	logrus.Info("Remote-coder stopped")
}

// IsRemoteCoderRunning returns whether the remote control service is running
func (s *Server) IsRemoteCoderRunning() bool {
	s.remoteCoderMu.Lock()
	defer s.remoteCoderMu.Unlock()
	return s.remoteCoderCancel != nil
}

// SyncRemoteCoderBots syncs bots with the remote control bot manager
func (s *Server) SyncRemoteCoderBots(ctx context.Context) error {
	if s.imbotSettingsHandler == nil {
		return fmt.Errorf("bot manager not available")
	}
	return s.imbotSettingsHandler.Sync(ctx)
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		s.ctx = nil
	}

	if s.httpServer == nil {
		return nil
	}

	// Stop remote control if running
	s.StopRemoteCoder()

	// Shutdown ImBot settings handler
	if s.imbotSettingsHandler != nil {
		s.imbotSettingsHandler.Shutdown()
		log.Println("ImBot settings handler stopped")
	}

	// Stop token refresher
	if s.oauthRefresher != nil {
		s.oauthRefresher.Stop()
		log.Println("OAuth token auto-refresh stopped")
	}

	// Stop provider quota auto-refresh
	if s.quotaManager != nil {
		s.quotaManager.StopAutoRefresh()
		log.Println("Provider quota auto-refresh stopped")
	}

	if err := s.undoCodexImportOnStop(); err != nil {
		logrus.WithError(err).Warn("Failed to auto-undo Codex import on stop")
	}

	// Stop debug middleware
	if s.errorMW != nil {
		s.errorMW.Stop()
	}

	// Stop configuration watcher
	if s.watcher != nil {
		s.watcher.Stop()
		log.Println("Configuration watcher stopped")
	}

	// Close all MCP sessions and terminate subprocesses
	if s.mcpRuntime != nil {
		s.mcpRuntime.Close()
		log.Println("MCP runtime stopped: all sessions closed and subprocesses terminated")
	}

	// Close all scenario recording sinks
	s.scenarioRecordSinksMu.Lock()
	for scenario, sink := range s.scenarioRecordSinks {
		if sink != nil {
			sink.Close()
			logrus.Debugf("Closed scenario recording sink for %s", scenario)
		}
	}
	s.scenarioRecordSinks = make(map[typ.RuleScenario]*obs.Sink)
	s.scenarioRecordSinksMu.Unlock()

	// Shutdown OTel meter setup
	if s.meterSetup != nil {
		if err := s.meterSetup.Shutdown(ctx); err != nil {
			logrus.Errorf("OTel shutdown error: %v", err)
		}
	}

	// Close all database stores via StoreManager
	if s.config.StoreManager() != nil {
		if err := s.config.StoreManager().Close(); err != nil {
			logrus.Errorf("Error closing stores: %v", err)
		}
	}

	fmt.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) undoCodexImportOnStop() error {
	if s == nil || s.config == nil {
		return nil
	}
	if !s.config.GetScenarioExtensionBool(typ.ScenarioCodex, codeximport.ImportStateAutoUndoOnStopKey()) {
		return nil
	}
	if !s.config.GetScenarioExtensionBool(typ.ScenarioCodex, codeximport.ImportStateActiveKey()) {
		return nil
	}

	targetProvider := s.config.GetScenarioExtensionString(typ.ScenarioCodex, codeximport.ImportStateSourceProviderKey())
	sourceProvider := s.config.GetScenarioExtensionString(typ.ScenarioCodex, codeximport.ImportStateTargetProviderKey())
	if sourceProvider == "" || targetProvider == "" {
		return nil
	}

	importer := codeximport.NewImporter()
	_, err := importer.ImportOpenAISessions(codeximport.ImportOpenAISessionsRequest{
		SourceProvider: sourceProvider,
		TargetProvider: targetProvider,
		CodexHome:      s.config.GetScenarioExtensionString(typ.ScenarioCodex, codeximport.ImportStateCodexHomeKey()),
		SqliteHome:     s.config.GetScenarioExtensionString(typ.ScenarioCodex, codeximport.ImportStateSqliteHomeKey()),
		StateDBPath:    s.config.GetScenarioExtensionString(typ.ScenarioCodex, codeximport.ImportStateStateDBPathKey()),
		DryRun:         false,
	})
	if err != nil {
		return err
	}
	return s.config.SetScenarioExtensions(typ.ScenarioCodex, map[string]interface{}{
		codeximport.ImportStateActiveKey():         false,
		codeximport.ImportStateSourceProviderKey(): nil,
		codeximport.ImportStateTargetProviderKey(): nil,
		codeximport.ImportStateCodexHomeKey():      nil,
		codeximport.ImportStateSqliteHomeKey():     nil,
		codeximport.ImportStateStateDBPathKey():    nil,
		codeximport.ImportStateAutoUndoOnStopKey(): false,
	})
}
