# ImBot 输出预估 — Predicted Per-Platform Chat Output, and the Noise Problem

> 目的：在做任何改动之前，先把"Remote 通过 ImBot 实际发到各平台聊天窗口里的东西长什么样"钉死成可核对的事实（附代码依据），再评估要不要优化。本文是预估 + 现状分析，**不包含代码改动**——是否优化、优化到什么程度，等待决策后再开新 PR。
>
> 关联文档：`.design/bot-arch.md`（资源/通道/consumer 三层模型）、`.design/ux-principles.md`（判断标准，尤其是 #6 合理默认值、#9 降低视觉噪声）、`.design/imbot-sync.md`。

## 1. 一句话结论

Remote 目前给用户的"每一轮对话"天然拆成 **3–4 条以上的独立消息**，其中至少两条（进度横幅 + Task-done 卡片）是无论内容多简单都固定发送的"结构性消息"；权限确认在**所有平台**上都会重复发送两次；7/10 个平台完全不支持编辑/撤下已发的消息，导致按钮消息永久留在聊天记录里失效但仍可点。这些不是某个平台的个别问题，而是当前架构的固定产出——值得作为一个整体优化项来看，而不是零散修 bug。

## 2. 两条产出消息的路径

Remote 里能往 IM 发消息的代码只有两条源头，二者共用同一个 `imbot.Bot.SendMessage`/`imchannel.Channel`，但触发方式和内容完全不同：

- **Flow A — notify（hook 驱动，claude_code scenario plugin）**
  `remote/scenario/builtin/claudecode/plugin.go`。适用于用户在自己终端/无人值守跑 Claude Code、配置了 hook 把事件 POST 到 `/tingly/:scenario/notify` 的场景（"旁观模式"）。`PostToolUse` → "Tool call finished: X"，`Stop` → 最后一句话（截断到 240 字）或 "Task completed"，`Notification` → 原样转发或 "Needs attention"（`plugin.go:355-375`）。**每个 hook 事件都是一条新消息**，`imchannel.Channel.Send`（`imchannel.go:90-108`）从不编辑、不聚合。
- **Flow B — chat control（`@cc` via remote_agent，BotHandler → ClaudeCodeExecutor → streamingMessageHandler）**
  这是日常使用 Remote 的主路径：用户在聊天里 `@cc <任务>`，tingly-box 直接托管 Claude Code 进程并把每个 `claude.Message` 渲染成聊天消息（`remote/control/remoteagent/stream.go`）。本文下面的"典型输出"以这条路径为主，因为它是绝大多数用户实际感知到的"输出"。

两条路径都不做跨消息合并，也都不知道对方的存在——同一个 chat 里同时开着 notify 绑定和 `@cc` 时，两条路径会各自往同一个聊天窗口发消息，互不去重。

## 3. 实测推演：一次典型 `@cc` 任务长什么样

以"改一个文件、跑一次工具调用、给出一句话结论"这种最简单的任务为例，按代码逐条还原实际会发送的消息（每一条都是**独立的物理消息**，除非标注"原地编辑"）：

1. `⏳ CC: Processing new session...`
   `───────────────`
   `💬 @cc`
   `📁 project-name`
   —— `executor_claude.go:90-97`，`BuildFooter`（`output.go:65-83`）。每轮固定发送。

2.（如果这轮触发了权限确认）弹出按钮消息 `🔐 Tool Permission Request / Tool: \`Bash\`` + [✅ Allow][❌ Deny][🔄 Always] —— `imprompter.go:396-435`。用户点按钮或回复文字后：
   - **在 Telegram / Feishu 上**：原消息被 `Restate` 原地改成 `... \n\n✅ *Approved*`，按钮消失（`imprompter.go:522-542`）。
   - **在其余 7 个平台上**：`RestateOrIgnore` 直接返回 false，退化成"再发一条新消息"，而**原来的按钮消息不会消失，永久留在聊天记录里，按钮仍可点击**（`core/restate.go:59-75`，见下节明细）。
   - **无论哪个平台**，还会**再多发一条**独立的确认消息：`✅ Allow · for tool: \`Bash\`` —— 这条来自 `remote/control/bot/prompt_reply.go:208`（`HandlePromptCallback`）/ `prompt_reply.go:276`（`HandlePromptTextReply`），和上面 `editPromptToResult` 是两处完全独立的代码，**永远同时触发，是一个稳定复现的重复消息 bug，和平台无关**。

