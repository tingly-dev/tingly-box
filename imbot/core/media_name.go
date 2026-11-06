package core

import (
	"mime"
	"path/filepath"
	"strings"
)

var knownExtTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
	".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xls": "application/vnd.ms-excel", ".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
}

// MimeTypeByFilename is the MIME type a file name's extension implies, or ""
// when it implies none. It knows the common document and image types itself, so
// the answer does not depend on the host's mime database.
func MimeTypeByFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := knownExtTypes[ext]; ok {
		return t
	}
	t, _, _ := strings.Cut(mime.TypeByExtension(ext), ";")
	return t
}
