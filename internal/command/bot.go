package command

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/app"
	"github.com/tingly-dev/tingly-box/internal/db"
	builtinserver "github.com/tingly-dev/tingly-box/internal/mcp/builtin_server"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
)

// ============== Kong Command Structures ==============

// BotCmdKong manages the IM bots. The hidden Default subcommand is
// marked default so `tingly-box bot` (no further args) lists them. Bots
// run in the server (`tingly-box start`), where chats talk to Claude Code
// through Projects.
type BotCmdKong struct {
	Default BotListCmdKong   `kong:"cmd,name='default',default='1',hidden,help='List bots (default)'"`
	List    BotListCmdKong   `kong:"cmd,help='List bots'"`
	Config  BotConfigCmdKong `kong:"cmd,help='Show or change the settings of a bot'"`
	Add     BotAddCmdKong    `kong:"cmd,help='Add a new bot configuration'"`
	Pair    BotPairCmdKong   `kong:"cmd,help='Manage TOFU pairing for an imbot'"`
}

// BotListCmdKong lists the bots
type BotListCmdKong struct{}

func (r *BotListCmdKong) Run(appManager *app.AppManager) error {
	return runBotList(appManager)
}

// BotConfigCmdKong shows a bot's settings, or changes the ones given.
type BotConfigCmdKong struct {
	Bot        string `kong:"arg,help='Bot UUID, or its number in bot list'"`
	Name       string `kong:"flag,name='name',help='Rename the bot'"`
	DefaultCwd string `kong:"flag,name='default-cwd',help='Folder a chat works in until it picks one with /cd'"`
	Enable     bool   `kong:"flag,name='enable',xor='enabled',help='Run the bot'"`
	Disable    bool   `kong:"flag,name='disable',xor='enabled',help='Stop the bot'"`
}

func (r *BotConfigCmdKong) Run(appManager *app.AppManager) error {
	return runBotConfig(appManager, r)
}

// BotAddCmdKong adds a new bot configuration (interactive).
type BotAddCmdKong struct{}

func (r *BotAddCmdKong) Run(appManager *app.AppManager) error {
	// No flag form exists — it's always interactive.
	if err := requireTTY("add a bot via the Web UI's Remote page instead"); err != nil {
		return err
	}
	return runBotAdd(appManager)
}

// BotPairCmdKong manages TOFU pairing for imbots
type BotPairCmdKong struct {
	Enable  BotPairEnableCmdKong  `kong:"cmd,help='Turn on RequirePairing for a bot'"`
	Disable BotPairDisableCmdKong `kong:"cmd,help='Turn off RequirePairing for a bot'"`
	Revoke  BotPairRevokeCmdKong  `kong:"cmd,help='Forget the pairing for a specific chat'"`
	Status  BotPairStatusCmdKong  `kong:"cmd,help='Show pairing status for a bot'"`
}

func (r *BotPairCmdKong) Run(appManager *app.AppManager) error {
	// Pair management is handled by subcommands
	return nil
}

// BotPairEnableCmdKong turns on RequirePairing for a bot
type BotPairEnableCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *BotPairEnableCmdKong) Run(appManager *app.AppManager) error {
	return BotPairEnable(appManager, r.BotUUID, true)
}

// BotPairDisableCmdKong turns off RequirePairing for a bot
type BotPairDisableCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *BotPairDisableCmdKong) Run(appManager *app.AppManager) error {
	return BotPairEnable(appManager, r.BotUUID, false)
}

// BotPairRevokeCmdKong forgets the pairing for a specific chat
type BotPairRevokeCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
	ChatID  string `kong:"arg,help='Chat ID to unpair'"`
}

func (r *BotPairRevokeCmdKong) Run(appManager *app.AppManager) error {
	return BotPairRevoke(appManager, r.BotUUID, r.ChatID)
}

// BotPairStatusCmdKong shows pairing status for a bot
type BotPairStatusCmdKong struct {
	BotUUID string `kong:"arg,help='Bot UUID'"`
}

func (r *BotPairStatusCmdKong) Run(appManager *app.AppManager) error {
	return BotPairStatus(appManager, r.BotUUID)
}

// MCPBuiltinCmdKong starts the builtin MCP server. Registered at the top level
// as "mcp-builtin" to match the legacy command path, which is consumed by
// internal/mcp/runtime/builtin_registry.go.
//
// Run takes no *app.AppManager: this process is spawned BY the running
// server as a stdio child and lives as long as it does. It serves web
// tools over stdio and has no business in the config directory — before
// AppManager became lazy it opened tingly.db and ran every migration just
// by being dispatched, a long-lived second writer next to the server
// (#1912).
type MCPBuiltinCmdKong struct{}

