package bot

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

const (
	defaultPageSizeV2 = 8
	stateExpiryV2     = 5 * time.Minute
)

// BindFlowStateV2 represents the state of an ongoing bind flow using the new interaction system
type BindFlowStateV2 struct {
	ChatID       string
	CurrentPath  string
	Page         int
	TotalDirs    int
	PageSize     int
	MessageID    string // Message ID to edit
	ExpiresAt    time.Time
	WaitingInput bool     // Waiting for custom path input
	PromptMsgID  string   // Prompt message ID for cleanup
	Dirs         []string // Current directory list (for navigation by index)
	RequestID    string   // Interaction request ID for response tracking
}

// DirectoryBrowserV2 manages directory navigation for bind flow using the new interaction system
type DirectoryBrowserV2 struct {
	states   map[string]*BindFlowStateV2
	mu       sync.RWMutex
	pageSize int
}

// NewDirectoryBrowserV2 creates a new directory browser with interaction support
func NewDirectoryBrowserV2() *DirectoryBrowserV2 {
	return &DirectoryBrowserV2{
		states:   make(map[string]*BindFlowStateV2),
		pageSize: defaultPageSizeV2,
	}
}

// Start begins a new bind flow for a chat
func (b *DirectoryBrowserV2) Start(chatID string) (*BindFlowStateV2, error) {
	homeDir, err := getHomeDirV2()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	state := &BindFlowStateV2{
		ChatID:      chatID,
		CurrentPath: homeDir,
		Page:        0,
		PageSize:    b.pageSize,
		ExpiresAt:   time.Now().Add(stateExpiryV2),
	}

	b.mu.Lock()
	b.states[chatID] = state
	b.mu.Unlock()

	return state, nil
}

// GetState returns the current state for a chat
func (b *DirectoryBrowserV2) GetState(chatID string) *BindFlowStateV2 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state, ok := b.states[chatID]
	if !ok || time.Now().After(state.ExpiresAt) {
		return nil
	}
	return state
}

// SetMessageID sets the message ID for editing
func (b *DirectoryBrowserV2) SetMessageID(chatID string, messageID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.states[chatID]; ok {
		state.MessageID = messageID
	}
}

// SetRequestID sets the interaction request ID for response tracking
func (b *DirectoryBrowserV2) SetRequestID(chatID string, requestID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.states[chatID]; ok {
		state.RequestID = requestID
	}
}

// Navigate navigates to a subdirectory
func (b *DirectoryBrowserV2) Navigate(chatID string, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", absPath)
	}

	state.CurrentPath = absPath
	state.Page = 0
	state.ExpiresAt = time.Now().Add(stateExpiryV2)

	return nil
}

