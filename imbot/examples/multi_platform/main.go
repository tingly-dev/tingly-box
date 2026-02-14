package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// ============================================================
// 配置
// ============================================================

// Whitelist - 留空表示允许所有用户
var Whitelist []string

func init() {
	Whitelist = []string{}
}

// ============================================================
// 命令系统 - 平台无关的业务逻辑
// ============================================================

// CommandHandler 命令处理函数类型
type CommandHandler func(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error

// Command 定义一个命令
type Command struct {
	Name        string         // 命令名称
	Description string         // 命令描述
	Handler     CommandHandler // 处理函数
	Aliases     []string       // 别名
}

// 注册的命令列表 - 所有平台共享
var Commands = []Command{
	{Name: "start", Description: "开始使用机器人", Handler: cmdStart, Aliases: []string{"help"}},
	{Name: "ping", Description: "检查机器人状态", Handler: cmdPing},
	{Name: "echo", Description: "复读消息", Handler: cmdEcho},
	{Name: "time", Description: "显示当前时间", Handler: cmdTime},
	{Name: "info", Description: "显示用户信息", Handler: cmdInfo},
	{Name: "status", Description: "显示机器人状态", Handler: cmdStatus},
	{Name: "platform", Description: "显示当前平台", Handler: cmdPlatform},
	{Name: "about", Description: "关于机器人", Handler: cmdAbout},
}

// ============================================================
// 主程序
// ============================================================

func main() {
	// 从环境变量读取配置
	configs := loadConfigs()
	if len(configs) == 0 {
		log.Fatal("❌ 请至少配置一个平台的环境变量")
	}

	// 创建带取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建管理器
	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(10),
		imbot.WithReconnectDelay(5000),
	)

	// 添加所有平台的 Bot
	if err := manager.AddBots(configs); err != nil {
		log.Fatalf("❌ 添加 Bot 失败: %v", err)
	}

	// 设置统一的消息处理器 - 核心业务逻辑只写一次
	manager.OnMessage(func(msg imbot.Message, platform core.Platform) {
		handleMessage(ctx, manager, msg, platform)
	})

	// 设置错误处理器
	manager.OnError(func(err error, platform core.Platform) {
		log.Printf("[%-10s] ❌ 错误: %v", platform, err)
	})

	// 设置连接处理器
	manager.OnConnected(func(platform core.Platform) {
		log.Printf("[%-10s] ✅ 已连接", platform)
	})

	manager.OnDisconnected(func(platform core.Platform) {
		log.Printf("[%-10s] ❌ 已断开", platform)
	})

	manager.OnReady(func(platform core.Platform) {
		log.Printf("[%-10s] 🚀 准备就绪", platform)
	})

	// 启动管理器
	log.Println("🤖 启动多平台机器人...")
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("❌ 启动失败: %v", err)
	}

	// 打印状态
	printStartupInfo(manager, configs)

	// 等待中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("\n🛑 正在关闭...")
	cancel()

	if err := manager.Stop(context.Background()); err != nil {
		log.Printf("关闭错误: %v", err)
	}

	log.Println("✅ 已停止")
}

// ============================================================
// 配置加载
// ============================================================

