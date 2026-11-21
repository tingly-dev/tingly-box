package script

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conv is a client conversation shaped like Claude Code's: it keeps the
// history, interleaves a role=system entry and sends tool results back.
type conv struct {
	t       *testing.T
	url     string
	system  any
	tools   []string
	session string
	stream  bool
	msgs    []map[string]any
}

func newConv(t *testing.T, url, system string, tools ...string) *conv {
	return &conv{t: t, url: url, system: []map[string]any{{"type": "text", "text": "x-anthropic-billing-header: cc_version=2.1.259.1; cc_entrypoint=test;"}, {"type": "text", "text": system}}, tools: tools, session: wireID("sess_", system)}
}

type answer struct {
	status int
	text   string
	calls  []map[string]any
	raw    string
}

func (c *conv) post() answer {
	c.t.Helper()
	tools := []map[string]any{}
	for _, n := range c.tools {
		tools = append(tools, map[string]any{"name": n, "input_schema": map[string]any{"type": "object"}})
	}
	msgs := append(append([]map[string]any{}, c.msgs...), map[string]any{"role": "system", "content": "<total_tokens>1</total_tokens>"})
	body, _ := json.Marshal(map[string]any{"model": "demo", "system": c.system, "tools": tools, "messages": msgs, "stream": c.stream, "max_tokens": 100})
	req, _ := http.NewRequest(http.MethodPost, c.url+"/v1/messages?beta=true", bytes.NewReader(body))
	req.Header.Set("X-Claude-Code-Session-Id", c.session)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	a := answer{status: resp.StatusCode, raw: buf.String()}
	if resp.StatusCode != 200 {
		return a
	}
	var content []map[string]any
	if c.stream {
		content = parseSSE(c.t, buf.String())
	} else {
		var m struct{ Content []map[string]any }
		require.NoError(c.t, json.Unmarshal(buf.Bytes(), &m))
		content = m.Content
	}
	for _, b := range content {
		switch b["type"] {
		case "text":
			a.text += b["text"].(string)
		case "tool_use":
			a.calls = append(a.calls, b)
		}
	}
	c.msgs = append(c.msgs, map[string]any{"role": "assistant", "content": content})
	return a
}

// say sends a user message.
func (c *conv) say(text string) answer {
	c.msgs = append(c.msgs, map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": text}}})
	return c.post()
}

// results answers every call of the last answer with the given texts.
func (c *conv) results(a answer, texts ...string) answer {
	var blocks []map[string]any
	for i, call := range a.calls {
		blocks = append(blocks, map[string]any{"type": "tool_result", "tool_use_id": call["id"], "content": texts[i]})
	}
	c.msgs = append(c.msgs, map[string]any{"role": "user", "content": blocks})
	return c.post()
}

func parseSSE(t *testing.T, s string) []map[string]any {
	var blocks []map[string]any
	var partial []string
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		switch ev["type"] {
		case "content_block_start":
			cb := ev["content_block"].(map[string]any)
			blocks = append(blocks, cb)
			partial = append(partial, "")
		case "content_block_delta":
			i := int(ev["index"].(float64))
			d := ev["delta"].(map[string]any)
			if d["type"] == "text_delta" {
				blocks[i]["text"] = blocks[i]["text"].(string) + d["text"].(string)
			} else {
				partial[i] += d["partial_json"].(string)
			}
		case "content_block_stop":
			i := int(ev["index"].(float64))
			if blocks[i]["type"] == "tool_use" {
				var in map[string]any
				require.NoError(t, json.Unmarshal([]byte(partial[i]), &in))
				blocks[i]["input"] = in
			}
		}
	}
	return blocks
}

func serve(t *testing.T, yaml string) (*Engine, string) {
	t.Helper()
	s, err := Decode([]byte(yaml))
	require.NoError(t, err)
	e, err := New("test", s)
	require.NoError(t, err)
	srv := httptest.NewServer(e)
	t.Cleanup(srv.Close)
	return e, srv.URL
}