3. 工具调用聚合消息，如 `🔧 3 tool call(s)\n• 🔧 Edit src/foo.go\n• ↳ ok\n• 🔧 Bash go test ./...\n…(+1 more)`（quiet 模式）或逐行展开（verbose 模式）—— `stream.go:198-364`。这是已经做了缓冲聚合的部分（见第 5 节），但缓冲区在遇到下一条"有文本"的消息时就会 flush，长任务里仍会拆成好几条。

4. 助手文字回复本身，例如 `已经把 foo.go 改好了，测试通过。` —— 单独一条。

5. `[RESULT] SUCCESS`
   `Duration: 4231ms (API: 3900ms)`
   `Cost: $0.0142`
   `Tokens: 812 in, 305 out`
   —— `render/formatter.go:366-420`。**这条固定发送，quiet 模式下也不例外**（`stream.go:257-263` 的过滤白名单里 `msgType == "result"` 直接放行）。用的是 `[SYSTEM]` `[RESULT]` `[SUBAGENT]` `[UNKNOWN]` `[STREAM] +JSON:...` 这种日志/调试风格的方括号标签，和同一个包里 `output.go` 已经定义好的 `✅ ❌ 🔧 💭` 图标体系完全不是一套东西——`render.TextFormatter` 没有复用 `remoteagent` 的图标常量。

6. `✅ Task done. Continue or /help.`
   `───────────────`
   `💬 @cc`
   `📁 project-name`
   + [🗑 Clear][📁 CD][🔧 Project] 按钮 —— `executor_claude.go:236,384-397`。同样固定发送。

**结果：一个 0 次权限确认、0 个额外工具调用之外的最简单任务，最少也是 4 条消息**（Processing → 回复 → [RESULT] → Task done）；真实的多步编码任务轻松做到 8–15+ 条，且第 1 条和第 6 条把同样的"分隔线 + agent 图标 + 项目路径"footer 在几秒内重复打印了两遍。

## 4. 噪声来源排名（按影响面从大到小）

1. **结构性消息是固定成本，与任务内容无关**：Processing 横幅 + [RESULT] 统计块 + Task-done 卡片，三条消息不随任务简单/复杂而增减，简单任务里占比反而最高。
2. **`[RESULT]` 调试风格输出**：`[SYSTEM]` `[RESULT]` `[SUBAGENT]` `[UNKNOWN]` 这类方括号标签是给人 debug 用的格式，直接进了终端用户的聊天窗口；且这条消息不受 quiet mode 控制，用户关不掉。
3. **权限确认重复发送两次**，在全部平台上稳定复现（第 3 节第 2 条），是一个可以独立修的 bug，不需要等"要不要优化"的大决策。
4. **7/10 平台不支持消息编辑/撤下**（`imbot.MessageRestater` 只有 `telegram`、`feishu`、内部测试用的 `tingly` 实现了；`grep -rl Restate imbot/platform/` 确认 Discord / Slack / Lark / DingTalk / Weixin / WeCom / WhatsApp 都没有）：所有进度消息、权限按钮消息，一旦发出就永远留在聊天记录里，越用越长，失效的按钮还留在那里可以误触。
5. **4/10 平台没有按钮能力**（`SupportsInteraction()` 靠 `inlineKeyboards`/`interactiveCards`/`components`/`blockKit` 四个 feature flag 之一，`core/platforms.go` 里 DingTalk / Weixin / WeCom / WhatsApp 都没有任何一个）：权限确认和 `AskUserQuestion` 在这 4 个平台上会退化成一段编号列表 + "回复数字"的纯文字说明（`imprompter.go:474-514`），比按钮更占屏幕、且要求用户手动输入。
6. **低 TextLimit 平台会被动二次拆条**：Discord 2000 字符、DingTalk/WeCom/WhatsApp ~4000 字符（`core/platforms.go`），`BaseBot.ChunkText()` 会把超长的 verbose 工具聚合块（最多 20 行才 flush，见 `toolBufferFlushThreshold`）或长回复自动切成 2+ 条，进一步增加消息数。
7. **verbose 默认开启**：`GetVerbose`（`handler_verbose.go:10-31`）里 `h.botSetting.Verbose == nil || *h.botSetting.Verbose` —— 没显式设置就是 true。目前只有 Weixin 通过 `PlatformBehavior.SuppressVerbose` 被强制静音（因为它的回复要绑定 `context_token`，多发根本会失败/错位，这是能力限制不是产品选择），WeCom 在能力上和 Weixin 几乎一样窄（`Features: ["streaming"]`，没有交互按钮）却没有拿到同样的 `SuppressVerbose`，是一个不对称的遗漏。

