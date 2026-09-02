package client

import (
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

// mockTransport is a minimal http.RoundTripper for testing.
type mockTransport struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

// Pinned client identity: every value here was captured from the official
// 2.1.258 native binary (see .design/claude-code-client-compat.md §2).
func TestClaudeSDKHeaders(t *testing.T) {
	assert.Equal(t, "claude-cli/2.1.258 (external, cli)", claudeCLIUserAgent)
	assert.Equal(t, "claude-cli/"+constant.ClaudeCodeVersion+" (external, cli)", claudeCLIUserAgent)
	assert.Equal(t, "v26.3.0", stainlessRuntimeVersion)
	assert.Equal(t, "0.112.1", stainlessPackageVersion)
	assert.Equal(t, "node", stainlessRuntime)
	assert.Equal(t, "js", stainlessLang)
	assert.Equal(t, "0", stainlessRetryCount)
	assert.Equal(t, "cli", claudeXApp)
	assert.Equal(t, "600", stainlessTimeout)
	assert.Equal(t, "2023-06-01", anthropicVersion)
}

// The JS SDK maps process.platform / process.arch to display names; the Go
// runtime names must be translated, not forwarded.
func TestStainlessOSArchNames(t *testing.T) {
	assert.Equal(t, "MacOS", stainlessOSName("darwin"))
	assert.Equal(t, "Linux", stainlessOSName("linux"))
	assert.Equal(t, "Windows", stainlessOSName("windows"))
	assert.Equal(t, "FreeBSD", stainlessOSName("freebsd"))
	assert.Equal(t, "Other:plan9", stainlessOSName("plan9"))
	assert.Equal(t, "Unknown", stainlessOSName(""))

	assert.Equal(t, "x64", stainlessArchName("amd64"))
	assert.Equal(t, "arm64", stainlessArchName("arm64"))
	assert.Equal(t, "x32", stainlessArchName("386"))
	assert.Equal(t, "arm", stainlessArchName("arm"))
	assert.Equal(t, "other:riscv64", stainlessArchName("riscv64"))
	assert.Equal(t, "unknown", stainlessArchName(""))
}

func TestRestoreToolNamesInMessage(t *testing.T) {
	t.Run("restores tool_use name", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", Name: "Bash"},
			},
		}
		restoreToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "bash", msg.Content[0].Name)
	})

	t.Run("noop for nil reverseMap", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "tool_use", Name: "Bash"},
			},
		}
		restoreToolNamesInMessage(msg, nil)
		assert.Equal(t, "Bash", msg.Content[0].Name)
	})

	t.Run("does not touch non-tool_use blocks", func(t *testing.T) {
		msg := &anthropic.Message{
			Content: []anthropic.ContentBlockUnion{
				{Type: "text", Name: ""},
			},
		}
		restoreToolNamesInMessage(msg, map[string]string{"Bash": "bash"})
		assert.Equal(t, "", msg.Content[0].Name)
	})
}
