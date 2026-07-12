package protocoltest

// Duo is the two-instance end-to-end verification environment born from the
// #1255 OOM investigation:
//
//	client ──(Anthropic v1/beta, streaming, large conversation)──▶ tb2 gateway
//	   tb2 ──(converted provider request over real HTTP)──────────▶ tb1 /virtual/...
//	   tb1 ──(vmodel SSE stream)───────────────────────────────────▶ tb2 ──▶ client
//
// tb1 and tb2 are SEPARATE PROCESSES, each a full production server booted
// through server.Start (background refreshers, config watcher, production
// http.Server timeouts) — the parent re-executes its own binary via the
// TINGLY_DUO_* env contract in duo_serve.go. Because each instance has its
// own Go runtime, memory is observed PER INSTANCE over the production
// /api/v1/debug/{memstats,pprof/heap} endpoints: a leak attributes directly
// to tb2 (gateway/conversion path) or tb1 (vmodel serving path) instead of
// disappearing into a shared heap.
//
// Every anthropic-source conversion route the production vmodel endpoint can
// back is wired (see AllDuoRoutes): {v1, beta} × {anthropic passthrough,
// OpenAI Chat, OpenAI Responses}. Each route also has a "-slow" backpressure
// variant where tb1 streams a large response slowly (see DuoSlowOpenAIModel)
// and the client can read slowly (DuoMemoryConfig.ReadDelay), exercising the
// buffering/pinning behaviour that instant mock responses hide. The Google
// target is not covered — the vmodel surface deliberately skips it for now.
//
// Two verification phases are provided:
//
//   - RunFunctionalChecks: protocol correctness through a conversion route
//     (streaming SSE shape + assembled content + usage; non-streaming body).
//   - RunMemoryPhase: per-instance allocation churn, post-GC retention slope
//     across request batches (a leak shows up as a positive slope; transient
//     spikes do not), concurrent-burst peak heap, goroutine counts, and
//     optional pprof heap profiles fetched from each instance.
//
// Consumed by `harness duo` (cli/harness) and by duo_test.go as functional +
// memory regression tests.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	debugmodule "github.com/tingly-dev/tingly-box/internal/server/module/debug"
)

// DuoRoute is one anthropic-source conversion route through tb2.
type DuoRoute struct {
	// Name identifies the route in flags, check names, and reports,
	// e.g. "beta-chat", "v1-responses", "beta-chat-slow".
	Name string
	// Beta selects the Anthropic beta source surface (?beta=true) over v1.
	Beta bool
	// Target is the provider protocol tb2 converts to: "chat", "responses",
	// or "anthropic" (passthrough).
	Target string
	// Slow selects the backpressure variant: tb1 answers with the slow/large
	// duo stream vmodel instead of the tiny instant builtin.
	Slow bool
}

// RequestModel returns the tb2 request model wired for this route.
func (r DuoRoute) RequestModel() string { return "duo-e2e-" + r.Name }

// SlowVariant returns the backpressure variant of the route.
func (r DuoRoute) SlowVariant() DuoRoute {
	if r.Slow {
		return r
	}
	return DuoRoute{Name: r.Name + "-slow", Beta: r.Beta, Target: r.Target, Slow: true}
}

// AllDuoRoutes lists every fast anthropic-source route the production vmodel
// endpoint can back: {v1, beta} × {anthropic, openai chat, openai responses}.
func AllDuoRoutes() []DuoRoute {
	var routes []DuoRoute
	for _, src := range []struct {
		prefix string
		beta   bool
	}{{"beta", true}, {"v1", false}} {
		for _, target := range []string{"chat", "responses", "anthropic"} {
			routes = append(routes, DuoRoute{Name: src.prefix + "-" + target, Beta: src.beta, Target: target})
		}
	}
	return routes
}

// allDuoRoutesWithSlow returns the fast routes plus their slow variants —
// the full rule set seeded on tb2.
func allDuoRoutesWithSlow() []DuoRoute {
	fast := AllDuoRoutes()
	all := make([]DuoRoute, 0, 2*len(fast))
	all = append(all, fast...)
	for _, r := range fast {
		all = append(all, r.SlowVariant())
	}
	return all
}

