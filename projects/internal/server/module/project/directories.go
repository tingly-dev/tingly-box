package project

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/server/module/apierr"
)

// DirectoryAuthenticatedKey marks a request already authenticated by the embedding host.
const DirectoryAuthenticatedKey = "projects.directory.authenticated"

// BrowseDirectoriesRequest specifies a path relative to the server user's home.
type BrowseDirectoriesRequest struct {
	Path string `json:"path"`
}
type BrowseDirectoriesResponse struct {
	Home        string   `json:"home"`
	Path        string   `json:"path"`
	Directories []string `json:"directories"`
}

// Browsing is deliberately narrower than opening a typed path. Root.Open keeps
// symlinks and concurrent filesystem changes from escaping the home directory.
func browseDirectories(home, path string) (BrowseDirectoriesResponse, error) {
	result := BrowseDirectoriesResponse{Home: home, Directories: []string{}}
	if path == "" {
		path = "."
	}
	if !filepath.IsLocal(path) {
		return result, os.ErrInvalid
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." || (strings.HasPrefix(part, ".") && part != ".") {
			return result, os.ErrInvalid
		}
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return result, err
	}
	defer root.Close()
	current := "."
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil || !info.IsDir() {
			return result, os.ErrInvalid
		}
	}
	dir, err := root.Open(path)
	if err != nil {
		return result, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return result, err
	}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			result.Directories = append(result.Directories, entry.Name())
		}
	}
	sort.Strings(result.Directories)
	result.Path = filepath.Clean(path)
	return result, nil
}

func (h *Handler) browseDirectories(c *gin.Context) {
	// An authenticated host may serve its UI from another origin (desktop or proxy).
	// For an unauthenticated standalone server, browsers set Sec-Fetch-Site themselves. Same-origin requests remain
	// same-origin when a reverse proxy rewrites Host; never trust forwarded
	// headers for this decision. Other origins still need to match Host.
	if origin := c.GetHeader("Origin"); origin != "" && !c.GetBool(DirectoryAuthenticatedKey) {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (!strings.EqualFold(u.Host, c.Request.Host) && c.GetHeader("Sec-Fetch-Site") != "same-origin") || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			apierr.Message(c, http.StatusForbidden, "cross-origin browsing is not allowed")
			return
		}
	}
	// A JSON-only POST cannot be made cross-origin by an HTML form.
	if c.ContentType() != "application/json" {
		apierr.Message(c, http.StatusUnsupportedMediaType, "application/json required")
		return
	}
	var req BrowseDirectoriesRequest
	if !apierr.Bind(c, &req) {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		apierr.Message(c, http.StatusInternalServerError, "home directory unavailable")
		return
	}
	result, err := browseDirectories(home, req.Path)
	if err != nil {
		apierr.BadRequest(c, "cannot browse this directory")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}
