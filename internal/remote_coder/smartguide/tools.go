package smartguide

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

// ToolContext provides context for tool execution
type ToolContext struct {
	ChatID      string
	ProjectPath string
	SessionID   string
}

// ToolExecutor handles tool execution with proper context
type ToolExecutor struct {
	bashAllowlist map[string]struct{}
	bashCwd       string // Per-execution working directory
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(allowlist []string) *ToolExecutor {
	allowlistMap := make(map[string]struct{})
	for _, cmd := range allowlist {
		allowlistMap[strings.ToLower(cmd)] = struct{}{}
	}

	return &ToolExecutor{
		bashAllowlist: allowlistMap,
		bashCwd:       "", // Start in current directory
	}
}

// SetWorkingDirectory sets the current working directory
func (e *ToolExecutor) SetWorkingDirectory(cwd string) {
	e.bashCwd = cwd
}

// GetWorkingDirectory returns the current working directory
func (e *ToolExecutor) GetWorkingDirectory() string {
	if e.bashCwd == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "/"
	}
	return e.bashCwd
}

// ExecuteBash executes a bash command with allowlist checking
func (e *ToolExecutor) ExecuteBash(ctx context.Context, cmd string, args ...string) (string, error) {
	// Check if command is allowed
	cmdLower := strings.ToLower(cmd)
	if _, allowed := e.bashAllowlist[cmdLower]; !allowed {
		return "", fmt.Errorf("command '%s' is not allowed. Allowed commands: %v",
			cmd, e.GetAllowedCommands())
	}

	// Build full command
	fullCmd := append([]string{cmd}, args...)

	// Create command execution
	execCmd := exec.CommandContext(ctx, fullCmd[0], fullCmd[1:]...)

	// Set working directory
	if e.bashCwd != "" {
		execCmd.Dir = e.bashCwd
	}

	// Execute and capture output
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return string(output), nil
}

// GetAllowedCommands returns the list of allowed commands
func (e *ToolExecutor) GetAllowedCommands() []string {
	cmds := make([]string, 0, len(e.bashAllowlist))
	for cmd := range e.bashAllowlist {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// ============================================================================
// Internal Tools
// ============================================================================

// GetStatusTool returns the current bot status
type GetStatusTool struct {
	executor *ToolExecutor
	getStatusFunc func(chatID string) (*StatusInfo, error)
}

// StatusInfo holds bot status information
type StatusInfo struct {
	CurrentAgent   string `json:"current_agent"`
	SessionID      string `json:"session_id"`
	ProjectPath    string `json:"project_path"`
	WorkingDir     string `json:"working_dir"`
	HasRunningTask bool   `json:"has_running_task"`
	Whitelisted    bool   `json:"whitelisted"`
}

// NewGetStatusTool creates a new GetStatusTool
func NewGetStatusTool(executor *ToolExecutor, getStatusFunc func(chatID string) (*StatusInfo, error)) *GetStatusTool {
	return &GetStatusTool{
		executor:      executor,
		getStatusFunc: getStatusFunc,
	}
}

// Call implements the tool interface
func (t *GetStatusTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	chatID, _ := kwargs["chat_id"].(string)

	if t.getStatusFunc == nil {
		return tool.TextResponse("Status function not configured"), nil
	}

	status, err := t.getStatusFunc(chatID)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error getting status: %v", err)), nil
	}

	// Format status response
	response := fmt.Sprintf("**Current Status:**\n"+
		"• Agent: %s\n"+
		"• Session: %s\n"+
		"• Project: %s\n"+
		"• Working Directory: %s\n"+
		"• Whitelisted: %v",
		status.CurrentAgent,
		status.SessionID,
		status.ProjectPath,
		status.WorkingDir,
		status.Whitelisted,
	)

	return tool.TextResponse(response), nil
}

// GetProjectTool returns the current project binding info
type GetProjectTool struct {
	executor *ToolExecutor
	getProjectFunc func(chatID string) (string, bool, error)
}

// NewGetProjectTool creates a new GetProjectTool
func NewGetProjectTool(executor *ToolExecutor, getProjectFunc func(chatID string) (string, bool, error)) *GetProjectTool {
	return &GetProjectTool{
		executor:       executor,
		getProjectFunc: getProjectFunc,
	}
}

// Call implements the tool interface
func (t *GetProjectTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	chatID, _ := kwargs["chat_id"].(string)

	if t.getProjectFunc == nil {
		return tool.TextResponse("Project function not configured"), nil
	}

	projectPath, ok, err := t.getProjectFunc(chatID)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error getting project: %v", err)), nil
	}

	if !ok {
		return tool.TextResponse("No project bound to this chat."), nil
	}

	return tool.TextResponse(fmt.Sprintf("Current project: %s", projectPath)), nil
}