// NavigateByIndex navigates to a subdirectory by index
func (b *DirectoryBrowserV2) NavigateByIndex(chatID string, index int) error {
	b.mu.RLock()
	state, ok := b.states[chatID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	if index < 0 || index >= len(state.Dirs) {
		return fmt.Errorf("invalid directory index: %d", index)
	}

	return b.Navigate(chatID, state.Dirs[index])
}

// NavigateUp navigates to the parent directory
func (b *DirectoryBrowserV2) NavigateUp(chatID string) error {
	b.mu.RLock()
	state, ok := b.states[chatID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	if !hasParentV2(state.CurrentPath) {
		return fmt.Errorf("already at root directory")
	}

	parentPath := filepath.Dir(state.CurrentPath)
	return b.Navigate(chatID, parentPath)
}

// NextPage moves to the next page of directories
func (b *DirectoryBrowserV2) NextPage(chatID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	dirs, err := listDirectoriesV2(state.CurrentPath)
	if err != nil {
		return err
	}

	totalPages := (len(dirs) + state.PageSize - 1) / state.PageSize
	if state.Page < totalPages-1 {
		state.Page++
		state.ExpiresAt = time.Now().Add(stateExpiryV2)
	}

	return nil
}

// PrevPage moves to the previous page of directories
func (b *DirectoryBrowserV2) PrevPage(chatID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	if state.Page > 0 {
		state.Page--
		state.ExpiresAt = time.Now().Add(stateExpiryV2)
	}

	return nil
}

// Clear removes the state for a chat
func (b *DirectoryBrowserV2) Clear(chatID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.states, chatID)
}

// SetWaitingInput sets the waiting for input state
func (b *DirectoryBrowserV2) SetWaitingInput(chatID string, waiting bool, promptMsgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.states[chatID]; ok {
		state.WaitingInput = waiting
		state.PromptMsgID = promptMsgID
		state.ExpiresAt = time.Now().Add(stateExpiryV2)
	}
}

// IsWaitingInput checks if the chat is waiting for custom path input
func (b *DirectoryBrowserV2) IsWaitingInput(chatID string) bool {
	state := b.GetState(chatID)
	if state == nil {
		return false
	}
	return state.WaitingInput
}

// GetCurrentPath returns the current path for a chat
func (b *DirectoryBrowserV2) GetCurrentPath(chatID string) string {
	state := b.GetState(chatID)
	if state == nil {
		return ""
	}
	return state.CurrentPath
}

// BuildInteractions builds platform-agnostic interactions for directory browsing
func (b *DirectoryBrowserV2) BuildInteractions(chatID string) (*BindFlowStateV2, []imbot.Interaction, string, error) {
	state := b.GetState(chatID)
	if state == nil {
		return nil, nil, "", fmt.Errorf("no active bind flow")
	}

	dirs, err := listDirectoriesV2(state.CurrentPath)
	if err != nil {
		return nil, nil, "", err
	}

	// Store dirs for navigation by index
	state.Dirs = dirs
	state.TotalDirs = len(dirs)

	// Calculate pagination
	totalPages := (len(dirs) + state.PageSize - 1) / state.PageSize
	if totalPages == 0 {
		totalPages = 1
	}

	startIdx := state.Page * state.PageSize
	endIdx := startIdx + state.PageSize
	if endIdx > len(dirs) {
		endIdx = len(dirs)
	}

	// Build interactions using the new builder
	builder := imbot.NewInteractionBuilder()

	// Directory buttons (1-indexed for easy text-based selection)
	for i := startIdx; i < endIdx; i++ {
		dirName := filepath.Base(dirs[i])
		buttonText := formatDirButtonV2(dirName, 20)
		// Use value as the index for navigation
		callbackData := fmt.Sprintf("dir:%d", i)
		builder.AddButton(fmt.Sprintf("dir-%d", i), buttonText, callbackData)
	}

	// Navigation buttons
	if hasParentV2(state.CurrentPath) {
		builder.AddButton("nav-up", "📁 ..", "up")
	}

	if state.Page > 0 {
		builder.AddButton("nav-prev", "◀ Prev", "prev")
	}
	if state.Page < totalPages-1 && len(dirs) > endIdx {
		builder.AddButton("nav-next", "Next ▶", "next")
	}

	// Action buttons
	builder.AddConfirm("select")
	builder.AddButton("custom", "✏️ Custom", "custom")
	builder.AddCancel("cancel")

	// Build message text
	shortPath := state.CurrentPath
	if len(shortPath) > 40 {
		shortPath = "..." + shortPath[len(shortPath)-37:]
	}

	pageInfo := ""
	if totalPages > 1 {
		pageInfo = fmt.Sprintf(" (Page %d/%d)", state.Page+1, totalPages)
	}

	text := fmt.Sprintf("📁 *Current:*\n`%s`\n\n📂 *Select a directory:*%s", shortPath, pageInfo)

	return state, builder.Build(), text, nil
}

// Helper functions for v2

func getHomeDirV2() (string, error) {
	usr, err := user.Current()
	if err == nil && usr.HomeDir != "" {
		return usr.HomeDir, nil
	}

	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		return homeDir, nil
	}

	return os.Getwd()
}

func listDirectoriesV2(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dirs = append(dirs, filepath.Join(path, entry.Name()))
		}
	}

	sort.Strings(dirs)
	return dirs, nil
}

func hasParentV2(path string) bool {
	parent := filepath.Dir(path)
	return parent != path && parent != ""
}

func formatDirButtonV2(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}

