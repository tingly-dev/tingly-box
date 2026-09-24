package imagegen

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
)

// maskEditAlphaThreshold splits the OpenAI mask into edit and keep: a pixel
// whose alpha is below it counts as transparent (edit). The Playground's masks
// are binary, so any mid value works; half keeps antialiased brush edges on
// the side they mostly belong to.
const maskEditAlphaThreshold = 0x80

// whiteEditMask converts an OpenAI mask (transparent = edit, opaque = keep)
// into the black-and-white form Qianfan and DashScope Wanx expect: white
// (255) = edit, black (0) = keep, same pixel size, as a PNG.
//
// The two conventions are opposite in more than colour — OpenAI reads only the
// alpha channel, these vendors read only the luminance — so passing the
// OpenAI mask through unchanged would edit exactly the region the user
// protected.
func whiteEditMask(openaiMask []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(openaiMask))
	if err != nil {
		return nil, fmt.Errorf("imagegen: mask must be a PNG: %w", err)
	}
	b := src.Bounds()
	out := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA() // 16-bit alpha
			if a>>8 < maskEditAlphaThreshold {
				out.SetGray(x, y, color.Gray{Y: 0xff})
			} else {
				out.SetGray(x, y, color.Gray{Y: 0x00})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, fmt.Errorf("imagegen: encode mask: %w", err)
	}
	return buf.Bytes(), nil
}