## 5. 已经存在的缓解措施（不是从零开始）

为避免这份文档显得"通篇都是问题"，值得明确记录当前代码里已经做对的部分，后续优化应该在这些基础上做增量：

- `streamingMessageHandler` 已经把 tool_use/tool_result 缓冲聚合成一条消息，而不是每个工具调用发一条（`stream.go:198-211, 302-327`），quiet 模式下还会把聚合结果压缩成"N 个工具调用 + 前 3 个预览 + (+N more)"（`stream.go:341-364`）。
- quiet/verbose 是逐 chat 可配置的（`chatStore` 里的 `Chat.Verbose`），不是全局唯一开关。
- 权限确认在 Telegram / Feishu 上已经用 `Restate` 做了"原地改写、按钮消失"，而不是无脑发新消息——`AsRestater` 这层抽象本身设计得很干净（`core/restate.go` 的注释写得很清楚：表达意图而非机制），只是目前只有两个平台接了。
- `api_retry`/`rate_limit` 这类瞬时失败通知，即使在 quiet 模式下也会保留（`isRetryNotice`，`stream.go:269-278`），这是刻意的、合理的例外，不是噪声。
- Weixin 的 `SuppressVerbose` 已经证明"按平台能力强制降噪，而不是留给用户/运营去配置"这个方向是可行的、已经在生产里跑着的模式（呼应 `.design/ux-principles.md` #6 合理默认值优于多一个开关）。

## 6. 分平台能力矩阵与预估体验

| Platform | 按钮支持 `SupportsInteraction()` | 编辑/撤下 `MessageRestater` | TextLimit | 预估体验 |
|---|---|---|---|---|
| Telegram | ✅ inlineKeyboards | ✅ 原生编辑 | 4096 | 相对最好：权限卡片会原地收起；但 Processing / 工具聚合 / [RESULT] / Task-done 仍然是几条独立消息，长任务照样刷屏 |
| Discord | ✅ components | ❌ | 2000 | 权限按钮消息永久残留可再点；限额低，容易被动拆条 |
| Slack | ✅ blockKit | ❌ | 40000 | 同 Discord 的"按钮残留"问题；限额宽松，不会被动拆条 |
| Feishu | ✅ interactiveCards | ✅ 卡片可 patch | 40000 | 和 Telegram 一样，权限卡片体验最好；其余消息仍不合并 |
| Lark | ✅ interactiveCards（能力声明与 Feishu 相同） | ❌ **未实现**，尽管声明了 `interactiveCards` | 40000 | 能力表看着和 Feishu 一样，实际是"Feishu 有、Lark 没有"的落差——同一套字节系产品体验不一致，容易被当成 bug 反馈 |
| DingTalk | ❌ 无按钮 feature | ❌ | 4000 | 权限/问答一律退化成编号文字列表，要求手动回复数字；限额低 |
| Weixin | ❌（默认能力表，`Features: []`） | ❌ | 默认（未声明，走保守默认） | 已被 `SuppressVerbose` 强制静音，是体验最差但也是唯一被"主动治理"过的平台；权限仍是文字列表；`context_token` 耦合使 notify 类主动消息更脆弱（回复不在同一上下文里会直接失败） |
| WeCom | ❌（`Features: ["streaming"]`，无按钮） | ❌ | 4000 | 能力和 Weixin 接近，但**没有**拿到 `SuppressVerbose`——目前会把完整的 verbose 消息流（含文字降级的权限请求）怼过去，预计是体验最差的平台 |
| WhatsApp | ❌ | ❌ | 4096 | 同 DingTalk：按钮全部退化成文字编号列表 |
| Tingly（内部测试） | ✅ 全部 | ✅ | 65536 | 仅供测试，不面向真实用户 |

