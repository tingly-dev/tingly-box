package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
)

// startDynamicCallbackServer starts a temporary callback server for a specific OAuth session
func (s *Server) startDynamicCallbackServer(sessionID string, port int) error {
	s.callbackServersMu.Lock()
	defer s.callbackServersMu.Unlock()

	// Check if a callback server already exists for this session
	if _, exists := s.callbackServers[sessionID]; exists {
		return fmt.Errorf("callback server already exists for session %s", sessionID)
	}

	// Providers with fixed callback ports (for example Codex/OpenAI on 1455)
	// can only have one active browser flow at a time. If a previous attempt
	// is still waiting or failed before cleanup, replace it so retrying OAuth
	// does not require waiting for the five minute timeout.
	if port > 0 {
		for existingSessionID, existingServer := range s.callbackServers {
			if existingServer.GetPort() != port {
				continue
			}

			_ = s.oauthManager.UpdateSessionStatus(existingSessionID, oauth.SessionStatusFailed, "", "Superseded by a new OAuth authorization attempt")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := existingServer.Stop(ctx); err != nil {
				cancel()
				return fmt.Errorf("failed to stop existing callback server on port %d: %w", port, err)
			}
			cancel()
			delete(s.callbackServers, existingSessionID)
			logrus.Debugf("[OAuth] Replaced existing dynamic callback server on port %d for session %s", port, existingSessionID)
		}
	}

	// Create an http.HandlerFunc that properly handles the OAuth callback
	handlerFunc := func(w http.ResponseWriter, r *http.Request) {
		// Ignore favicon requests
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		logrus.Debugf("[OAuth] Callback received: %s %s", r.Method, r.URL.Path)
		logrus.Debugf("[OAuth] Query params: %v", r.URL.Query())

		callbackOpts := oauthmodule.OAuthOptionsForSession(s.oauthManager, sessionID, fmt.Sprintf("http://localhost:%d", port))

		// Delegate to the OAuth callback handler
		// We need to directly call the oauth manager since gin won't work here
		token, err := s.oauthManager.HandleCallback(r.Context(), r, callbackOpts...)
		if err != nil {
			logrus.Debugf("[OAuth] Callback error: %v", err)
			if sessionID != "" {
				_ = s.oauthManager.UpdateSessionStatus(sessionID, oauth.SessionStatusFailed, "", err.Error())
				s.stopDynamicCallbackServerAfterResponse(sessionID)
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "<html><body><h1>OAuth Error</h1><p>%s</p></body></html>", err.Error())
			return
		}

		// Use oauth handler to create the provider
		providerUUID, err := s.oauthHandler.CreateProviderFromToken(token, token.Provider, "", token.SessionID)
		if err != nil {
			logrus.Debugf("[OAuth] Failed to create provider: %v", err)
			failedSessionID := token.SessionID
			if failedSessionID == "" {
				failedSessionID = sessionID
			}
			if failedSessionID != "" {
				_ = s.oauthManager.UpdateSessionStatus(failedSessionID, oauth.SessionStatusFailed, "", err.Error())
			}
			s.stopDynamicCallbackServerAfterResponse(sessionID)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "<html><body><h1>OAuth Error</h1><p>Failed to create provider: %v</p></body></html>", err)
			return
		}

		// Update session status to success if session ID exists
		if token.SessionID != "" {
			_ = s.oauthManager.UpdateSessionStatus(token.SessionID, oauth.SessionStatusSuccess, providerUUID, "")
		}

		logrus.Debugf("[OAuth] Callback successful for provider %s, created provider %s", token.Provider, providerUUID)

		// Stop the dynamic callback server after successful callback
		s.stopDynamicCallbackServerAfterResponse(sessionID)

		// Return success HTML page
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>OAuth Success</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 50px auto; padding: 20px; }
        .success { background: #d4edda; border: 1px solid #c3e6cb; color: #155724; padding: 20px; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="success">
        <h1>OAuth Authorization Successful!</h1>
        <p>You can close this window and return to the application.</p>
        <h2>Provider: %s</h2>
        <p>Token: %s</p>
    </div>
</body>
</html>`, string(token.Provider), oauthmodule.SafeTokenPreview(token.AccessToken))
	}

	// Create a new callback server with the handler
	callbackServer := oauth.NewCallbackServer(handlerFunc)

	// Start the callback server on the specified port
	if err := callbackServer.Start(port); err != nil {
		return fmt.Errorf("failed to start callback server on port %d: %w", port, err)
	}

	// Store the callback server reference
	s.callbackServers[sessionID] = callbackServer

	logrus.Debugf("[OAuth] Started dynamic callback server on port %d for session %s", port, sessionID)

	// Auto-shutdown after 5 minutes
	go func() {
		time.Sleep(5 * time.Minute)
		s.stopDynamicCallbackServer(sessionID)
	}()

	return nil
}

func (s *Server) stopDynamicCallbackServerAfterResponse(sessionID string) {
	go func() {
		time.Sleep(1 * time.Second) // Give time for the response to be sent
		s.stopDynamicCallbackServer(sessionID)
	}()
}

// stopDynamicCallbackServer stops and removes a dynamic callback server
func (s *Server) stopDynamicCallbackServer(sessionID string) {
	s.callbackServersMu.Lock()
	defer s.callbackServersMu.Unlock()

	callbackServer, exists := s.callbackServers[sessionID]
	if !exists {
		return
	}

	// Shutdown the callback server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := callbackServer.Stop(ctx); err != nil {
		logrus.Debugf("[OAuth] Error stopping callback server for session %s: %v", sessionID, err)
	}

	// Remove from map
	delete(s.callbackServers, sessionID)

	logrus.Debugf("[OAuth] Stopped dynamic callback server for session %s", sessionID)
}

// StartDynamicCallbackServer starts a temporary callback server for OAuth
// Implements CallbackServerManager interface for oauth module
func (s *Server) StartDynamicCallbackServer(sessionID string, port int) error {
	return s.startDynamicCallbackServer(sessionID, port)
}

// StopDynamicCallbackServer stops a temporary callback server for OAuth
// Implements CallbackServerManager interface for oauth module
func (s *Server) StopDynamicCallbackServer(sessionID string) {
	s.stopDynamicCallbackServer(sessionID)
}
