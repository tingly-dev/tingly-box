# Feishu / Lark Channel & Keyboard Interaction (oapi-sdk-go v3)

Reference notes for the Feishu/Lark bot in `imbot/platform/feishu`. Distilled from the
SDK source (`github.com/larksuite/oapi-sdk-go/v3 v3.7.5`) and the upstream channel doc:
https://github.com/larksuite/oapi-sdk-go/blob/v3_main/doc/channel.md

The upstream `channel.md` documents a high-level `Channel` wrapper (`OnMessage`,
`OnReaction`, `OnCardAction`, lifecycle hooks). We use the lower-level
`ws.Client` + `event.EventDispatcher` directly, which is what `Channel` is built on.

## Receiving events (WebSocket long connection)

```go
handler := dispatcher.NewEventDispatcher("", "").
    OnP2MessageReceiveV1(b.handleP2MessageReceiveV1).         // text/post/media messages
    OnP2CardActionTrigger(b.handleCardActionTrigger).          // interactive card button clicks
    OnP2MessageReactionCreatedV1(b.handleMessageReactionCreated).
    OnP2MessageReactionDeletedV1(b.handleMessageReactionDeleted)

ws := larkws.NewClient(appID, appSecret,
    larkws.WithEventHandler(handler),
    larkws.WithDomain(domain),           // lark.FeishuBaseUrl or lark.LarkBaseUrl
    larkws.WithLogLevel(larkcore.LogLevelInfo))
ws.Start(ctx)
```

Notes:
- The dispatcher panics if the **same** event type is registered twice; different types
  on one dispatcher are fine.
- Every event type the app is **subscribed to** must have a handler, or the SDK logs
  `handle message failed ... not found handler`. We register no-op reaction handlers for
  this reason.
- `larkws.WithCardHandler` is **commented out** in v3.7.5 — card actions over WebSocket
  must come through the dispatcher's `OnP2CardActionTrigger`, not a separate card handler.

### Relevant P2 (v2) message handlers (package `service/im/v1`)
`OnP2MessageReceiveV1`, `OnP2MessageReadV1`, `OnP2MessageRecalledV1`,
`OnP2MessageReactionCreatedV1`, `OnP2MessageReactionDeletedV1`,
`OnP2ChatMemberUserAddedV1` / `…DeletedV1`, `OnP2ChatMemberBotAddedV1` / `…DeletedV1`,
`OnP2ChatAccessEventBotP2pChatEnteredV1`, `OnP2ChatUpdatedV1`, `OnP2ChatDisbandedV1`.

### Card action handler (package `event/dispatcher/callback`)
```go
func(ctx context.Context, e *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error)
```
Key fields on `e.Event` (`*callback.CardActionTriggerRequest`):
- `Action.Value map[string]interface{}` — the button's `value` payload. We route on
  `Value["callback"]` (set by both `buildInteractiveCard` and `feature.FeishuCardRenderer`).
- `Operator` — `UserID *string`, `OpenID string` (the user who clicked).
- `Context` — `OpenMessageID`, `OpenChatID` (the message/chat hosting the card).
- `Token` — credential for updating the card in the response.

Return value: a `*CardActionTriggerResponse` may carry a `Toast` and/or a replacement
`Card`. We return `nil, nil` (silent ack) because remote_control posts a fresh result
message instead of mutating the card in place.

## Sending messages (HTTP client)

`client.Im.Message.Create` with a `receive_id_type` that **must match** the id format —
mismatches cause API error `99992361 "open_id cross app"`.

`getReceiveIdType` rules (see `bot_sdk.go`):

| id prefix / form | receive_id_type |
|------------------|-----------------|
| `ou_…`           | `open_id`       |
| `on_…`           | `union_id`      |
| `oc_…`           | `chat_id` (p2p **and** group) |
| contains `@`     | `email`         |
| otherwise        | `user_id`       |

Message types: `text` (`{"text":"..."}`), `interactive` (card JSON). Markdown is sent as
an interactive card with a `lark_md` div. Edit via `client.Im.Message.Patch` (interactive
content only). React via `client.Im.MessageReaction.Create`.

