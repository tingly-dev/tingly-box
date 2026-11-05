package imagefmt

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want Format
		ok   bool
	}{
		{"png", []byte("\x89PNG\r\n\x1a\nrest"), PNG, true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}, JPEG, true},
		{"gif87a", []byte("GIF87a\x01\x00\x01\x00"), GIF, true},
		{"gif89a", []byte("GIF89a\x01\x00\x01\x00"), GIF, true},
		{"webp", []byte("RIFF\x24\x00\x00\x00WEBPVP8 "), WebP, true},
		{"riff but not webp", []byte("RIFF\x24\x00\x00\x00WAVEfmt "), "", false},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "", false},
		{"xml prolog svg", []byte(`<?xml version="1.0"?><svg/>`), "", false},
		{"html", []byte("<!doctype html><script>alert(1)</script>"), "", false},
		{"empty", nil, "", false},
		{"truncated png", []byte("\x89PNG\r\n"), "", false},
		{"truncated gif", []byte("GIF89"), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Sniff(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestSniffBase64(t *testing.T) {
	enc := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

	got, ok := SniffBase64(enc([]byte("GIF89a\x01\x00\x01\x00pad-pad-pad")))
	assert.True(t, ok)
	assert.Equal(t, GIF, got)
	assert.Equal(t, "R0lGODlh", enc([]byte("GIF89a\x01\x00\x01\x00pad-pad-pad"))[:8], "header prefix Codex also checks")

	got, ok = SniffBase64(enc([]byte("\x89PNG\r\n\x1a\nxxxx")))
	assert.True(t, ok)
	assert.Equal(t, PNG, got)

	// Whitespace around the payload and a data URL prefix are tolerated.
	got, ok = SniffBase64("  data:image/gif;base64," + enc([]byte("GIF87a\x01\x00\x01\x00")) + "\n")
	assert.True(t, ok)
	assert.Equal(t, GIF, got)

	_, ok = SniffBase64("")
	assert.False(t, ok)
	_, ok = SniffBase64("!!!not base64!!!")
	assert.False(t, ok)
	_, ok = SniffBase64(enc([]byte("<svg/>")))
	assert.False(t, ok)
}

func TestFormatAttributes(t *testing.T) {
	assert.Equal(t, "image/png", PNG.MIME())
	assert.Equal(t, "image/jpeg", JPEG.MIME())
	assert.Equal(t, "image/webp", WebP.MIME())
	assert.Equal(t, "image/gif", GIF.MIME())
	assert.Equal(t, ".png", PNG.Ext())
	assert.Equal(t, ".jpg", JPEG.Ext())
	assert.Equal(t, ".webp", WebP.Ext())
	assert.Equal(t, ".gif", GIF.Ext())
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Format{"png": PNG, "PNG": PNG, "jpeg": JPEG, "jpg": JPEG, "webp": WebP, "gif": GIF, " gif ": GIF} {
		got, ok := Parse(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "svg", "bmp", "tiff", "gif89a"} {
		_, ok := Parse(in)
		assert.False(t, ok, in)
	}
}
