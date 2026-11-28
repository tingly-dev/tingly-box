package module

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	rt "github.com/tingly-dev/tingly-box/projects/internal/project"
)

// multipartBody builds a multipart body with one file in the "files" field.
func multipartBody(t *testing.T, name string, data []byte) (body []byte, contentType string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("files", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.FormDataContentType()
}

// serveUpload runs one request through an engine whose route is exactly the
// upload handler under test.
func serveUpload(t *testing.T, route gin.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/upload", route)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func uploadHandler(c *gin.Context) {
	files, ok := ParseUpload(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": len(files)})
}

func TestParseUploadReadsTheFilesField(t *testing.T) {
	body, contentType := multipartBody(t, "notes.txt", []byte("hello"))
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)

	w := serveUpload(t, uploadHandler, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"files":1}` {
		t.Fatalf("body = %s", got)
	}
}

func TestParseUploadRejectsANonMultipartBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(`{"files":[]}`))
	req.Header.Set("Content-Type", "application/json")

	w := serveUpload(t, uploadHandler, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != `expected multipart form with field "files"` {
		t.Fatalf("error = %q", body.Error)
	}
}

// A body cut mid-transfer (the old ReadTimeout failure, a client disconnect)
// keeps its own reason instead of being reported as "not multipart".
func TestParseUploadKeepsTheUnderlyingError(t *testing.T) {
	body, contentType := multipartBody(t, "big.bin", make([]byte, 4096))
	truncated := body[:len(body)-40]
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(truncated))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(truncated))

	w := serveUpload(t, uploadHandler, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	msg := w.Body.String()
	if strings.Contains(msg, "expected multipart form") {
		t.Fatalf("a truncated body was reported as not multipart: %s", msg)
	}
	if !strings.Contains(msg, "could not read the upload:") {
		t.Fatalf("body = %s", msg)
	}
}

// The body cap is installed by the middleware itself, so this exercises the
// same code the routes run: a body over maxFile×MaxAttachments is refused
// while it is being read, before ParseMultipartForm spools it to disk.
func TestUploadRequestCapsTheBodyWith413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/upload", UploadRequest(32<<10), uploadHandler) // cap ≈ 1.6 MB

	body, contentType := multipartBody(t, "big.bin", make([]byte, 2<<20))
	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "larger than") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestUploadErrorMapsTooLargeTo413(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/upload", func(c *gin.Context) {
		UploadError(c, &rt.TooLargeError{Name: "movie.mp4", Limit: 200 << 20})
	})
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", nil))

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "movie.mp4 is larger than 200 MB" {
		t.Fatalf("error = %q", body.Error)
	}
}

func TestUploadErrorLeavesOtherErrorsAs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/upload", func(c *gin.Context) { UploadError(c, io.ErrUnexpectedEOF) })
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// throttledBody sends data in chunks with a pause after each, so the server
// is still reading the request body long after the request started.
func throttledBody(t *testing.T, data []byte, chunk int, pause time.Duration) io.ReadCloser {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close(); _ = pr.Close() })
	go func() {
		for i := 0; i < len(data); i += chunk {
			end := min(i+chunk, len(data))
			if _, err := pw.Write(data[i:end]); err != nil {
				return
			}
			time.Sleep(pause)
		}
		_ = pw.Close()
	}()
	return pr
}

// A slow upload must survive http.Server's ReadTimeout: the route middleware
// clears the deadline the server armed at the start of the request, the same
// way @1384 did for streaming responses. The bare route is the control and
// proves the mechanism (the body is cut and parsing fails).
func TestUploadRequestOutlivesTheServersReadTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/guarded", UploadRequest(200<<20), uploadHandler)
	engine.POST("/bare", uploadHandler)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: engine, ReadTimeout: time.Second}
	go srv.Serve(ln)
	t.Cleanup(func() { _ = srv.Close() })
	base := "http://" + ln.Addr().String()

	body, contentType := multipartBody(t, "notes.txt", make([]byte, 256<<10))
	const chunks, pause = 12, 100 * time.Millisecond // ~1.2s of body, over the 1s ReadTimeout
	post := func(path string) (int, string, error) {
		req, err := http.NewRequest(http.MethodPost, base+path, throttledBody(t, body, len(body)/chunks, pause))
		if err != nil {
			return 0, "", err
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), nil
	}

	if code, msg, err := post("/guarded"); err != nil || code != http.StatusOK {
		t.Fatalf("guarded upload = %d %s, %v; the read deadline was not cleared (%d ms of body after a %d ms server ReadTimeout)", code, msg, err, chunks*int(pause/time.Millisecond), time.Second/time.Millisecond)
	}
	// The control proves the mechanism: without the middleware the same body
	// is cut and the handler reports the read timeout it really hit.
	code, msg, err := post("/bare")
	if err == nil && code == http.StatusOK {
		t.Fatal("the bare route survived ReadTimeout; the test setup no longer exercises the deadline")
	}
	if err == nil && (code != http.StatusBadRequest || !strings.Contains(msg, "could not read the upload")) {
		t.Fatalf("bare upload = %d %s, want 400 with the underlying cause", code, msg)
	}
}
