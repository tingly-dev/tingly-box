package smart_guide

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// DetectMediaType tests
// ============================================================================

func TestDetectMediaType_ImageExtensions(t *testing.T) {
	imageExts := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	for _, ext := range imageExts {
		result := DetectMediaType("file" + ext)
		assert.Equal(t, "image", result, "extension %s should be image", ext)
	}
}

func TestDetectMediaType_ImageExtensionsCaseInsensitive(t *testing.T) {
	imageExts := []string{".JPG", ".JPEG", ".PNG", ".GIF", ".WEBP"}
	for _, ext := range imageExts {
		result := DetectMediaType("file" + ext)
		assert.Equal(t, "image", result, "uppercase extension %s should be image", ext)
	}
}

func TestDetectMediaType_DocumentExtensions(t *testing.T) {
	docPaths := []string{
		"file.txt", "file.pdf", "file.go", "file.zip",
		"file.md", "file.csv", "file.json", "file.log",
		"file.docx", "file.xlsx", "file.tar.gz",
	}
	for _, p := range docPaths {
		result := DetectMediaType(p)
		assert.Equal(t, "document", result, "path %s should be document", p)
	}
}

func TestDetectMediaType_NoExtension(t *testing.T) {
	result := DetectMediaType("Makefile")
	assert.Equal(t, "document", result)
}

// ============================================================================
// SendFileTool core behavior tests
// ============================================================================

// makeTempProjectDir creates a temp dir to act as project path, returns cleanup func.
func makeTempProjectDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sg-project-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// writeFile creates a file with given content inside dir, returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

// makeSendFileTool creates a SendFileTool with a mock executor and toolCtx.
func makeSendFileTool(
	executor *ToolExecutor,
	toolCtx *ToolContext,
) *SendFileTool {
	return NewSendFileTool(executor, toolCtx)
}

func TestSendFileTool_Name(t *testing.T) {
	tool := makeSendFileTool(NewToolExecutor(nil), &ToolContext{})
	assert.Equal(t, "send_file", tool.Name())
}

func TestSendFileTool_DescriptionMentionsFilePath(t *testing.T) {
	tool := makeSendFileTool(NewToolExecutor(nil), &ToolContext{})
	assert.Contains(t, strings.ToLower(tool.Description()), "file")
}

func TestSendFileTool_HappyPath_InProjectPath(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	filePath := writeFile(t, projectDir, "report.txt", "hello world")

	var sentPath, sentCaption string
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			sentPath = path
			sentCaption = caption
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: filePath, Caption: "Here you go"})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)

	assert.Equal(t, filePath, sentPath)
	assert.Equal(t, "Here you go", sentCaption)
	assert.Contains(t, text, "sent")
}

func TestSendFileTool_HappyPath_RelativePath(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	writeFile(t, projectDir, "output.csv", "a,b,c")

	var sentPath string
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			sentPath = path
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: "output.csv"})
	require.NoError(t, err)
	_ = extractTextFromResponse(resp)

	// Relative path should be resolved to absolute
	assert.Equal(t, filepath.Join(projectDir, "output.csv"), sentPath)
}

func TestSendFileTool_HappyPath_ImageDetected(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	filePath := writeFile(t, projectDir, "photo.png", "PNG_DATA")

	var sentPath string
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			sentPath = path
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: filePath})
	require.NoError(t, err)
	_ = extractTextFromResponse(resp)
	assert.Equal(t, filePath, sentPath)
}

func TestSendFileTool_FileNotFound(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: filepath.Join(projectDir, "does_not_exist.txt")})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "error")
}

func TestSendFileTool_PathIsDirectory(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: projectDir})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "error")
	assert.NotContains(t, text, "✅") // should NOT show success checkmark
}

func TestSendFileTool_EmptyFilePath(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error { return nil },
	}
	executor := NewToolExecutor(nil)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: ""})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "error")
}

