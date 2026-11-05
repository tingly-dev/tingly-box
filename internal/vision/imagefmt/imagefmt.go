// Package imagefmt identifies raster image formats from their leading bytes.
// The gateway never trusts a requested or declared format over the bytes, and
// it never decodes pixels: only the header is read.
package imagefmt

import (
	"bytes"
	"encoding/base64"
	"strings"
)

type Format string

const (
	PNG  Format = "png"
	JPEG Format = "jpeg"
	WebP Format = "webp"
	GIF  Format = "gif"
)

func (f Format) MIME() string {
	if f == JPEG {
		return "image/jpeg"
	}
	return "image/" + string(f)
}

func (f Format) Ext() string {
	if f == JPEG {
		return ".jpg"
	}
	return "." + string(f)
}

// Parse maps an output_format request value onto a Format; "jpg" is accepted
// as a spelling of jpeg. Anything else, including svg, is rejected.
func Parse(s string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "png":
		return PNG, true
	case "jpeg", "jpg":
		return JPEG, true
	case "webp":
		return WebP, true
	case "gif":
		return GIF, true
	}
	return "", false
}

// Sniff identifies a raster format from its magic bytes. SVG, HTML and every
// other format report false.
func Sniff(b []byte) (Format, bool) {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return PNG, true
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return JPEG, true
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return GIF, true
	case len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return WebP, true
	}
	return "", false
}

// SniffBase64 decodes only the first few bytes of a base64 payload (a data URL
// prefix and surrounding whitespace are tolerated) and sniffs them.
func SniffBase64(s string) (Format, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "base64,"); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+len("base64,"):]
	}
	// 16 base64 chars decode to 12 bytes, enough for every signature above.
	if len(s) > 16 {
		s = s[:16]
	}
	s = s[:len(s)/4*4]
	if s == "" {
		return "", false
	}
	head, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", false
	}
	return Sniff(head)
}