// FindDuoRoute resolves a route by name, including "-slow" variants.
func FindDuoRoute(name string) (DuoRoute, bool) {
	for _, r := range allDuoRoutesWithSlow() {
		if r.Name == name {
			return r, true
		}
	}
	return DuoRoute{}, false
}

// DuoDefaultRoute is the memory-phase default: the Claude Code hot path
// (Anthropic beta client → OpenAI Chat provider) where #1255 was reported.
var DuoDefaultRoute = DuoRoute{Name: "beta-chat", Beta: true, Target: "chat"}

// duoTargetVModel maps a route to the tb1 vmodel that serves it.
func duoTargetVModel(route DuoRoute) string {
	switch {
	case route.Slow && route.Target == "anthropic":
		return DuoSlowAnthropicModel
	case route.Slow:
		return DuoSlowOpenAIModel
	case route.Target == "anthropic":
		return "virtual-claude-3" // anthropic registry
	default:
		return "virtual-gpt-4" // openai registry (chat + responses surfaces)
	}
}

// ─── Instances ────────────────────────────────────────────────────────────────

// DuoInstance is one child tingly-box process.
type DuoInstance struct {
	Name       string
	ConfigDir  string
	Port       int
	BaseURL    string
	UserToken  string
	ModelToken string

	cmd     *exec.Cmd
	out     *tailBuffer
	done    chan struct{}
	exitErr error
	hc      *http.Client
}

// OutputTail returns the last captured child stdout/stderr output.
func (inst *DuoInstance) OutputTail() string { return inst.out.String() }

// exited reports whether the child process has terminated.
func (inst *DuoInstance) exited() bool {
	select {
	case <-inst.done:
		return true
	default:
		return false
	}
}

// MemStats fetches a runtime memory snapshot from the instance's debug
// endpoint. With gc=true the instance forces a full GC first, so
// HeapAllocBytes is its post-GC retained set; the endpoint throttles forced
// GCs, so a throttled sample is retried until the GC actually ran.
func (inst *DuoInstance) MemStats(gc bool) (*debugmodule.MemStatsResponse, error) {
	const gcRetries = 3
	for attempt := 0; ; attempt++ {
		m, err := inst.memStatsOnce(gc)
		if err != nil {
			return nil, err
		}
		if !gc || m.GCForced || attempt >= gcRetries {
			return m, nil
		}
		time.Sleep(1100 * time.Millisecond) // just past the endpoint's forced-GC throttle window
	}
}

func (inst *DuoInstance) memStatsOnce(gc bool) (*debugmodule.MemStatsResponse, error) {
	url := inst.BaseURL + "/api/v1/debug/memstats"
	if gc {
		url += "?gc=true"
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+inst.UserToken)
	resp, err := inst.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s memstats: %w", inst.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("%s memstats: status %d: %s", inst.Name, resp.StatusCode, b)
	}
	var m debugmodule.MemStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("%s memstats: %w", inst.Name, err)
	}
	return &m, nil
}

