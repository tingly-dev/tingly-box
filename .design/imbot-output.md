# ImBot 输出预估 — Predicted Per-Platform Chat Output, and the Noise Problem

> 目的：在做任何改动之前，先把"Remote 通过 ImBot 实际发到各平台聊天窗口里的东西长什么样"钉死成可核对的事实（附代码依据），再评估要不要优化。本文是预估 + 现状分析，**不包含代码改动**——是否优化、优化到什么程度，等待决策后再开新 PR。
>
> 关联文档：`.design/bot-arch.md`（资源/通道/consumer 三层模型）、`.design/ux-principles.md`（判断标准，尤其是 #6 合理默认值、#9 降低视觉噪声）、`.design/imbot-sync.md`。
>
> **状态**：§7 第 1、2 条已实现并提交（去重复确认消息 / `[RESULT]` 纳入 quiet mode）。§6 表格在实现第 5 条（补齐 `Restate`）时发现两处记录错误，已在下方标注更正——**结论变化较大，先读 §6 表格和 §4 第 4 条再看别处**：Lark 其实已经有 `Restate`（继承自 Feishu，之前 `grep` 漏判）；Discord / Slack 的问题比文档原先说的更严重——不是"按钮不会消失"，而是**按钮从来没渲染过**，且因为能力声明（`SupportsInteraction()`）谎报支持交互，连文字兜底说明也没触发，权限确认/多选题在这两个平台上目前实际是**用户看不出怎么回复**。

## 1. 一句话结论

Remote 目前给用户的"每一轮对话"天然拆成 **3–4 条以上的独立消息**，其中至少两条（进度横幅 + Task-done 卡片）是无论内容多简单都固定发送的"结构性消息"；权限确认在**所有平台**上都会重复发送两次（**已修复**，见下方"状态"）；6/10 个平台完全不支持编辑/撤下已发的消息，导致按钮消息永久留在聊天记录里失效但仍可点；其中 Discord / Slack 更严重——按钮从来没有渲染过，权限确认目前在这两个平台上是用户看不出怎么回复的功能性 bug，不只是噪声问题。这些不是某个平台的个别问题，而是当前架构的固定产出——值得作为一个整体优化项来看，而不是零散修 bug。

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
4. **（更正）真正没有 `Restate` 的只有 Discord / Slack / DingTalk / Weixin / WeCom / WhatsApp**：`imbot.MessageRestater` 由 `telegram`、`feishu`、内部测试用的 `tingly` 实现；`lark.Bot` 用 Go 的接口方法提升（embeds `*feishu.Bot`）**继承了** `Restate`，所以字面 `grep -rl Restate imbot/platform/` 找不到 `lark.go` 并不代表 Lark 没有这个能力——上一版文档把 Lark 也算进"没有"是错的。对确实没有的这 6 个平台，进度消息、权限按钮消息一旦发出就永远留在聊天记录里，越用越长，失效的按钮还留在那里可以误触。
5. **（更正，比原文严重）Discord / Slack 的按钮能力是声明与实现脱节，不是"没有编辑能力"这么简单**：`core/platforms.go` 给两者都声明了交互 feature（`components`/`blockKit`），`SupportsInteraction()` 因此判 true，`imprompter.go` 就既不渲染文字兜底说明（`buildTextPermissionInstructions`/`buildTextSelectionInstructions` 只在 `!supportsKeyboard` 时才追加），也指望平台把 `opts.Actions` 渲染成按钮——但 `imbot/platform/discord/discord.go`、`imbot/platform/slack/slack.go` 的 `SendMessage`/`sendText` 从未读取过 `opts.Actions`（`grep -rn "opts\.Actions\|\.Actions\b" imbot/platform/{discord,slack}/*.go` 零匹配），**按钮从来没有渲染过**。结果是：Discord/Slack 上的权限确认 / `AskUserQuestion` 提示今天发出去就是一段光秃秃的文字，既没有按钮可点，也没有"回复 y/n/数字"的说明——用户实际上**看不出怎么回复**，直到会话超时按默认策略处理。这是一个功能性 bug，不是噪声/体验优化问题。
6. **DingTalk / Weixin / WeCom / WhatsApp 反而是"诚实"的一档**：`core/platforms.go` 对这 4 个平台没有声明任何交互 feature（DingTalk `Features` 里没有、Weixin 走默认空能力表、WeCom 只有 `"streaming"`、WhatsApp 没有），`SupportsInteraction()` 正确判 false，`imprompter.go` 正确追加"编号列表 + 回复数字"的文字兜底说明（`imprompter.go:474-514`）。比按钮更占屏幕、要求手动输入，但**至少用户知道该怎么回复**——好于 Discord/Slack 现在的状态。
7. **低 TextLimit 平台会被动二次拆条**：Discord 2000 字符、DingTalk/WeCom/WhatsApp ~4000 字符（`core/platforms.go`），`BaseBot.ChunkText()` 会把超长的 verbose 工具聚合块（最多 20 行才 flush，见 `toolBufferFlushThreshold`）或长回复自动切成 2+ 条，进一步增加消息数。
8. **verbose 默认开启**：`GetVerbose`（`handler_verbose.go:10-31`）里 `h.botSetting.Verbose == nil || *h.botSetting.Verbose` —— 没显式设置就是 true。目前只有 Weixin 通过 `PlatformBehavior.SuppressVerbose` 被强制静音（因为它的回复要绑定 `context_token`，多发根本会失败/错位，这是能力限制不是产品选择），WeCom 在能力上和 Weixin 几乎一样窄（`Features: ["streaming"]`，没有交互按钮）却没有拿到同样的 `SuppressVerbose`，是一个不对称的遗漏。

