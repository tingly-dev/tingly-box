# ImBot 平台接缝 — 中立出站载荷、能力表、三层抽象边界

imbot 模块化抹平了**连接层**的平台差异，但没抹平**交互层**。这份文档记录接缝
（seam）的设计：出站交互载荷如何进入类型系统、平台能力如何查表、以及**什么该抽象
什么不该**的判据。

前置：`.design/bot-arch.md`（resource / channel / consumer 三层模型）、
`.design/ux-principles.md`（#3 命名碰撞、#6 合理默认）。

目录：

1. 问题
2. 根因 — 出站载荷没有类型
3. 三层模型 — 什么该抽象，什么不该
4. 四条 seam
5. Telegram 内联键盘 — 最强约束不能当最小公分母
6. 阶段与落地状态
7. 已修复的缺陷
8. 归属与命名规则
9. 验收标准
10. 测试

## 1. 问题

`internal/remote_control` 在 imbot 独立之后，仍然扮演"渲染器 + 能力判官"这两个本该
属于 imbot 的角色。平台字面量散布在 19 个非测试文件里，按性质归类为六类：

| # | 类别 | 站点数 | 性质 |
|---|------|--------|------|
| A | 出站键盘/卡片按平台预渲染 | 13 | 抽象缺失 |
| B | Weixin `context_token` 手工透传 | 19 | 抽象缺失 |
| C | Telegram-only 的消息编辑 / 撤键盘 | 8 | 抽象缺失 |
| D | 平台能力/默认值的 switch | 5 | 表格缺失 |
| E | 平台专有实现寄居在 remote_control | 3 个文件 / 819 行 | 归属错误 |
| F | 命名误导（内容中立却叫平台名） | 3 个文件 | 命名债 |

A 类里有一条在 `remote/channel/imchannel/imprompter.go` —— 泄漏已经越过
remote_control 的边界进了共享 prompter，只修 remote_control 修不干净。

## 2. 根因 — 出站载荷没有类型

> imbot 抽象了**入站**（`core.Message` 归一化）和**发送动作**（`SendMessage`），
> 但没有抽象**出站载荷**。载荷走的是无类型的 `Metadata map[string]interface{}`，
> 于是调用方必须知道"我在跟谁说话"，才能往这个口袋里塞对的东西。

历史形态（已消除）：同一个调用点为三个平台各渲染一份，指望正确的那份被捡起来。

```go
metadata["replyMarkup"] = tgKeyboard                     // Telegram 形状
metadata["card"]        = card                           // 中立形状
if platform == Feishu { metadata["card_json"] = ... }    // Feishu 形状
```

后两份**从来没有任何发送路径读过**；第一份携带 go-telegram 类型，Feishu 解不了——
于是 Feishu 用户收到的是无按钮卡片（§7.1）。

一个佐证：`imbot/platform/tingly/adapter.go` 曾经为了兼容**调用方的历史习惯**而做
双形状解码，注释里写明"remote_control 广泛使用 `BuildTelegramActionKeyboard`"。
平台适配器迁就调用方——抽象方向反了。

**另一个发现同样重要**：四件目标结构早就存在于 imbot，只是没人用得上——
`imbot/menu` 的 Adapter+Registry（imbot 之外零引用）、
`core.PlatformDescriptor` 单一事实表（只被用过一次）、
`core.Message.ContextToken`（入站归一化了，出站要调用方手工搬）、
`imbot/platform.PlatformConfigs`（`manager.go` 另写了两套并行 switch）。
所以本项工作大部分是**接通已有抽象**，而不是发明新抽象。

## 3. 三层模型 — 什么该抽象，什么不该

**不是所有平台差异都该抹平。** 判据是这个差异对用户有没有意义：

| 层 | 例子 | 处理 |
|---|---|---|
| **Tier 1 通用动作** | 可点击动作、打开链接 | **抽象**。`core.Action` 的自有字段 |
| **Tier 2 能力门控** | 打开小程序（TG WebApp / Feishu 小程序 / 其余退化成 URL） | **抽象语义，不抽象实现**。`ActionKind` + `Fallback` 必须显式 |
| **Tier 3 平台专有** | `pay`、`callback_game`、`switch_inline_query` | **不抽象**。走类型化逃生舱 |

