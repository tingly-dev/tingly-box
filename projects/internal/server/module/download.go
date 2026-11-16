package module

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServeDownload sends an already opened file as an attachment, with range and
// conditional requests. It serves the handle it is given instead of opening a
// path again (as gin's FileAttachment does), so whatever access checks were
// made when the file was opened still hold. The type comes from the extension
// and nosniff is set, so the browser never guesses a document type for it.
// name is what the file is saved as, which is not always the name it is stored
// under.
func ServeDownload(c *gin.Context, f *os.File, info fs.FileInfo, name string) {
	ctype := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	c.Header("Content-Type", ctype)
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, name, info.ModTime(), f)
}
