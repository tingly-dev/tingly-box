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

type sendFileArgs struct {
	Path    string `json:"path" jsonschema:"Path to the file to publish in this conversation"`
	Caption string `json:"caption,omitempty" jsonschema:"Optional description shown with the file"`
}

type sendFileResult struct {
	MessageID string `json:"message_id"`
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
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-projects", Version: "1.0.0"}, nil)
	var coordinatorTurn *coordinatorToolTurn
	if f.service.Coordinator != nil {
		thread, err := f.service.Store.GetThread(threadID)
		if err != nil {
			return "", err
		}
		p, err := f.service.Store.GetProject(thread.ProjectID)
		if err != nil {
			return "", err
		}
		if p.MainThreadID == threadID {
			turn := &coordinatorToolTurn{}
			srv, err = f.service.Coordinator.coordinatorMCPForTurn(turn)
			if err != nil {
				return "", err
			}
			f.coordinators[threadID] = turn
			coordinatorTurn = turn
		}
	}
	mcp.AddTool(srv, &mcp.Tool{Name: "send_file", Description: f.sendFileDescription(threadID)}, func(ctx context.Context, _ *mcp.CallToolRequest, args sendFileArgs) (*mcp.CallToolResult, sendFileResult, error) {
		if coordinatorTurn != nil {
			coordinatorTurn.mu.Lock()
			defer coordinatorTurn.mu.Unlock()
			snapshot := coordinatorTurn.snapshot
			if snapshot == nil || coordinatorTurn.quiet.Load() {
				return nil, sendFileResult{}, fmt.Errorf("no coordinator turn is active")
			}
			current, err := f.service.Store.GetProject(snapshot.ID)
			if err != nil {
				return nil, sendFileResult{}, err
			}
			if current.Paused || current.ArchivedAt != nil || len(configurationChanges(snapshot, current)) > 0 {
				return nil, sendFileResult{}, fmt.Errorf("project settings changed or the project is paused")
			}
		}
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
