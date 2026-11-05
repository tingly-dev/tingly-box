package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/vision/imagefmt"
)

// GIF limits mirror Codex's GifOptions (codex-rs/codex-api/src/images.rs).
const (
	MinGIFFrames          = 2
	MaxGIFFrames          = 16
	MinGIFFrameDurationMS = 20
	MaxGIFFrameDurationMS = 2000
	// MaxGIFImages caps n for GIF requests: each one is a slow, costly upstream call.
	MaxGIFImages = 4
)

// OutputOptions is what a caller asked for beyond the OpenAI SDK's typed
// fields. The SDK request struct keeps output_format but drops the frame
// controls, so the handler parses them separately and hands them to the
// clients through the request context. A zero value means upstream defaults.
type OutputOptions struct {
	Format          imagefmt.Format
	FrameCount      int
	FrameDurationMS int
}

func (o OutputOptions) IsGIF() bool { return o.Format == imagefmt.GIF }

// OutputOptionError is a caller mistake that should become a 400.
type OutputOptionError struct{ Message string }

func (e *OutputOptionError) Error() string { return e.Message }

func outputOptionErrorf(format string, args ...any) error {
	return &OutputOptionError{Message: fmt.Sprintf(format, args...)}
}

// ParseOutputOptionsJSON reads output_format, frame_count and
// frame_duration_ms from a JSON request body. A body that is not JSON yields
// the zero value: the caller's own decoding reports that.
func ParseOutputOptionsJSON(body []byte) (OutputOptions, error) {
	var raw struct {
		OutputFormat    json.RawMessage `json:"output_format"`
		FrameCount      json.RawMessage `json:"frame_count"`
		FrameDurationMS json.RawMessage `json:"frame_duration_ms"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return OutputOptions{}, nil
	}
	var format string
	if len(raw.OutputFormat) > 0 && !bytes.Equal(raw.OutputFormat, []byte("null")) {
		if err := json.Unmarshal(raw.OutputFormat, &format); err != nil {
			return OutputOptions{}, outputOptionErrorf("output_format must be a string")
		}
	}
	frames, err := rawInt("frame_count", raw.FrameCount)
	if err != nil {
		return OutputOptions{}, err
	}
	duration, err := rawInt("frame_duration_ms", raw.FrameDurationMS)
	if err != nil {
		return OutputOptions{}, err
	}
	return newOutputOptions(format, frames, duration)
}

func rawInt(name string, raw json.RawMessage) (int, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, outputOptionErrorf("%s must be an integer", name)
	}
	return n, nil
}

// ParseOutputOptionsForm is the multipart counterpart: each argument is the
// raw form value ("" when absent).
func ParseOutputOptionsForm(format, frameCount, frameDurationMS string) (OutputOptions, error) {
	atoi := func(name, v string) (int, error) {
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, outputOptionErrorf("%s must be an integer", name)
		}
		return n, nil
	}
	frames, err := atoi("frame_count", frameCount)
	if err != nil {
		return OutputOptions{}, err
	}
	duration, err := atoi("frame_duration_ms", frameDurationMS)
	if err != nil {
		return OutputOptions{}, err
	}
	return newOutputOptions(format, frames, duration)
}

func newOutputOptions(format string, frames, durationMS int) (OutputOptions, error) {
	var o OutputOptions
	if strings.TrimSpace(format) != "" {
		f, ok := imagefmt.Parse(format)
		if !ok {
			return OutputOptions{}, outputOptionErrorf("output_format %q is not supported; use png, jpeg, webp or gif", format)
		}
		o.Format = f
	}
	if frames != 0 || durationMS != 0 {
		if !o.IsGIF() {
			return OutputOptions{}, outputOptionErrorf("frame_count and frame_duration_ms only apply to output_format \"gif\"")
		}
	}
	if frames != 0 {
		if frames < MinGIFFrames || frames > MaxGIFFrames {
			return OutputOptions{}, outputOptionErrorf("frame_count must be between %d and %d", MinGIFFrames, MaxGIFFrames)
		}
		o.FrameCount = frames
	}
	if durationMS != 0 {
		if durationMS < MinGIFFrameDurationMS || durationMS > MaxGIFFrameDurationMS {
			return OutputOptions{}, outputOptionErrorf("frame_duration_ms must be between %d and %d", MinGIFFrameDurationMS, MaxGIFFrameDurationMS)
		}
		o.FrameDurationMS = durationMS
	}
	return o, nil
}

// SupportsOutputFormat reports whether a request for format can be served by
// this vendor. Only GIF is gated: the other formats are lenient (an upstream
// that ignores output_format returns what it returns, and the gateway
// reports the sniffed truth). GIF is animation the caller asked for, so a
// vendor that cannot produce it must refuse rather than hand back a still.
//
// GIF is experimental: Codex's own source is the only evidence for it
// (.research/codex-imagegen-gif.md). OpenAI-compatible generation passes it
// through so a Tingly Box in front of a Codex-backed gateway keeps working;
// an upstream that rejects it surfaces its own error.
func (v Vendor) SupportsOutputFormat(f imagefmt.Format, edit bool) bool {
	if f != imagefmt.GIF {
		return true
	}
	switch v {
	case VendorCodex:
		return true
	case VendorOpenAICompat:
		return !edit
	}
	return false
}

type outputOptionsKey struct{}

// WithOutputOptions carries the parsed options to the provider clients,
// keeping the client interface unchanged.
func WithOutputOptions(ctx context.Context, o OutputOptions) context.Context {
	return context.WithValue(ctx, outputOptionsKey{}, o)
}

func OutputOptionsFrom(ctx context.Context) OutputOptions {
	o, _ := ctx.Value(outputOptionsKey{}).(OutputOptions)
	return o
}