Tier 3 的形态是平台包里的构造器：

```go
// imbot/platform/telegram
func WebAppButton(label, url string) core.Action      // 填 Action.Ext[PlatformTelegram]
func SwitchInlineButton(label, query string) core.Action
```

调用方写 `telegram.WebAppButton(...)`，**这行 import 本身就是"我在写 Telegram
专属代码"的声明**。

为什么类型化逃生舱比 `Metadata` 口袋好（两者都能装平台专有的东西）：

| | `Metadata["replyMarkup"]` | Tier 3 构造器 |
|---|---|---|
| 平台意图 | 不可见，要读实现才知道给谁的 | import 即声明 |
| grep / lint | 不能 | 能 |
| 类型安全 | 无——塞错形状静默丢（§7.1 即此） | 编译期检查 |
| 其他平台行为 | 未定义 | `Fallback` 显式声明 |

**核心判据：要消灭的不是"平台特化"，而是"看不见的平台特化"。**

规则：**平台无法渲染的动作必须按 `Fallback` 降级，不得静默丢弃。**

## 4. 四条 seam

### Seam 1 — 出站交互载荷进类型系统 ✅ 已落地

`core.SendMessageOptions.Actions *core.ActionSet`，各平台自行渲染
（`imbot/core/action.go`、各 `platform/*/action_render.go`）。

`ActionSet` 保留**行结构**——布局有意义（目录浏览器依赖它），没有行概念的平台可以
自己 flatten。

**按钮身份是 `core.Payload`（有序 segment），不是 `callback_data`**（2b 落地）。
编码是平台自己的事：Feishu 放进 button value 的 JSON 数组，Telegram 能塞进 64 字节
就 join，塞不下就寄存换短 token。见 §5。

兼容期：`Action.CallbackData` 与 `Metadata["replyMarkup"]` 各保留一个版本，读到时
打 deprecation 日志。

### Seam 2 — 回复上下文由 imbot 自己记 ⏳ 计划中

入站 `weixin/adapter.go` 已把 `context_token` 归一化进 `core.Message.ContextToken`，
但出站仍要调用方从 metadata 里挖出来再塞回去（19 处）。目标是 `BaseBot` 维护
per-chat 的最近入站上下文，`SendMessage` 自动补齐。

**未决的设计选择**：下沉成什么形态？

- **per-chat 缓存最近入站** —— 简单，但并发聊天里新入站会覆盖，**比现状语义弱**。
  今天调用方传的是它正在回复的那条 `hCtx.Message` 的 token，是精确的。
- **回复绑定具体入站消息**（`SendMessageOptions.InReplyTo`）—— 忠实，但要再动一次
  核心类型。

即"删掉透传赌 SDK 自理"并不是本 seam 的实现方式；`weixin.go:243` 那句"新 SDK 内部
管理 context token"只说明缺省路径存在，不说明手工透传冗余。

**风险提示**：这条 seam 只影响 Weixin/WeCom，而本仓无这两个平台的测试手段。在完全
无法验证的平台上做语义可能变弱的改动，是全盘风险回报最差的一项——应当在有真机
验证条件时再做。

### Seam 3 — 能力与默认值统一到平台表 ✅ 已落地

`core.PlatformDescriptor.Behavior`（`imbot/core/platforms.go`）承载**产品级默认值**：

```go
type PlatformBehavior struct {
    RequiresPairingByDefault bool  // 仅凭 token 就能获得完整 DM 权限的平台
    SuppressVerbose          bool  // 无法承载中间态进度消息的平台
}
```

刻意用 `SuppressVerbose` 而非 `SupportsVerbose`——零值即"可以 verbose"，对除例外
以外的所有平台都成立，未登记的平台不会因为忘了填而被误关。

落地对照：

