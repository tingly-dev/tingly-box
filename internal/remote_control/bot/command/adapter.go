package command

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot"
)

// BotHandlerAdapter adapts the existing BotHandler to the command.Handler interface.
// This is a temporary bridge during the refactoring.
type BotHandlerAdapter struct {
	// These are function pointers that will be set to BotHandler methods
	sendTextFunc                func(chatID, text string) error
	getProjectPathFunc          func(chatID string) (string, error)
	setProjectPathFunc          func(chatID, path string) error
	getProjectPathForGroupFunc  func(chatID, platform string) (string, bool)
	stopExecutionFunc           func(chatID string) bool
	getCurrentAgentFunc         func(chatID string) (string, error)
	setVerboseFunc              func(chatID string, enabled bool)
	getVerboseFunc              func(chatID string) bool
	isWhitelistedFunc           func(groupID string) bool
	addToWhitelistFunc          func(groupID, platform, userID string) error
	getBashCwdFunc              func(chatID string) (string, error)
	setBashCwdFunc              func(chatID, path string) error
	resolveChatIDFunc           func(input string) (string, error)
	getDefaultProjectPathFunc   func() string
	getBashAllowlistFunc        func() map[string]struct{}
	listProjectPathsFunc        func(ownerID, platform string) ([]string, error)
	verifyAndPairFunc           func(botUUID, chatID, senderID, platform, code string) error
	clearSessionFunc            func(chatID, agentType string) error
	findOrCreateSessionFunc     func(chatID, agentType, projectPath string) (*SessionInfo, error)
	updatePermissionModeFunc    func(sessionID, mode string) error
	getSessionFunc              func(chatID, agentType, projectPath string) (*SessionInfo, error)
}

// NewBotHandlerAdapter creates a new adapter from a BotHandler-like struct
func NewBotHandlerAdapter(sendText func(chatID, text string) error) *BotHandlerAdapter {
	return &BotHandlerAdapter{
		sendTextFunc: sendText,
	}
}

// WithSendText sets the SendText function
func (a *BotHandlerAdapter) WithSendText(fn func(chatID, text string) error) *BotHandlerAdapter {
	a.sendTextFunc = fn
	return a
}

// WithProjectPathFuncs sets the project path related functions
func (a *BotHandlerAdapter) WithProjectPathFuncs(
	get func(chatID string) (string, error),
	set func(chatID, path string) error,
	getGroup func(chatID, platform string) (string, bool),
) *BotHandlerAdapter {
	a.getProjectPathFunc = get
	a.setProjectPathFunc = set
	a.getProjectPathForGroupFunc = getGroup
	return a
}

// WithStopExecution sets the StopExecution function
func (a *BotHandlerAdapter) WithStopExecution(fn func(chatID string) bool) *BotHandlerAdapter {
	a.stopExecutionFunc = fn
	return a
}

// WithAgentFuncs sets the agent-related functions
func (a *BotHandlerAdapter) WithAgentFuncs(
	getCurrent func(chatID string) (string, error),
) *BotHandlerAdapter {
	a.getCurrentAgentFunc = getCurrent
	return a
}

// WithVerboseFuncs sets the verbose-related functions
func (a *BotHandlerAdapter) WithVerboseFuncs(
	get func(chatID string) bool,
	set func(chatID string, enabled bool),
) *BotHandlerAdapter {
	a.getVerboseFunc = get
	a.setVerboseFunc = set
	return a
}

// WithWhitelistFuncs sets the whitelist-related functions
func (a *BotHandlerAdapter) WithWhitelistFuncs(
	isWhitelisted func(groupID string) bool,
	addToWhitelist func(groupID, platform, userID string) error,
) *BotHandlerAdapter {
	a.isWhitelistedFunc = isWhitelisted
	a.addToWhitelistFunc = addToWhitelist
	return a
}

