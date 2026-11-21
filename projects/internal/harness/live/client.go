package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// api talks JSON to a server.
type api struct {
	base string
	http *http.Client
}

func newAPI(base string) *api { return &api{base: strings.TrimRight(base, "/"), http: &http.Client{}} }

func (a *api) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s %s: %w", method, path, err)
		}
	}
	return resp.StatusCode, nil
}

// The parts of Projects' API responses the runner reads.
type project struct {
	ID           string `json:"id"`
	MainThreadID string `json:"main_thread_id"`
}

type thread struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Branch       string `json:"branch"`
	CommitsAhead int    `json:"commits_ahead"`
	WaitReason   string `json:"wait_reason"`
}

type event struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
	Payload string `json:"payload"`
}

type projectDetail struct {
	Threads         []thread `json:"threads"`
	CoordinatorBusy bool     `json:"coordinator_busy"`
}

// projects is the user's side of a Projects server.
type projects struct {
	*api
	project project
}

func (p *projects) detail(ctx context.Context) (projectDetail, error) {
	var d projectDetail
	_, err := p.do(ctx, http.MethodGet, "/api/v1/projects/"+p.project.ID, nil, &d)
	return d, err
}

func (p *projects) threads(ctx context.Context) ([]thread, error) {
	var out struct {
		Threads []thread `json:"threads"`
	}
	_, err := p.do(ctx, http.MethodGet, "/api/v1/projects/"+p.project.ID+"/threads", nil, &out)
	return out.Threads, err
}

func (p *projects) events(ctx context.Context, threadID string) ([]event, error) {
	var out struct {
		Events []event `json:"events"`
	}
	_, err := p.do(ctx, http.MethodGet, "/api/v1/threads/"+threadID+"/events", nil, &out)
	return out.Events, err
}

// poll calls f until it reports done, f fails or ctx ends; last describes
// what was seen, for the timeout message.
func poll(ctx context.Context, what string, f func() (done bool, last string, err error)) error {
	last := ""
	for {
		done, seen, err := f()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if seen != "" {
			last = seen
		}
		select {
		case <-ctx.Done():
			if last != "" {
				return fmt.Errorf("timed out waiting for %s (last seen: %s)", what, last)
			}
			return fmt.Errorf("timed out waiting for %s", what)
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// thread waits until a thread with this title exists.
func (p *projects) thread(ctx context.Context, title string) (thread, error) {
	var found thread
	err := poll(ctx, "thread "+quote(title), func() (bool, string, error) {
		ths, err := p.threads(ctx)
		if err != nil {
			return false, "", err
		}
		var titles []string
		for _, t := range ths {
			if t.Title == title {
				found = t
				return true, "", nil
			}
			titles = append(titles, quote(t.Title))
		}
		return false, "threads " + strings.Join(titles, ", "), nil
	})
	return found, err
}

func (p *projects) waitStatus(ctx context.Context, title, status string) error {
	t, err := p.thread(ctx, title)
	if err != nil {
		return err
	}
	return poll(ctx, quote(title)+" to be "+status, func() (bool, string, error) {
		var d struct {
			Thread thread `json:"thread"`
		}
		if _, err := p.do(ctx, http.MethodGet, "/api/v1/threads/"+t.ID, nil, &d); err != nil {
			return false, "", err
		}
		return d.Thread.Status == status, "status " + d.Thread.Status, nil
	})
}

// answer waits for the thread's next approval card and answers it.
func (p *projects) answer(ctx context.Context, title string, allow bool, scope string) (string, error) {
	t, err := p.thread(ctx, title)
	if err != nil {
		return "", err
	}
	if scope == "" {
		scope = "once"
	}
	var card event
	err = poll(ctx, "an approval card in "+quote(title), func() (bool, string, error) {
		var out struct {
			Requests []event `json:"requests"`
		}
		if _, err := p.do(ctx, http.MethodGet, "/api/v1/threads/"+t.ID+"/requests", nil, &out); err != nil {
			return false, "", err
		}
		for _, r := range out.Requests {
			if r.Kind == "approval" {
				card = r
				return true, "", nil
			}
		}
		return false, fmt.Sprintf("%d open requests", len(out.Requests)), nil
	})
	if err != nil {
		return "", err
	}
	var payload struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(card.Payload), &payload); err != nil || payload.RequestID == "" {
		return "", fmt.Errorf("approval card without request_id: %s", card.Payload)
	}
	_, err = p.do(ctx, http.MethodPost, "/api/v1/threads/"+t.ID+"/respond", map[string]any{"request_id": payload.RequestID, "allow": allow, "scope": scope}, nil)
	return card.Content, err
}

// settle waits until the coordinator and every thread are idle.
func (p *projects) settle(ctx context.Context) error {
	return poll(ctx, "the project to settle", func() (bool, string, error) {
		d, err := p.detail(ctx)
		if err != nil {
			return false, "", err
		}
		var busy []string
		if d.CoordinatorBusy {
			busy = append(busy, "coordinator")
		}
		for _, t := range d.Threads {
			if t.Status == "working" {
				busy = append(busy, quote(t.Title))
			}
		}
		return len(busy) == 0, "busy: " + strings.Join(busy, ", "), nil
	})
}

// model is the driver's side of the vmodel script.
type model struct {
	*api
	id string
}

func (m *model) served(ctx context.Context, step string, timeout time.Duration) error {
	q := url.Values{"step": {step}, "timeout": {timeout.String()}}
	_, err := m.do(ctx, http.MethodGet, "/_vmodel/scripts/"+url.PathEscape(m.id)+"/served?"+q.Encode(), nil, nil)
	return err
}

func (m *model) signal(ctx context.Context, name string) error {
	_, err := m.do(ctx, http.MethodPost, "/_vmodel/scripts/"+url.PathEscape(m.id)+"/signals/"+url.PathEscape(name), nil, nil)
	return err
}

// ScriptReport is the part of the vmodel script report the runner shows.
type ScriptReport struct {
	OK       bool `json:"ok"`
	Requests int  `json:"requests"`
	Failures []struct {
		Kind, Track, Step, Detail string
		Seq                       int
	} `json:"failures"`
	Warnings []struct {
		Kind, Track, Step, Detail string
		Seq                       int
	} `json:"warnings"`
	Tracks []struct {
		Name      string   `json:"name"`
		Bound     bool     `json:"bound"`
		Served    []string `json:"served"`
		Remaining []string `json:"remaining"`
	} `json:"tracks"`
}

func (m *model) finish(ctx context.Context) (*ScriptReport, error) {
	var r ScriptReport
	_, err := m.do(ctx, http.MethodPost, "/_vmodel/scripts/"+url.PathEscape(m.id)+"/finish", nil, &r)
	return &r, err
}
