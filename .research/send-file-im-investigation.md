# send file 在 IM 平台上不到达：调查记录

日期：2026-11-03。现象（用户实测）：Web 前端和飞书的 send file 正常；钉钉不送达；微信怀疑也有问题。

## 链路

`send_file` MCP 工具（`projects/internal/project/session_tools_mcp.go:62`）→ `PublishFile` 持久化并发布会话消息 → Relay（`projects/internal/project/relay.go:413`）对每个绑定的 IM chat 调 `SendChannelFile` → `ChatBridge.SendChannelFile`（`internal/server/module/projects/chatbridge.go:277`）找到 bot 的 channel 并断言 `SendFile` → `imchannel.Channel.SendFile`（`remote/channel/imchannel/imchannel.go`）→ `imbot.Bot.SendMessage(Media=[file://...])` → 各平台适配器。失败只写 Warn 日志并通过 `OnFileError` 在 Thread 里发错误消息，Web 里的文件不受影响。

## 根因（逐平台）

1. **白名单拒绝（钉钉、微信、企业微信共同）**：`imchannel.SendFile` 开头有 `platform != telegram/tingly/feishu/lark` 就返回 `file upload is not supported by <platform>` 的硬编码判断。三个平台根本没走到适配器。
2. **钉钉另有三处**：
   - `imbot/platform/dingtalk/dingtalk.go` 的 `sendMedia` 是空壳，对所有类型都返回 `ErrMediaNotSupported`（“requires upload via REST API first”）。
   - `SendMessage` 先判断 `opts.Text != ""` 再判断 media，`SendFile` 总带 caption，所以即使放开白名单，附件也会被当纯文本路径丢掉，且不报错。
   - 钉钉 Stream 模式的 session webhook 只能发文本/markdown 等，不能发图片/文件，且会过期；必须走机器人 OpenAPI：先 `oapi /media/upload` 得 media_id，群聊 `groupMessages/send`、单聊 `oToMessages/batchSend`。
3. **微信（Weixin）**：适配器上传 + 发送已完整（SDK `WechatBot.SendMedia`），问题只在白名单；另有两个小缺陷：SDK 的 `SendMedia` 只发附件，`msg.Text` 说明被静默丢弃；`SendFile` 没带 `tokenMetadata`，主动发送时缺回复上下文 token（文本路径 `Send` 带了），会 ret=-2。本地文件不存在时还会把路径当 URL 去下载，报错难懂。
4. **企业微信**：SDK 提供 `UploadMedia`（WebSocket 分片上传，返回 media_id），但适配器 `sendMedia` 从不调用，要求调用方在 Metadata 里预先放 `wecom_media_id`，而 `SendFile` 永远不会放。说明文字同样被丢。
5. **Slack / Discord / WhatsApp**：同类问题但性质不同——适配器本身是占位（发 URL 文本、空 Reader、只传 link）。白名单挡住它们是正确的，保留。本次不实现，记入 TEP。

## 修复

见提交 `fix(imbot): deliver send_file to DingTalk, WeChat and WeCom`。关键点：

- 白名单改成显式 `fileUploadPlatforms` 并加入 dingtalk/weixin/wecom；`SendFile` 带回复上下文 token。
- 钉钉：新增 `imbot/platform/dingtalk/media.go`（上传、OpenAPI 发送、`invalidStaffIdList`/HTTP/errcode 均转为错误）；记录每个会话是否单聊及 staffId；`SendMessage` 先走 media。
- 企业微信：本地文件先 `UploadMedia` 再 `SendMedia`；上传类型与消息类型保持一致；预置 `wecom_media_id` 仍可跳过上传。为测试加了 `outbound` 接口缝隙。
- 微信、企业微信、钉钉的说明文字作为附件后的单独消息（与飞书一致）。

## 验证

- 单元测试（`go test ./imbot/... ./remote/channel/... ./internal/server/module/projects/...` 全过）：
  - 钉钉：`httptest` 模拟 gettoken、media/upload、两个机器人发送接口，校验 multipart 上传、msgKey/msgParam、群聊/单聊分流、caption、HTTP 403/errcode/invalidStaffIdList/缺文件/超限/未知会话的错误。
  - 微信：`httptest` 模拟 ilink getuploadurl、CDN（返回 x-encrypted-param）、sendmessage，走真实 SDK 的加密上传流程，校验 file_item/image_item、说明消息、context_token。
  - 企业微信：接口替身校验上传类型、media_id、消息类型、caption、错误路径。
  - `imchannel`：dingtalk/weixin/wecom/telegram 放行并携带 token，slack/discord/whatsapp 拒绝，发送错误上传。
- **没有**对真实钉钉、微信、企业微信发过请求（无凭据、无网络访问）。请求格式依据平台公开文档和示例核对：钉钉 `sampleImageMsg.photoURL` 接受 media_id（社区实测，官方页面无法在本环境打开）、`sampleFile.fileType` 官方仅列 xlsx/pdf/zip/rar/doc/docx（其他扩展名行为未知）、20MB 上限取自第三方文档。

## 需要用户实机确认

1. 钉钉：群聊发图片、群聊发 pdf、单聊发图片、单聊发非常见扩展名文件（如 .md/.zip/.txt）；机器人需有“企业内部机器人发送消息”权限（`qyapi_robot_sendmsg`）。注意重启后需对方先发一条消息才能发文件（和文本同样限制）。
2. 微信：单聊发图片、发 pdf，含重启后的主动发送。
3. 企业微信：单聊/群聊发图片、发文件。