const projectsScript = `
tracks:
  coordinator:
    bind: {tools_include: ["__start_thread"], system_contains: "You are the project coordinator"}
    steps:
      - id: plan
        expect: {last_text_contains: "CHANGELOG"}
        reply:
          tool_calls:
            - id: start
              name: mcp__tingly-projects__start_thread
              arguments: {title: Add CHANGELOG, task: Create CHANGELOG.md}
              spawns: thread
              expect_result: {contains: "started thread"}
      - id: started
        reply: {content: "Started one thread."}
      - id: report
        expect: {last_text_contains: 'kind="check-in"'}
        reply: {content: "It is ready for review."}
  thread:
    capture:
      cwd: {from: system, regex: "worktree \\(([^)]+)\\)"}
    steps:
      - id: write
        reply:
          content: I'll create it.
          tool_calls:
            - {id: write, name: Write, arguments: {file_path: "{{cwd}}/CHANGELOG.md"}, expect_result: {is_error: false}}
      - id: done
        reply: {content: "Done."}
`

func TestTracksRouteByWhatTheyCarryNotByArrival(t *testing.T) {
	e, url := serve(t, projectsScript)
	coord := newConv(t, url, "You are the project coordinator.", "Bash", "mcp__tingly-projects__start_thread")
	a := coord.say(`<event source="web">Add a CHANGELOG</event>`)
	require.Equal(t, 200, a.status, a.raw)
	require.Len(t, a.calls, 1)
	assert.Equal(t, "Add CHANGELOG", a.calls[0]["input"].(map[string]any)["title"])

	// The thread starts before the coordinator's next request arrives.
	thread := newConv(t, url, "Your task: Add CHANGELOG\nin your own worktree (/w/tp-1).", "Bash", "Write", "mcp__tingly-projects__notify_coordinator")
	thread.stream = true
	ta := thread.say("Create CHANGELOG.md")
	require.Equal(t, 200, ta.status, ta.raw)
	assert.Equal(t, "I'll create it.", ta.text)
	assert.Equal(t, "/w/tp-1/CHANGELOG.md", ta.calls[0]["input"].(map[string]any)["file_path"])

	ca := coord.results(a, `started thread 5a62 "Add CHANGELOG"`)
	assert.Equal(t, "Started one thread.", ca.text)
	assert.Equal(t, "Done.", thread.results(ta, "File created").text)
	assert.Equal(t, "It is ready for review.", coord.say(`<event kind="check-in">Done.</event>`).text)

	rep := e.Finish()
	assert.True(t, rep.OK, rep.String())
	assert.Equal(t, []string{"plan", "started", "report"}, rep.Tracks[0].Served)
	assert.Equal(t, []string{"write", "done"}, rep.Tracks[1].Served)
	vias := []string{}
	for _, en := range e.Trace() {
		vias = append(vias, en.Track+":"+en.Via)
	}
	assert.Equal(t, []string{"coordinator:bind", "thread:bind", "coordinator:continuation", "thread:continuation", "coordinator:continuation"}, vias)
}

func TestConcurrentConversationsKeepTheirOwnSteps(t *testing.T) {
	e, url := serve(t, `
tracks:
  a:
    bind: {system_contains: "agent a"}
    steps: [{reply: {content: a1, tool_calls: [{name: Bash, arguments: {command: echo a}}]}}, {reply: {content: a2}}]
  b:
    bind: {system_contains: "agent b"}
    steps: [{reply: {content: b1, tool_calls: [{name: Bash, arguments: {command: echo b}}]}}, {reply: {content: b2}}]
`)
	var wg sync.WaitGroup
	got := map[string]string{}
	var mu sync.Mutex
	for _, n := range []string{"a", "b"} {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			c := newConv(t, url, "agent "+n, "Bash")
			first := c.say("go")
			second := c.results(first, "ok")
			mu.Lock()
			got[n] = first.text + second.text
			mu.Unlock()
		}(n)
	}
	wg.Wait()
	assert.Equal(t, map[string]string{"a": "a1a2", "b": "b1b2"}, got)
	assert.True(t, e.Finish().OK)
}

func TestSubagentBindsByTheCallThatSpawnsIt(t *testing.T) {
	e, url := serve(t, `
tracks:
  parent:
    bind: {class: [main]}
    steps:
      - reply: {tool_calls: [{name: Agent, arguments: {description: count, prompt: "Count the files."}, spawns: sub}]}
      - reply: {content: launched}
  sub:
    bind: {class: [subagent]}
    steps: [{reply: {content: "3 files"}}]
`)
	parent := newConv(t, url, "main loop", "Agent", "Bash")
	a := parent.say("count files")
	require.Len(t, a.calls, 1)
	sub := newConv(t, url, "You are an agent", "Bash")
	sub.system = []map[string]any{{"type": "text", "text": "x-anthropic-billing-header: cc_version=1; cc_is_subagent=true;"}}
	sub.session = parent.session // a subagent shares its parent's session id
	assert.Equal(t, "3 files", sub.say("Count the files.").text)
	assert.Equal(t, "launched", parent.results(a, "Async agent launched").text)
	assert.True(t, e.Finish().OK)
}