func TestSendFileTool_FileTooLarge(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	filePath := writeFile(t, projectDir, "big.bin", "data")

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error { return nil },
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)

	// Create tool with a very small size limit for testing
	tool := NewSendFileToolWithLimit(executor, toolCtx, 3) // 3 bytes max
	resp, err := tool.Call(ctx, SendFileParams{FilePath: filePath})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "too large")
}

func TestSendFileTool_CrossPath_ApprovalGranted(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)

	// File OUTSIDE projectDir
	outsideDir := makeTempProjectDir(t)
	outsideFile := writeFile(t, outsideDir, "outside.txt", "secret")

	approvalCalled := false
	var sentPath string
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			sentPath = path
			return nil
		},
		RequestApproval: func(ctx context.Context, prompt string) (bool, error) {
			approvalCalled = true
			return true, nil // grant
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: outsideFile})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)

	assert.True(t, approvalCalled, "approval should be requested for cross-path file")
	assert.Equal(t, outsideFile, sentPath)
	assert.Contains(t, text, "sent")
}

func TestSendFileTool_CrossPath_ApprovalDenied(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)

	outsideDir := makeTempProjectDir(t)
	outsideFile := writeFile(t, outsideDir, "outside.txt", "secret")

	sendCalled := false
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			sendCalled = true
			return nil
		},
		RequestApproval: func(ctx context.Context, prompt string) (bool, error) {
			return false, nil // deny
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: outsideFile})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)

	assert.False(t, sendCalled, "SendFile should not be called when approval is denied")
	assert.Contains(t, strings.ToLower(text), "denied")
}

func TestSendFileTool_CrossPath_NoApprovalCallback(t *testing.T) {
	// When there is no RequestApproval callback, cross-path sends are denied
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	outsideDir := makeTempProjectDir(t)
	outsideFile := writeFile(t, outsideDir, "outside.txt", "secret")

	sendCalled := false
	toolCtx := &ToolContext{
		ProjectPath:     projectDir,
		RequestApproval: nil, // no callback
		SendFile: func(ctx context.Context, path, caption string) error {
			sendCalled = true
			return nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: outsideFile})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)

	assert.False(t, sendCalled)
	assert.Contains(t, strings.ToLower(text), "error")
}

func TestSendFileTool_CrossPath_ApprovalPromptContainsPath(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	outsideDir := makeTempProjectDir(t)
	outsideFile := writeFile(t, outsideDir, "secret.txt", "data")

	var capturedPrompt string
	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile:    func(ctx context.Context, path, caption string) error { return nil },
		RequestApproval: func(ctx context.Context, prompt string) (bool, error) {
			capturedPrompt = prompt
			return true, nil
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	_, err := tool.Call(ctx, SendFileParams{FilePath: outsideFile})
	require.NoError(t, err)

	// Approval prompt should mention the file path
	assert.Contains(t, capturedPrompt, outsideFile)
}

func TestSendFileTool_NoSendFileCallback(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	filePath := writeFile(t, projectDir, "file.txt", "data")

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile:    nil, // no callback configured
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: filePath})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "error")
}

func TestSendFileTool_SendFileCallbackError(t *testing.T) {
	ctx := context.Background()
	projectDir := makeTempProjectDir(t)
	filePath := writeFile(t, projectDir, "file.txt", "data")

	toolCtx := &ToolContext{
		ProjectPath: projectDir,
		SendFile: func(ctx context.Context, path, caption string) error {
			return errors.New("platform upload failed")
		},
	}
	executor := NewToolExecutor(nil)
	executor.SetWorkingDirectory(projectDir)
	tool := makeSendFileTool(executor, toolCtx)

	resp, err := tool.Call(ctx, SendFileParams{FilePath: filePath})
	require.NoError(t, err)
	text := extractTextFromResponse(resp)
	assert.Contains(t, strings.ToLower(text), "error")
	assert.Contains(t, text, "platform upload failed")
}