// SendDirectoryBrowserV2 sends the directory browser using the new interaction system
func SendDirectoryBrowserV2(
	ctx context.Context,
	handler *BotHandler,
	browser *DirectoryBrowserV2,
	chatID string,
	platform imbot.Platform,
	botUUID string,
	editMessageID string,
) (string, error) {
	state, interactions, text, err := browser.BuildInteractions(chatID)
	if err != nil {
		return "", err
	}

	// Generate a unique request ID for this interaction
	requestID := fmt.Sprintf("bind-browser-%s-%d", chatID, time.Now().UnixNano())
	browser.SetRequestID(chatID, requestID)

	// Get bot
	bot := handler.manager.GetBot(botUUID, platform)
	if bot == nil {
		return "", fmt.Errorf("bot not found")
	}

	// Try to edit existing message if we have a message ID
	if editMessageID != "" && state.MessageID != "" {
		if tgBot, ok := imbot.AsTelegramBot(bot); ok {
			// For Telegram, try to edit the message
			// Get adapter to build markup
			adapter, ok := handler.interaction.GetAdapter(platform)
			if ok {
				markup, err := adapter.BuildMarkup(interactions)
				if err == nil {
					if err := tgBot.EditMessageWithKeyboard(ctx, chatID, editMessageID, text, markup); err == nil {
						return editMessageID, nil
					}
				}
			}
		}
	}

	// Send new message
	result, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
	})
	if err != nil {
		return "", err
	}

	// Store message ID for future edits
	browser.SetMessageID(chatID, result.MessageID)

	return result.MessageID, nil
}

// BuildActionInteractionsV2 builds interactions for action menu (Clear/Bind/Project)
func BuildActionInteractionsV2() []imbot.Interaction {
	builder := imbot.NewInteractionBuilder()
	builder.AddButton("action-clear", "🗑 Clear", "clear")
	builder.AddButton("action-bind", "📁 Bind", "bind")
	builder.AddButton("action-project", "📁 Project", "project")
	return builder.Build()
}

// BuildCancelInteractionsV2 builds interactions for cancel button
func BuildCancelInteractionsV2() []imbot.Interaction {
	builder := imbot.NewInteractionBuilder()
	builder.AddCancel("cancel")
	return builder.Build()
}

// BuildCreateConfirmInteractionsV2 builds interactions for directory creation confirmation
func BuildCreateConfirmInteractionsV2(path string) ([]imbot.Interaction, string) {
	builder := imbot.NewInteractionBuilder()
	builder.AddButton("create", "✅ Create", "create:"+imbot.FormatDirPath(path))
	builder.AddCancel("cancel")
	return builder.Build(), fmt.Sprintf("📁 *The path doesn't exist. Create it?*\n\n`%s`", path)
}

// BuildBindConfirmInteractionsV2 builds interactions for bind confirmation
func BuildBindConfirmInteractionsV2() []imbot.Interaction {
	builder := imbot.NewInteractionBuilder()
	builder.AddConfirm("confirm")
	builder.AddButton("custom", "✏️ Change", "custom")
	builder.AddCancel("cancel")
	return builder.Build()
}

// BuildBindConfirmPromptV2 returns the text for bind confirmation prompt
func BuildBindConfirmPromptV2(proposedPath string) string {
	return fmt.Sprintf("📁 *No project bound.*\n\nBind to current directory?\n\n`%s`", proposedPath)
}

// BuildCustomPathPromptV2 returns the text for custom path input prompt
func BuildCustomPathPromptV2() string {
	return "✏️ *Please type the path you want to bind:*\n\n" +
		"Examples:\n" +
		"• my-project (relative to current)\n" +
		"• ~/workspace/new-project\n" +
		"• /home/user/my-project\n\n" +
		"The directory will be created if it doesn't exist.\n\n" +
		"Type your path or click Cancel below."
}

// ValidateProjectPathV2 checks if the path exists and is accessible
func ValidateProjectPathV2(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// ExpandPathV2 expands ~ to home directory and returns absolute path
func ExpandPathV2(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot get absolute path: %w", err)
	}

	return absPath, nil
}
