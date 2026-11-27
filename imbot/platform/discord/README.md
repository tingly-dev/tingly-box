# platform/discord

Discord bot implementation using [discordgo](https://github.com/bwmarrin/discordgo).

## Authentication

**Type:** `token`

Create a bot application in the [Discord Developer Portal](https://discord.com/developers/applications),
generate a bot token under **Bot → Token**, and invite the bot to your server with the required permissions.

```go
Auth: core.AuthConfig{
    Type:  "token",
    Token: "MTxxxxxxxxxxxxxxxxxxxxxxxx.Gxxxxx.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
}
```

The `"Bot "` prefix is added automatically if omitted from the token string.

## Configuration

```go
&core.Config{
    Platform: core.PlatformDiscord,
    Auth: core.AuthConfig{
        Type:  "token",
        Token: os.Getenv("DISCORD_BOT_TOKEN"),
    },
    Options: map[string]interface{}{
        // Gateway intents to subscribe to.
        // Default: Guilds, DirectMessages, GuildMessages, MessageContent.
        "intents": []interface{}{
            "guilds",
            "directMessages",
            "guildMessages",
            "messageContent",
        },
    },
}
```

### Available intent strings

`guilds`, `guildMembers`, `guildBans`, `guildEmojis`, `guildIntegrations`,
`guildWebhooks`, `guildInvites`, `guildVoiceStates`, `guildPresences`,
`guildMessages`, `guildMessageReactions`, `guildMessageTyping`,
`directMessages`, `directMessageReactions`, `directMessageTyping`,
`messageContent`, `guildScheduledEvents`.

Note: `messageContent` is a **privileged intent** that must be enabled in the
Developer Portal for bots in 100+ servers.

## Connection model

`Connect` opens a **WebSocket connection to the Discord Gateway** via
`discordgo.Session.Open()`. The session is managed by discordgo and
reconnects automatically on transient failures.

Two Gateway event handlers are registered:

| Event | Purpose |
|---|---|
| `MessageCreate` | Incoming guild and DM messages |
| `Ready` | Marks the bot as ready after gateway handshake |

## Message ID format

Discord encodes message references as `"channelID:messageID"` (colon-separated).
`React`, `EditMessage`, and `DeleteMessage` all parse this format to call the
appropriate discordgo API.

## ID routing

Inbound `MessageCreate` events store `msg.ChannelID` as `Recipient.ID`
(`adapter.go`), and that channel id is what `SendMessage` posts to. Discord
channel, user and guild ids are all snowflakes, so the shape alone cannot tell
them apart; the record's origin (a received message) is what makes it a
channel. A user id must not be used as a send target.

## Capabilities

| Feature | Supported |
|---|---|
| Send text (markdown native) | ✅ |
| Send media (images, files) | ✅ |
| Reply (message reference) | ✅ |
| Reactions | ✅ |
| Edit message | ✅ |
| Delete message | ✅ |
| Mentions / DMs | ✅ |
| Components (buttons, select menus) | ❌ |
| Threads | ❌ |
| Text limit | 2 000 chars |

## Files

| File | Purpose |
|---|---|
| `discord.go` | `Bot` struct, lifecycle, send/react/edit/delete; intent parsing |
| `adapter.go` | Converts `discordgo.Message` → `core.Message`; media/embed extraction |
