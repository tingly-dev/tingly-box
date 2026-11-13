// Package coordtest plays the coordinator's model in tests.
//
// The coordinator is a Claude Code session; under agentboot/engine/enginetest
// a script stands in for Claude Code. Script tells the coordinator's turns
// from the threads' and gives each coordinator turn a Turn, whose Call uses
// the Projects MCP tools the session was launched with, over the same
// loopback server Claude Code would connect to. What the script says becomes
// the coordinator's reply.
package coordtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
)

// NoUpdate is the text an older model answers with to stay quiet.
const NoUpdate = "NO_UPDATE"

// Model is the model coordinator turns report running on.
const Model = "coord-model"

// IsCoordinator reports whether a turn was launched as the coordinator's
// session rather than a thread's.
func IsCoordinator(opts agentboot.ExecutionOptions) bool {
	return opts.StrictMcpConfig && slices.Contains(opts.AllowedTools, "mcp__tingly-projects")
}

// Script runs coordinator turns with coord and every other turn with thread
// (nil thread turns just finish). A nil coord answers nothing.
func Script(coord func(*Turn) error, thread func(*enginetest.Turn) error) func(*enginetest.Turn) error {
	return func(t *enginetest.Turn) error {
		if !IsCoordinator(t.Opts) {
			if thread == nil {
				return nil
			}
			return thread(t)
		}
		t.Init(Model)
		if coord == nil {
			return nil
		}
		turn := &Turn{Turn: t}
		defer turn.close()
		return coord(turn)
	}
}

// Turn is one coordinator turn as its script sees it.
type Turn struct {
	*enginetest.Turn

	mu      sync.Mutex
	session *mcp.ClientSession
	calls   []Call
}

// Call is one tool call a turn made and what it got back.
type Call struct {
	Name    string
	Args    string
	Out     string
	IsError bool
}

// Call runs a Projects MCP tool and returns its text and whether it failed.
// A failure to reach the server is reported as a failed call.
func (t *Turn) Call(name string, args any) (string, bool) {
	out, isErr := t.call(name, args)
	b, _ := json.Marshal(args)
	t.mu.Lock()
	t.calls = append(t.calls, Call{Name: name, Args: string(b), Out: out, IsError: isErr})
	t.mu.Unlock()
	return out, isErr
}

func (t *Turn) call(name string, args any) (string, bool) {
	session, err := t.connect()
	if err != nil {
		return "coordtest: " + err.Error(), true
	}
	if args == nil {
		args = map[string]any{}
	}
	res, err := session.CallTool(t.Ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return "coordtest: " + err.Error(), true
	}
	var text []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			text = append(text, tc.Text)
		}
	}
	return strings.Join(text, "\n"), res.IsError
}

// Calls are the tool calls this turn made, in order.
func (t *Turn) Calls() []Call {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Call(nil), t.calls...)
}

// Tools lists the tools the session was given.
func (t *Turn) Tools() []string {
	session, err := t.connect()
	if err != nil {
		return nil
	}
	res, err := session.ListTools(t.Ctx, nil)
	if err != nil {
		return nil
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// Reply says text as the coordinator's answer.
func (t *Turn) Reply(text string) { t.Say(text, 1, 1) }

var (
	eventRe = regexp.MustCompile(`(?s)<event([^>]*)>\n(.*?)\n</event>`)
	kindRe  = regexp.MustCompile(`\bkind="([^"]*)"`)
)

type event struct{ attrs, body string }

func (t *Turn) events() []event {
	var out []event
	for _, m := range eventRe.FindAllStringSubmatch(t.Prompt, -1) {
		out = append(out, event{attrs: m[1], body: m[2]})
	}
	return out
}

func (e event) kind() string {
	if m := kindRe.FindStringSubmatch(e.attrs); m != nil {
		return m[1]
	}
	return ""
}

// input is the event that woke this turn: the last one that is not the
// system's check-in or settings note after it.
func (t *Turn) input() (event, bool) {
	evs := t.events()
	for i := len(evs) - 1; i >= 0; i-- {
		if k := evs[i].kind(); k != "check-in" && k != "settings-changed" {
			return evs[i], true
		}
	}
	return event{}, false
}

// Input is the text of the event that woke this turn (a user's message, a
// thread's report), without its envelope.
func (t *Turn) Input() string {
	e, _ := t.input()
	return e.body
}

// InputEnvelope is the opening of the waking event's envelope, its
// attributes as the coordinator saw them.
func (t *Turn) InputEnvelope() string {
	e, _ := t.input()
	return "<event" + e.attrs + ">"
}

// FromUser reports whether a user's message woke this turn.
func (t *Turn) FromUser() bool {
	e, ok := t.input()
	return ok && strings.Contains(e.attrs, `trust="principal"`)
}

// CheckIn reports whether a thread's report woke this turn.
func (t *Turn) CheckIn() bool { return t.hasKind("check-in") }

// SettingsChanged reports whether a settings change woke this turn.
func (t *Turn) SettingsChanged() bool { return t.hasKind("settings-changed") }

func (t *Turn) hasKind(kind string) bool {
	for _, e := range t.events() {
		if e.kind() == kind {
			return true
		}
	}
	return false
}

// ThreadRef is the id prefix list_threads shows for the titled thread ("" when
// there is none).
func (t *Turn) ThreadRef(title string) string {
	out, isErr := t.call("list_threads", map[string]any{"limit": 40})
	if isErr {
		return ""
	}
	m := regexp.MustCompile(`(?m)^- (\w+) ` + regexp.QuoteMeta(strconv.Quote(title))).FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return m[1]
}

// ToolSession connects to the Projects tools of a session whose MCP config is
// at configPath (ExecutionOptions.MCPConfigPath). The caller closes it.
func ToolSession(ctx context.Context, configPath string) (*mcp.ClientSession, error) {
	url, err := serverURL(configPath)
	if err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "coordtest", Version: "1"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: url}, nil)
}

func (t *Turn) connect() (*mcp.ClientSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != nil {
		return t.session, nil
	}
	url, err := serverURL(t.Opts.MCPConfigPath)
	if err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "coordtest", Version: "1"}, nil)
	session, err := client.Connect(t.Ctx, &mcp.StreamableClientTransport{Endpoint: url}, nil)
	if err != nil {
		return nil, err
	}
	t.session = session
	return session, nil
}

func (t *Turn) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != nil {
		_ = t.session.Close()
		t.session = nil
	}
}

// serverURL reads the Projects server's URL from a session's MCP config.
func serverURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var config struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return "", err
	}
	url := config.MCPServers["tingly-projects"].URL
	if url == "" {
		return "", fmt.Errorf("no tingly-projects server in %s", path)
	}
	return url, nil
}
