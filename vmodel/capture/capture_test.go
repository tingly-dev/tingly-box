package capture

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// start serves h behind a tap on a loopback listener.
func start(t *testing.T, opts Options, h http.Handler) (*Tap, string, func()) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cap")
	tap, err := New(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: tap.Handler(h)}
	tap.Install(srv)
	go func() { _ = srv.Serve(tap.Listener(ln)) }()
	return tap, ln.Addr().String(), func() { _ = srv.Close(); _ = tap.Close() }
}

func readIndex(t *testing.T, dir string) []entry {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []entry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("bad index line %q: %v", sc.Text(), err)
		}
		out = append(out, e)
	}
	return out
}

func waitIndex(t *testing.T, dir string, n int) []entry {
	t.Helper()
	var got []entry
	for i := 0; i < 200; i++ {
		got = readIndex(t, dir)
		if len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("index has %d entries, want %d", len(got), n)
	return nil
}

func rawFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "raw", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// readResponse consumes one full HTTP response from br.
func readResponse(t *testing.T, br *bufio.Reader) *http.Response {
	t.Helper()
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

// The capture keeps header-name casing, header order and duplicate headers,
// which net/http would canonicalize/merge, and locates two keep-alive requests
// exactly in the raw file.
func TestRawBytesAndKeepAliveRanges(t *testing.T) {
	tap, addr, stop := start(t, Options{Out: true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte("ok"))
	}))
	defer stop()

	req1 := "POST /v1/messages?beta=true HTTP/1.1\r\nHost: x\r\nanthropic-beta: b,a\r\nx-app: cli\r\nX-DUP: 1\r\nx-dup: 2\r\nAuthorization: Bearer sk-fake\r\nContent-Length: 11\r\n\r\n{\"a\":\"<\"}\n\n"
	req2 := "GET /healthz HTTP/1.1\r\nHost: x\r\n\r\n"
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	if _, err := c.Write([]byte(req1)); err != nil {
		t.Fatal(err)
	}
	readResponse(t, br)
	if _, err := c.Write([]byte(req2)); err != nil {
		t.Fatal(err)
	}
	readResponse(t, br)
	c.Close()

	idx := waitIndex(t, tap.Dir(), 2)
	in := rawFile(t, tap.Dir(), "conn-0001.in")
	if string(in) != req1+req2 {
		t.Fatalf("raw input differs:\n%q", in)
	}
	if idx[0].Seq != 1 || idx[1].Seq != 2 || idx[0].Conn != 1 {
		t.Fatalf("seq/conn: %+v", idx)
	}
	if got := string(in[idx[0].In[0]:idx[0].In[1]]); got != req1 {
		t.Fatalf("first range = %q", got)
	}
	if got := string(in[idx[1].In[0]:idx[1].In[1]]); got != req2 {
		t.Fatalf("second range = %q", got)
	}
	if idx[0].Method != "POST" || idx[0].Target != "/v1/messages?beta=true" || idx[0].Status != 200 {
		t.Fatalf("entry: %+v", idx[0])
	}
	out := rawFile(t, tap.Dir(), "conn-0001.out")
	first := string(out[idx[0].Out[0]:idx[0].Out[1]])
	if !strings.HasPrefix(first, "HTTP/1.1 200 OK") || !strings.HasSuffix(first, "ok") {
		t.Fatalf("first out range = %q", first)
	}
	if idx[1].Out[0] != idx[0].Out[1] {
		t.Fatalf("out ranges not contiguous: %+v %+v", idx[0].Out, idx[1].Out)
	}
}