func loadConfigs() []*imbot.Config {
	var configs []*imbot.Config

	// Telegram
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		configs = append(configs, &imbot.Config{
			Platform: core.PlatformTelegram,
			Enabled:  true,
			Auth: imbot.AuthConfig{
				Type:  "token",
				Token: token,
			},
			Logging: &imbot.LoggingConfig{Level: "info"},
		})
		log.Println("✓ Telegram 已配置")
	}

	// DingTalk
	if appKey := os.Getenv("DINGTALK_APP_KEY"); appKey != "" {
		appSecret := os.Getenv("DINGTALK_APP_SECRET")
		if appSecret != "" {
			configs = append(configs, &imbot.Config{
				Platform: core.PlatformDingTalk,
				Enabled:  true,
				Auth: imbot.AuthConfig{
					Type:         "oauth",
					ClientID:     appKey,
					ClientSecret: appSecret,
				},
				Logging: &imbot.LoggingConfig{Level: "info"},
			})
			log.Println("✓ DingTalk 已配置")
		}
	}

	// Feishu
	if appID := os.Getenv("FEISHU_APP_ID"); appID != "" {
		appSecret := os.Getenv("FEISHU_APP_SECRET")
		if appSecret != "" {
			configs = append(configs, &imbot.Config{
				Platform: core.PlatformFeishu,
				Enabled:  true,
				Auth: imbot.AuthConfig{
					Type:         "oauth",
					ClientID:     appID,
					ClientSecret: appSecret,
				},
				Logging: &imbot.LoggingConfig{Level: "info"},
			})
			log.Println("✓ Feishu 已配置")
		}
	}

	// Discord
	if token := os.Getenv("DISCORD_BOT_TOKEN"); token != "" {
		configs = append(configs, &imbot.Config{
			Platform: core.PlatformDiscord,
			Enabled:  true,
			Auth: imbot.AuthConfig{
				Type:  "token",
				Token: token,
			},
			Logging: &imbot.LoggingConfig{Level: "info"},
			Options: map[string]interface{}{
				"intents": []string{"Guilds", "GuildMessages", "DirectMessages", "MessageContent"},
			},
		})
		log.Println("✓ Discord 已配置")
	}

	return configs
}

// ============================================================
// 消息处理 - 平台无关的核心业务逻辑
// ============================================================

// handleMessage 统一处理所有平台的消息
func handleMessage(ctx context.Context, manager *imbot.Manager, msg imbot.Message, platform core.Platform) {
	// 日志记录
	log.Printf("[%-10s] %s: %s",
		platform,
		msg.GetSenderDisplayName(),
		truncateText(msg.GetText(), 50),
	)

	// 获取 Bot 实例
	bot := manager.GetBot(platform)
	if bot == nil {
		log.Printf("[%-10s] Bot 未找到", platform)
		return
	}

	// 白名单检查
	if !isWhitelisted(msg.Sender.ID) {
		log.Printf("[%-10s] 用户 %s 不在白名单中", platform, msg.Sender.ID)
		sendReply(ctx, bot, msg, "⛔ 抱歉，您没有权限使用此机器人。")
		return
	}

	// 处理回调 (按钮点击等)
	if isCallback(msg) {
		handleCallback(ctx, bot, msg)
		return
	}

	// 处理文本消息
	if msg.IsTextContent() {
		handleTextMessage(ctx, bot, msg)
		return
	}

	// 处理媒体消息
	if msg.Content.ContentType() == "media" {
		handleMediaMessage(ctx, bot, msg)
		return
	}

	log.Printf("[%-10s] 未处理的内容类型: %s", platform, msg.Content.ContentType())
}

// handleTextMessage 处理文本消息
func handleTextMessage(ctx context.Context, bot imbot.Bot, msg imbot.Message) {
	text := strings.TrimSpace(msg.GetText())

	// 命令处理
	if strings.HasPrefix(text, "/") {
		executeCommand(ctx, bot, msg, text)
		return
	}

	// 默认：复读消息
	cmdEcho(ctx, bot, msg, []string{text})
}

// executeCommand 执行命令
func executeCommand(ctx context.Context, bot imbot.Bot, msg imbot.Message, text string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}

	// 提取命令名 (去掉 / 前缀)
	cmdName := strings.ToLower(strings.TrimPrefix(parts[0], "/"))
	args := parts[1:]

	// 查找命令
	for _, cmd := range Commands {
		if cmd.Name == cmdName || slices.Contains(cmd.Aliases, cmdName) {
			if err := cmd.Handler(ctx, bot, msg, args); err != nil {
				log.Printf("命令 /%s 错误: %v", cmd.Name, err)
				sendReply(ctx, bot, msg, fmt.Sprintf("❌ 执行命令时出错: %v", err))
			}
			return
		}
	}

	// 未知命令
	sendReply(ctx, bot, msg, fmt.Sprintf("❓ 未知命令: /%s\n\n使用 /help 查看可用命令。", cmdName))
}