// WriteHeapProfile fetches a post-GC pprof heap profile from the instance
// and writes it under dir, returning the file path. The endpoint throttles
// profile serialization, so a 429 is retried past the throttle window.
func (inst *DuoInstance) WriteHeapProfile(dir, name string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, inst.BaseURL+"/api/v1/debug/pprof/heap?gc=true", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+inst.UserToken)
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		resp, err = inst.hc.Do(req)
		if err != nil {
			return "", fmt.Errorf("%s heap profile: %w", inst.Name, err)
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt >= 3 {
			break
		}
		resp.Body.Close()
		time.Sleep(1100 * time.Millisecond) // just past the endpoint's profile throttle window
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("%s heap profile: status %d: %s", inst.Name, resp.StatusCode, b)
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

// ─── Environment ──────────────────────────────────────────────────────────────

// DuoEnvConfig parameterizes NewDuoEnv.
type DuoEnvConfig struct {
	// StreamKB / StreamMS shape tb1's slow backpressure vmodels: an
	// approximately StreamKB-sized response streamed over roughly 2×StreamMS
	// wall time. Defaults: 256 KB over ~1 s.
	StreamKB int
	StreamMS int
	// ChildLog, when non-nil, receives both children's stdout/stderr live
	// (in addition to the per-instance tail buffer used for diagnostics).
	ChildLog io.Writer
	// BootTimeout caps how long each instance may take to become healthy
	// (default 90s — first boot may attempt a provider-template fetch).
	BootTimeout time.Duration
}

func (c *DuoEnvConfig) withDefaults() {
	if c.StreamKB <= 0 {
		c.StreamKB = 256
	}
	if c.StreamMS <= 0 {
		c.StreamMS = 500
	}
	if c.BootTimeout <= 0 {
		c.BootTimeout = 90 * time.Second
	}
}

// DuoEnv holds the two running child instances and the wiring between them.
type DuoEnv struct {
	TB1 *DuoInstance // upstream: serves /virtual vmodel endpoints
	TB2 *DuoInstance // gateway under test: converts + proxies to tb1

	client *http.Client
	dirs   []string
}

// NewDuoEnv boots tb1 (vmodel upstream) and tb2 (gateway under test) as two
// full server processes and wires one tb2 rule per route in
// allDuoRoutesWithSlow to tb1's virtual endpoints. Callers must Close() the
// returned env.
func NewDuoEnv(cfg DuoEnvConfig) (*DuoEnv, error) {
	cfg.withDefaults()
	env := &DuoEnv{client: &http.Client{Timeout: 120 * time.Second}}

	tb1, err := env.startInstance("tb1", cfg, map[string]string{
		duoEnvStreamKB: strconv.Itoa(cfg.StreamKB),
		duoEnvStreamMS: strconv.Itoa(cfg.StreamMS),
	})
	if err != nil {
		env.Close()
		return nil, fmt.Errorf("boot tb1: %w", err)
	}
	env.TB1 = tb1

	tb2, err := env.startInstance("tb2", cfg, map[string]string{
		duoEnvUpstreamURL:   tb1.BaseURL,
		duoEnvUpstreamToken: tb1.ModelToken,
	})
	if err != nil {
		env.Close()
		return nil, fmt.Errorf("boot tb2: %w", err)
	}
	env.TB2 = tb2
	return env, nil
}

// startInstance spawns one child instance (re-executing this binary under
// the duo env contract), waits for it to become healthy, and reads its
// tokens from the child's config.json.
func (env *DuoEnv) startInstance(name string, cfg DuoEnvConfig, extra map[string]string) (*DuoInstance, error) {
	dir, err := os.MkdirTemp("", "duo-"+name+"-*")
	if err != nil {
		return nil, err
	}
	env.dirs = append(env.dirs, dir)

	port, err := pickFreePort()
	if err != nil {
		return nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}

	inst := &DuoInstance{
		Name:      name,
		ConfigDir: dir,
		Port:      port,
		BaseURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
		out:       newTailBuffer(64 * 1024),
		done:      make(chan struct{}),
		hc:        &http.Client{Timeout: 60 * time.Second},
	}

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		duoEnvRole+"="+duoRoleServe,
		duoEnvName+"="+name,
		duoEnvConfigDir+"="+dir,
		duoEnvPort+"="+strconv.Itoa(port),
	)
	for k, v := range extra {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var sink io.Writer = inst.out
	if cfg.ChildLog != nil {
		sink = io.MultiWriter(inst.out, cfg.ChildLog)
	}
	cmd.Stdout = sink
	cmd.Stderr = sink
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn %s: %w", name, err)
	}
	inst.cmd = cmd
	go func() {
		inst.exitErr = cmd.Wait()
		close(inst.done)
	}()

	if err := inst.waitReady(cfg.BootTimeout); err != nil {
		return inst, fmt.Errorf("%s not ready: %w\n--- %s output tail ---\n%s", name, err, name, inst.OutputTail())
	}
	if err := inst.readTokens(); err != nil {
		return inst, fmt.Errorf("%s tokens: %w", name, err)
	}
	return inst, nil
}

// waitReady polls the unauthenticated health endpoint until the instance
// answers 200 or the deadline passes.
func (inst *DuoInstance) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	url := inst.BaseURL + "/api/v1/info/health"
	for time.Now().Before(deadline) {
		if inst.exited() {
			return fmt.Errorf("process exited during boot: %v", inst.exitErr)
		}
		resp, err := inst.hc.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("health check timed out after %s", timeout)
}

