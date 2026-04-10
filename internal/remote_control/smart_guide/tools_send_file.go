package smart_guide

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

// SendFileMaxSize is the default maximum file size for outbound file sends (50MB).
const SendFileMaxSize int64 = 50 * 1024 * 1024

// imageExtensions is the set of file extensions treated as images.
var imageExtensions = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
	".webp": {},
}

// DetectMediaType returns "image" for image file extensions, "document" for all others.
func DetectMediaType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := imageExtensions[ext]; ok {
		return "image"
	}
	return "document"
}

// ============================================================================
// SendFileTool
// ============================================================================

// SendFileParams defines the parameters for the send_file tool.
type SendFileParams struct {
	FilePath string `json:"file_path" jsonschema:"description=Path to the local file to send (absolute or relative to working directory)"`
	Caption  string `json:"caption,omitempty" jsonschema:"description=Optional caption or message to accompany the file"`
}

// SendFileTool sends a local file to the user via the IM bot.
type SendFileTool struct {
	executor *ToolExecutor
	toolCtx  *ToolContext
	maxSize  int64
}

// NewSendFileTool creates a SendFileTool with the default 50MB limit.
func NewSendFileTool(executor *ToolExecutor, toolCtx *ToolContext) *SendFileTool {
	return &SendFileTool{
		executor: executor,
		toolCtx:  toolCtx,
		maxSize:  SendFileMaxSize,
	}
}

// NewSendFileToolWithLimit creates a SendFileTool with a custom size limit (for testing).
func NewSendFileToolWithLimit(executor *ToolExecutor, toolCtx *ToolContext, maxSize int64) *SendFileTool {
	return &SendFileTool{
		executor: executor,
		toolCtx:  toolCtx,
		maxSize:  maxSize,
	}
}

// Name returns the tool name.
func (t *SendFileTool) Name() string {
	return "send_file"
}

// Description returns the tool description.
func (t *SendFileTool) Description() string {
	return `Send a local file to the user via the messaging platform.

The file path can be absolute or relative to the current working directory.
Files inside the project path are sent directly. Files outside require explicit user approval.

Examples:
- Send a report: file_path="output/report.pdf", caption="Here is your analysis"
- Send a chart: file_path="chart.png"
- Send an archive: file_path="/tmp/export.zip", caption="Exported data"`
}

// Call executes the send_file tool.
func (t *SendFileTool) Call(ctx context.Context, params SendFileParams) (*tool.ToolResponse, error) {
	// 1. Validate file_path is provided
	if params.FilePath == "" {
		return tool.TextResponse("Error: 'file_path' parameter is required"), nil
	}

	// 2. Check SendFile callback is available
	if t.toolCtx == nil || t.toolCtx.SendFile == nil {
		return tool.TextResponse("Error: file sending is not available in this context"), nil
	}

	// 3. Resolve to absolute path
	absPath := t.executor.ResolvePath(params.FilePath)

	// 4. Stat the file
	info, err := os.Stat(absPath)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: cannot access file '%s': %v", absPath, err)), nil
	}

	// 5. Must be a regular file
	if !info.Mode().IsRegular() {
		return tool.TextResponse(fmt.Sprintf("Error: '%s' is not a regular file (directories cannot be sent)", absPath)), nil
	}

	// 6. Check size
	if info.Size() > t.maxSize {
		return tool.TextResponse(fmt.Sprintf("Error: file too large (%d bytes, max %d bytes)", info.Size(), t.maxSize)), nil
	}

	// 7. Security check: if file is outside project path, require user approval
	projectPath := ""
	if t.toolCtx != nil {
		projectPath = t.toolCtx.ProjectPath
	}

	if projectPath != "" && !isUnderPath(absPath, projectPath) {
		approved, err := t.requestCrossPathApproval(ctx, absPath, info.Size())
		if err != nil {
			return tool.TextResponse(fmt.Sprintf("Error: approval request failed: %v", err)), nil
		}
		if !approved {
			return tool.TextResponse(fmt.Sprintf("Error: sending file '%s' was denied by user (file is outside project path)", absPath)), nil
		}
	}

	// 8. Send the file
	if err := t.toolCtx.SendFile(ctx, absPath, params.Caption); err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: failed to send file: %v", err)), nil
	}

	mediaType := DetectMediaType(absPath)
	return tool.TextResponse(fmt.Sprintf("✅ File sent successfully: %s (%s, %d bytes)", filepath.Base(absPath), mediaType, info.Size())), nil
}

// requestCrossPathApproval requests user approval for sending a file outside the project path.
// Returns (false, nil) if no approval callback is configured (deny by default).
func (t *SendFileTool) requestCrossPathApproval(ctx context.Context, absPath string, size int64) (bool, error) {
	if t.toolCtx == nil || t.toolCtx.RequestApproval == nil {
		return false, nil
	}

	prompt := fmt.Sprintf(
		"Send file outside project path?\n\nFile: %s\nSize: %d bytes\n\nThis file is outside the bound project directory. Allow sending?",
		absPath, size,
	)
	return t.toolCtx.RequestApproval(ctx, prompt)
}

// isUnderPath reports whether target is under (or equal to) base.
func isUnderPath(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	// If rel starts with "..", target is outside base
	return !strings.HasPrefix(rel, "..")
}