// ============================================================================
// External Tools (Bash)
// ============================================================================

// BashCDTool changes directory
type BashCDTool struct {
	executor       *ToolExecutor
	updateProjectFunc func(chatID string, projectPath string) error // Optional: updates project path in chat store
}

// NewBashCDTool creates a new BashCDTool
func NewBashCDTool(executor *ToolExecutor, updateProjectFunc func(chatID string, projectPath string) error) *BashCDTool {
	return &BashCDTool{
		executor:         executor,
		updateProjectFunc: updateProjectFunc,
	}
}

// Call implements the tool interface
func (t *BashCDTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	path, ok := kwargs["path"].(string)
	if !ok {
		return tool.TextResponse("Error: 'path' parameter is required"), nil
	}

	chatID, _ := kwargs["chat_id"].(string)

	// Resolve path
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.executor.GetWorkingDirectory(), path)
	}

	// Check if directory exists
	info, err := os.Stat(path)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: %v", err)), nil
	}
	if !info.IsDir() {
		return tool.TextResponse(fmt.Sprintf("Error: '%s' is not a directory", path)), nil
	}

	// Update working directory
	t.executor.SetWorkingDirectory(path)

	// Also update project path in chat store if callback provided and chatID is set
	if t.updateProjectFunc != nil && chatID != "" {
		if err := t.updateProjectFunc(chatID, path); err != nil {
			logrus.WithError(err).Warn("Failed to update project path in chat store")
		}
	}

	return tool.TextResponse(fmt.Sprintf("Changed directory to: %s", path)), nil
}

// BashLSTool lists directory contents
type BashLSTool struct {
	executor *ToolExecutor
}

// NewBashLSTool creates a new BashLSTool
func NewBashLSTool(executor *ToolExecutor) *BashLSTool {
	return &BashLSTool{executor: executor}
}

// Call implements the tool interface
func (t *BashLSTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	path := t.executor.GetWorkingDirectory()
	if p, ok := kwargs["path"].(string); ok && p != "" {
		path = p
		if !filepath.IsAbs(path) {
			path = filepath.Join(t.executor.GetWorkingDirectory(), path)
		}
	}

	// Execute ls
	output, err := t.executor.ExecuteBash(ctx, "ls", "-la", path)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: %v", err)), nil
	}

	return tool.TextResponse(fmt.Sprintf("Directory listing for %s:\n%s", path, output)), nil
}

// BashPWDTool prints working directory
type BashPWDTool struct {
	executor *ToolExecutor
}

// NewBashPWDTool creates a new BashPWDTool
func NewBashPWDTool(executor *ToolExecutor) *BashPWDTool {
	return &BashPWDTool{executor: executor}
}

// Call implements the tool interface
func (t *BashPWDTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	cwd := t.executor.GetWorkingDirectory()
	return tool.TextResponse(fmt.Sprintf("Current directory: %s", cwd)), nil
}

// GitCloneTool clones a git repository
type GitCloneTool struct {
	executor *ToolExecutor
}

// NewGitCloneTool creates a new GitCloneTool
func NewGitCloneTool(executor *ToolExecutor) *GitCloneTool {
	return &GitCloneTool{executor: executor}
}