// WithBashCwdFuncs sets the bash cwd related functions
func (a *BotHandlerAdapter) WithBashCwdFuncs(
	get func(chatID string) (string, error),
	set func(chatID, path string) error,
) *BotHandlerAdapter {
	a.getBashCwdFunc = get
	a.setBashCwdFunc = set
	return a
}

// WithChatIDResolution sets the ResolveChatID function
func (a *BotHandlerAdapter) WithChatIDResolution(fn func(input string) (string, error)) *BotHandlerAdapter {
	a.resolveChatIDFunc = fn
	return a
}

// WithDefaultProjectPath sets the GetDefaultProjectPath function
func (a *BotHandlerAdapter) WithDefaultProjectPath(fn func() string) *BotHandlerAdapter {
	a.getDefaultProjectPathFunc = fn
	return a
}

// WithBashAllowlist sets the GetBashAllowlist function
func (a *BotHandlerAdapter) WithBashAllowlist(fn func() map[string]struct{}) *BotHandlerAdapter {
	a.getBashAllowlistFunc = fn
	return a
}

// WithProjectPathsList sets the ListProjectPaths function
func (a *BotHandlerAdapter) WithProjectPathsList(fn func(ownerID, platform string) ([]string, error)) *BotHandlerAdapter {
	a.listProjectPathsFunc = fn
	return a
}

// WithPairing sets the VerifyAndPair function
func (a *BotHandlerAdapter) WithPairing(fn func(botUUID, chatID, senderID, platform, code string) error) *BotHandlerAdapter {
	a.verifyAndPairFunc = fn
	return a
}

// WithSessionFuncs sets the session-related functions
func (a *BotHandlerAdapter) WithSessionFuncs(
	clear func(chatID, agentType string) error,
	findOrCreate func(chatID, agentType, projectPath string) (*SessionInfo, error),
	updatePermissionMode func(sessionID, mode string) error,
	get func(chatID, agentType, projectPath string) (*SessionInfo, error),
) *BotHandlerAdapter {
	a.clearSessionFunc = clear
	a.findOrCreateSessionFunc = findOrCreate
	a.updatePermissionModeFunc = updatePermissionMode
	a.getSessionFunc = get
	return a
}

// Handler interface implementation

func (a *BotHandlerAdapter) SendText(chatID, text string) error {
	if a.sendTextFunc == nil {
		return fmt.Errorf("SendText not configured")
	}
	return a.sendTextFunc(chatID, text)
}

func (a *BotHandlerAdapter) GetProjectPath(chatID string) (string, error) {
	if a.getProjectPathFunc == nil {
		return "", fmt.Errorf("GetProjectPath not configured")
	}
	return a.getProjectPathFunc(chatID)
}

func (a *BotHandlerAdapter) SetProjectPath(chatID, path string) error {
	if a.setProjectPathFunc == nil {
		return fmt.Errorf("SetProjectPath not configured")
	}
	return a.setProjectPathFunc(chatID, path)
}

func (a *BotHandlerAdapter) GetProjectPathForGroup(chatID, platform string) (string, bool) {
	if a.getProjectPathForGroupFunc == nil {
		return "", false
	}
	return a.getProjectPathForGroupFunc(chatID, platform)
}

func (a *BotHandlerAdapter) StopExecution(chatID string) bool {
	if a.stopExecutionFunc == nil {
		return false
	}
	return a.stopExecutionFunc(chatID)
}

func (a *BotHandlerAdapter) GetCurrentAgent(chatID string) (string, error) {
	if a.getCurrentAgentFunc == nil {
		return "", fmt.Errorf("GetCurrentAgent not configured")
	}
	return a.getCurrentAgentFunc(chatID)
}

func (a *BotHandlerAdapter) SetVerbose(chatID string, enabled bool) {
	if a.setVerboseFunc != nil {
		a.setVerboseFunc(chatID, enabled)
	}
}

