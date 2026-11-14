package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	assets "github.com/tingly-dev/tingly-box/internal"
)

func init() {
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".png", "image/png")
}

// The web UI is the Vite build embedded in the binary (internal/web/dist).
//
// Opening the home page costs ~40 requests / ~2 MB of scripts, so how they are
// served decides how long the page takes:
//
//   - `pnpm build` writes a .br and .gz sibling next to every compressible
//     file (frontend/scripts/precompress.mjs); the best one the client accepts
//     is served as is, with no per-request CPU. A build without them (dev
//     builds) is still gzipped, once, and the result is kept.
//   - Vite names chunks <name>-<hash>.<ext>: the URL changes with the bytes,
//     so they are immutable. index.html and the few unhashed files are
//     `no-cache` with an ETag: always revalidated, a 304 when unchanged.
//   - Each representation has its own ETag; conditional and range requests are
//     answered by http.ServeContent straight from the embedded bytes.
//   - A missing /assets file is a 404, never index.html: otherwise a stale
//     chunk name returns HTML with status 200 and fails as a script error.
//
// Design notes: .design/webui-static-serving.md

const (
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"

	// below this a compressed copy is not worth a Content-Encoding header
	minCompressBytes = 512
)

// noClose is the closer of in-memory representations.
var noClose io.Closer = io.NopCloser(nil)

// hashedAsset matches Vite's content-hashed names: <name>-<8 chars>.<ext>.
// The hash must contain a digit or an upper-case letter so that a plain word
// ("foo-barbaric.png" in public/) is not mistaken for one and cached forever.
var hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8}\.[A-Za-z0-9]+$`)

func isHashedAsset(name string) bool {
	loc := hashedAsset.FindStringIndex(name)
	if loc == nil {
		return false
	}
	hash := name[loc[0]+1 : loc[0]+9]
	return strings.ContainsAny(hash, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

// compressible extensions get an on-the-fly gzip when no .gz sibling exists.
var compressible = map[string]bool{
	".js": true, ".mjs": true, ".css": true, ".html": true, ".json": true,
	".svg": true, ".txt": true, ".map": true, ".xml": true, ".wasm": true,
}

type webStatic struct {
	fsys fs.FS // root of the build: index.html, assets/…

	etags sync.Map // path -> string, hash of the file's bytes
	gzips sync.Map // path -> *gzipEntry, on-the-fly gzip of files with no .gz sibling
}

type gzipEntry struct {
	once sync.Once
	data []byte // nil: not worth compressing
}

// representation is one encoding of a file, ready for http.ServeContent.
type representation struct {
	body     io.ReadSeeker
	closer   io.Closer
	encoding string
	etag     string
}

func (w *webStatic) exists(name string) bool {
	st, err := fs.Stat(w.fsys, name)
	return err == nil && st.Mode().IsRegular()
}

// open returns a seekable view of name (embedded files are seekable and not
// copied; anything else is read into memory).
func (w *webStatic) open(name string) (io.ReadSeeker, io.Closer, error) {
	f, err := w.fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	if rs, ok := f.(io.ReadSeeker); ok {
		return rs, f, nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, nil, err
	}
	return bytes.NewReader(data), noClose, nil
}

// fileHash is the identity of name's bytes, computed once.
func (w *webStatic) fileHash(name string) (string, error) {
	if v, ok := w.etags.Load(name); ok {
		return v.(string), nil
	}
	data, err := fs.ReadFile(w.fsys, name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:8])
	w.etags.Store(name, h)
	return h, nil
}

// accepts reports whether the Accept-Encoding header allows coding.
func accepts(header, coding string) bool {
	star := false
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.ToLower(strings.TrimSpace(token))
		q := 1.0
		if params != "" {
			if k, v, ok := strings.Cut(strings.TrimSpace(params), "="); ok && strings.TrimSpace(k) == "q" {
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					q = f
				}
			}
		}
		switch token {
		case coding:
			return q > 0
		case "*":
			star = q > 0
		}
	}
	return star
}

// pick chooses the representation of name for this Accept-Encoding.
func (w *webStatic) pick(name, acceptEncoding string) (*representation, error) {
	h, err := w.fileHash(name)
	if err != nil {
		return nil, err
	}
	ext := path.Ext(name)

	serve := func(file, encoding string) (*representation, error) {
		body, closer, err := w.open(file)
		if err != nil {
			return nil, err
		}
		tag := `"` + h
		if encoding != "" {
			tag += "-" + encoding
		}
		return &representation{body: body, closer: closer, encoding: encoding, etag: tag + `"`}, nil
	}

	if accepts(acceptEncoding, "br") && w.exists(name+".br") {
		return serve(name+".br", "br")
	}
	if accepts(acceptEncoding, "gzip") {
		if w.exists(name + ".gz") {
			return serve(name+".gz", "gzip")
		}
		if compressible[ext] {
			if data := w.gzipOnce(name); data != nil {
				return &representation{
					body: bytes.NewReader(data), closer: noClose,
					encoding: "gzip", etag: `"` + h + `-gzip"`,
				}, nil
			}
		}
	}
	return serve(name, "")
}

