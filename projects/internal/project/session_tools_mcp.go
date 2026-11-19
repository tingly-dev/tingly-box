package project

import (
	"context"
	"os"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/mcphub/loopback"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// sessionToolsMCP hosts thread-scoped tools on a loopback listener. Each
// capability URL is private to the Claude process for that thread; the
// listener mechanics live in mcphub/loopback.
type sessionToolsMCP struct {
	service *Service
	lb      *loopback.Server
	lbOnce  sync.Once
}

// loop is the loopback server, made on first use.
func (f *sessionToolsMCP) loop() *loopback.Server {
	f.lbOnce.Do(func() { f.lb = loopback.New("tingly-projects", "tingly-projects-mcp") })
	return f.lb
}

type sendFileResult struct {
	MessageID string `json:"message_id"`
}

func (f *sessionToolsMCP) config(threadID string) (string, error) {
	_, path, _, err := f.loop().Add(threadID, func() (*mcp.Server, any, error) {
		return f.buildFor(threadID)
	})
	return path, err
}

// revoke ends a thread's capability URL: its path stops answering and its
// config file goes. The next config(threadID) issues a fresh one. Call it only
// once the thread's session process is gone (a resident process keeps the URL
// it started with), and not for a project's main thread, whose turn the
// coordinator shares.
func (f *sessionToolsMCP) revoke(threadID string) {
	f.loop().Revoke(threadID)
}

// sendFileDescription is what the model reads about send_file, including where
// it may save the file; the output directory is created so a write into it
// needs no mkdir.
func (f *sessionToolsMCP) sendFileDescription(t *db.Thread, p *db.Project) string {
	_ = os.MkdirAll(threadOutputDir(f.service.Layout, p, t), 0o755)
	return "Publish a file to this conversation so the user can see it. " + ArtifactGuidance(f.service.Layout, p, t)
}

// coordinatorTurn returns the state shared with the main thread's MCP server.
// config must have successfully created that server first.
func (f *sessionToolsMCP) coordinatorTurn(threadID string) *coordinatorToolTurn {
	if state, _ := f.loop().State(threadID).(*coordinatorToolTurn); state != nil {
		return state
	}
	return nil
}

func (f *sessionToolsMCP) close(ctx context.Context) error {
	return f.loop().Close(ctx)
}
