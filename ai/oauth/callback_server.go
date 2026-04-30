package oauth

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// CallbackServer manages a temporary HTTP server for OAuth callbacks
type CallbackServer struct {
	server   *http.Server
	listener net.Listener
	handler  http.HandlerFunc
	port     int
	started  bool
	mu       sync.Mutex
	done     chan struct{}
}

// NewCallbackServer creates a new callback server manager
func NewCallbackServer(handler http.HandlerFunc) *CallbackServer {
	return &CallbackServer{
		handler: handler,
		done:    make(chan struct{}),
	}
}

// Start starts the callback server on the specified port
// If port is 0, it will try to bind to any available port
func (cs *CallbackServer) Start(port int) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.started {
		return fmt.Errorf("callback server already started")
	}

	// Try to bind to the specified port
	var listener net.Listener
	var err error

	if port > 0 {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return fmt.Errorf("failed to bind to port %d: %w", port, err)
		}
	} else {
		// Try to find an available port
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return fmt.Errorf("failed to bind to any port: %w", err)
		}
	}

	// Get the actual port (in case 0 was specified)
	cs.port = listener.Addr().(*net.TCPAddr).Port

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", cs.handler)

	cs.server = &http.Server{
		Handler: mux,
	}

	cs.listener = listener
	cs.started = true

	// Start server in background
	go func() {
		if err := cs.server.Serve(cs.listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Callback server error: %v", err)
		}
		close(cs.done)
	}()

	log.Printf("OAuth callback server started on port %d", cs.port)
	return nil
}

// GetPort returns the port the server is listening on
func (cs *CallbackServer) GetPort() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.port
}

// GetURL returns the base URL for the callback server
func (cs *CallbackServer) GetURL() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return fmt.Sprintf("http://localhost:%d", cs.port)
}

// Stop stops the callback server gracefully
func (cs *CallbackServer) Stop(ctx context.Context) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.started {
		return nil
	}

	log.Printf("Stopping OAuth callback server on port %d", cs.port)

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := cs.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown callback server: %w", err)
	}

	cs.started = false
	return nil
}

// Wait waits for the server to finish (e.g., after Shutdown)
func (cs *CallbackServer) Wait() {
	<-cs.done
}

// IsRunning returns true if the server is running
func (cs *CallbackServer) IsRunning() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.started
}
