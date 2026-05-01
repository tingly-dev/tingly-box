package telegram

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

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
)

const (
	defaultPageSize = 8
	stateExpiry     = 5 * time.Minute
)

// BindFlowState represents the state of an ongoing bind flow
type BindFlowState struct {
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
}

// DirectoryBrowser manages directory navigation for bind flow
type DirectoryBrowser struct {
	states   map[string]*BindFlowState
	mu       sync.RWMutex
	pageSize int
}

// NewDirectoryBrowser creates a new directory browser
func NewDirectoryBrowser() *DirectoryBrowser {
	return &DirectoryBrowser{
		states:   make(map[string]*BindFlowState),
		pageSize: defaultPageSize,
	}
}

// Start begins a new bind flow for a chat
func (b *DirectoryBrowser) Start(chatID string) (*BindFlowState, error) {
	homeDir, err := getHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	state := &BindFlowState{
		ChatID:      chatID,
		CurrentPath: homeDir,
		Page:        0,
		PageSize:    b.pageSize,
		ExpiresAt:   time.Now().Add(stateExpiry),
	}

	b.mu.Lock()
	b.states[chatID] = state
	b.mu.Unlock()

	return state, nil
}

// GetState returns the current state for a chat
func (b *DirectoryBrowser) GetState(chatID string) *BindFlowState {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state, ok := b.states[chatID]
	if !ok || time.Now().After(state.ExpiresAt) {
		return nil
	}
	return state
}

// SetMessageID sets the message ID for editing
func (b *DirectoryBrowser) SetMessageID(chatID string, messageID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.states[chatID]; ok {
		state.MessageID = messageID
	}
}

// Navigate navigates to a subdirectory
func (b *DirectoryBrowser) Navigate(chatID string, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	// Validate path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", absPath)
	}

	state.CurrentPath = absPath
	state.Page = 0
	state.ExpiresAt = time.Now().Add(stateExpiry)

	return nil
}

// NavigateByIndex navigates to a subdirectory by index (stored in state.Dirs)
func (b *DirectoryBrowser) NavigateByIndex(chatID string, index int) error {
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
func (b *DirectoryBrowser) NavigateUp(chatID string) error {
	b.mu.RLock()
	state, ok := b.states[chatID]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	if !hasParent(state.CurrentPath) {
		return fmt.Errorf("already at root directory")
	}

	parentPath := filepath.Dir(state.CurrentPath)
	return b.Navigate(chatID, parentPath)
}

// NextPage moves to the next page of directories
func (b *DirectoryBrowser) NextPage(chatID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	dirs, err := listDirectories(state.CurrentPath)
	if err != nil {
		return err
	}

	totalPages := (len(dirs) + state.PageSize - 1) / state.PageSize
	if state.Page < totalPages-1 {
		state.Page++
		state.ExpiresAt = time.Now().Add(stateExpiry)
	}

	return nil
}

// PrevPage moves to the previous page of directories
func (b *DirectoryBrowser) PrevPage(chatID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	state, ok := b.states[chatID]
	if !ok {
		return fmt.Errorf("no active bind flow")
	}

	if state.Page > 0 {
		state.Page--
		state.ExpiresAt = time.Now().Add(stateExpiry)
	}

	return nil
}

// Clear removes the state for a chat
func (b *DirectoryBrowser) Clear(chatID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.states, chatID)
}

// SetWaitingInput sets the waiting for input state
func (b *DirectoryBrowser) SetWaitingInput(chatID string, waiting bool, promptMsgID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if state, ok := b.states[chatID]; ok {
		state.WaitingInput = waiting
		state.PromptMsgID = promptMsgID
		state.ExpiresAt = time.Now().Add(stateExpiry)
	}
}

// IsWaitingInput checks if the chat is waiting for custom path input
func (b *DirectoryBrowser) IsWaitingInput(chatID string) bool {
	state := b.GetState(chatID)
	if state == nil {
		return false
	}
	return state.WaitingInput
}

