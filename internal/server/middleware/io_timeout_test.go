package middleware

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// sseHandler streams `events` SSE chunks, one every `interval`, ending with a
// terminal "completed" event. It mirrors the shape of the Responses API
// passthrough: flush after every write, terminal marker last.
func sseHandler(events int, interval time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		for i := 0; i < events; i++ {
			fmt.Fprintf(c.Writer, "event: delta\ndata: {\"i\":%d}\n\n", i)
			flusher.Flush()
			time.Sleep(interval)
		}
		fmt.Fprint(c.Writer, "event: completed\ndata: {}\n\n")
		flusher.Flush()
	}
}

// startServer runs a real http.Server (so Server.WriteTimeout arms real
// connection deadlines, unlike httptest.ResponseRecorder) and returns its
// base URL plus a shutdown func.
func startServer(t *testing.T, engine *gin.Engine, writeTimeout time.Duration) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler:      engine,
		ReadTimeout:  writeTimeout,
		WriteTimeout: writeTimeout,
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

// TestClearServerIOTimeouts_StreamOutlivesWriteTimeout is the regression test
// for issue #1384: an SSE response that runs longer than http.Server's
// WriteTimeout must still reach its terminal event on routes wrapped with
// ClearServerIOTimeouts. The control route without the middleware proves the
// truncation mechanism is real (connection killed before "completed").
func TestClearServerIOTimeouts_StreamOutlivesWriteTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	const events = 6
	const interval = 150 * time.Millisecond // total ~900ms vs 300ms WriteTimeout
	engine.GET("/guarded", ClearServerIOTimeouts(), sseHandler(events, interval))
	engine.GET("/bare", sseHandler(events, interval))

	base := startServer(t, engine, 300*time.Millisecond)
	client := &http.Client{Timeout: 10 * time.Second}

	read := func(path string) (string, error) {
		resp, err := client.Get(base + path)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		return string(body), err
	}

	// Without the middleware the write deadline (armed at request start,
	// never extended) kills the connection mid-stream: EOF before the
	// terminal event — exactly what Codex reports as "stream closed before
	// response.completed".
	bareBody, bareErr := read("/bare")
	if bareErr == nil && strings.Contains(bareBody, "event: completed") {
		t.Fatal("control route unexpectedly survived WriteTimeout; test setup no longer exercises the deadline")
	}

	// With the middleware the stream must complete.
	body, err := read("/guarded")
	if err != nil {
		t.Fatalf("guarded stream failed: %v", err)
	}
	for i := 0; i < events; i++ {
		if want := fmt.Sprintf("{\"i\":%d}", i); !strings.Contains(body, want) {
			t.Errorf("guarded stream missing chunk %s", want)
		}
	}
	if !strings.Contains(body, "event: completed") {
		t.Error("guarded stream missing terminal completed event")
	}
}
