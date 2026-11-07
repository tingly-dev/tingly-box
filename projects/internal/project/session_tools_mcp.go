package project

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionToolsMCP hosts thread-scoped tools on a loopback listener. Each
// capability URL is private to the Claude process for that thread.
type sessionToolsMCP struct {
	mu           sync.Mutex
	service      *Service
	server       *http.Server
	base         string
	urls         map[string]string
	configs      map[string]string
	handlers     map[string]http.Handler
	coordinators map[string]*coordinatorToolTurn
}

type sendFileResult struct {
	MessageID string `json:"message_id"`
}

// scopeOf is the scope of the session of thread threadID. A thread that cannot
// be found, or a service without a coordinator, gets the worker scope: the
// tools every thread has.
func (f *sessionToolsMCP) scopeOf(threadID string) Scope {
	if f.service.Coordinator == nil {
		return ScopeWorker
	}
	t, err := f.service.Store.GetThread(threadID)
	if err != nil {
		return ScopeWorker
	}
	p, err := f.service.Store.GetProject(t.ProjectID)
	if err != nil {
		return ScopeWorker
	}
	return scopeOf(p, t)
}

func (f *sessionToolsMCP) config(threadID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if path := f.configs[threadID]; path != "" {
		return path, nil
	}
	if f.server == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		f.urls = make(map[string]string)
		f.configs = make(map[string]string)
		f.handlers = make(map[string]http.Handler)
		f.coordinators = make(map[string]*coordinatorToolTurn)
		f.base = "http://" + ln.Addr().String()
		f.server = &http.Server{Handler: http.HandlerFunc(f.serveHTTP)}
		go func() { _ = f.server.Serve(ln) }()
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	path := "/" + hex.EncodeToString(token[:])
	scope := f.scopeOf(threadID)
	var turn *coordinatorToolTurn
	if scope == ScopeCoordinator {
		turn = &coordinatorToolTurn{}
	}
	srv, err := f.buildServer(threadID, scope, turn, f.tools(threadID))
	if err != nil {
		return "", err
	}
	if turn != nil {
		f.coordinators[threadID] = turn
	}
	f.handlers[path] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	f.urls[threadID] = f.base + path
	file, err := os.CreateTemp("", "tingly-projects-mcp-*.json")
	if err != nil {
		delete(f.handlers, path)
		delete(f.urls, threadID)
		delete(f.coordinators, threadID)
		return "", err
	}
	config := map[string]interface{}{"mcpServers": map[string]interface{}{"tingly-projects": map[string]interface{}{"type": "http", "url": f.urls[threadID]}}}
	err = json.NewEncoder(file).Encode(config)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(file.Name())
		delete(f.handlers, path)
		delete(f.urls, threadID)
		delete(f.coordinators, threadID)
		return "", err
	}
	f.configs[threadID] = file.Name()
	return file.Name(), nil
}

// sendFileDescription is what the model reads about send_file, including where
// it may save the file; the output directory is created so a write into it
// needs no mkdir.
func (f *sessionToolsMCP) sendFileDescription(threadID string) string {
	desc := "Publish a file to this conversation so the user can see it."
	t, err := f.service.Store.GetThread(threadID)
	if err != nil {
		return desc
	}
	p, err := f.service.Store.GetProject(t.ProjectID)
	if err != nil {
		return desc
	}
	_ = os.MkdirAll(threadOutputDir(f.service.Layout, p, t), 0o755)
	return desc + " " + ArtifactGuidance(f.service.Layout, p, t)
}

// coordinatorTurn returns the state shared with the main thread's MCP server.
// config must have successfully created that server first.
func (f *sessionToolsMCP) coordinatorTurn(threadID string) *coordinatorToolTurn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.coordinators[threadID]
}

func (f *sessionToolsMCP) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	h := f.handlers[r.URL.Path]
	f.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

func (f *sessionToolsMCP) close(ctx context.Context) error {
	f.mu.Lock()
	srv := f.server
	for _, path := range f.configs {
		_ = os.Remove(path)
	}
	f.mu.Unlock()
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}
