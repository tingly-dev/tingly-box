// Package vision holds the vision-channel vocabulary and the canonical probe
// image fixture shared by the probe subsystem, the protocol test harness
// (internal/protocoltest content_shapes), and future vision health checks.
// One source of truth keeps the shapes the harness proves identical to the
// shapes the probes send — see .design/multimodal-content.md.
//
// It mirrors internal/protocol/thinking: a protocol-level capability ladder
// that probe-facing types alias instead of redefining.
package visionproxy

// Canonical probe image: a 256×256 solid-red PNG (the same dimensions as the
// issue #1606 reproduction). Large enough to pass providers' minimum-size
// validation — degenerate 1×1 images are rejected by several vision
// endpoints — while compressing to a few hundred bytes, and unambiguous
// enough that a vision-capable model answers Prompt with "red" — so a human
// reading the probe result can tell working vision ("red") from a silent
// image drop ("I don't see any image").
const (
	// FixtureMediaType is the MIME type of the fixture image.
	FixtureMediaType = "image/png"
	// FixturePNGBase64 is the base64 payload of the 256×256 red PNG.
	FixturePNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAQAAAAEACAIAAADTED8xAAAB+0lEQVR42u3TQQkAAAjAwPUvrX8reHAJBmsK3pIAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwABgADAAGAAMAAYAA4ABwAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgAJMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAyAASTAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAYAAwABgADgAHAAGAAMAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADAAGAAOAAcAAYAAwABgADADHAllMDvLkz2XNAAAAAElFTkSuQmCC"
	// FixtureDataURL is the same image as an OpenAI-style data URL.
	FixtureDataURL = "data:" + FixtureMediaType + ";base64," + FixturePNGBase64

	// Prompt is the one-word question asked about the fixture image.
	Prompt = "What color is this image? Answer with one word."

	// ToolUserText, ToolName, ToolCallID, and ToolResultText script the
	// tool-channel turn: user ask → assistant calls ToolName → tool result
	// returns ToolResultText plus the image.
	ToolUserText   = "Analyze the image using the capture tool."
	ToolName       = "vision_capture"
	ToolCallID     = "call_vision_1"
	ToolResultText = "Image captured. " + Prompt
)