// readTokens reads the child-generated user/model tokens from its
// config.json (written before the server starts listening).
func (inst *DuoInstance) readTokens() error {
	raw, err := os.ReadFile(filepath.Join(inst.ConfigDir, "config.json"))
	if err != nil {
		return err
	}
	var c struct {
		UserToken  string `json:"user_token"`
		ModelToken string `json:"model_token"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	if c.UserToken == "" || c.ModelToken == "" {
		return fmt.Errorf("config.json missing tokens")
	}
	inst.UserToken = c.UserToken
	inst.ModelToken = c.ModelToken
	return nil
}

// Close terminates both instances and removes their config dirs.
func (env *DuoEnv) Close() {
	for _, inst := range []*DuoInstance{env.TB1, env.TB2} {
		if inst == nil || inst.cmd == nil || inst.cmd.Process == nil {
			continue
		}
		if !inst.exited() {
			_ = inst.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-inst.done:
			case <-time.After(5 * time.Second):
				_ = inst.cmd.Process.Kill()
				select {
				case <-inst.done:
				case <-time.After(5 * time.Second):
				}
			}
		}
	}
	for _, d := range env.dirs {
		os.RemoveAll(d)
	}
}

// pickFreePort reserves an ephemeral localhost port and releases it for the
// child to bind. (Small race window; boot failure surfaces via waitReady.)
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// tailBuffer is a concurrency-safe writer that keeps the last max bytes.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newTailBuffer(max int) *tailBuffer { return &tailBuffer{max: max} }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// ─── Request driving ─────────────────────────────────────────────────────────

// BuildConversationBody builds a Claude-Code-shaped Anthropic request of
// approximately totalBytes for the given route: alternating user/assistant
// text messages, so the gateway parses and converts a realistically large
// agentic context. The shape is valid for both the v1 and beta surfaces.
func BuildConversationBody(route DuoRoute, totalBytes int, streaming bool) []byte {
	const msgBytes = 40 * 1024
	msgs := totalBytes / msgBytes
	if msgs < 1 {
		msgs = 1
	}
	filler := strings.Repeat("The quick brown fox jumps over the lazy dog. ", msgBytes/45+1)[:msgBytes]
	fb, _ := json.Marshal(filler)

	var sb strings.Builder
	fmt.Fprintf(&sb, `{"model":%q,"max_tokens":1024,"stream":%v,"messages":[`, route.RequestModel(), streaming)
	for i := 0; i < msgs; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		fmt.Fprintf(&sb, `{"role":%q,"content":[{"type":"text","text":%s}]}`, role, string(fb))
	}
	sb.WriteString(`]}`)
	return []byte(sb.String())
}

// post sends one request through tb2's anthropic endpoint for the route's
// source surface (v1 or beta).
func (env *DuoEnv) post(route DuoRoute, body []byte) (*http.Response, error) {
	path := "/tingly/anthropic/v1/messages"
	if route.Beta {
		path += "?beta=true"
	}
	req, err := http.NewRequest(http.MethodPost, env.TB2.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.TB2.ModelToken)
	return env.client.Do(req)
}

// slowReader throttles SSE consumption to simulate a slow client: each Read
// is capped to a small window and followed by a pause, so TCP backpressure
// builds up against the gateway the way a slow real consumer causes it to.
type slowReader struct {
	r     io.Reader
	delay time.Duration
}

func (s *slowReader) Read(p []byte) (int, error) {
	const window = 8 * 1024
	if len(p) > window {
		p = p[:window]
	}
	n, err := s.r.Read(p)
	if n > 0 && s.delay > 0 {
		time.Sleep(s.delay)
	}
	return n, err
}

// DrainStreaming drives one streaming request over the route and fully
// drains the SSE body, returning the number of `event:` lines seen.
// A non-zero readDelay reads the body slowly (see slowReader).
func (env *DuoEnv) DrainStreaming(route DuoRoute, body []byte, readDelay time.Duration) (int, error) {
	resp, err := env.post(route, body)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	var r io.Reader = resp.Body
	if readDelay > 0 {
		r = &slowReader{r: resp.Body, delay: readDelay}
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	events := 0
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event:") {
			events++
		}
	}
	if events == 0 {
		return 0, fmt.Errorf("no SSE events received")
	}
	return events, sc.Err()
}
