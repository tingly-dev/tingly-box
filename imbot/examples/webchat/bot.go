package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// SimpleBot is a simple echo bot with some basic commands
type SimpleBot struct {
	name string
}

// NewSimpleBot creates a new simple bot
func NewSimpleBot(name string) *SimpleBot {
	return &SimpleBot{name: name}
}

// HandleMessage processes incoming messages and returns responses
func (b *SimpleBot) HandleMessage(_ context.Context, msg imbot.Message) (string, bool) {
	text := strings.TrimSpace(msg.GetText())

	// Handle commands
	if strings.HasPrefix(text, "/") {
		return b.handleCommand(context.Background(), msg, text), true
	}

	// Echo with a prefix
	return fmt.Sprintf("📨 %s\n\n💭 You said: \"%s\"", b.getResponseGreeting(), text), true
}

// handleCommand handles bot commands
func (b *SimpleBot) handleCommand(_ context.Context, msg imbot.Message, cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "❓ Empty command"
	}

	cmdName := strings.ToLower(parts[0][1:])
	args := parts[1:]

	switch cmdName {
	case "start", "help":
		return b.getHelpMessage()

	case "ping":
		return "🏓 Pong! ⚡ Your message reached me instantly!"

	case "time":
		now := time.Now()
		return fmt.Sprintf("🕐 Current time:\n📅 %s\n⏰ %s",
			now.Format("2006-01-02 Monday (MST)"),
			now.Format("15:04:05"))

	case "date":
		now := time.Now()
		return fmt.Sprintf("📅 Today's date: %s", now.Format("January 2, 2006"))

	case "echo":
		if len(args) == 0 {
			return "📢 Usage: /echo <message>\nExample: /echo Hello World"
		}
		return fmt.Sprintf("📢 %s", strings.Join(args, " "))

	case "reverse":
		if len(args) == 0 {
			return "🔃 Usage: /reverse <text>\nExample: /reverse hello"
		}
		text := strings.Join(args, " ")
		runes := []rune(text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return fmt.Sprintf("🔃 Reversed: %s", string(runes))

	case "upper":
		if len(args) == 0 {
			return "🔠 Usage: /upper <text>\nExample: /upper hello"
		}
		return fmt.Sprintf("🔠 %s", strings.ToUpper(strings.Join(args, " ")))

	case "lower":
		if len(args) == 0 {
			return "🔡 Usage: /lower <text>\nExample: /lower HELLO"
		}
		return fmt.Sprintf("🔡 %s", strings.ToLower(strings.Join(args, " ")))

	case "info":
		return fmt.Sprintf(`👤 Your information:
🆔 User ID: %s
👤 Display Name: %s
🌐 Platform: %s`,
			msg.Sender.ID,
			msg.GetSenderDisplayName(),
			msg.Platform)

	case "about":
		return fmt.Sprintf(`ℹ️ About %s

I'm a simple demo bot powered by imbot framework!

🤖 Platform: WebChat
📦 Framework: github.com/tingly-dev/tingly-box/imbot
✨ Features: Echo, commands, real-time chat

Try /help to see all commands!`, b.name)

	case "joke":
		jokes := []string{
			"Why do programmers prefer dark mode? Because light attracts bugs! 🐛",
			"Why did the developer go broke? Because he used up all his cache! 💰",
			"There are only 10 types of people in the world: those who understand binary and those who don't! 🔢",
			"A SQL query walks into a bar, walks up to two tables, and asks: 'Can I join you?' 🍺",
			"Why do Java developers wear glasses? Because they can't C! 👓",
		}
		return "😄 " + jokes[int(time.Now().Unix())%len(jokes)]

	case "quote":
		quotes := []string{
			"\"The only way to do great work is to love what you do.\" - Steve Jobs",
			"\"Code is like humor. When you have to explain it, it's bad.\" - Cory House",
			"\"First, solve the problem. Then, write the code.\" - John Johnson",
			"\"Experience is the name everyone gives to their mistakes.\" - Oscar Wilde",
			"\"The best error message is the one that never shows up.\" - Thomas Fuchs",
		}
		return "💭 " + quotes[int(time.Now().Unix())%len(quotes)]

	case "roll":
		max := 100
		if len(args) > 0 {
			fmt.Sscanf(args[0], "%d", &max)
		}
		roll := int(time.Now().Unix()) % max
		return fmt.Sprintf("🎲 You rolled: %d (out of %d)", roll+1, max)

	case "flip":
		result := "Heads"
		if int(time.Now().Unix())%2 == 0 {
			result = "Tails"
		}
		return fmt.Sprintf("🪙 Coin flip: %s", result)

	case "8ball":
		responses := []string{
			"It is certain! ✅",
			"Without a doubt! 👍",
			"Yes, definitely! 💯",
			"Ask again later 🤔",
			"Cannot predict now 🎱",
			"Don't count on it ❌",
			"My sources say no 🚫",
		}
		return "🎱 " + responses[int(time.Now().Unix())%len(responses)]

	default:
		return fmt.Sprintf("❓ Unknown command: /%s\n\nType /help to see available commands.", cmdName)
	}
}

// getHelpMessage returns the help message
func (b *SimpleBot) getHelpMessage() string {
	return `👋 Welcome to the WebChat Demo Bot!

📚 Available Commands:

🤖 Basic Commands:
  /start, /help - Show this help message
  /ping        - Check if bot is responsive
  /about       - About this bot

💬 Text Commands:
  /echo <text>    - Echo back your message
  /reverse <text> - Reverse the text
  /upper <text>   - Convert to UPPERCASE
  /lower <text>   - Convert to lowercase

📊 Info Commands:
  /time   - Show current time
  /date   - Show today's date
  /info   - Show your user information

🎮 Fun Commands:
  /roll [max]  - Roll a dice (default: 100)
  /flip        - Flip a coin
  /8ball       - Magic 8-ball response
  /joke        - Tell a random joke
  /quote       - Show an inspirational quote

💡 Tip: You can also just send any text and I'll echo it back!`
}

// getResponseGreeting returns a random greeting
func (b *SimpleBot) getResponseGreeting() string {
	greetings := []string{
		"Hello there!",
		"Hi!",
		"Hey!",
		"Howdy!",
		"Greetings!",
	}
	return greetings[int(time.Now().Unix())%len(greetings)]
}
