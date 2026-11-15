// Package loopback serves MCP servers on a loopback listener, one unguessable
// capability URL per key: what an agent process is handed so it can reach the
// tools its host offers. The host builds the servers; loopback owns the
// listener, the secret paths and the config files an agent reads them from.
package loopback

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server hosts per-key MCP servers on one loopback listener.
type Server struct {
	// name is what the agent's config calls the server (the mcpServers key).
	name string
	// filePrefix names the temp config files handed to agent processes.
	filePrefix string

	mu       sync.Mutex
	server   *http.Server
	base     string
	urls     map[string]string
	configs  map[string]string
	handlers map[string]http.Handler
	states   map[string]any
}

// New makes a Server that lazily listens on 127.0.0.1:0 (the listener starts
// with the first Add). name is the mcpServers key an agent's config uses;
// filePrefix names the temp config files.
func New(name, filePrefix string) *Server {
	return &Server{name: name, filePrefix: filePrefix,
		urls: map[string]string{}, configs: map[string]string{}, handlers: map[string]http.Handler{}, states: map[string]any{}}
}

// Add gives key its capability URL, building the server for it if it does not
// have one yet. build returns the server and any host state to keep with the
// key (the URL, a turn the host's tools share, anything Revoke should end).
// A key that already has a URL keeps it: the config an agent process started
// with stays valid. configPath is the temp JSON file naming the server for an
// agent's MCP config; the caller hands its path to the agent.
func (s *Server) Add(key string, build func() (*mcp.Server, any, error)) (url, configPath string, state any, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if url := s.urls[key]; url != "" {
		return url, s.configs[key], s.states[key], nil
	}
	if s.server == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", "", nil, err
		}
		s.base = "http://" + ln.Addr().String()
		s.server = &http.Server{Handler: http.HandlerFunc(s.serveHTTP)}
		go func() { _ = s.server.Serve(ln) }()
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", "", nil, err
	}
	path := "/" + hex.EncodeToString(token[:])
	srv, state, err := build()
	if err != nil {
		return "", "", nil, err
	}
	s.handlers[path] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	s.urls[key] = s.base + path
	s.states[key] = state
	file, err := os.CreateTemp("", s.filePrefix+"-*.json")
	if err != nil {
		s.drop(path, key)
		return "", "", nil, err
	}
	config := map[string]interface{}{"mcpServers": map[string]interface{}{s.name: map[string]interface{}{"type": "http", "url": s.urls[key]}}}
	err = json.NewEncoder(file).Encode(config)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file.Name())
		s.drop(path, key)
		return "", "", nil, err
	}
	s.configs[key] = file.Name()
	return s.urls[key], file.Name(), state, nil
}

// URL is key's capability URL, or "" when it has none.
func (s *Server) URL(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.urls[key]
}

// State is what build returned for key, or nil.
func (s *Server) State(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.states[key]
}

// Revoke ends a key's capability URL: its path stops answering and its config
// file goes. The next Add issues a fresh one. Call it only once the agent
// process is gone (a resident process keeps the URL it started with).
func (s *Server) Revoke(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if url := s.urls[key]; url != "" {
		delete(s.handlers, strings.TrimPrefix(url, s.base))
	}
	s.drop("", key)
}

// drop removes key's entry (and, when given, its serving path).
func (s *Server) drop(path, key string) {
	if path != "" {
		delete(s.handlers, path)
	}
	delete(s.urls, key)
	delete(s.states, key)
	if p := s.configs[key]; p != "" {
		_ = os.Remove(p)
	}
	delete(s.configs, key)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	h := s.handlers[r.URL.Path]
	s.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

// Close stops the listener and removes every config file.
func (s *Server) Close(ctx context.Context) error {
	s.mu.Lock()
	srv := s.server
	for _, path := range s.configs {
		_ = os.Remove(path)
	}
	s.mu.Unlock()
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}
