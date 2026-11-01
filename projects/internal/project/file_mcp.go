package project

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fileMCP hosts a thread-scoped send_file tool on a loopback listener. Each
// capability URL is private to the Claude process for that thread.
type fileMCP struct {
	mu       sync.Mutex
	service  *Service
	server   *http.Server
	base     string
	urls     map[string]string
	configs  map[string]string
	handlers map[string]http.Handler
}

type sendFileArgs struct {
	Path    string `json:"path" jsonschema:"Path to the file to publish in this conversation"`
	Caption string `json:"caption,omitempty" jsonschema:"Optional description shown with the file"`
}

type sendFileResult struct {
	MessageID string `json:"message_id"`
}

func (f *fileMCP) config(threadID string) (string, error) {
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
		f.base = "http://" + ln.Addr().String()
		f.server = &http.Server{Handler: http.HandlerFunc(f.serveHTTP)}
		go func() { _ = f.server.Serve(ln) }()
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	path := "/" + hex.EncodeToString(token[:])
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-projects", Version: "1.0.0"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "send_file", Description: "Publish a file to this conversation so the user can see it."}, func(ctx context.Context, _ *mcp.CallToolRequest, args sendFileArgs) (*mcp.CallToolResult, sendFileResult, error) {
		if args.Path == "" {
			return nil, sendFileResult{}, fmt.Errorf("path is required")
		}
		e, err := f.service.PublishFile(ctx, threadID, args.Path, args.Caption)
		if err != nil {
			return nil, sendFileResult{}, err
		}
		return nil, sendFileResult{MessageID: e.ID}, nil
	})
	f.handlers[path] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	f.urls[threadID] = f.base + path
	file, err := os.CreateTemp("", "tingly-projects-mcp-*.json")
	if err != nil {
		delete(f.handlers, path)
		delete(f.urls, threadID)
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
		return "", err
	}
	f.configs[threadID] = file.Name()
	return file.Name(), nil
}

func (f *fileMCP) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	h := f.handlers[r.URL.Path]
	f.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

func (f *fileMCP) close(ctx context.Context) error {
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
