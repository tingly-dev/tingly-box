package remoteagent

import (
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// maxFileNameBytes keeps a name, and the id Projects puts in front of it, under
// the 255-byte file name limit.
const maxFileNameBytes = 120

// SanitizeFileName makes a sender-supplied name safe to store: no directories
// (either separator), NUL or control characters, no leading dots, at most
// maxFileNameBytes with the extension kept. It returns "" when nothing usable
// is left.
func SanitizeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(strings.TrimSpace(name), ".")
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len(name) > maxFileNameBytes {
		ext := path.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		name = strings.ToValidUTF8(name[:maxFileNameBytes-len(ext)], "") + ext
	}
	return name
}

// contentDispositionName is the file name a download's Content-Disposition
// header gives (RFC 6266 / 5987 are decoded by mime.ParseMediaType).
func contentDispositionName(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	return params["filename"]
}

// extForBytes is the extension (with the dot) for a file, from what its bytes
// show first and the declared MIME type second; "" when neither knows.
func extForBytes(head []byte, mimeType string) string {
	for _, t := range []string{sniffType(head), mimeType} {
		switch t {
		case "image/jpeg":
			return ".jpg"
		case "image/png":
			return ".png"
		case "image/gif":
			return ".gif"
		case "image/webp":
			return ".webp"
		case "application/pdf":
			return ".pdf"
		case "text/markdown":
			return ".md"
		case "text/csv":
			return ".csv"
		case "text/plain":
			return ".txt"
		case "application/msword":
			return ".doc"
		case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
			return ".docx"
		case "application/vnd.ms-excel":
			return ".xls"
		case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
			return ".xlsx"
		}
	}
	return ""
}

// sniffType is the specific type http.DetectContentType finds in head, or ""
// when it can only say "some text" or "some bytes" (which the declared type
// then settles).
func sniffType(head []byte) string {
	t, _, _ := strings.Cut(http.DetectContentType(head), ";")
	if t == "application/octet-stream" || t == "text/plain" || t == "application/zip" {
		return ""
	}
	return t
}

// chooseFileName names a downloaded file: the name the message gave it, else
// the download's Content-Disposition header, else a generated one ("image-<time>" for
// images, "file-<time>" for the rest). The URL is never used: on most
// platforms its last part is a media id. A name without an extension gets the
// one the bytes (or declared type) show.
func chooseFileName(hint, contentDisposition, mimeType string, head []byte, now time.Time) string {
	name := SanitizeFileName(hint)
	if name == "" {
		name = SanitizeFileName(contentDispositionName(contentDisposition))
	}
	ext := extForBytes(head, mimeType)
	if name == "" {
		prefix := "file"
		if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(sniffType(head), "image/") {
			prefix = "image"
		}
		return prefix + "-" + now.Format("20060102-150405") + ext
	}
	if path.Ext(name) == "" && ext != "" {
		name += ext
	}
	return name
}
