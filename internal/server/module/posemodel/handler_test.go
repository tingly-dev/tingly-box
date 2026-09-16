package posemodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// A fetcher that serves canned bodies by URL, so the tests never touch the
// network and can hand back exactly the bytes — or the wrong bytes.
func cannedFetcher(bodies map[string][]byte) Fetcher {
	return func(_ context.Context, url string, dst io.Writer) error {
		body, ok := bodies[url]
		if !ok {
			return errors.New("no such url")
		}
		_, err := dst.Write(body)
		return err
	}
}

// withPinnedArtifacts swaps the real manifest for a tiny one for the test.
func withPinnedArtifacts(t *testing.T, bodies map[string][]byte) map[string][]byte {
	t.Helper()
	saved := Artifacts
	t.Cleanup(func() { Artifacts = saved })
	Artifacts = nil
	for name, body := range bodies {
		sum := sha256.Sum256(body)
		Artifacts = append(Artifacts, Artifact{
			Name: name, URL: "mem://" + name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body)),
		})
	}
	byURL := map[string][]byte{}
	for name, body := range bodies {
		byURL["mem://"+name] = body
	}
	return byURL
}

func ensureVia(t *testing.T, h *Handler) (int, StatusResponse) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/pose/model/ensure", nil)
	h.Ensure(c)
	var out StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
	return rec.Code, out
}

func TestEnsureDownloadsVerifiesAndServes(t *testing.T) {
	byURL := withPinnedArtifacts(t, map[string][]byte{
		"a.wasm": []byte("wasm-bytes"),
		"m.task": []byte("model-bytes"),
	})
	dir := t.TempDir()
	h := NewHandlerWithFetcher(dir, cannedFetcher(byURL))

	code, out := ensureVia(t, h)
	if code != http.StatusOK || !out.Ready || !out.Success {
		t.Fatalf("ensure: code=%d ready=%v success=%v files=%+v", code, out.Ready, out.Success, out.Files)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "a.wasm")); string(got) != "wasm-bytes" {
		t.Fatalf("file on disk: %q", got)
	}
	// No leftovers from the atomic write.
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) != 0 {
		t.Fatalf("temp files left behind: %v", parts)
	}

	// Served, with the immutable cache header and the wasm type.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/pose-model/a.wasm", nil)
	c.Params = gin.Params{{Key: "name", Value: "a.wasm"}}
	h.ServeFile(c)
	if rec.Code != http.StatusOK || rec.Body.String() != "wasm-bytes" {
		t.Fatalf("serve: %d %q", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/wasm" {
		t.Fatalf("content type: %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("no cache header")
	}

	// A second ensure touches nothing and is still ready.
	h.fetch = func(context.Context, string, io.Writer) error { t.Fatal("must not fetch again"); return nil }
	if code, out := ensureVia(t, h); code != http.StatusOK || !out.Ready {
		t.Fatalf("second ensure: %d %+v", code, out)
	}
}

func TestEnsureRefusesTamperedBytes(t *testing.T) {
	byURL := withPinnedArtifacts(t, map[string][]byte{"m.task": []byte("model-bytes")})
	// Same length, different content: size passes, the hash must not.
	byURL["mem://m.task"] = []byte("model-BYTES")
	dir := t.TempDir()
	h := NewHandlerWithFetcher(dir, cannedFetcher(byURL))

	code, out := ensureVia(t, h)
	if code != http.StatusBadGateway || out.Ready || out.Success {
		t.Fatalf("tampered download accepted: code=%d %+v", code, out)
	}
	if out.Files[0].Error == "" {
		t.Fatalf("no error reported for the bad file")
	}
	if _, err := os.Stat(filepath.Join(dir, "m.task")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bad file installed anyway: %v", err)
	}
	// And the file route does not serve what is not there.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/pose-model/m.task", nil)
	c.Params = gin.Params{{Key: "name", Value: "m.task"}}
	h.ServeFile(c)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("served a missing file: %d", rec.Code)
	}
}

func TestServeFileOnlyKnowsPinnedNames(t *testing.T) {
	dir := t.TempDir()
	h := NewHandlerWithFetcher(dir, cannedFetcher(nil))
	// A real file in the directory that is not an artifact must still be
	// unreachable: the route is a whitelist, not a directory listing.
	if err := os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"secret.txt", "../config.json", ""} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/pose-model/x", nil)
		c.Params = gin.Params{{Key: "name", Value: name}}
		h.ServeFile(c)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%q served: %d", name, rec.Code)
		}
	}
}

func TestStatusIsCheapAndHonest(t *testing.T) {
	byURL := withPinnedArtifacts(t, map[string][]byte{"m.task": []byte("model-bytes")})
	dir := t.TempDir()
	h := NewHandlerWithFetcher(dir, cannedFetcher(byURL))
	if s := h.status(); s.Ready || s.TotalBytes != int64(len("model-bytes")) {
		t.Fatalf("empty dir reported ready: %+v", s)
	}
	// A file of the wrong size is not "present", however it got there.
	if err := os.WriteFile(filepath.Join(dir, "m.task"), []byte("short"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := h.status(); s.Ready {
		t.Fatalf("wrong-size file reported ready")
	}
}