// handleCallback 处理回调
func handleCallback(ctx context.Context, bot imbot.Bot, msg imbot.Message) {
	text := msg.GetText()
	if !strings.HasPrefix(text, "callback:") {
		return
	}

	data := strings.TrimPrefix(text, "callback:")
	switch data {
	case "help":
		cmdStart(ctx, bot, msg, nil)
	case "status":
		cmdStatus(ctx, bot, msg, nil)
	case "time":
		cmdTime(ctx, bot, msg, nil)
	default:
		sendReply(ctx, bot, msg, fmt.Sprintf("收到回调: %s", data))
	}
}

// handleMediaMessage 处理媒体消息
func handleMediaMessage(ctx context.Context, bot imbot.Bot, msg imbot.Message) {
	media := msg.GetMedia()
	if len(media) == 0 {
		return
	}

	var response string
	switch media[0].Type {
	case "image":
		response = "🖼️ 收到图片！"
	case "video":
		response = "🎬 收到视频！"
	case "audio":
		response = "🎵 收到音频！"
	case "document":
		response = "📄 收到文档！"
	case "sticker":
		response = "😊 收到贴纸！"
	default:
		response = fmt.Sprintf("📎 收到媒体文件: %s", media[0].Type)
	}

	sendReply(ctx, bot, msg, response)
}

// ============================================================
// 命令处理器 - 所有平台共享
// ============================================================

func cmdStart(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	help := `🤖 欢迎使用多平台机器人！

这个机器人在多个平台同时运行，无论你从哪个平台发消息，都能得到相同的回复。

📝 可用命令:
/start, /help - 显示帮助
/ping - 检查机器人状态
/echo <消息> - 复读消息
/time - 显示当前时间
/info - 显示用户信息
/status - 显示机器人状态
/platform - 显示当前平台
/about - 关于机器人

💡 提示: 这个机器人的代码只写了一次，就能在所有平台运行！`

	return sendReply(ctx, bot, msg, help)
}

func cmdPing(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	start := time.Now()
	if err := sendReply(ctx, bot, msg, "🏓 Pong!"); err != nil {
		return err
	}
	latency := time.Since(start).Milliseconds()
	return sendReply(ctx, bot, msg, fmt.Sprintf("⏱️ 延迟: %dms", latency))
}

func cmdEcho(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	if len(args) == 0 {
		return sendReply(ctx, bot, msg, "📢 请输入要复读的内容。\n用法: /echo <消息>")
	}
	return sendReply(ctx, bot, msg, fmt.Sprintf("📢 %s", strings.Join(args, " ")))
}

func cmdTime(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	now := time.Now()
	timeStr := fmt.Sprintf("🕐 当前时间:\n📅 %s\n⏰ %s",
		now.Format("2006-01-02 Monday"),
		now.Format("15:04:05 MST"))
	return sendReply(ctx, bot, msg, timeStr)
}

func cmdInfo(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	info := fmt.Sprintf(`👤 用户信息:

🆔 ID: %s
👤 显示名: %s
📱 平台: %s`,
		msg.Sender.ID,
		msg.GetSenderDisplayName(),
		msg.Platform)

	if msg.Sender.Username != "" {
		info += fmt.Sprintf("\n🔒 用户名: %s", msg.Sender.Username)
	}

	return sendReply(ctx, bot, msg, info)
}

func cmdStatus(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	status := bot.Status()

	statusStr := fmt.Sprintf(`🤖 机器人状态:

🔗 连接: %s
🔐 认证: %s
✅ 就绪: %s`,
		boolEmoji(status.Connected),
		boolEmoji(status.Authenticated),
		boolEmoji(status.Ready))

	if status.Error != "" {
		statusStr += fmt.Sprintf("\n❌ 错误: %s", status.Error)
	}

	return sendReply(ctx, bot, msg, statusStr)
}