## 5. 已经存在的缓解措施（不是从零开始）

为避免这份文档显得"通篇都是问题"，值得明确记录当前代码里已经做对的部分，后续优化应该在这些基础上做增量：

- `streamingMessageHandler` 已经把 tool_use/tool_result 缓冲聚合成一条消息，而不是每个工具调用发一条（`stream.go:198-211, 302-327`），quiet 模式下还会把聚合结果压缩成"N 个工具调用 + 前 3 个预览 + (+N more)"（`stream.go:341-364`）。
- quiet/verbose 是逐 chat 可配置的（`chatStore` 里的 `Chat.Verbose`），不是全局唯一开关。
- 权限确认在 Telegram / Feishu（以及继承 Feishu 实现的 Lark）上已经用 `Restate` 做了"原地改写、按钮消失"，而不是无脑发新消息——`AsRestater` 这层抽象本身设计得很干净（`core/restate.go` 的注释写得很清楚：表达意图而非机制），只是目前只有三个平台接了。
- `api_retry`/`rate_limit` 这类瞬时失败通知，即使在 quiet 模式下也会保留（`isRetryNotice`，`stream.go:269-278`），这是刻意的、合理的例外，不是噪声。
- Weixin 的 `SuppressVerbose` 已经证明"按平台能力强制降噪，而不是留给用户/运营去配置"这个方向是可行的、已经在生产里跑着的模式（呼应 `.design/ux-principles.md` #6 合理默认值优于多一个开关）。

## 6. 分平台能力矩阵与预估体验

`按钮实际渲染` 列是这次更正新加的：`SupportsInteraction()` 只反映 `core/platforms.go` 的**声明**，不代表平台代码真的把 `opts.Actions` 画成了按钮——Discord/Slack 就是声明与实现对不上的两个反例。判定方法：`grep -rn "opts\.Actions\|\.Actions\b" imbot/platform/<name>/*.go`（排除 `_test.go`）。

