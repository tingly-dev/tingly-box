package posemodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// BaseURL is the path the file route is mounted at.
const BaseURL = "/pose-model/"

// Fetcher downloads one URL to a writer. Swapped out in tests.
type Fetcher func(ctx context.Context, url string, dst io.Writer) error

// Handler owns the on-disk copy of the artifacts.
type Handler struct {
	dir   string
	fetch Fetcher
	// One download at a time: two tabs pressing the button together must
	// not race each other over the same temp files.
	mu sync.Mutex
}

// NewHandler stores the artifacts under <configDir>/pose-model.
func NewHandler(configDir string) *Handler {
	return &Handler{dir: filepath.Join(configDir, DirName), fetch: defaultFetch}
}

// NewHandlerWithFetcher is NewHandler with the network replaced.
func NewHandlerWithFetcher(dir string, fetch Fetcher) *Handler {
	return &Handler{dir: dir, fetch: fetch}
}

// Dir is where the files live.
func (h *Handler) Dir() string { return h.dir }

// --- status ------------------------------------------------------------------

// present is the cheap check: right name, right size. The hash is verified
// once, when the file is written; re-hashing twelve megabytes on every status
// call would make the button feel slow for no safety.
func (h *Handler) present(a *Artifact) bool {
	info, err := os.Stat(filepath.Join(h.dir, a.Name))
	return err == nil && info.Mode().IsRegular() && info.Size() == a.Size
}

func (h *Handler) status() StatusResponse {
	out := StatusResponse{Success: true, Ready: true, BaseURL: BaseURL}
	for i := range Artifacts {
		a := &Artifacts[i]
		ok := h.present(a)
		out.Ready = out.Ready && ok
		out.TotalBytes += a.Size
		out.Files = append(out.Files, FileStatus{Name: a.Name, Size: a.Size, Ready: ok})
	}
	return out
}

// GetStatus reports what is on disk without touching the network.
func (h *Handler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, h.status())
}

// --- ensure ------------------------------------------------------------------

// Ensure downloads whatever is missing or the wrong size, verifies each file
// against its pinned hash, and reports the result. Idempotent: a second call
// with everything present does nothing and answers immediately.
func (h *Handler) Ensure(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	// Generous: the largest file is twelve megabytes and a slow link is the
	// normal case for a machine that needed this downloaded in the first
	// place. Bounded all the same, so an abandoned request cannot hold the
	// lock forever.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()

	out := StatusResponse{Success: true, Ready: true, BaseURL: BaseURL}
	for i := range Artifacts {
		a := &Artifacts[i]
		out.TotalBytes += a.Size
		fs := FileStatus{Name: a.Name, Size: a.Size, Ready: h.present(a)}
		if !fs.Ready {
			if err := h.download(ctx, a); err != nil {
				fs.Error = err.Error()
				out.Success = false
			} else {
				fs.Ready = true
			}
		}
		out.Ready = out.Ready && fs.Ready
		out.Files = append(out.Files, fs)
	}
	status := http.StatusOK
	if !out.Success {
		// The browser cannot run the estimator; say so with a status the
		// caller does not have to parse the body to notice.
		status = http.StatusBadGateway
	}
	c.JSON(status, out)
}

// download writes to a temp file beside the target, verifies it, and renames
// it into place, so a half-finished download can never be mistaken for the
// real thing by the file route or by `present`.
func (h *Handler) download(ctx context.Context, a *Artifact) error {
	final := filepath.Join(h.dir, a.Name)
	tmp, err := os.CreateTemp(h.dir, a.Name+".*.part")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	hash := sha256.New()
	counted := &countingWriter{w: io.MultiWriter(tmp, hash)}
	fetchErr := h.fetch(ctx, a.URL, counted)
	closeErr := tmp.Close()
	if fetchErr != nil {
		return fmt.Errorf("download %s: %w", a.Name, fetchErr)
	}
	if closeErr != nil {
		return fmt.Errorf("write %s: %w", a.Name, closeErr)
	}
	if counted.n != a.Size {
		return fmt.Errorf("%s: got %d bytes, expected %d", a.Name, counted.n, a.Size)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != a.SHA256 {
		return fmt.Errorf("%s: checksum mismatch", a.Name)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return fmt.Errorf("install %s: %w", a.Name, err)
	}
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// defaultFetch streams a URL to the writer, retrying transient failures
// twice with a short backoff. Streams rather than reads into memory: the
// wasm alone is twelve megabytes.
func defaultFetch(ctx context.Context, url string, dst io.Writer) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				return lastErr
			}
			continue
		}
		_, err = io.Copy(dst, resp.Body)
		resp.Body.Close()
		if err == nil {
			return nil
		}
		lastErr = err
		// A partial body cannot be resumed into a hashed stream; the caller
		// verifies the size and hash and will fail cleanly.
		return lastErr
	}
	if lastErr == nil {
		lastErr = errors.New("download failed")
	}
	return lastErr
}

// --- serving -----------------------------------------------------------------

// ServeFile hands one artifact to the browser. Unauthenticated by design:
// MediaPipe fetches its wasm with a bare `fetch` that cannot carry our
// Authorization header, and these are public Apache-2.0 files. Only the
// pinned names are served, only from our directory, and only once they are
// complete — a `.part` file is never reachable.
func (h *Handler) ServeFile(c *gin.Context) {
	a := ArtifactByName(c.Param("name"))
	if a == nil || !h.present(a) {
		// AbortWithStatus writes the header now; a bare Status only records
		// it, which leaves a recorder — or a downstream handler — seeing 200.
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	// Immutable content, pinned by hash: let the browser keep it.
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	if strings.HasSuffix(a.Name, ".wasm") {
		c.Header("Content-Type", "application/wasm")
	}
	c.File(filepath.Join(h.dir, a.Name))
}