| 原站点 | 现在 |
|---|---|
| `chat_store.go` 配对默认 switch | `imbot.GetPlatformBehavior(p).RequiresPairingByDefault` |
| `handler_verbose.go` 被注释掉的判定 | 恢复启用，读 `SuppressVerbose`（§7.3） |
| `manager.go` 菜单 switch | `imbot.SetupCommandMenu(bot, platform, reg)` |
| `manager.go` `buildAuthConfig` | `imbot.BuildAuthConfig(platform, auth)` |
| `manager.go` `hasValidAuth` | `imbot.MissingAuthKeys(platform, auth)` |
| `manager.go` Weixin 专有 options | `imbot.AuthOptions(platform, auth)` |

**两处刻意分开的表**：

1. **菜单安装不进 `core`。** `command` 依赖 `core`，所以 `core` 不能持有
   `func(Bot, *command.CommandRegistry) error`——会成环。菜单 dispatch 落在
   `imbot/platform/menu_setup.go`，与 bot-creator registry 并列：数据在描述符表，
   行为 dispatch 在平台 registry。
2. **`AuthMapping` 不复用 `FieldSpec`。** 原计划让 `buildAuthConfig` 由
   `PlatformConfigs.Fields` 驱动，但 `Fields` 描述的是**设置界面表单**，而 Weixin
   的凭据来自扫码流程、表单字段为空——按 `Fields` 推导会让 Weixin bot 直接失去认证。
   两者是正交的轴（UX 原则 #4），故 `PlatformAuthConfig.Auth` 是独立的 wire mapping。

`manager.go` 现已无任何平台字面量（仅剩注释）。

### Seam 4 — 「消息重述」能力，而不是「编辑消息」能力 ✅ 已落地

编辑是*手段*不是*意图*。调用方真正想说的是"这条消息连同按钮已经过期了，用新状态
取代它"；平台自行选择原地编辑、卡片更新、还是发替代消息 + 撤旧键盘。

```go
type MessageRestater interface {
    Restate(ctx context.Context, ref MessageRef, opts RestateOptions) error
}
type RestateOptions struct {
    Text      string      // "" = 保留原正文（只想撤控件时）
    Actions   *ActionSet  // nil = 移除全部控件
    ParseMode ParseMode
}
func AsRestater(bot Bot) (MessageRestater, bool)   // 接口断言，非具体类型
```

三个平台已实现：Telegram（原地编辑 / `editMessageReplyMarkup`）、
Feishu/Lark（`PATCH /im/v1/messages/:id` 卡片原地更新）、tingly（记录
`EventRestate`，让"用过的菜单被撤下"这件事能在**非 Telegram 平台上**被断言）。

**"Feishu 卡片更新会不会推新通知"这个悬念已解**：`PatchMessage` 是原地更新，不产生
新通知——所以 feishu 选择了"编辑"而非"发替代消息"。但要注意接口设计本身没有押注在
这个结论上：真机若发现相反，改的是 feishu 包内部，接口不动。

调用方统一用 `imbot.RestateOrIgnore(...)`：平台不支持就什么都不做并返回 false，
撤菜单是 best-effort，绝不能打断按钮按下之后的流程。

**一个平台差异没有被抹平，也不该被抹平**：`RestateOptions.Text` 为空表示"只撤控件、
正文别动"。Telegram 能做到（`editMessageReplyMarkup` 单独改控件），**Feishu 做不到**
——卡片 patch 替换整个 content，照字面执行会把用户的消息正文清空。Feishu 因此对这类
请求返回 `ErrNotSupported`，`RestateOrIgnore` 转成 no-op。丢正文比留一个陈旧菜单更糟。

所以 §7.2 的修复在 Feishu 上是**部分**的：调用方给出替代文本的场景（目录浏览翻页、
prompt 结果回填）可以工作；纯"撤菜单"的三处仍是 no-op，除非将来保留原正文。

`AsTelegramBot` 缩回真正 Telegram 专有的 `ResolveChatID`（`/join` 命令，已用
`WithPlatforms` 正确声明，是合理特化）。