// Call implements the tool interface
func (t *GitCloneTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	repo, ok := kwargs["repo"].(string)
	if !ok {
		return tool.TextResponse("Error: 'repo' parameter is required"), nil
	}

	dest, ok := kwargs["dest"].(string)
	if !ok {
		// Use repo name as default destination
		parts := strings.Split(repo, "/")
		dest = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	// Resolve destination path
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(t.executor.GetWorkingDirectory(), dest)
	}

	// Check if destination already exists
	if _, err := os.Stat(dest); err == nil {
		return tool.TextResponse(fmt.Sprintf("Error: '%s' already exists", dest)), nil
	}

	logrus.WithField("repo", repo).WithField("dest", dest).Info("Cloning repository")

	// Clone the repository
	// Note: git is not in the default allowlist, so we need special handling
	// For now, we'll use os/exec directly
	cmd := exec.CommandContext(ctx, "git", "clone", repo, dest)
	cmd.Dir = t.executor.GetWorkingDirectory()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Clone failed: %v\n%s", err, string(output))), nil
	}

	return tool.TextResponse(fmt.Sprintf("Successfully cloned %s to %s", repo, dest)), nil
}

// GitStatusTool shows git repository status
type GitStatusTool struct {
	executor *ToolExecutor
}

// NewGitStatusTool creates a new GitStatusTool
func NewGitStatusTool(executor *ToolExecutor) *GitStatusTool {
	return &GitStatusTool{executor: executor}
}

// Call implements the tool interface
func (t *GitStatusTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	cwd := t.executor.GetWorkingDirectory()

	// Check if we're in a git repo
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = cwd
	if err := cmd.Run(); err != nil {
		return tool.TextResponse(fmt.Sprintf("Not a git repository: %s", cwd)), nil
	}

	// Get git status
	cmd = exec.CommandContext(ctx, "git", "status", "-sb")
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: %v", err)), nil
	}

	return tool.TextResponse(fmt.Sprintf("Git status:\n%s", string(output))), nil
}

// ============================================================================
// Tool Registration Helper
// ============================================================================

// RegisterTools registers all smart guide tools with a toolkit
func RegisterTools(toolkit *tool.Toolkit, executor *ToolExecutor,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	getProjectFunc func(chatID string) (string, bool, error),
	updateProjectFunc func(chatID string, projectPath string) error) error {

	// Create tool groups first
	if err := toolkit.CreateToolGroup("internal", "Internal tools for bot status and project management", true, ""); err != nil {
		return fmt.Errorf("failed to create internal tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("bash", "Bash command tools for file system operations", true, ""); err != nil {
		return fmt.Errorf("failed to create bash tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("git", "Git version control tools", true, ""); err != nil {
		return fmt.Errorf("failed to create git tool group: %w", err)
	}

	// Internal tools
	getStatusTool := NewGetStatusTool(executor, getStatusFunc)
	if err := toolkit.Register(getStatusTool, &tool.RegisterOptions{
		GroupName: "internal",
	}); err != nil {
		return fmt.Errorf("failed to register get_status tool: %w", err)
	}

	getProjectTool := NewGetProjectTool(executor, getProjectFunc)
	if err := toolkit.Register(getProjectTool, &tool.RegisterOptions{
		GroupName: "internal",
	}); err != nil {
		return fmt.Errorf("failed to register get_project tool: %w", err)
	}

	// Bash tools
	cdTool := NewBashCDTool(executor, updateProjectFunc)
	if err := toolkit.Register(cdTool, &tool.RegisterOptions{
		GroupName: "bash",
	}); err != nil {
		return fmt.Errorf("failed to register cd tool: %w", err)
	}

	lsTool := NewBashLSTool(executor)
	if err := toolkit.Register(lsTool, &tool.RegisterOptions{
		GroupName: "bash",
	}); err != nil {
		return fmt.Errorf("failed to register ls tool: %w", err)
	}

	pwdTool := NewBashPWDTool(executor)
	if err := toolkit.Register(pwdTool, &tool.RegisterOptions{
		GroupName: "bash",
	}); err != nil {
		return fmt.Errorf("failed to register pwd tool: %w", err)
	}

	// Git tools
	cloneTool := NewGitCloneTool(executor)
	if err := toolkit.Register(cloneTool, &tool.RegisterOptions{
		GroupName: "git",
	}); err != nil {
		return fmt.Errorf("failed to register git_clone tool: %w", err)
	}

	statusTool := NewGitStatusTool(executor)
	if err := toolkit.Register(statusTool, &tool.RegisterOptions{
		GroupName: "git",
	}); err != nil {
		return fmt.Errorf("failed to register git_status tool: %w", err)
	}

	logrus.Info("Smart guide tools registered successfully")
	return nil
}