func (m *MCPBuiltinCmdKong) Run() error {
	return builtinserver.Serve()
}

// runBotAdd is a wrapper for runBotAddInteractive
func runBotAdd(appManager *app.AppManager) error {
	return runBotAddInteractive(bufio.NewReader(os.Stdin), appManager)
}

// Business logic functions

// runBotConfig shows a bot's settings, or changes the ones given and tells
// a running server to pick them up.
func runBotConfig(appManager *app.AppManager, r *BotConfigCmdKong) error {
	store, err := openSettingsStore(appManager)
	if err != nil {
		return err
	}
	uuid, err := resolveBotRef(store, r.Bot)
	if err != nil {
		return err
	}
	setting, err := store.GetSettingsByUUID(uuid)
	if err != nil {
		return err
	}
	if setting.UUID == "" {
		return fmt.Errorf("bot %s not found", r.Bot)
	}

	changed := false
	if r.Name != "" {
		setting.Name, changed = r.Name, true
	}
	if r.DefaultCwd != "" {
		dir, err := remoteagent.ExpandPath(r.DefaultCwd)
		if err != nil {
			return err
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a folder", dir)
		}
		setting.DefaultCwd, changed = dir, true
	}
	if r.Enable || r.Disable {
		setting.Enabled, changed = r.Enable, true
	}
	if changed {
		if err := store.UpdateSettings(uuid, setting); err != nil {
			return err
		}
		if bots, ok := notifyServerBotReload(appManager); ok && !botRunningAfterReload(bots, uuid) && setting.Enabled {
			fmt.Println("Server was notified, but the bot is not running — check `tingly-box log` for startup errors.")
		}
		PrintSuccess("Bot settings saved.")
		fmt.Println()
	}

	printBotSetting(setting)
	return nil
}

// resolveBotRef turns a bot UUID, or its number in `bot list`, into its UUID.
func resolveBotRef(store *db.ImBotSettingsStore, ref string) (string, error) {
	n, err := strconv.Atoi(ref)
	if err != nil {
		return ref, nil
	}
	settings, err := store.ListSettings()
	if err != nil {
		return "", err
	}
	if n < 1 || n > len(settings) {
		return "", fmt.Errorf("no bot %d (there are %d); see `tingly-box bot list`", n, len(settings))
	}
	return settings[n-1].UUID, nil
}

func printBotSetting(s db.Settings) {
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	name := s.Name
	if name == "" {
		name = "-"
	}
	cwd := s.DefaultCwd
	if cwd == "" {
		cwd = "- (the server's working directory)"
	}
	fmt.Printf("Name:        %s\n", name)
	fmt.Printf("UUID:        %s\n", s.UUID)
	fmt.Printf("Platform:    %s\n", s.Platform)
	fmt.Printf("Enabled:     %s\n", onOff(s.Enabled))
	fmt.Printf("Pairing:     %s (tingly-box bot pair)\n", onOff(s.RequirePairing != nil && *s.RequirePairing))
	fmt.Printf("Default cwd: %s\n", cwd)
}

// runBotList lists all configured bots
func runBotList(appManager *app.AppManager) error {
	cfg := appManager.AppConfig().GetGlobalConfig()
	store, err := db.NewImBotSettingsStore(cfg.ConfigDir)
	if err != nil {
		return err
	}

	settings, err := store.ListSettings()
	if err != nil {
		return err
	}

	if len(settings) == 0 {
		fmt.Println("No bot settings found.")
		fmt.Println("Configure bots through the web UI or add settings directly.")
		return nil
	}

	fmt.Println("Bot Settings:")
	fmt.Println()
	fmt.Printf("%-6s %-36s %-12s %-15s %-8s\n", "ID", "UUID", "Platform", "Name", "Enabled")
	fmt.Println(strings.Repeat("-", 82))
	for i, s := range settings {
		enabled := "No"
		if s.Enabled {
			enabled = "Yes"
		}
		name := s.Name
		if name == "" {
			name = "-"
		}
		fmt.Printf("%-6d %-36s %-12s %-15s %-8s\n", i+1, s.UUID, s.Platform, name, enabled)
	}
	fmt.Println()
	fmt.Printf("Total: %d bot(s)\n", len(settings))

	return nil
}
