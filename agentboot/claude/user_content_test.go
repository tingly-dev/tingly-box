package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

func TestWireContentPlainPrompt(t *testing.T) {
	// No blocks: the content stays the plain string, as before.
	assert.Equal(t, "hi", wireContent("hi", nil))
	assert.Equal(t, "", wireContent("", nil))
}

func TestWireContentPromptThenImages(t *testing.T) {
	wire := wireContent("look", []agentboot.ContentBlock{
		agentboot.ImageContent("image/png", "AAAA"),
	})
	blocks, ok := wire.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 2)
	assert.Equal(t, map[string]any{"type": "text", "text": "look"}, blocks[0])
	img := blocks[1].(map[string]any)
	assert.Equal(t, "image", img["type"])
	src := img["source"].(map[string]any)
	assert.Equal(t, "base64", src["type"])
	assert.Equal(t, "image/png", src["media_type"])
	assert.Equal(t, "AAAA", src["data"])
}

func TestWireContentTextBlockIsNotDuplicated(t *testing.T) {
	wire := wireContent("look", []agentboot.ContentBlock{agentboot.TextContent("look")}).([]any)
	require.Len(t, wire, 1)
	assert.Equal(t, map[string]any{"type": "text", "text": "look"}, wire[0])
}