func cmdPlatform(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	platformInfo := bot.PlatformInfo()
	info := fmt.Sprintf(`📱 当前平台信息:

🏷️ 名称: %s
📝 显示名: %s
🔢 类型: %s`,
		msg.Platform,
		platformInfo.Name,
		msg.Platform)

	return sendReply(ctx, bot, msg, info)
}

func cmdAbout(ctx context.Context, bot imbot.Bot, msg imbot.Message, args []string) error {
	about := `ℹ️ 关于这个机器人

这是一个多平台机器人示例，展示了 imbot 框架的 "Write Once, Run Everywhere" 能力。

✨ 特性:
• 统一的消息处理逻辑
• 支持多个即时通讯平台
• 命令系统抽象
• 自动重连

📦 框架: github.com/tingly-dev/tingly-box/imbot
📌 版本: 1.0.0`

	return sendReply(ctx, bot, msg, about)
}

// ============================================================
// 辅助函数
// ============================================================

// sendReply 发送回复 - 处理不同平台的回复目标差异
func sendReply(ctx context.Context, bot imbot.Bot, msg imbot.Message, text string) error {
	// 获取正确的回复目标
	target := getReplyTarget(msg)
	_, err := bot.SendText(ctx, target, text)
	return err
}

// getReplyTarget 获取回复目标
// 不同平台的回复目标可能不同：
// - Telegram: Sender.ID (用户ID，但实际发送到聊天)
// - DingTalk: Recipient.ID (会话ID)
// - Discord: Recipient.ID (频道ID)
// - Feishu: Recipient.ID (会话ID)
func getReplyTarget(msg imbot.Message) string {
	switch msg.Platform {
	case core.PlatformDingTalk, core.PlatformFeishu:
		// 这些平台使用会话 ID 作为发送目标
		return msg.Recipient.ID
	default:
		// Telegram, Discord 等可以直接使用发送者 ID
		return msg.Sender.ID
	}
}

// isWhitelisted 检查白名单
func isWhitelisted(userID string) bool {
	if len(Whitelist) == 0 {
		return true
	}
	return slices.Contains(Whitelist, userID)
}

// isCallback 检查是否为回调消息
func isCallback(msg imbot.Message) bool {
	if msg.Metadata == nil {
		return false
	}
	if isCb, ok := msg.Metadata["is_callback"].(bool); ok {
		return isCb
	}
	return false
}

// boolEmoji 布尔值转 emoji
func boolEmoji(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}

// truncateText 截断文本
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// printStartupInfo 打印启动信息
func printStartupInfo(manager *imbot.Manager, configs []*imbot.Config) {
	log.Println()
	log.Println("═══════════════════════════════════════════")
	log.Println("🤖 多平台机器人已启动!")
	log.Println("═══════════════════════════════════════════")
	log.Printf("已启用平台: %d 个\n", len(configs))

	statuses := manager.GetStatus()
	for _, cfg := range configs {
		status := statuses[string(cfg.Platform)]
		emoji := "❌"
		if status != nil && status.Connected {
			emoji = "✅"
		}
		log.Printf("  %s %s", emoji, cfg.Platform)
	}

	log.Println()
	log.Println("📝 可用命令:")
	for _, cmd := range Commands {
		aliases := ""
		if len(cmd.Aliases) > 0 {
			aliases = fmt.Sprintf(" (别名: /%s)", strings.Join(cmd.Aliases, ", /"))
		}
		log.Printf("  /%-10s - %s%s", cmd.Name, cmd.Description, aliases)
	}

	log.Println()
	log.Println("按 Ctrl+C 停止机器人")
	log.Println("═══════════════════════════════════════════")
}