## 5. Telegram 内联键盘 — 最强约束不能当最小公分母

这是整件事最深的一层。

| | 按钮身份 | 载荷上限 | ACK 义务 | 更新方式 |
|---|---|---|---|---|
| **Telegram** | `callback_data` | **64 字节**（硬限，超了整条消息被拒） | **必须** answer，否则转圈 ~15s | 原地编辑 |
| **Feishu/Lark** | button `value` | 任意 **JSON**，无实际上限 | 无 | 重发 card |
| **Discord** | `custom_id` | 100 字符 | **3 秒**内 ACK | 编辑 components |
| **Slack** | `action_id`+`value` | value ≤2000 | **3 秒**内 ACK | `response_url` |

Telegram 在**每一列**都是最紧的。它的约束已经泄漏成全平台的应用架构：

- **索引式导航**：`feature/telegram_dir_browser.go` 的注释是自白——
  `// use index instead of path to avoid 64-byte limit`。路径塞不进去，按钮只能带
  下标，于是必须在服务端存 `BindFlowState.Dirs` 快照做还原，还得配套过期与翻页
  状态机。**这套东西只为 Telegram 而存在，Feishu 完全不需要却也得吃。**
- **`FormatDirPath` 的 NUL 编码**：把 `:` 换成 NUL 字节，只因为
  `FormatCallbackData` 用 `:` 做分隔符——这个"扁平字符串 + 冒号"协议本身就是 64
  字节逼出来的。而 Feishu 的 button value 是 JSON，NUL 进 JSON 字符串会坏。
- **64 字节从未被校验**：全仓只有 `interaction/keyboard.go` 一行注释提到它。
  `feature/action_menu.go` 的 `bind:create:<path>` 前缀占 12 字节，路径超过
  **52 字节**就会被 Telegram 拒（`BUTTON_DATA_INVALID`），整条消息发不出去。

**结论：按钮身份不能是 `callback_data`** —— 那是把 Telegram 的传输编码当成了按钮
的身份。已落地形态是 `core.Payload`（**有序 segment**，`imbot/core/payload.go`），
由平台负责投递：Feishu 直接进 button value 的 JSON 数组；Telegram 能 join 进 64
字节就照旧 join（**线上字节不变**，老键盘继续可用），放不下、含 `:`、或撞上保留前缀
就寄存进 per-bot vault，`callback_data` 只带 `@<n>`。索引导航、`Dirs` 快照、NUL
编码三样已全部删除。

#### 为什么是 segment 而不是 `map[string]any`

原计划写的是 `map[string]any`。实现时改为有序 segment，理由：

1. **全仓的 dispatch 都是位置式的**（`parts[0]` / `parts[1]`…）。segment 与之一一
   对应，迁移是机械的、可逐行 review 的；而这是整个计划里**唯一碰回调协议**的一步，
   改错的表现是"按钮点了没反应"——最不该在这一步引入大面积重写。
2. **名字会是发明出来的，不是发现出来的**。今天的数据本来就是位置式的
   （`action:sub:arg`），套一层 key 只是给它编名字。
3. 两种形态下"编码归平台管"这条关键性质都成立——而那才是这条 seam 的目的。

`Payload` 只有 `Name()` / `Arg(i)` 两个读取器，且**越界返回 `""`**：老版本渲染的
按钮 segment 更少时，读到的是"缺数据"而不是 panic。

#### token 寄存的边界

vault 是**进程内、有上限（4096 条）的 FIFO**。回调载荷描述的是"某个会话里某条消息
上的某个控件"，其寿命不该长过产生它的流程本身——而本仓每个这类流程本来就把状态放
内存里并带超时。上限保证长跑的 bot 不会把这张表撑大。

token 解析不到时（bot 重启，或被挤出），**明确告诉用户按钮已失效**，而不是把这次
点击吞掉。旧行为下"点了没反应"和"bot 坏了"在用户端是同一件事。

另外两件必须显式建模的：

