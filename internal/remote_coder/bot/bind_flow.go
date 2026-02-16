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

	"github.com/sirupsen/logrus"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
	WaitingInput bool   // Waiting for custom path input
	PromptMsgID  string // Prompt message ID for cleanup
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

	// Build keyboard
	kb := imbot.NewKeyboardBuilder()

	// Directory buttons
	for i := startIdx; i < endIdx; i++ {
		dirPath := dirs[i]
		dirName := filepath.Base(dirPath)
		buttonText := imbot.FormatDirButton(dirName, 20)
		callbackData := imbot.FormatCallbackData("bind", "nav", imbot.FormatDirPath(dirPath))
		kb.AddRow(imbot.CallbackButton(buttonText, callbackData))
	}

	// Navigation row
	var navButtons []imbot.InlineKeyboardButton

	// Parent directory button
	if hasParent(state.CurrentPath) {
		parentPath := filepath.Dir(state.CurrentPath)
		navButtons = append(navButtons, imbot.CallbackButton("📁 ..", imbot.FormatCallbackData("bind", "nav", imbot.FormatDirPath(parentPath))))
	}

	// Pagination buttons
	if state.Page > 0 {
		navButtons = append(navButtons, imbot.CallbackButton("◀ Prev", imbot.FormatCallbackData("bind", "prev")))
	}
	if state.Page < totalPages-1 && len(dirs) > endIdx {
		navButtons = append(navButtons, imbot.CallbackButton("Next ▶", imbot.FormatCallbackData("bind", "next")))
	}

	if len(navButtons) > 0 {
		kb.AddRow(navButtons...)
	}

	// Select current directory button and custom path button
	kb.AddRow(
		imbot.CallbackButton("✓ Select This", imbot.FormatCallbackData("bind", "select", imbot.FormatDirPath(state.CurrentPath))),
		imbot.CallbackButton("✏️ Custom", imbot.FormatCallbackData("bind", "custom")),
	)

	// Cancel button
	kb.AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")))

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

	return state, kb, text, nil
}

// BuildActionKeyboard builds the inline keyboard for actions (Clear/Bind)
func BuildActionKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("🗑 Clear", imbot.FormatCallbackData("action", "clear")),
			imbot.CallbackButton("📁 Bind", imbot.FormatCallbackData("action", "bind")),
		)
}

// BuildCustomPathPrompt returns the text for custom path input prompt
func BuildCustomPathPrompt() string {
	return "✏️ *Please type the path you want to bind:*\n\n" +
		"Examples:\n" +
		"• my-project (relative to current)\n" +
		"• ~/workspace/new-project\n" +
		"• /home/user/my-project\n\n" +
		"The directory will be created if it doesn't exist.\n\n" +
		"Type your path or click Cancel below."
}

// BuildCancelKeyboard builds a simple cancel keyboard
func BuildCancelKeyboard() *imbot.KeyboardBuilder {
	return imbot.NewKeyboardBuilder().
		AddRow(imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")))
}

// BuildCreateConfirmKeyboard builds the confirmation keyboard for creating a directory
func BuildCreateConfirmKeyboard(path string) (*imbot.KeyboardBuilder, string) {
	kb := imbot.NewKeyboardBuilder().
		AddRow(
			imbot.CallbackButton("✅ Create", imbot.FormatCallbackData("bind", "create", imbot.FormatDirPath(path))),
			imbot.CallbackButton("❌ Cancel", imbot.FormatCallbackData("bind", "cancel")),
		)

	text := fmt.Sprintf("📁 *The path doesn't exist. Create it?*\n\n`%s`", path)
	return kb, text
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
		tgKeyboard := convertToTelegramKeyboard(kb.Build())
		if err := tgBot.EditMessageWithKeyboard(ctx, chatID, editMessageID, text, tgKeyboard); err != nil {
			logrus.WithError(err).Warn("Failed to edit message, sending new one")
			// Fall through to send new message
		} else {
			return editMessageID, nil
		}
	}

	// Convert keyboard for Telegram
	tgKeyboard := convertToTelegramKeyboard(kb.Build())

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

// convertToTelegramKeyboard converts imbot.InlineKeyboardMarkup to tgbotapi.InlineKeyboardMarkup
func convertToTelegramKeyboard(kb imbot.InlineKeyboardMarkup) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		var buttons []tgbotapi.InlineKeyboardButton
		for _, btn := range row {
			tgBtn := tgbotapi.InlineKeyboardButton{
				Text: btn.Text,
			}
			if btn.CallbackData != "" {
				tgBtn.CallbackData = &btn.CallbackData
			}
			if btn.URL != "" {
				tgBtn.URL = &btn.URL
			}
			buttons = append(buttons, tgBtn)
		}
		rows = append(rows, buttons)
	}
	return tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
}
