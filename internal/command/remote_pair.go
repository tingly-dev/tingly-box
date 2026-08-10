package command

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/control/bot"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// RemotePairEnable enables or disables RequirePairing for a bot.
// Takes effect at next bot start.
func RemotePairEnable(appManager *AppManager, botUUID string, enable bool) error {
	store, err := openSettingsStore(appManager)
	if err != nil {
		return err
	}
	setting, err := store.GetSettingsByUUID(botUUID)
	if err != nil {
		return err
	}
	if setting.UUID == "" {
		return fmt.Errorf("bot %s not found", botUUID)
	}
	v := enable
	setting.RequirePairing = &v
	if err := store.UpdateSettings(botUUID, setting); err != nil {
		return err
	}
	state := "enabled"
	if !enable {
		state = "disabled"
	}
	fmt.Printf("RequirePairing %s for bot %s.\n", state, botUUID)
	fmt.Println("Restart the bot for the change to take effect.")
	if enable {
		fmt.Println("On restart the server will print a one-time pairing code; " +
			"send `/bind <code>` from the bot's DM to bind the chat.")
	}
	return nil
}

// RemotePairRevoke forgets the pairing for a specific chat.
// The chat will need to re-bind to issue commands.
func RemotePairRevoke(appManager *AppManager, botUUID, chatID string) error {
	cfg := appManager.AppConfig().GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config not available")
	}
	sm, err := db.NewStoreManager(cfg.ConfigDir)
	if err != nil {
		return fmt.Errorf("open store manager: %w", err)
	}
	defer sm.Close()
	chatStore := sm.RemoteChats()

	if !chatStore.IsChatPaired(chatID, botUUID) {
		fmt.Printf("Chat %s is not paired with bot %s.\n", chatID, botUUID)
		return nil
	}
	if err := chatStore.ClearPaired(chatID); err != nil {
		return err
	}
	logrus.WithFields(logrus.Fields{
		"action":   "imbot.pair.revoked",
		"bot_uuid": botUUID,
		"chat_id":  chatID,
		"by":       "cli",
	}).Info("pairing revoked via CLI")
	fmt.Printf("Revoked pairing for chat %s on bot %s. The chat will need to re-/bind.\n",
		chatID, botUUID)
	return nil
}

// RemotePairStatus shows whether RequirePairing is on and where to find the code.
func RemotePairStatus(appManager *AppManager, botUUID string) error {
	store, err := openSettingsStore(appManager)
	if err != nil {
		return err
	}
	setting, err := store.GetSettingsByUUID(botUUID)
	if err != nil {
		return err
	}
	if setting.UUID == "" {
		return fmt.Errorf("bot %s not found", botUUID)
	}
	source := "explicit"
	var require bool
	if setting.RequirePairing != nil {
		require = *setting.RequirePairing
	} else {
		require = bot.PlatformDefaultsRequirePairing(setting.Platform)
		source = "platform default"
	}
	fmt.Printf("Bot:             %s (%s)\n", setting.Name, setting.UUID)
	fmt.Printf("Platform:        %s\n", setting.Platform)
	fmt.Printf("RequirePairing:  %t (%s)\n", require, source)
	if require {
		fmt.Println("\nPairing codes are minted in memory at bot start. Look for")
		fmt.Println("    [tingly-box] Bot \"...\" pairing code: ...")
		fmt.Println("on the server's stderr/log. Restart the bot to mint a new code.")
		if source == "platform default" {
			fmt.Println("\nThis is the platform default. To pin it explicitly, run:")
			fmt.Printf("    remote pair enable %s\n", setting.UUID)
		}
	} else {
		fmt.Println("\nThis bot does not require pairing — any chat that knows the bot")
		fmt.Println("token can issue commands. Run `remote pair enable` to harden it.")
	}
	return nil
}

func openSettingsStore(appManager *AppManager) (*db.ImBotSettingsStore, error) {
	if appManager == nil || appManager.AppConfig() == nil {
		return nil, fmt.Errorf("app configuration is not initialized")
	}
	cfg := appManager.AppConfig().GetGlobalConfig()
	if cfg == nil {
		return nil, fmt.Errorf("global config not available")
	}
	return db.NewImBotSettingsStore(cfg.ConfigDir)
}
