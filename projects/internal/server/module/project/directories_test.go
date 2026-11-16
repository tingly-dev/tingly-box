package project

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowseDirectoriesConfinedToHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(home, "visible"), 0700))
	require.NoError(t, os.Mkdir(filepath.Join(home, ".hidden"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "file"), nil, 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(home, "link")))
	got, err := browseDirectories(home, ".")
	require.NoError(t, err)
	require.Equal(t, []string{"visible"}, got.Directories)
	for _, path := range []string{"../outside", outside, ".hidden", "link", "visible/../../outside", "file"} {
		_, err := browseDirectories(home, path)
		require.Error(t, err, path)
	}
	got, err = browseDirectories(home, "visible")
	require.NoError(t, err)
	require.Equal(t, "visible", got.Path)
}

func TestBrowseDirectoriesRejectsCrossOriginAndNonJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", (&Handler{}).browseDirectories)
	for _, tc := range []struct {
		origin, contentType string
		status              int
	}{
		{"https://evil.example", "application/json", http.StatusForbidden},
		{"null", "application/json", http.StatusForbidden},
		{"", "text/plain", http.StatusUnsupportedMediaType},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost/", strings.NewReader(`{}`))
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", tc.contentType)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code)
	}
}

// A proxy may rewrite Host while the browser still sends a same-origin request.
// A cross-site browser cannot set Sec-Fetch-Site to same-origin.
func TestBrowseDirectoriesBehindProxy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	require.NoError(t, os.Mkdir(filepath.Join(home, "work"), 0700))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/", (&Handler{}).browseDirectories)
	for _, tc := range []struct {
		origin, site string
		status       int
	}{
		{"https://app.example", "same-origin", http.StatusOK},
		{"https://evil.example", "cross-site", http.StatusForbidden},
		{"https://evil.example", "same-site", http.StatusForbidden},
		{"null", "same-origin", http.StatusForbidden},
		{"https://app.example", "", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:12580/", strings.NewReader(`{"path":"."}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Sec-Fetch-Site", tc.site)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, tc)
		if tc.status == http.StatusOK {
			require.Contains(t, w.Body.String(), `"work"`)
		}
	}
}