1. **ACK 语义**。今天 `telegram.go` 在 `EmitMessage` 之后立刻无条件 answer，所以不
   转圈；但 Discord/Slack 的 3 秒硬性 ACK 让这件事不能一直含糊。接口先立住
   （Telegram 实现可为 no-op），接 Discord 时不用返工。
2. **inline vs reply keyboard 是两个概念**。前者绑在**某条消息**上，后者是**整个
   会话**的输入区键盘，生命周期完全不同。故字段名为 `Actions`（消息级）而非
   `Keyboard`——按 UX 原则 #3 给未来的会话级键盘留出独立的轴。

## 6. 阶段与落地状态

| 阶段 | 内容 | 状态 |
|---|---|---|
| **1** | 归属搬迁：Feishu 渲染器进 `platform/feishu`；Weixin QR 客户端去重；`telegram_keyboard.go` → `action_menu.go` | ✅ 已落地 |
| **2a** | Seam 1 上半：`Actions` 进类型系统、13 个调用点迁移、Tier 3 逃生舱 | ✅ 已落地 |
| **2b** | Seam 1 下半：按钮身份换 `Payload`、Telegram token 降级、补 64 字节校验、删索引导航与 NUL 编码；顺带把 Feishu 卡片回调接上 | ✅ 已落地 |
| **3** | Seam 3 + Seam 4：能力表、`MessageRestater` | ✅ 已落地 |
| **4** | Seam 2（回复上下文自动化）+ `FileResolver` | ⏳ |
| **5** | `menu` 包归位：让它建立在新 seam 之上 / 降级 / 删除，三选一 | ⏳ |
| **6** | 移除 `Metadata["replyMarkup"]` 兼容路径 | ⏳ **依赖 5** |

**2a / 2b 拆分的理由**：2b 是唯一触碰**回调协议**的一步，改错就是"按钮点了没反应"。
拆开让 2a 的用户价值（Feishu 按钮可见）不被 2b 的风险绑架。

**2b 里为什么还带了 Feishu 卡片回调**：做 2b 时才发现 2a 的修复只到一半——按钮渲染
出来了，但**点击根本没有入站路径**（§7.6）。而 2b 改的正是 button value 的形状；
没有消费方，这个形状就无从验证。两者是同一条回路的两端，分开做等于把一端焊死在
不可验证的状态。

**Phase 5 刻意排在 seam 之后**：先让新 seam 的形状被真实使用验证过，再决定 `menu`
该不该活，而不是反过来。

**兼容路径的移除必须排在 Phase 5 之后**（原计划把它放在 Phase 4，顺序是错的）。
`Metadata["replyMarkup"]` 至今仍有三个 in-repo 生产者：imbot 自己的通用交互
Handler（`imbot/interaction.go:198`）、`menu` 包的 telegram/feishu 适配器
（`telegram/menu.go:147`、`feishu/menu.go:170`）、以及 `examples/`。
先删兼容路径会让 imbot 自身的交互路径哑掉——生产者迁完才轮到它。

**Phase 4 的两件事互不依赖**：`FileResolver` 自足且低风险，随时可单独做；
Seam 2 则有个未决的设计选择（见下），且只影响本仓无法测试的 Weixin/WeCom，
是全盘里风险回报最差的一项，不宜赶。

### 已知的排序取舍

- **`SendMessageOptions.Card` 推迟**。`Card` 类型族有 48 处外部引用，且
  `CardAction.Type` 牵连 `ActionType`；要放进 `SendMessageOptions` 就得把整族搬进
  `core`（`interaction` 依赖 `core`，反过来成环）。而 `Actions` 单独已足以修复
  §7.1，`Card` 目前没有任何发送路径消费。等有真实消费方再上。
- **`Action.Style` 推迟**，跟 `Card` 一起。原 `BuildActionCard` 给 Clear 标过
  danger，但那条路径从未被读过，故不算回退。

## 7. 已修复的缺陷

### 7.1 Feishu/Lark 上内联按钮全部丢失（2a 已修）