## 7. 值得讨论的优化方向（未实现，供决策）

按"改动成本 vs. 收益"粗排，不代表最终优先级：

1. **去掉权限确认的重复发送**（第 4 节第 3 条）——`prompt_reply.go` 里的 `send(...)` 和 `imprompter.go` 的 `editPromptToResult` 二选一保留即可，逻辑上就是同一件事被两处独立实现。改动面小，全平台都受益，且不涉及"要不要更激进地精简输出"这种产品判断，可以单独拆出来先修。
2. **`[RESULT]` 统计块**：要么纳入 quiet mode 的过滤白名单（默认不发，`/stats`或详情按钮里按需查看），要么至少换成和 `output.go` 一致的图标风格而不是 `[SYSTEM]`/`[RESULT]` 方括号——不管选哪个，"调试格式直接进用户聊天窗口"本身在任何决策下都算缺陷，呼应 ux-principles #9（降低视觉噪声，让主体成为视觉锚点：这里的"主体"应该是助手的回答，不是 duration/cost 元信息）。
3. **合并/精简结构性消息**：Processing 横幅 + Task-done 卡片各带一份 footer，能否把 Processing 横幅在支持 `Restate` 的平台（Telegram/Feishu）上直接原地改写成 Task-done 卡片，而不是分开发两条；不支持 `Restate` 的平台再退化成两条。这样"编辑能力强的平台体验更干净"能自然形成分层，而不是所有平台都按最低能力对齐。
4. **verbose 默认值按平台能力收敛，而不是按平台白名单强制关闭**：现状是"Weixin 特例静音，其余全部默认 verbose=true"，更一致的做法是让默认值跟着 `SupportsInteraction()`/`TextLimit` 这类已有能力信号走（例如：无按钮能力的平台默认 quiet，因为它们每条消息的相对噪声成本更高），WeCom 至少应该先补上和 Weixin 一致的 `SuppressVerbose`（这是个能力事实，不是产品选择，可以直接修，参考第 4 节第 7 条）。
5. **补齐 `Restate` 到 Discord / Slack / Lark**：三个平台原生都有"编辑已发消息"的 API（Discord message edit、Slack `chat.update`、Lark 卡片 callback，和 Feishu 是同一套协议），实现成本应该接近 Feishu 已经做过的那份，收益是这三个平台的权限按钮不再永久残留。DingTalk/Weixin/WeCom/WhatsApp 没有对应能力，只能在"少发消息"上做文章，不能指望编辑。
6. **DingTalk/Weixin/WeCom/WhatsApp 的文字降级问答**：既然这 4 个平台注定要用纯文字，可以把"编号列表 + 回复说明"做得更紧凑（现状已经比较克制，`imprompter.go:474-514`），但更大的收益还是来自 3、4 两条——少发消息本身比优化单条消息的排版更有效。

## 8. 待决策的问题

- 第 7 节第 1 条（重复确认消息）建议直接当 bug 修，不需要等"整体要不要精简输出"的产品决策——需要确认吗？
- `[RESULT]` 统计块是否要默认隐藏：这涉及"技术用户想看 cost/tokens" vs "普通用户觉得是噪声"的取舍，可能需要一个用户可见的开关，而不是完全去掉（呼应 ux-principles #6：默认值要选对，但"合理默认值优于多一个开关"不代表永远不能有开关，只是默认值要选对）。
- 是否值得把 Flow A（hook notify）和 Flow B（`@cc` 流式）在同一个 chat 里共存时做去重/合流——目前没有证据表明这是常见用法，暂不列入本轮范围，但架构上两条路径完全独立这一点值得记录以防未来踩坑。
