package localsessions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/swagger"
)

func TestLocalAccessBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, peer, host, header, value string
		allowed                         bool
	}{
		{"direct", "127.0.0.1:51234", "localhost:12580", "", "", true},
		{"ipv6", "[::1]:51234", "[::1]:12580", "", "", true},
		{"same_origin", "127.0.0.1:1", "localhost:12580", "Origin", "http://localhost:12580", true},
		{"desktop", "127.0.0.1:1", "localhost:12580", "Origin", "wails://localhost", true},
		{"windows_desktop", "127.0.0.1:1", "localhost:12580", "Origin", "http://wails.localhost", true},
		{"remote", "203.0.113.4:1", "localhost:12580", "", "", false},
		{"public_host", "127.0.0.1:1", "example.com", "", "", false},
		{"forwarded", "127.0.0.1:1", "localhost:12580", "X-Forwarded-For", "127.0.0.1", false},
		{"forwarded_rfc", "127.0.0.1:1", "localhost:12580", "Forwarded", "for=127.0.0.1", false},
		{"proxy_proto", "127.0.0.1:1", "localhost:12580", "X-Forwarded-Proto", "http", false},
		{"real_ip", "127.0.0.1:1", "localhost:12580", "X-Real-IP", "127.0.0.1", false},
		{"foreign_origin", "127.0.0.1:1", "localhost:12580", "Origin", "https://example.com", false},
		{"other_local_app", "127.0.0.1:1", "localhost:12580", "Origin", "http://localhost:8080", false},
		{"cross_site", "127.0.0.1:1", "localhost:12580", "Sec-Fetch-Site", "cross-site", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://localhost/", nil)
			r.RemoteAddr = tc.peer
			r.Host = tc.host
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			if got := localRequest(r); got != tc.allowed {
				t.Fatalf("allowed=%v", got)
			}
		})
	}
}
func testRouter(t *testing.T, root string, enabled *bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mgr := swagger.NewRouteManager(r)
	v2 := mgr.NewGroup("api", "v2", "")
	v2.AddMiddleware(func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer owner" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	newHandler(root, func() bool { return *enabled }).RegisterRoutes(&module.Routes{V2: v2})
	return r
}
func call(r *gin.Engine, path, peer, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "http://localhost:12580"+path, nil)
	req.RemoteAddr = peer
	req.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func TestRoutesRequireAuthLocalityAndExplicitEnable(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "-work"), 0700)
	os.WriteFile(filepath.Join(root, "-work", "s.jsonl"), []byte("{\"type\":\"user\",\"message\":{\"content\":\"hello\"}}\n"), 0600)
	enabled := false
	r := testRouter(t, root, &enabled)
	for _, tc := range []struct {
		path, peer, auth string
		status           int
	}{
		{"/api/v2/local-sessions/projects", "127.0.0.1:1", "", 401},
		{"/api/v2/local-sessions/projects", "203.0.113.1:1", "Bearer owner", 403},
		{"/api/v2/local-sessions/projects", "127.0.0.1:1", "Bearer owner", 403},
	} {
		w := call(r, tc.path, tc.peer, tc.auth)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	enabled = true
	w := call(r, "/api/v2/local-sessions/projects", "127.0.0.1:1", "Bearer owner")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("listing: %d %s", w.Code, w.Body.String())
	}
	var listing LocalSessionListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil || len(listing.Data.Entries) != 1 {
		t.Fatal("listing response")
	}
	w = call(r, "/api/v2/local-sessions/projects/"+listing.Data.Entries[0].ID+"/sessions", "127.0.0.1:1", "Bearer owner")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	json.Unmarshal(w.Body.Bytes(), &listing)
	w = call(r, "/api/v2/local-sessions/sessions/"+listing.Data.Entries[0].ID+"/events", "127.0.0.1:1", "Bearer owner")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Body.String())
	}
	enabled = false
	w = call(r, "/api/v2/local-sessions/sessions/"+listing.Data.Entries[0].ID+"/events", "127.0.0.1:1", "Bearer owner")
	if w.Code != 403 {
		t.Fatal("disabling must revoke existing identifiers")
	}
}
func TestStatusDoesNotDiscoverHistory(t *testing.T) {
	enabled := false
	r := testRouter(t, filepath.Join(t.TempDir(), "missing"), &enabled)
	w := call(r, "/api/v2/local-sessions/status", "203.0.113.1:1", "Bearer owner")
	var status LocalSessionStatusResponse
	json.Unmarshal(w.Body.Bytes(), &status)
	if w.Code != 200 || status.Data.LocalAccess || status.Data.Enabled {
		t.Fatal(w.Body.String())
	}
}

func TestPublicBindingIsUnavailableEvenForLocalClient(t *testing.T) {
	for _, host := range []string{"", "0.0.0.0", "::", "192.0.2.1"} {
		if NewHandler(nil, host).localBinding {
			t.Fatalf("public bind %q allowed", host)
		}
	}
	enabled := true
	h := newHandler(t.TempDir(), func() bool { return enabled })
	h.localBinding = false
	r := gin.New()
	r.GET("/projects", h.ListProjects)
	w := call(r, "/projects", "127.0.0.1:1", "Bearer owner")
	if w.Code != 403 || h.store != nil {
		t.Fatalf("publicly bound instance opened history: %d", w.Code)
	}
}
