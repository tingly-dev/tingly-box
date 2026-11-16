package core

import (
	"mime"
	"path/filepath"
	"strings"
)

// knownTypes pairs each common document and image extension with its MIME
// type; the first extension listed for a type is the one ExtensionByMimeType
// gives.
var knownTypes = []struct{ ext, mime string }{
	{".jpg", "image/jpeg"}, {".jpeg", "image/jpeg"}, {".png", "image/png"}, {".gif", "image/gif"}, {".webp", "image/webp"},
	{".pdf", "application/pdf"}, {".txt", "text/plain"}, {".md", "text/markdown"}, {".csv", "text/csv"},
	{".doc", "application/msword"}, {".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	{".xls", "application/vnd.ms-excel"}, {".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
}

// ExtensionByMimeType is the extension (with the dot) for a MIME type, or ""
// when it is not one of the common document and image types.
func ExtensionByMimeType(mimeType string) string {
	for _, k := range knownTypes {
		if k.mime == mimeType {
			return k.ext
		}
	}
	return ""
}

// MimeTypeByFilename is the MIME type a file name's extension implies, or ""
// when it implies none. It knows the common document and image types itself, so
// the answer does not depend on the host's mime database.
func MimeTypeByFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	for _, k := range knownTypes {
		if k.ext == ext {
			return k.mime
		}
	}
	t, _, _ := strings.Cut(mime.TypeByExtension(ext), ";")
	return t
}