`remote_control` 恒定传 `models.InlineKeyboardMarkup`，而 feishu 的
`buildInteractiveCard` 类型开关只认 `interaction.InlineKeyboardMarkup` 和
`map[string]interface{}` → `buttons` 为空 → 发出**只有文本、没有任何按钮**的卡片。
Clear / CD / Project、目录浏览、`/resume` 选择器对 Feishu 用户全部不可见。

同时为 Feishu 写的 161 行渲染器产出的 `card_json` **从未被消费**——是死代码。

现在：Feishu 从中立 action set 渲染；遇到不认识的 legacy 形状打 warning，而不是
静默发一张残废卡片。

> 仍待真机验证：本仓无 Feishu 凭据，类型开关的推导是确定的，但"用户实际看到什么"
> 应由真机确认背书。

**这条修复只到一半，另一半见 §7.6**：按钮渲染出来了，但当时没有任何入站路径接收
点击。写"已修"时没查回路的另一端，是这次调研里最该记下的教训。

### 7.2 非 Telegram 平台键盘撤不掉（Seam 4 已修）

`AsTelegramBot` 是**具体类型断言**（`bot.(*telegram.Bot)`），Feishu/tingly 永远走
不进去。用户在 Feishu 上点完按钮旧键盘一直挂着，可重复点击进入陈旧状态——而代码
注释明说撤键盘就是为了防这个。

现在 7 处调用点走 `imbot.RestateOrIgnore`，Feishu 通过卡片 patch 拿到该能力
——但仅限调用方给出替代文本的场景，原因见 Seam 4 末尾。

### 7.3 `handler_verbose.go` 的能力判定被注释掉（Seam 3 已修）

函数体注释掉、文档注释还留着（"Returns false for platforms that don't support
verbose mode (e.g., Weixin)"）。说的和做的不一致——这是没有能力表可依的直接后果。

现已恢复启用，读 `PlatformBehavior.SuppressVerbose`。**这对 Weixin 是行为变更**：
中间态进度消息不再发送。这是恢复文档记载的原意，不是新策略。
`handler_constructor.go` 里那段与函数分家的孤儿注释也一并删除，避免两处说法再漂移。

### 7.4 Weixin QR 客户端重复实现（1 已修）

`imbot/platform/weixin.QRClient`（Web 模块用，有测试）与
`feature.WeChatQRClient`（CLI 用，无测试）是同一组 API 的两份实现，且已在超时处理
上漂移：CLI 版把任何网络超时当 `"wait"` 继续轮询，Web 版只认 `ctx.DeadlineExceeded`
——而 `httpClient.Timeout`(35s) 通常先触发。合并时保留了 CLI 的更稳行为。

### 7.5 Lark bot 根本起不来（Seam 3 已修）

`manager.go` 的两个手写 switch **都漏了 `"lark"`**，而 `PlatformConfigs` 里 lark
是有条目的（AuthType `oauth`，表单收 clientId/clientSecret）。后果是两处独立失败：

1. `hasValidAuth` 走 default 分支 → 要求 `auth["token"]`，而 Lark 表单根本不收
   token → 一律判定"no valid auth credentials, not starting"。
2. 即使绕过，`buildAuthConfig` 也走 default → `Type: "token"`，而
   `feishu.NewBot`（Lark 复用其实现）在 `Auth.Type != "oauth"` 时直接报错。

**同一个根因**：一张有条目的表，旁边并行维护着两个手写 switch。这正是把表变成唯一
事实来源的价值——`TestAuthMappingCoversEveryConfiguredPlatform` 现在会在下一次
漏填时直接失败。

### 7.6 Feishu 卡片按钮**点了没有任何入站**（2b 已修）

2a 修好了"按钮渲染不出来"，但只修了一半。Feishu 的 bot 只订阅了消息事件：

```go
dispatcher.NewEventDispatcher("", "").
    OnP2MessageReceiveV1(b.handleP2MessageReceiveV1)   // 仅此一条
```

