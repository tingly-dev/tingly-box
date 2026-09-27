package imageasset

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	_ "image/gif"  // register for DecodeConfig
	_ "image/jpeg" // register for DecodeConfig
	_ "image/png"  // register for DecodeConfig
	"net/http"
	"strings"
)

// Formats a reference may be in: what the image endpoints accept.
var imageExtensions = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// sniffImage reports the image's MIME type from its bytes — never from the
// name or a declared type — and its pixel size where it can be read.
func sniffImage(data []byte) (mime string, width, height int, err error) {
	mime = http.DetectContentType(data)
	if _, ok := imageExtensions[mime]; !ok {
		return "", 0, 0, invalid("not a PNG, JPEG, WebP or GIF image")
	}
	if mime == "image/webp" {
		width, height = webpSize(data)
		return mime, width, height, nil
	}
	if config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data)); decodeErr == nil {
		width, height = config.Width, config.Height
	}
	return mime, width, height, nil
}

// webpSize reads a WebP's canvas size from its header (lossy VP8, lossless
// VP8L, or extended VP8X), or zeros when the header is not one of those.
// The standard library has no WebP decoder, and the size is all that is
// needed.
func webpSize(data []byte) (int, int) {
	if len(data) < 30 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0
	}
	chunk := data[12:]
	switch string(chunk[0:4]) {
	case "VP8 ":
		// Frame tag (3 bytes) and start code (3 bytes) precede two 14-bit sizes.
		if len(chunk) < 18 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint16(chunk[14:16]) & 0x3fff), int(binary.LittleEndian.Uint16(chunk[16:18]) & 0x3fff)
	case "VP8L":
		// Signature byte, then width-1 and height-1 as 14-bit fields.
		if len(chunk) < 13 || chunk[8] != 0x2f {
			return 0, 0
		}
		bits := binary.LittleEndian.Uint32(chunk[9:13])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1
	case "VP8X":
		// Flags (4 bytes), then canvas width-1 and height-1 as 24-bit fields.
		if len(chunk) < 18 {
			return 0, 0
		}
		width := int(chunk[12]) | int(chunk[13])<<8 | int(chunk[14])<<16
		height := int(chunk[15]) | int(chunk[16])<<8 | int(chunk[17])<<16
		return width + 1, height + 1
	}
	return 0, 0
}

// decodeDataURL returns the bytes of a base64 `data:` URL. The declared media
// type is ignored; sniffImage decides what the bytes are.
func decodeDataURL(value string) ([]byte, error) {
	header, payload, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return nil, invalid("data_url must be a base64 data: URL")
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > maxImageBytes+3 {
		return nil, invalid("image is too large")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, invalid("data_url is not valid base64")
	}
	return data, nil
}