## Inbound media (resource download)

Image/file/audio/media messages carry a key, not a URL. `convertLarkMessageToCore` turns
them into a `core.MediaContent` whose attachment has:
- `URL = "feishu://" + fileKey` (a marker; not an HTTP URL),
- `Raw["feishu_message_id" | "feishu_file_key" | "feishu_res_type"]` (resType is
  `image` or `file`),
- a best-effort `MimeType` (`image/png` for images; inferred from the file extension for
  files — office formats may fall back to `application/octet-stream` if the host lacks a
  mime.types entry, in which case the upload is rejected).

The bytes are fetched with `client.Im.MessageResource.Get(messageID, fileKey, resType)`,
exposed as `FeishuBot.DownloadMessageResource`. In the remote bot, `FileStore` recognizes
the `feishu://` scheme and downloads via that callback (`SetFeishuDownloader` /
`DownloadFeishuResource`).

## Outbound media

`sendMedia` uploads each attachment, then sends it:
- images (`image`/`sticker`/`gif`) → `client.Im.Image.Create` (`image_type:"message"`) →
  `image_key` → `image` message.
- everything else → `client.Im.File.Create` (`file_type` mapped from the extension via
  `feishuFileType`, e.g. pdf/doc/xls/ppt/mp4/opus, else `stream`) → `file_key` → `file`
  message.

Attachment sources: local paths (optionally `file://`) and `http(s)://` URLs
(`openMediaReader`). Note `SendMessage` routes to `sendText` when `opts.Text` is non-empty,
so media and text are sent as separate messages.

## Message editing & capabilities

Feishu/Lark capabilities now include `messageEditing` and `callbackQueries`.
`Bot.EditMessage` patches a message to a markdown card (which also drops any inline
keyboard). The remote bot uses a capability check (`SupportsFeature("messageEditing")`)
rather than a Telegram-only cast to retire prompt/resume keyboards in place
(`closePromptMessage`, `stripKeyboardWithStatus`).

The post-completion action menu is platform-aware: the `CD`/bind button (which needs the
edit-in-place directory browser) is omitted on platforms where `SupportsDirectoryBrowser`
is false — i.e. everywhere except Telegram today.

## How tingly-box wires this

- **Reply target = chat id (`oc_…`)** for both inbound messages
  (`convertLarkMessageToCore`) and card callbacks (`handleCardActionTrigger`). Using the
  chat id uniformly keeps the per-chat conversation key stable across messages and button
  clicks (bind flow, directory browser) and avoids the `open_id cross app` failure.
- **Callback → core.Message contract** (so the shared Telegram-style router in
  `internal/remote_control/bot` handles Feishu unchanged): metadata
  `is_callback=true`, `callback_data=<string>`, `message_id=<open_message_id>`,
  `original_chat_id`, `feishu_card_token`.
- **Group whitelist (`/join`):** `resolveGroupChatID` is platform-aware — Telegram resolves
  @username/invite-link/ID, while Feishu/Lark take the group `chat_id` (`oc_…`) directly
  (the bot logs it on the first group message). Extend `joinSupportedPlatforms` +
  `resolveGroupChatID` for new platforms.
- **Sending keyboards:** `imbot.BuildKeyboardMetadata(platform, kb)` emits the neutral
  `interaction.InlineKeyboardMarkup` under `replyMarkup` for Feishu/Lark (rendered by
  `buildInteractiveCard`), or the Telegram struct otherwise. Pre-rendered cards are passed
  as `card_json` and sent verbatim by `sendText`/`sendRawCard`.

## Button value contract

Buttons must carry the routing string under the `callback` key so `handleCardActionTrigger`
can extract it:
```go
larkcard.NewMessageCardEmbedButton().
    Text(larkcard.NewMessageCardPlainText().Content(label)).
    Value(map[string]interface{}{"callback": callbackData, "actionId": id})
```
</content>