func (w *webStatic) gzipOnce(name string) []byte {
	v, _ := w.gzips.LoadOrStore(name, &gzipEntry{})
	e := v.(*gzipEntry)
	e.once.Do(func() {
		raw, err := fs.ReadFile(w.fsys, name)
		if err != nil || len(raw) < minCompressBytes {
			return
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		_, _ = zw.Write(raw)
		if zw.Close() == nil && buf.Len() < len(raw) {
			e.data = buf.Bytes()
		}
	})
	return e.data
}

// serve writes name with the given Cache-Control. name must be a file of the build.
func (w *webStatic) serve(c *gin.Context, name, cacheControl string) {
	acceptEncoding := c.GetHeader("Accept-Encoding")
	if c.GetHeader("Range") != "" {
		// byte ranges address the file itself, not one of its encodings
		acceptEncoding = ""
	}
	rep, err := w.pick(name, acceptEncoding)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer rep.closer.Close()

	h := c.Writer.Header()
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", rep.etag)
	h.Add("Vary", "Accept-Encoding")
	if rep.encoding != "" {
		h.Set("Content-Encoding", rep.encoding)
		// ServeContent only sets Content-Length for unencoded bodies
		if n, err := rep.body.Seek(0, io.SeekEnd); err == nil {
			_, _ = rep.body.Seek(0, io.SeekStart)
			h.Set("Content-Length", strconv.FormatInt(n, 10))
		}
	}
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)

	// zero modtime: no Last-Modified (an embedded file has none worth sending);
	// validation is by ETag, If-Range and Range are handled here too.
	http.ServeContent(c.Writer, c.Request, path.Base(name), time.Time{}, rep.body)
}

func (w *webStatic) serveIndex(c *gin.Context) {
	if !w.exists("index.html") {
		c.Status(http.StatusInternalServerError)
		return
	}
	// index.html names the current chunks, so it is always revalidated — but
	// with an ETag that is a 304, not a re-download.
	w.serve(c, "index.html", cacheRevalidate)
}

func (w *webStatic) serveAsset(c *gin.Context) {
	name := "assets" + c.Param("filepath")
	if !fs.ValidPath(name) || !w.exists(name) || w.isVariant(name) {
		c.Header("Cache-Control", "no-cache")
		c.String(http.StatusNotFound, "not found")
		return
	}
	cc := cacheRevalidate
	if isHashedAsset(name) {
		cc = cacheImmutable
	}
	w.serve(c, name, cc)
}

// isVariant reports whether name is the .br/.gz copy of another file: those
// are an implementation detail of content negotiation, not addressable assets.
func (w *webStatic) isVariant(name string) bool {
	ext := path.Ext(name)
	return (ext == ".br" || ext == ".gz") && w.exists(strings.TrimSuffix(name, ext))
}

func UseWebStaticEndpoints(engine *gin.Engine) {
	log.Printf("Using embedded assets on main server")
	sub, err := fs.Sub(assets.WebDistAssets, "web/dist")
	if err != nil {
		log.Printf("web UI assets unavailable: %v", err)
		return
	}
	registerWebStatic(engine, sub)
}

// registerWebStatic mounts the web UI (build rooted at fsys) on engine.
func registerWebStatic(engine *gin.Engine, fsys fs.FS) {
	w := &webStatic{fsys: fsys}

	engine.GET("/assets/*filepath", w.serveAsset)
	engine.HEAD("/assets/*filepath", w.serveAsset)

	// SPA catch-all - must be registered LAST
	// Serves index.html for all non-API frontend routes, letting React Router handle navigation
	// NoRoute handles unmatched paths including nested routes like /provider/settings/detail/123
	engine.NoRoute(func(c *gin.Context) {
		// Don't serve index.html for API routes - let them return 404s
		urlPath := c.Request.URL.Path
		// Check if this looks like an API route
		if urlPath == "" || strings.HasPrefix(urlPath, "/api/v") || strings.HasPrefix(urlPath, "/v") || strings.HasPrefix(urlPath, "/openai") || strings.HasPrefix(urlPath, "/anthropic") || strings.HasPrefix(urlPath, "/tingly") {
			// This looks like an API route, return 404
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "API endpoint not found",
					"type":    "invalid_request_error",
					"code":    "not_found",
				},
			})
			c.Abort()
		}
	}, w.serveIndex)
}
