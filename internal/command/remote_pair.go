package command

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/audit"
)

// remotePairCommand creates the `remote pair` subcommand and its children.
//
// Pairing codes themselves are minted in-memory by the running server
// process and printed to its logs/stderr at bot start; they are not
// reachable from a separate CLI invocation. The CLI here is for the
// persistent half of the workflow:
//
//	tingly-box remote pair enable  <bot-uuid>      # turn RequirePairing on
//	tingly-box remote pair disable <bot-uuid>      # turn RequirePairing off
//	tingly-box remote pair revoke  <bot-uuid> <chat-id>
//	                                               # forget a paired chat
//	tingly-box remote pair status  <bot-uuid>      # show pairing status
func remotePairCommand(appManager *AppManager) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Manage TOFU pairing for an imbot",
		Long: "Manage pairing for an imbot. When RequirePairing is enabled, a chat " +
			"must successfully respond with /bind <code> before it can issue\n" +
			"commands to the bot. The code itself is printed to the server's\n" +
			"stderr/log when the bot starts; rotate by restarting the bot.",
	}
	cmd.AddCommand(remotePairEnableCommand(appManager, true))
	cmd.AddCommand(remotePairEnableCommand(appManager, false))
	cmd.AddCommand(remotePairRevokeCommand(appManager))
	cmd.AddCommand(remotePairStatusCommand(appManager))
	return cmd
}

func remotePairEnableCommand(appManager *AppManager, enable bool) *cobra.Command {
	use := "enable"
	short := "Turn on RequirePairing for a bot (takes effect at next start)"
	if !enable {
		use = "disable"
		short = "Turn off RequirePairing for a bot"
	}
	return &cobra.Command{
		Use:   use + " <bot-uuid>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openSettingsStore(appManager)
			if err != nil {
				return err
			}
			setting, err := store.GetSettingsByUUID(args[0])
			if err != nil {
				return err
			}
			if setting.UUID == "" {
				return fmt.Errorf("bot %s not found", args[0])
			}
			v := enable
			setting.RequirePairing = &v
			if err := store.UpdateSettings(args[0], setting); err != nil {
				return err
			}
			state := "enabled"
			if !enable {
				state = "disabled"
			}
			fmt.Printf("RequirePairing %s for bot %s.\n", state, args[0])
			fmt.Println("Restart the bot for the change to take effect.")
			if enable {
				fmt.Println("On restart the server will print a one-time pairing code; " +
					"send `/bind <code>` from the bot's DM to bind the chat.")
			}
			return nil
		},
	}
}

func remotePairRevokeCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <bot-uuid> <chat-id>",
		Short: "Forget the pairing for a specific chat (the chat must re-bind)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			botUUID, chatID := args[0], args[1]
			cfg := appManager.AppConfig().GetGlobalConfig()
			if cfg == nil {
				return fmt.Errorf("global config not available")
			}
			chatStore, err := bot.NewChatStoreJSON(filepath.Join(cfg.ConfigDir, "bot_chats.json"))
			if err != nil {
				return fmt.Errorf("open chat store: %w", err)
			}
			defer chatStore.Close()

			if !chatStore.IsChatPaired(chatID, botUUID) {
				fmt.Printf("Chat %s is not paired with bot %s.\n", chatID, botUUID)
				return nil
			}
			if err := chatStore.ClearPaired(chatID); err != nil {
				return err
			}
			auditLog := audit.NewLogger(audit.Config{Console: true})
			auditLog.Info("imbot.pair.revoked", "", "", "pairing revoked via CLI",
				map[string]interface{}{"bot_uuid": botUUID, "chat_id": chatID, "by": "cli"})
			fmt.Printf("Revoked pairing for chat %s on bot %s. The chat will need to re-/bind.\n",
				chatID, botUUID)
			return nil
		},
	}
}

func remotePairStatusCommand(appManager *AppManager) *cobra.Command {
	return &cobra.Command{
		Use:   "status <bot-uuid>",
		Short: "Show whether RequirePairing is on and where to find the code",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openSettingsStore(appManager)
			if err != nil {
				return err
			}
			setting, err := store.GetSettingsByUUID(args[0])
			if err != nil {
				return err
			}
			if setting.UUID == "" {
				return fmt.Errorf("bot %s not found", args[0])
			}
			require := setting.RequirePairing != nil && *setting.RequirePairing
			fmt.Printf("Bot:             %s (%s)\n", setting.Name, setting.UUID)
			fmt.Printf("Platform:        %s\n", setting.Platform)
			fmt.Printf("RequirePairing:  %t\n", require)
			if require {
				fmt.Println("\nPairing codes are minted in memory at bot start. Look for")
				fmt.Println("    [tingly-box] Bot \"...\" pairing code: ...")
				fmt.Println("on the server's stderr/log. Restart the bot to mint a new code.")
			} else {
				fmt.Println("\nThis bot does not require pairing — any chat that knows the bot")
				fmt.Println("token can issue commands. Run `remote pair enable` to harden it.")
			}
			return nil
		},
	}
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