func TestARepeatedRequestGetsTheSameAnswer(t *testing.T) {
	e, url := serve(t, `
tracks:
  t:
    bind: {}
    steps: [{reply: {tool_calls: [{name: Bash, arguments: {command: ls}}]}}, {reply: {content: second}}]
`)
	c := newConv(t, url, "s", "Bash")
	a := c.say("hi")
	again := c.msgs[:1]
	b := c.results(a, "files")
	assert.Equal(t, "second", b.text)
	// The same first request again, and the second one again.
	retry := &conv{t: t, url: url, system: c.system, tools: c.tools, session: c.session, msgs: again}
	ra := retry.post()
	assert.Equal(t, a.calls[0]["id"], ra.calls[0]["id"])
	c.msgs = c.msgs[:len(c.msgs)-1]
	assert.Equal(t, "second", c.post().text)
	rep := e.Finish()
	assert.True(t, rep.OK, rep.String())
	assert.Len(t, rep.Warnings, 2)
}

func TestUnexpectedRequestsFail(t *testing.T) {
	e, url := serve(t, `
tracks:
  t:
    bind: {system_contains: coordinator}
    steps: [{reply: {content: only}}]
`)
	a := newConv(t, url, "a thread", "Bash").say("hi")
	assert.Equal(t, 400, a.status)
	assert.Contains(t, a.raw, "no track expects this main request")
	assert.Contains(t, a.raw, `system lacks \"coordinator\"`)
	c := newConv(t, url, "coordinator", "Bash")
	first := c.say("hi")
	assert.Equal(t, "only", first.text)
	assert.Equal(t, 400, c.say("more").status, "the track has no step left")
	rep := e.Finish()
	require.False(t, rep.OK)
	kinds := []string{}
	for _, f := range rep.Failures {
		kinds = append(kinds, f.Kind)
	}
	assert.ElementsMatch(t, []string{IssueUnmatched, IssueExhausted}, kinds)
}

func TestNonStrictAnswersAndWarns(t *testing.T) {
	e, url := serve(t, `
strict: false
tracks:
  t:
    bind: {system_contains: nobody}
    steps: [{optional: true, reply: {content: x}}]
`)
	a := newConv(t, url, "someone", "Bash").say("hi")
	assert.Equal(t, 200, a.status)
	assert.Equal(t, "ok", a.text)
	rep := e.Finish()
	assert.True(t, rep.OK, rep.String())
	require.Len(t, rep.Warnings, 1)
	assert.Equal(t, IssueUnmatched, rep.Warnings[0].Kind)
}

func TestFinishReportsWhatNeverHappened(t *testing.T) {
	e, url := serve(t, `
background:
  - {id: title, when: {class: [auxiliary]}, reply: {content: "{}"}, min: 1}
tracks:
  t:
    bind: {}
    steps:
      - reply: {tool_calls: [{id: ls, name: Bash, arguments: {command: ls}, expect_result: {contains: README}}]}
      - {id: never, reply: {content: x}}
      - {id: maybe, optional: true, reply: {content: y}}
`)
	newConv(t, url, "s", "Bash").say("hi")
	rep := e.Finish()
	var got []string
	for _, f := range rep.Failures {
		got = append(got, f.Kind+" "+f.Step)
	}
	assert.ElementsMatch(t, []string{"unconsumed never", "result_missed 0", "background "}, got)
	assert.Equal(t, 400, newConv(t, url, "late", "Bash").say("hi").status)
}