func (a *BotHandlerAdapter) GetVerbose(chatID string) bool {
	if a.getVerboseFunc == nil {
		return true // default to verbose
	}
	return a.getVerboseFunc(chatID)
}

func (a *BotHandlerAdapter) IsWhitelisted(groupID string) bool {
	if a.isWhitelistedFunc == nil {
		return false
	}
	return a.isWhitelistedFunc(groupID)
}

func (a *BotHandlerAdapter) AddToWhitelist(groupID, platform, userID string) error {
	if a.addToWhitelistFunc == nil {
		return fmt.Errorf("AddToWhitelist not configured")
	}
	return a.addToWhitelistFunc(groupID, platform, userID)
}

func (a *BotHandlerAdapter) GetBashCwd(chatID string) (string, error) {
	if a.getBashCwdFunc == nil {
		return "", fmt.Errorf("GetBashCwd not configured")
	}
	return a.getBashCwdFunc(chatID)
}

func (a *BotHandlerAdapter) SetBashCwd(chatID, path string) error {
	if a.setBashCwdFunc == nil {
		return fmt.Errorf("SetBashCwd not configured")
	}
	return a.setBashCwdFunc(chatID, path)
}

func (a *BotHandlerAdapter) ResolveChatID(input string) (string, error) {
	if a.resolveChatIDFunc == nil {
		return "", fmt.Errorf("ResolveChatID not configured")
	}
	return a.resolveChatIDFunc(input)
}

func (a *BotHandlerAdapter) GetDefaultProjectPath() string {
	if a.getDefaultProjectPathFunc == nil {
		return ""
	}
	return a.getDefaultProjectPathFunc()
}

func (a *BotHandlerAdapter) GetBashAllowlist() map[string]struct{} {
	if a.getBashAllowlistFunc == nil {
		return make(map[string]struct{})
	}
	return a.getBashAllowlistFunc()
}

func (a *BotHandlerAdapter) ListProjectPaths(ownerID, platform string) ([]string, error) {
	if a.listProjectPathsFunc == nil {
		return nil, fmt.Errorf("ListProjectPaths not configured")
	}
	return a.listProjectPathsFunc(ownerID, platform)
}

func (a *BotHandlerAdapter) VerifyAndPair(botUUID, chatID, senderID, platform, code string) error {
	if a.verifyAndPairFunc == nil {
		return fmt.Errorf("VerifyAndPair not configured")
	}
	return a.verifyAndPairFunc(botUUID, chatID, senderID, platform, code)
}

func (a *BotHandlerAdapter) ClearSession(chatID, agentType string) error {
	if a.clearSessionFunc == nil {
		return fmt.Errorf("ClearSession not configured")
	}
	return a.clearSessionFunc(chatID, agentType)
}

func (a *BotHandlerAdapter) FindOrCreateSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	if a.findOrCreateSessionFunc == nil {
		return nil, fmt.Errorf("FindOrCreateSession not configured")
	}
	return a.findOrCreateSessionFunc(chatID, agentType, projectPath)
}

func (a *BotHandlerAdapter) UpdatePermissionMode(sessionID, mode string) error {
	if a.updatePermissionModeFunc == nil {
		return fmt.Errorf("UpdatePermissionMode not configured")
	}
	return a.updatePermissionModeFunc(sessionID, mode)
}

func (a *BotHandlerAdapter) GetSession(chatID, agentType, projectPath string) (*SessionInfo, error) {
	if a.getSessionFunc == nil {
		return nil, fmt.Errorf("GetSession not configured")
	}
	return a.getSessionFunc(chatID, agentType, projectPath)
}

// SendTextWithBot sends a text message using the bot interface
func (a *BotHandlerAdapter) SendTextWithBot(ctx context.Context, bot imbot.Bot, chatID, text string) error {
	_, err := bot.SendMessage(ctx, chatID, &imbot.SendMessageOptions{
		Text: text,
	})
	return err
}
