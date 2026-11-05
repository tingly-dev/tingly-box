package core

import (
	"bytes"
	"path/filepath"
	"strings"
)

// ClassifyMedia decides how an outgoing local file is sent: as a platform
// "image" or as a "document". The leading bytes decide; the declared MIME type
// and file name are only a fallback when no bytes are available.
//
// Only PNG, JPEG, WebP and BMP go out as images. A GIF goes out as a document
// so platforms that re-encode image messages cannot flatten the animation.
// SVG and everything else are documents: an SVG must never reach a platform
// image API (Telegram sendPhoto would reject it, and no client may render it).
// The returned MIME describes the bytes, or "application/octet-stream" when
// the bytes contradict an image declaration.
func ClassifyMedia(head []byte, filename, declaredMIME string) (kind, mime string) {
	declared := strings.ToLower(strings.TrimSpace(strings.SplitN(declaredMIME, ";", 2)[0]))

	if len(head) > 0 {
		switch {
		case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
			return "image", "image/png"
		case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
			return "image", "image/jpeg"
		case len(head) >= 12 && bytes.HasPrefix(head, []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
			return "image", "image/webp"
		case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
			return "document", "image/gif"
		case len(head) >= 14 && bytes.HasPrefix(head, []byte("BM")) && bytes.Equal(head[6:10], []byte{0, 0, 0, 0}):
			return "image", "image/bmp"
		}
		if strings.HasPrefix(declared, "image/") && declared != "image/svg+xml" {
			return "document", "application/octet-stream"
		}
		if declared == "" {
			return "document", "application/octet-stream"
		}
		return "document", declared
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".svg" || ext == ".svgz" || declared == "image/svg+xml" {
		return "document", "image/svg+xml"
	}
	switch declared {
	case "image/png", "image/jpeg", "image/webp", "image/bmp":
		return "image", declared
	}
	if declared == "" {
		return "document", "application/octet-stream"
	}
	return "document", declared
}
