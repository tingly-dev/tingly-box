package builtinserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startSerperStub points serperAPIEndpoint at a stub server for the test and
// shrinks the retry backoff so retry paths run fast.
func startSerperStub(t *testing.T, handler http.HandlerFunc) *int32 {
	t.Helper()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	origEndpoint := serperAPIEndpoint
	origWait := webSearchRetryBaseWait
	serperAPIEndpoint = srv.URL
	webSearchRetryBaseWait = time.Millisecond
	t.Cleanup(func() {
		serperAPIEndpoint = origEndpoint
		webSearchRetryBaseWait = origWait
	})

	t.Setenv("SERPER_API_KEY", "test-key")
	return &calls
}

func searchArgs(query string) map[string]interface{} {
	return map[string]interface{}{"query": query}
}

const validEmptyBody = `{"searchParameters":{"q":"test"},"organic":[]}`
const validResultBody = `{"searchParameters":{"q":"test"},"organic":[{"title":"Go","link":"https://go.dev","snippet":"The Go programming language"}]}`

func TestWebSearchSuccess(t *testing.T) {
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(validResultBody))
	})

	out, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "go.dev") {
		t.Fatalf("expected result link in output, got: %q", out)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected 1 call, got %d", got)
	}
}

func TestWebSearchRetriesEmptyThenSucceeds(t *testing.T) {
	var n int32
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) < 3 {
			_, _ = w.Write([]byte(validEmptyBody))
			return
		}
		_, _ = w.Write([]byte(validResultBody))
	})

	out, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "go.dev") {
		t.Fatalf("expected result link in output, got: %q", out)
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

func TestWebSearchEmptyAfterAllRetries(t *testing.T) {
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(validEmptyBody))
	})

	out, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No results found") {
		t.Fatalf("expected explicit no-results marker, got: %q", out)
	}
	if got := atomic.LoadInt32(calls); got != int32(webSearchMaxAttempts) {
		t.Fatalf("expected %d calls, got %d", webSearchMaxAttempts, got)
	}
}

func TestWebSearchMalformed200IsError(t *testing.T) {
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	})

	_, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err == nil {
		t.Fatal("expected error for 200 body without results, got nil")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected backend message in error, got: %v", err)
	}
	if got := atomic.LoadInt32(calls); got != int32(webSearchMaxAttempts) {
		t.Fatalf("expected %d calls, got %d", webSearchMaxAttempts, got)
	}
}

func TestWebSearchMissingSearchParametersIsError(t *testing.T) {
	startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	})

	_, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err == nil {
		t.Fatal("expected error for 200 body missing searchParameters, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed-response error, got: %v", err)
	}
}

func TestWebSearchRetriesServerError(t *testing.T) {
	var n int32
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(validResultBody))
	})

	out, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "go.dev") {
		t.Fatalf("expected result link in output, got: %q", out)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestWebSearchDoesNotRetryClientError(t *testing.T) {
	calls := startSerperStub(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	})

	_, err := webSearchImpl(context.Background(), searchArgs("golang"))
	if err == nil {
		t.Fatal("expected error for 403, got nil")
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected 1 call (no retry on 4xx), got %d", got)
	}
}