| Platform | 声明支持交互 `SupportsInteraction()` | 按钮实际渲染 | 编辑/撤下 `MessageRestater` | TextLimit | 预估体验 |
|---|---|---|---|---|---|
| Telegram | ✅ inlineKeyboards | ✅ | ✅ 原生编辑 | 4096 | 相对最好：权限卡片会原地收起；但 Processing / 工具聚合 / [RESULT] / Task-done 仍然是几条独立消息，长任务照样刷屏 |
| Feishu | ✅ interactiveCards | ✅ | ✅ 卡片可 patch | 40000 | 和 Telegram 一样，权限卡片体验最好；其余消息仍不合并 |
| Lark | ✅ interactiveCards | ✅（继承自 Feishu，`lark.Bot` 内嵌 `*feishu.Bot`） | ✅ **继承自 Feishu**（上一版文档写错，见 §4 第 4 条更正） | 40000 | 和 Feishu 体验一致，`lark.go` 本身只覆盖 `PlatformInfo`/`GetWebhookURL`，其余方法（含 `Restate`）全部走方法提升 |
| Discord | ✅ components（**声明与实现不符**） | ❌ **从未渲染**（`sendText` 不读 `opts.Actions`，无 `Components` 字段） | ❌ | 2000 | ⚠️ **功能性 bug**：权限确认/多选题发出后是纯文字，无按钮也无文字兜底说明（因为 `SupportsInteraction()` 误判为 true，跳过了兜底文案），用户不知道怎么回复 |
| Slack | ✅ blockKit（**声明与实现不符**） | ❌ **从未渲染**（同 Discord，无 Block Kit 组装） | ❌ | 40000 | 同 Discord：权限/多选题今天等同"发了个用户回不了的提示" |
| DingTalk | ❌ 无按钮 feature（声明正确） | ❌（符合声明） | ❌ | 4000 | 权限/问答正确退化成编号文字列表，要求手动回复数字；限额低 |
| Weixin | ❌（默认能力表，`Features: []`，声明正确） | ❌（符合声明） | ❌ | 默认（未声明，走保守默认） | 已被 `SuppressVerbose` 强制静音，是体验最差但也是唯一被"主动治理"过的平台；权限仍是文字列表；`context_token` 耦合使 notify 类主动消息更脆弱（回复不在同一上下文里会直接失败） |
| WeCom | ❌（`Features: ["streaming"]`，声明正确） | ❌（符合声明） | ❌ | 4000 | 能力和 Weixin 接近，但**没有**拿到 `SuppressVerbose`——目前会把完整的 verbose 消息流（含文字降级的权限请求）怼过去，预计是体验最差的平台 |
| WhatsApp | ❌（声明正确） | ❌（符合声明） | ❌ | 4096 | 同 DingTalk：按钮全部退化成文字编号列表，但至少可用 |
| Tingly（内部测试） | ✅ 全部 | ✅ | ✅ | 65536 | 仅供测试，不面向真实用户 |

## 7. 值得讨论的优化方向（未实现，供决策）

按"改动成本 vs. 收益"粗排，不代表最终优先级：

1. ~~去掉权限确认的重复发送~~ **已实现**（commit `c4e2bbf`）。
2. ~~`[RESULT]` 统计块纳入 quiet mode~~ **已实现**（commit `0f53c29`）；方括号调试风格（`[SYSTEM]`/`[RESULT]`/`[SUBAGENT]`/`[UNKNOWN]`）本身还没改，`render.TextFormatter` 仍未复用 `output.go` 的图标常量——留作后续。
3. **合并/精简结构性消息**：Processing 横幅 + Task-done 卡片各带一份 footer，能否把 Processing 横幅在支持 `Restate` 的平台（Telegram/Feishu/Lark）上直接原地改写成 Task-done 卡片，而不是分开发两条；不支持 `Restate` 的平台再退化成两条。这样"编辑能力强的平台体验更干净"能自然形成分层，而不是所有平台都按最低能力对齐。
4. **verbose 默认值按平台能力收敛，而不是按平台白名单强制关闭**：现状是"Weixin 特例静音，其余全部默认 verbose=true"，更一致的做法是让默认值跟着 `SupportsInteraction()`/`TextLimit` 这类已有能力信号走（例如：无按钮能力的平台默认 quiet，因为它们每条消息的相对噪声成本更高），WeCom 至少应该先补上和 Weixin 一致的 `SuppressVerbose`（这是个能力事实，不是产品选择，可以直接修，参考第 4 节第 8 条）。
5. **（更正）补齐 `Restate` 到 Discord / Slack**：Lark 已经有 `Restate`（继承自 Feishu），**不需要**额外实现，上一版文档把它也列进"待补齐"是错的。Discord/Slack 才是真正缺的两个，但缺的不只是 `Restate`——它们连按钮渲染和按钮点击的入站回调都没有（第 4 节第 5 条），"补 `Restate`" 这个提法本身预设了按钮已经存在，实际上要做的是三件事叠在一起的更大工作：① 把 `core.ActionSet` 渲染成 Discord message components / Slack Block Kit 按钮，② 接入站的 `InteractionCreate`（Discord）/ `interactive_message` 回调（Slack）事件，翻译成现有的 `payload`/`callback_data` 机制，③ 再实现 `Restate`。三者环环相扣，不能只做第③步；工作量和一个新平台接入接近，不是"小改动"，**本轮未做**，留作单独排期。**最小修复已实现**（commit `ae04e93`）：把 `core/platforms.go` 里 Discord/Slack 的 `Features` 去掉 `components`/`blockKit`，`SupportsInteraction()` 如实返回 false——权限确认/多选题现在会正确退化成 DingTalk 同款的文字兜底说明，"用户完全不知道怎么回复"的功能性 bug 已解决；真正的按钮支持仍是待办。
6. ~~DingTalk/Weixin/WeCom/WhatsApp（现在也包括 Discord/Slack）的文字降级问答~~ **部分已实现**（commit `d252666`）：回复说明现在放在消息最前面而不是最后——窄预览通知（微信这类平台的推送预览通常只截取开头几个字）之前会把说明截没了，现在不会。顺带修了 `AskUserQuestion` 的文字兜底会把问题/选项渲染两遍、结尾永远显示"Click a button below to select"（明明没有按钮）的问题——两个原因都是同一处代码（`imprompter.go` 在 `buildPromptText` 之后再 `append` 一段独立拼出来的说明）造成的，现在每个 `ToolPromptBuilder.BuildPrompt(req, supportsKeyboard)` 自己一次性拼完，不再有拼接两次的地方。**没做的**：回复内容无法识别时目前是静默 `return false`，交给下游当成普通聊天消息处理（不会给用户任何反馈，也可能被当成发给 agent 的新消息）。要不要在这种情况下改成"claim 消息 + 提示重试"，涉及一个需要产品判断的取舍——会改变"权限确认挂起时，用户还能在同一个 chat 里正常说话"这条现有行为，所以没有直接改，见 §8。

