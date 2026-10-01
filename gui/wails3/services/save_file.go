package services

import (
	"crypto/subtle"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxSaveFileBytes bounds one save. Exports are logs, JSONL and images —
// far below this; the cap only stops a runaway body from filling memory.
const maxSaveFileBytes = 512 << 20

// saveFileHandler serves POST /api/v1/gui/save?name=<file name> with the file
// as the raw request body: the desktop bridge's saveFile
// (frontend/src/host/desktop.ts). The webview has no download handling —
// wails v3 wires no download delegate, so an <a download> click in WKWebView
// does nothing — so the shell asks where to save with a native dialog and
// writes the file itself.
//
// prompt shows the dialog and returns the chosen path, or "" when the user
// cancels. name is only the suggested file name; the user picks the path.
func saveFileHandler(userToken func() string, prompt func(name string) (string, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		want := "Bearer " + userToken()
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte(want)) != 1 {
			c.Status(http.StatusForbidden)
			return
		}

		data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSaveFileBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "file too large"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
			return
		}

		suggested := suggestedFileName(c.Query("name"))
		path, err := prompt(suggested)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		if path == "" {
			// Cancelled: not an error, and the caller must not fall back to
			// another way of saving.
			c.JSON(http.StatusOK, gin.H{"success": true, "saved": false})
			return
		}
		path = withSuggestedExtension(path, suggested)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "saved": true, "path": path})
	}
}

// suggestedFileName reduces the page's name to a bare file name for the
// dialog's default: never a path, so the page cannot steer the dialog's
// directory.
func suggestedFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == ".." || name == "/" || name == "" {
		return "download"
	}
	return name
}

// withSuggestedExtension gives path the suggested name's extension when the
// user typed a name without one. wails v3 does not pre-fill the dialog's name
// field on Linux (GTK4: SetFilename is never passed to the chooser), so the
// user types the name from scratch there and easily leaves the extension
// off — saving "slices" instead of "slices.zip".
func withSuggestedExtension(path, suggested string) string {
	if filepath.Ext(path) != "" {
		return path
	}
	return path + filepath.Ext(suggested)
}