func TestExpectationsAndCaptures(t *testing.T) {
	e, url := serve(t, `
tracks:
  t:
    bind: {}
    steps:
      - expect: {last_user_contains: "please"}
        reply: {tool_calls: [{id: mk, name: start_thread, arguments: {title: T}, expect_result: {contains: started}}]}
      - capture: {id: {from: "tool_result:0.mk", regex: "thread ([0-9a-f]+)"}}
        reply: {content: "thread {{id}}, {{missing}}"}
`)
	c := newConv(t, url, "s", "start_thread")
	a := c.say("do it")
	b := c.results(a, "started thread 5a62")
	assert.Equal(t, "thread 5a62, {{missing}}", b.text)
	rep := e.Finish()
	var got []string
	for _, f := range rep.Failures {
		got = append(got, f.Kind)
	}
	assert.ElementsMatch(t, []string{IssueExpect, IssueTemplate}, got)
	assert.Contains(t, e.Trace()[1].Checks, Check{On: "expect_result 0.mk", OK: true})
}

func TestAfterHoldsTheReply(t *testing.T) {
	e, url := serve(t, `
hold_timeout: 2s
tracks:
  a:
    bind: {system_contains: "agent a"}
    steps: [{after: ["b.0", "signal:go"], reply: {content: a}}]
  b:
    bind: {system_contains: "agent b"}
    steps: [{reply: {content: b}}]
`)
	done := make(chan answer, 1)
	go func() { done <- newConv(t, url, "agent a", "Bash").say("hi") }()
	time.Sleep(100 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("a answered before b and the signal")
	default:
	}
	assert.Equal(t, "b", newConv(t, url, "agent b", "Bash").say("hi").text)
	e.Signal("go")
	assert.Equal(t, "a", (<-done).text)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, e.WaitServed(ctx, "a.0"))
	assert.True(t, e.Finish().OK)
}

func TestAfterTimesOut(t *testing.T) {
	e, url := serve(t, `
hold_timeout: 100ms
tracks:
  a:
    bind: {}
    steps: [{after: ["signal:never"], reply: {content: a}}]
`)
	a := newConv(t, url, "s", "Bash").say("hi")
	assert.Equal(t, 529, a.status)
	rep := e.Finish()
	require.Len(t, rep.Failures, 1)
	assert.Equal(t, IssueAfterTimeout, rep.Failures[0].Kind)
}

func TestIDsAreStableAcrossRuns(t *testing.T) {
	run := func() string {
		_, url := serve(t, `
tracks:
  t:
    bind: {}
    steps: [{reply: {tool_calls: [{name: Bash, arguments: {command: ls}}]}}]
`)
		c := newConv(t, url, "s", "Bash")
		c.stream = true
		return c.say("hi").calls[0]["id"].(string)
	}
	first := run()
	assert.Equal(t, first, run())
	assert.True(t, strings.HasPrefix(first, "toolu_"))
}

func TestValidate(t *testing.T) {
	for name, yaml := range map[string]string{
		"no tracks":       `tracks: {}`,
		"no bind":         `tracks: {t: {steps: [{reply: {content: x}}]}}`,
		"unknown spawn":   `tracks: {t: {bind: {}, steps: [{reply: {tool_calls: [{name: x, spawns: y}]}}]}}`,
		"bad after":       `tracks: {t: {bind: {}, steps: [{after: [nope], reply: {content: x}}]}}`,
		"empty reply":     `tracks: {t: {bind: {}, steps: [{reply: {}}]}}`,
		"bad class":       `tracks: {t: {bind: {class: [x]}, steps: [{reply: {content: x}}]}}`,
		"bad regex":       `tracks: {t: {bind: {system_regex: "("}, steps: [{reply: {content: x}}]}}`,
		"unknown field":   `tracks: {t: {bind: {}, steps: [{reply: {content: x}, nope: 1}]}}`,
		"chunks mismatch": `tracks: {t: {bind: {}, steps: [{reply: {content: ab, stream_chunks: [a]}}]}}`,
	} {
		_, err := Decode([]byte(yaml))
		assert.Error(t, err, name)
	}
}

func TestAuxiliaryEndpoints(t *testing.T) {
	_, url := serve(t, "count_tokens: 7\ntracks: {t: {bind: {}, steps: [{reply: {content: x}}]}}")
	resp, err := http.Post(url+"/v1/messages/count_tokens?beta=true", "application/json", strings.NewReader(`{"model":"m"}`))
	require.NoError(t, err)
	var n map[string]int
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&n))
	resp.Body.Close()
	assert.Equal(t, 7, n["input_tokens"])
	head, err := http.Head(url + "/api/hello")
	require.NoError(t, err)
	head.Body.Close()
	assert.Equal(t, 200, head.StatusCode)
}