## 8. 待决策的问题

- ~~第 7 节第 1 条（重复确认消息）建议直接当 bug 修~~ 已按此原则处理并实现。
- `[RESULT]` 统计块是否要默认隐藏：这涉及"技术用户想看 cost/tokens" vs "普通用户觉得是噪声"的取舍，可能需要一个用户可见的开关，而不是完全去掉（呼应 ux-principles #6：默认值要选对，但"合理默认值优于多一个开关"不代表永远不能有开关，只是默认值要选对）。已按"默认隐藏，verbose 下仍可见"实现（commit `0f53c29`），如果需要一个和 verbose 分开的、更细的开关（例如"quiet 但仍想看 cost"），目前还没有。
- **无法识别的文字回复要不要 claim 并提示重试**（第 7 节第 6 条遗留）：现状是静默放行给下游当普通聊天处理。改成"claim + 提示"能让用户不再对着一段回复了没反应的 prompt 干等，但会改变"权限确认挂起时，用户仍可在同一个 chat 里正常说话/切换 agent"这条现有行为——`prompt_reply.go` 已经对 `@cc`/`@tb`/`/cc`/`/tb`/`/mock` 这类 handoff 命令显式放行，说明"挂起时仍可做别的事"是刻意保留的能力，不是疏漏。这个需要先确认：是否所有其他文字都应该被视为"这是在回答"，还是要留一个逃生舱（比如仍然放行、只是额外发一条不 claim 的提示）——不是纯技术判断，等确认方向再动。
- **（更正后的根因）消息层完全没有"这条消息是哪个 session/service 发出去的"这个关联，不是 pending-request 排序这一处小问题**。第一版记录只写到"`pendingRequests` 是 map、取 `[0]` 不保证是最新"，用户指出这只是症状——根因是**关联关系从消息发送这一层就不完整**，往下追了一遍，证据如下：
  1. **发送本身不携带身份**：`imbot.SendMessageOptions` / `SendResult`（`imbot/core/bot.go:52-90`）没有任何字段能装 session_id / request_id / agent type——`ReplyTo`/`ThreadID` 是平台层的回复/话题定位，不是身份；`Metadata` 是通用透传字段，但没有任何调用方真的把身份塞进去。
  2. **session 的消息记录只有单向**：`SessionMgr.AppendMessage` 唯一的生产调用点在 `executor_claude.go:74`，只记 inbound 的用户消息；助手的流式回复（`stream.go` 的 `sendMessage`）直接调 `bot.SendMessage`，从不写回 session。连"完成"事件本身都没把回复内容留痕——`SetCompleted(sessionID, "")`（`executor_claude.go:316`）永远传空字符串。
  3. **发出去的消息 ID 没人接**：`SendResult.MessageID` 每次 Send 都会返回，但 `remote/` 里除测试外只有接口声明用到它（`imchannel.go:34`），没有任何调用方把它存下来；`chat_store.go` 的 `ChatStoreInterface` 是 chat 粒度（绑定项目、当前 agent、白名单），没有消息粒度的表；找不到任何地方持久化过"这条 message_id 属于哪个 session"。
  4. **这不是"未来才会遇到"的假设性问题**：`remote/session/manager.go` 的 `Session.ChatID/Agent/Project` + `FindBy(chatID, agent, project)`/`ListByChat(chatID)`（`manager.go:209-266`）本来就允许同一个 chatID 下同时存在多个 session（不同 agent、不同 project，或者未来同 agent 多个并发实例）；`Chat.CurrentAgent`（`chat_store.go:125-126`）只是"未加前缀的消息默认路由到哪个 agent"的单值指针，不是并发锁，不阻止多个 session 同时活着。也就是说，**session 层已经具备多会话并发的结构，真正卡住的是消息层没有身份**——`IMPrompter.pendingRequests` 按 chatID 猜"最新一条"这个具体 bug，只是这个更大缺口暴露出来的第一个症状。

  往前看，要解的不是"给 pendingRequests 排个序"，而是给整条发送路径补上身份：`SendMessageOptions`（或其 `Metadata`）需要能稳定携带 `session_id`/`request_id`，Send 的调用方需要把它传进去，`SendResult.MessageID` 需要有地方接住并和身份关联存起来（哪怕只是内存表，先解决"同一进程内能查"）。有了这层关联，`reply-to` 原生回复、`pendingRequests` 排序这些具体修法才有地基——不然每个都是在同一个空洞上各自打补丁。

  **已实现（分支 `claude/gifted-feynman-lwrxtb-msg-session-identity`，commit `cab1786`），按确认过的 3 步范围**：① `SendMessageOptions.SessionID`（`imbot/core/bot.go`）——所有平台零改动，字段不用就是空；② 两条产消息路径把 session_id 传进去：Flow A（`claudecode/plugin.go` 的 push `Notify` 通过 `Meta["session_id"]`，交互式路径本来就有 `ask.Request.SessionID`，`imprompter.go` 的首次发送和非-Restate 平台的 fallback 发送都补上了）、Flow B（`streamingMessageHandler` 新增 `sessionID` 字段，从 `PreparedRequest.SessionID` 一路传进去）；③ 修真正的 bug——`GetPendingRequestsForChat` 之前直接遍历 map 返回，`HandlePromptTextReply` 取的 `[0]` 是 Go map 遍历顺序决定的，同一 chat 有 ≥2 个 pending request 时等于随机选；现在按 `createdAt` 降序排序，`[0]` 变成确定性的"最新一条"。

  **进一步讨论后又加了一层（commit `440946f`）**：纯按时间排序还是"猜"，只是猜得确定了。讨论时先提过"排队"（同一时间只放一个 pending，参考 `Executions.begin` 的单飞模式）——这个方向被否了，因为 Flow A（notify）和 Flow B（remote_agent）是刻意解耦的两个 consumer（见 `bot-arch.md` §1），排队等于把两者重新耦合，而且会莫名卡住 Flow A 里外部 Claude Code 进程的 hook 请求。改成给 `ask.Request` 加一个 `Source` 字段（`ask.SourceNotify` / `ask.SourceRemoteAgent`，字面量对应 `bot.NotifyConsumerName`/`binding.RemoteAgentScenario` 现成的名字，不引入新概念），在两个来源分别打标签，`GetPendingRequestsForChat` 排序规则升级成两级：**同一个 chat 里，`remote_agent` 来源永远排在 `notify` 来源前面，同来源内部再按时间**。原本也考虑过在 prompt 文案里加图标区分来源（🔔/💬）方便人眼分辨，但这本身是给"减少视觉噪声"这个大方向增负担，权衡后放弃——标签只用于内部排序，不体现在文案里。

  **这条优先级规则本身站不站得住**：`remote_agent` 优先的假设是"用户此刻在跟 agent 对话，回复大概率是回它"——如果用户其实是特意去处理那条 notify 推送，这条规则会猜错。**这个规则现在已经降级成兜底**——见下面"赌错会怎样"一节：reply-to 精确匹配已经实现并覆盖 8/9 平台，只有匹配不到（没有 reply-to 信号，或用户没有真的点"回复"这条具体消息，就是直接在 chat 里打字）时，才会退回到这条 Source 优先级规则去猜。

  **没做的**：`SendResult.MessageID` 仍然没人持久化，`SessionMgr.AppendMessage` 仍然只记 inbound——这两块本轮明确排除在范围外，之前讨论时已经确认。

  **"赌错会怎样"这个问题的解法梳理（讨论后决定：先记录，不执行）**——`Source` 优先级排序不是"解决"，是"把纯随机的猜测换成一个有理由但仍会猜错的默认值"，真正能做到"不用猜"的方案有以下几层，按"彻底程度 vs 成本"排列：

  1. ~~原生 reply-to 匹配~~ **已实现（分支 `claude/gifted-feynman-lwrxtb-reply-to`，commit `91eb37c`、`5d7f5b7`）**：调研之前的假设（只有 Telegram/Discord/Slack/Feishu/Lark 5 个平台有原生回复）是错的——查了 imbot 每个平台的入站适配器，Weixin、WeCom 其实也已经把原生回复能力（`ReplyToID`）接进了 `core.Message.ThreadContext.ParentMessageID`；WhatsApp 的 webhook 本来就带 `context.id`，只是 `MessageEvent` 结构体没声明这个字段，字段一直被静默丢弃，这次补上了。**最终覆盖 8 个平台**（Telegram/Discord/Feishu/Lark/Slack/Weixin/WeCom/WhatsApp），真正没有任何原生回复信号的只剩 **DingTalk**（Stream 模式的 `BotCallbackDataModel` 里确实没有任何 context/parent 字段）。实现上：`ask.Request` 加了 `MessageID` 字段（`GetPendingRequestsForChat` 返回时回填，构造请求时还不知道），新增 `bot.ReplyToMessageID(msg)` 读取入站消息的回复目标，`bot.SelectPendingRequest(pendingReqs, replyToID)` 把"精确匹配"和"Source+时间兜底"这两层判断分开，精确匹配命中就直接用，不命中/没有回复目标才退回旧的启发式。

     **DingTalk 这条结论额外做了联网核实（不只看我们 vendor 的 SDK 代码），结论不变，但性质更明确了**：钉钉官方"机器人接收消息"协议文档（`open.dingtalk.com/document/robots/receive-message`）列出的回调字段——`conversationId`/`atUsers`/`chatbotCorpId`/`chatbotUserId`/`msgId`/`senderNick`/`isAdmin`/`senderStaffId`/`sessionWebhook(ExpiredTime)`/`createAt`/`senderCorpId`/`conversationType`/`senderId`/`conversationTitle`/`isInAtList`/`text`/`robotCode`/`msgtype`/`content`——和我们 vendor 的 `dingtalk-stream-sdk-go@v0.9.2-beta.1` 里 `chatbot.BotCallbackDataModel` 逐字段对得上，协议本身就没有 `quotedMsgId`/`parentMsgId` 这类字段，**不是 SDK 没跟上协议、是协议里压根没有这个概念**。阿里云开发者社区上也有开发者问过同样的问题（"机器人单聊，如何查看被引用（回复）的消息"），至今没有官方给出字段级别的答案，侧面印证这不是一个大家都知道怎么解的常见能力。也就是说，DingTalk 这个平台上"reply-to 精确匹配"从根上就不可能做到零成本——第②层（`[2]` 短标记文字兜底）就是它唯一可行的"真解决"路径，不是"暂时没查"，是"协议层限制，查了也没有"。
  2. ~~文字平台的短标记兜底~~ **放弃，不做**：调研后范围已经缩小成只剩 DingTalk 一个平台需要它，但评估后认为这条路线本身不值得——为一个平台单独引入一套"`[2]` 标记 + `2y` 回复格式"的特殊交互，边际收益（DingTalk 上两个 pending 同时撞车本来就是稀有事件）配不上引入的特殊分支和用户要多记一种回复格式的成本。**结论：DingTalk 就接受现状**——落回 Source 优先级兜底，猜错时用户依然可以在同一个 chat 里正常说话，不是不可恢复的错误，只是偶尔要多说一句话纠正。
  3. **兜底不猜，直接问**：连带②一起放弃考虑——②都不做了，③（撞车时改成主动问一句）也就没有单独存在的理由，一并归为不做。
  4. **`Source` 优先级排序**（commit `440946f`）：现在是 DingTalk 上**唯一在跑、也是最终形态**的兜底——覆盖 8/9 平台之后，只有 DingTalk、或者 reply-to 信号丢失/不匹配的边缘情况才会用到它，其余情况都已经被①的精确匹配吃掉了。
- 是否值得把 Flow A（hook notify）和 Flow B（`@cc` 流式）在同一个 chat 里共存时做去重/合流——目前没有证据表明这是常见用法，暂不列入本轮范围，但架构上两条路径完全独立这一点值得记录以防未来踩坑。