// GetCurrentPath returns the current path for a chat
func (b *DirectoryBrowser) GetCurrentPath(chatID string) string {
	state := b.GetState(chatID)
	if state == nil {
		return ""
	}
	return state.CurrentPath
}

// BuildKeyboard builds the inline keyboard for directory browsing
func (b *DirectoryBrowser) BuildKeyboard(chatID string) (*BindFlowState, *imbot.KeyboardBuilder, string, error) {
	state := b.GetState(chatID)
	if state == nil {
		return nil, nil, "", fmt.Errorf("no active bind flow")
	}

	dirs, err := listDirectories(state.CurrentPath)
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

	// Build keyboard using Telegram keyboard builder
	kbBuilder := NewKeyboardBuilder()
	browserState := &DirectoryBrowserState{
		CurrentPath: state.CurrentPath,
		Page:        state.Page,
		PageSize:    state.PageSize,
		TotalPages:  totalPages,
		StartIdx:    startIdx,
		EndIdx:      endIdx,
		HasParent:   hasParent(state.CurrentPath),
		Dirs:        dirs,
	}
	kb, text, err := kbBuilder.BuildDirectoryKeyboard(browserState)
	if err != nil {
		return nil, nil, "", err
	}

	return state, kb, text, nil
}

// SendDirectoryBrowser sends or updates the directory browser message
func SendDirectoryBrowser(ctx context.Context, bot imbot.Bot, browser *DirectoryBrowser, chatID string, editMessageID string) (string, error) {
	state, kb, text, err := browser.BuildKeyboard(chatID)
	if err != nil {
		return "", err
	}

	// Try to cast bot to TelegramBot for editing
	tgBot, ok := imbot.AsTelegramBot(bot)
	if ok && editMessageID != "" && state.MessageID != "" {
		// Edit existing message
		tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())
		if err := tgBot.EditMessageWithKeyboard(ctx, chatID, editMessageID, text, &tgKeyboard); err != nil {
			logrus.WithError(err).Warn("Failed to edit message, sending new one")
			// Fall through to send new message
		} else {
			return editMessageID, nil
		}
	}

	// Convert keyboard for Telegram
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	// Send new message with keyboard
	result, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup": tgKeyboard,
		},
	})
	if err != nil {
		return "", err
	}

	// Store message ID for future edits
	browser.SetMessageID(chatID, result.MessageID)

	return result.MessageID, nil
}

// EditDirectoryBrowserMessage edits the directory browser message to show status and remove keyboard
func EditDirectoryBrowserMessage(ctx context.Context, bot imbot.Bot, chatID, msgID, text string) {
	tgBot, ok := imbot.AsTelegramBot(bot)
	if !ok {
		return
	}

	// Remove the keyboard first
	if err := tgBot.RemoveMessageKeyboard(ctx, chatID, msgID); err != nil {
		logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to remove directory browser keyboard")
	} else {
		// Successfully removed keyboard, now edit the text
		if err := tgBot.EditMessageWithKeyboard(ctx, chatID, msgID, text, nil); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to edit directory browser text")
		}
	}
}

// Helper functions

func getHomeDir() (string, error) {
	// Try to get current user's home directory
	usr, err := user.Current()
	if err == nil && usr.HomeDir != "" {
		return usr.HomeDir, nil
	}

	// Fallback to HOME environment variable
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		return homeDir, nil
	}

	// Fallback to current working directory
	return os.Getwd()
}

func listDirectories(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			dirs = append(dirs, filepath.Join(path, entry.Name()))
		}
	}

	// Sort alphabetically
	sort.Strings(dirs)

	return dirs, nil
}

func hasParent(path string) bool {
	parent := filepath.Dir(path)
	return parent != path && parent != ""
}

// BuildCustomPathPrompt builds the custom path input prompt text
func BuildCustomPathPrompt() string {
	return "📝 *Enter the project path:*\n\nUse `~` for home directory or absolute path.\n\nExample: `~/projects/myapp`"
}