`HandleCardAction` 是个直接返回 `not implemented` 的桩，而全应用的回调分发都挂在
`Metadata["is_callback"]` 上——只有 Telegram 和 tingly 会设。所以 Feishu 用户点下
按钮的结果是：转圈，然后什么都没有。

**这比没有按钮更糟**。没有按钮是"功能缺失"，有按钮点了不动是"这 bot 坏了"。

现已注册 `OnP2CardActionTrigger`，把卡片回调归一化成 `core.Message`（带 `Payload`）
后走与其它平台完全相同的分发路径。

> 待真机验证：本仓无 Feishu 凭据。SDK v3.9.7 的 WS 客户端把 `MessageTypeCard` 直接
> 丢弃，但 `card.action.trigger` 是走 `MessageTypeEvent` 经 `EventDispatcher.Do`
> 分发的（`Do` 会先查 `callbackType2CallbackHandler`）——推导如此，仍需真机背书。
> 最坏情况是事件不到达，行为与今天一致，不构成回退。

### 7.7 `getReceiveIdType` 的前缀匹配全是死分支（2b 已修）

```go
prefix := targetID[:4]        // 4 字符
switch prefix {
case "ou_":  ...              // 3 字符，永远不等
case "oc_":  ...              // 永远不等
}
```

四字符切片不可能等于三字符常量，所以 `oc_` / `ou_` 两个分支**不可达**，除了字面
`cli_` 之外的一切目标都落到 default → `open_id`。而且映射本身也是反的：`oc_` 是
会话（chat_id），`ou_` 是用户（open_id），原码写成了互换。

之所以在 2b 里修：卡片回调的回复目标就是 `oc_` 开头的 chat_id，不修则 §7.6 接上了
入站也回不出去。已补 `TestGetReceiveIdType`。

### 7.8 `bind:create` 建完目录就沉默（2b 已修）

`telegram_callback.go` 的 `case "create"` 只做了 `os.MkdirAll`，成功之后既不绑定也
不回话，直接 fallthrough 出 switch。用户点"✅ Create"，目录在磁盘上出现了，聊天里
一个字都没有——无从判断绑定是否发生。现已补上 `completeBind` + 清理浏览状态。

## 8. 归属与命名规则

**两类平台专有代码，性质不同，去处不同：**

- **平台专有的「基础设施」→ 进 imbot，remote_control 永不可见。**
  菜单注册、文件 URL 解析、键盘渲染、卡片序列化。
- **平台专有的「产品特性」→ 留在 remote_control，但必须大声。**
  "在 Telegram 上用 WebApp 按钮打开仪表盘"是刻意给某平台更好体验的产品决策，不是
  技术债。形状照抄 `/join` 命令：`WithPlatforms(...)` 显式声明 + 其他平台明确提示。

命名：内容中立的文件不得叫平台名（`telegram_keyboard.go` → `action_menu.go`）；
反之，含真 Telegram 调用的文件在 Seam 1+4 完成前**保留原名**，否则名字只是换个
方向撒谎（`telegram_dir_browser.go`、`telegram_callback.go` 仍待改）。

已消除的错误注释：Feishu 渲染器原有一句"defined in internal/remote_control to avoid
import cycles with imbot/platform packages"——不成立，它只需要 `interaction.Card`，
而 feishu 包本来就 import 了 `interaction`。

## 9. 验收标准

1. **`internal/remote_control` 里不再有*隐式*平台分支。** 判据不是"平台字面量归零"
   ——那会连带禁掉合理的产品特化。每一处残留必须属于：显式产品特化（带
   `WithPlatforms` / `Fallback`）、Tier 3 逃生舱调用、或注释与日志文案。
   **基础设施类**分支必须归零：`buildAuthConfig` / `hasValidAuth` / 菜单 switch /
   `AsTelegramBot` / 键盘预渲染 / `context_token` 透传。
   目前只剩 `context_token` 透传（Seam 2，Phase 4）——其余均已归零。
