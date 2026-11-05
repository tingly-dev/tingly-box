package core

import "testing"

func TestClassifyMedia(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nrest")
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	webp := []byte("RIFF\x10\x00\x00\x00WEBPVP8 ")
	gif := []byte("GIF89a\x01\x00\x01\x00")
	bmp := []byte("BM\x46\x00\x00\x00\x00\x00\x00\x00\x36\x00\x00\x00")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	html := []byte("<!doctype html><script>x</script>")

	cases := []struct {
		name     string
		head     []byte
		filename string
		declared string
		wantKind string
		wantMIME string
	}{
		{"png", png, "a.png", "image/png", "image", "image/png"},
		{"jpeg", jpeg, "a.jpg", "image/jpeg", "image", "image/jpeg"},
		{"webp", webp, "a.webp", "image/webp", "image", "image/webp"},
		{"bmp", bmp, "a.bmp", "image/bmp", "image", "image/bmp"},
		{"gif goes as a document so animation survives", gif, "a.gif", "image/gif", "document", "image/gif"},
		{"png bytes under a gif name", png, "a.gif", "image/gif", "image", "image/png"},
		{"gif bytes under a png name", gif, "a.png", "image/png", "document", "image/gif"},
		{"svg is never an image", svg, "a.svg", "image/svg+xml", "document", "image/svg+xml"},
		{"svg under a png name", svg, "a.png", "image/png", "document", "application/octet-stream"},
		{"html under a png name", html, "a.png", "image/png", "document", "application/octet-stream"},
		{"pdf", []byte("%PDF-1.7"), "a.pdf", "application/pdf", "document", "application/pdf"},
		{"unknown bytes keep a non-image declared type", []byte("hello"), "a.txt", "text/plain", "document", "text/plain"},
		{"no declared type", []byte("hello"), "a.bin", "", "document", "application/octet-stream"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, mime := ClassifyMedia(c.head, c.filename, c.declared)
			if kind != c.wantKind || mime != c.wantMIME {
				t.Fatalf("got (%q, %q), want (%q, %q)", kind, mime, c.wantKind, c.wantMIME)
			}
		})
	}
}

func TestClassifyMediaWithoutBytesFallsBackToDeclared(t *testing.T) {
	cases := []struct {
		filename, declared, wantKind string
	}{
		{"a.png", "image/png", "image"},
		{"a.jpg", "image/jpeg", "image"},
		{"a.webp", "image/webp", "image"},
		{"a.gif", "image/gif", "document"},
		{"a.svg", "image/svg+xml", "document"},
		{"a.svgz", "image/svg+xml", "document"},
		{"a.svg", "image/png", "document"},
		{"a.pdf", "application/pdf", "document"},
	}
	for _, c := range cases {
		kind, _ := ClassifyMedia(nil, c.filename, c.declared)
		if kind != c.wantKind {
			t.Errorf("%s (%s): got %q, want %q", c.filename, c.declared, kind, c.wantKind)
		}
	}
}