// A chunked body is stored exactly as framed on the wire.
func TestChunkedBodyVerbatim(t *testing.T) {
	tap, addr, stop := start(t, Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer stop()
	req := "POST /x HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nabc\r\n2;ext=1\r\nde\r\n0\r\n\r\n"
	c, _ := net.Dial("tcp", addr)
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	readResponse(t, bufio.NewReader(c))
	c.Close()
	idx := waitIndex(t, tap.Dir(), 1)
	in := rawFile(t, tap.Dir(), "conn-0001.in")
	if string(in) != req || string(in[idx[0].In[0]:idx[0].In[1]]) != req {
		t.Fatalf("chunked body not verbatim: %q %+v", in, idx[0])
	}
	if idx[0].Out != nil {
		t.Fatal("out range present although Out is off")
	}
	if _, err := os.Stat(filepath.Join(tap.Dir(), "raw", "conn-0001.out")); err == nil {
		t.Fatal("conn-N.out written although Out is off")
	}
}

// Requests the HTTP server rejects before any handler runs are still in the
// raw file and are pointed at by an unparsed index entry.
func TestUnparsedBytesAreIndexed(t *testing.T) {
	called := false
	tap, addr, stop := start(t, Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer stop()
	garbage := "NOT AN HTTP REQUEST\r\n\r\n"
	c, _ := net.Dial("tcp", addr)
	_, _ = c.Write([]byte(garbage))
	_, _ = io.Copy(io.Discard, c) // server answers 400 and closes
	c.Close()
	idx := waitIndex(t, tap.Dir(), 1)
	if called || !idx[0].Unparsed || idx[0].Seq != 0 {
		t.Fatalf("called=%v entry=%+v", called, idx[0])
	}
	if got := rawFile(t, tap.Dir(), "conn-0001.in"); string(got) != garbage || idx[0].In != [2]int64{0, int64(len(garbage))} {
		t.Fatalf("raw=%q entry=%+v", got, idx[0])
	}
}

// A client that hangs up before the response is complete leaves an
// incomplete entry rather than nothing.
func TestAbortedRequestIsIncomplete(t *testing.T) {
	release := make(chan struct{})
	tap, addr, stop := start(t, Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer stop()
	c, _ := net.Dial("tcp", addr)
	_, _ = c.Write([]byte("GET /slow HTTP/1.1\r\nHost: x\r\n\r\n"))
	time.Sleep(50 * time.Millisecond)
	c.Close()
	close(release)
	idx := waitIndex(t, tap.Dir(), 1)
	if idx[0].Target != "/slow" || idx[0].Seq != 1 {
		t.Fatalf("entry: %+v", idx[0])
	}
}

// Concurrent connections get distinct seq numbers and their own raw files.
func TestConcurrentConnections(t *testing.T) {
	tap, addr, stop := start(t, Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer stop()
	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			c, _ := net.Dial("tcp", addr)
			_, _ = c.Write([]byte("GET /c HTTP/1.1\r\nHost: x\r\n\r\n"))
			readResponse(t, bufio.NewReader(c))
			c.Close()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 5; i++ {
		<-done
	}
	idx := waitIndex(t, tap.Dir(), 5)
	seen := map[int64]bool{}
	conns := map[int64]bool{}
	for _, e := range idx {
		seen[e.Seq], conns[e.Conn] = true, true
	}
	if len(seen) != 5 || len(conns) != 5 {
		t.Fatalf("seq=%v conns=%v", seen, conns)
	}
}

func TestStreamingStillFlushes(t *testing.T) {
	_, addr, stop := start(t, Options{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("data: 1\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer stop()
	c, _ := net.Dial("tcp", addr)
	defer c.Close()
	_, _ = c.Write([]byte("GET /s HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "data: 1\n\n" {
		t.Fatalf("body %q", b)
	}
}

func TestRequestIDOptIn(t *testing.T) {
	for _, on := range []bool{false, true} {
		_, addr, stop := start(t, Options{RequestID: on}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("x"))
		}))
		c, _ := net.Dial("tcp", addr)
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			t.Fatal(err)
		}
		got := resp.Header.Get("request-id")
		if on && got != "req_vmodel_0001" || !on && got != "" {
			t.Fatalf("on=%v request-id=%q", on, got)
		}
		c.Close()
		stop()
	}
}

func TestNewRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir, Options{}); err == nil {
		t.Fatal("expected error for non-empty directory")
	}
	if _, err := New(t.TempDir(), Options{}); err != nil {
		t.Fatalf("empty dir should be accepted: %v", err)
	}
}

func TestWarnsOnceForNonFakeCredential(t *testing.T) {
	var warnings []string
	_, addr, stop := start(t, Options{Warn: func(s string) { warnings = append(warnings, s) }}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer stop()
	for _, auth := range []string{"Bearer sk-ant-oat01-fake", "Bearer sk-ant-oat01-REAL", "Bearer sk-ant-oat01-REAL2"} {
		c, _ := net.Dial("tcp", addr)
		_, _ = c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\nAuthorization: " + auth + "\r\n\r\n"))
		readResponse(t, bufio.NewReader(c))
		c.Close()
	}
	if len(warnings) != 1 || strings.Contains(warnings[0], "REAL") {
		t.Fatalf("warnings: %q", warnings)
	}
}