2. Feishu/Lark 上 Clear / CD / Project 可见且可点。（2a ✅）
3. Feishu 上点完按钮旧键盘被撤除。（Seam 4 ✅）
4. `handler_verbose.go` 的能力判定恢复启用。（Seam 3 ✅）
5. 在 imbot 新增一个假想平台，`remote_control` **零改动**即可跑通
   `manager_channel_test.go` 的 notify 全链路。
6. 回调载荷的长度约束不再泄漏到调用方：64 字节作为**生效的约束**只存在于
   `imbot/platform/telegram/callback_codec.go`；`interaction` 与
   `internal/remote_control` 里仅剩解释历史的注释。（2b ✅）
7. `internal/remote_control` 与 `remote/channel` 里不再有 `FormatCallbackData` /
   `CallbackButton` / `FormatDirPath` 调用——按钮一律用 `ActionButton` /
   `NewPayload` 声明 segment。（2b ✅）
8. Feishu 卡片按钮点击有入站路径，且与其它平台走同一套 dispatch。（2b ✅，
   待真机验证）

## 10. 测试

- `imbot/platform/telegram/action_render_test.go` — 行结构保留、URL 动作、
  无 callback 也无 URL 的动作被丢弃（Telegram 会整条拒绝）、Tier 3
  `WebAppButton` / `SwitchInlineButton` 往返与 `Fallback` 声明。
- `imbot/platform/feishu/action_render_test.go` — **§7.1 的回归测试**（按钮数量与
  callback 必须存活）、Tier 3 在 Feishu 上按 `Fallback` 降级、legacy 形状解码与
  未知形状报空。
- `imbot/platform/feishu/card_render_test.go` — 卡片 JSON 合法性、分节字段、
  中立样式 → Feishu 按钮类型的映射。
- `imbot/platform_auth_test.go` — 每平台的 auth 映射、缺失凭据的具名报告、
  **Lark 的回归测试**，以及 `TestAuthMappingCoversEveryConfiguredPlatform`
  ——它会在下一次给表加平台却漏填 wire mapping 时直接失败。
- `imbot/core/behavior_test.go` — 配对默认值（往宽松方向错等于把 DM 命令权
  交给任何持有泄漏 token 的人，值得显式钉住）、verbose 抑制、未知平台取零值。
- `imbot/platform/tingly/restate_test.go` — 撤菜单、换文本+控件、
  以及不支持平台/空引用时 `RestateOrIgnore` 安静返回 false。
- `imbot/core/payload_test.go` — 越界读取返回 `""` 而非 panic（老版本按钮 segment
  更少）、`HasSeparator`、`EffectivePayload` 的兼容优先级、`IsLink` 必须把 payload
  算进去（算错就把控件渲染成纯链接，回调永不触发）。
- `imbot/platform/telegram/callback_codec_test.go` — **线上字节不变**（短载荷仍是
  `bind:up`，老键盘不失效）、超长载荷编码后必定 ≤64（§5 那条会整条消息发不出去的
  缺陷）、含 `:` 的路径往返且不产生 NUL、保留前缀不被误认成 token、失效 token 可被
  区分、FIFO 逐出。
- `imbot/platform/feishu/card_callback_test.go` — button value 携带 segment 往返、
  含 `:` 的路径、legacy 扁平串仍可解、JSON 化后的 `[]interface{}` 形状，以及
  `getReceiveIdType` 的前缀映射（§7.7）。
- `../internal/remote_control/feature/telegram_dir_browser_test.go` — 目录按钮携带**路径**
  而非索引、含 `:` 的目录名可导航、create 确认按钮携带原始路径。
- `imbot/platform/tingly/tingly_test.go` — 新契约（`Actions`）与兼容期
  （legacy metadata）各一。原 `TestBot_SendWithTelegramKeyboard` 已删除：它断言的
  双形状解码正是本次消灭的耦合。

**注意 `imbot/tests/` 在 `//go:build e2e` 之后**，默认 `go test ./...` 编译不到。
改动 imbot 公共 API 后须跑 `go vet -tags e2e ./...`，否则会出现"主干已改坏、测试
全绿"的假象。
