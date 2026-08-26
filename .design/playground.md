# Playground — 高度可定制的端到端测试工作台

> 适用对象：tingly-box 前端 / 后端贡献者。
> 状态：**设计方案**（未实现）。线框图见 [`playground.pencil.md`](./playground.pencil.md)。
> 前置阅读：[`probe.md`](./probe.md)、[`rule-flags.md`](./rule-flags.md)、[`ux-principles.md`](./ux-principles.md)。

---

## 1. 定位：它是什么、不是什么

TB 已有两类端到端验证能力：

| 已有能力 | 形态 | 回答的问题 | 局限 |
|----------|------|------------|------|
| Probe（E2E dialog） | 目标附着的弹窗诊断 | "这个 provider/rule 现在通吗、怎么走的、回了什么" | 固定 fixture、单条 message、只能测 rule **已落库**的 flags、次要轴折叠在 Advanced 里 |
| Rule-flag harness（`protocoltest`） | 后端测试套件 | "这个 flag 的行为契约还成立吗" | 开发者工具，用户不可达；fixture 固定 |

**Playground 是第三个**：一个一级页的测试工作台——"放大的 probe，高度可定制"。
它把 probe 的所有旋钮全部摊开常驻，加上两个 probe 没有的自由度：

1. **自定义 conversation**（system + 多轮消息），不再只有固定 fixture + 单条 override；
2. **Flags overlay**——任选 rule flag 组合**临时**应用于本次请求，不落库、不改动任何 rule。

所有维度可以自由叠加（轴 × flags × conversation 任意组合），也可以只动一个旋钮做独立测试。

**它不是什么**：

- 不替代 Probe dialog。Probe 是"就地快速诊断"（在 provider 卡片 / rule 齿轮处一键打开，
  两次点击拿到结论）；Playground 是"离开原地、自由实验"。两者共享同一套后端（`/api/v2/probe*`）
  与同一套结果组件，但入口心智不同，都保留。
- 不是 chat 客户端。它发的是**一次**探测请求并解剖其全过程，没有会话状态、没有持续对话。
  （对话式试用是另一个产品面，不在本设计范围。）
- 命名全局唯一（ux-principles #3）：产品内 "Playground" 只指这个页面；这个页面只叫 Playground。

### 用户问题驱动的信息架构（ux-principles #1）

页面按用户脑中的三个问题分区，而不是按后端字段分类：

| 用户的问题 | 页面分区 |
|------------|----------|
| ① 我要发什么？ | 左栏 **Compose**（Target + Axes + Plugins overlay）+ 中栏 **Conversation** |
| ② 实际发出的是什么？ | 右栏 **Payload**（Request / cURL，实时构造） |
| ③ 发生了什么？ | 中栏 **Result**（Status → Journey → Response → Raw） |

---

## 2. 页面布局

三栏工作台（宽屏），窄屏时 Payload 栏下沉为底部折叠区。完整线框见
[`playground.pencil.md`](./playground.pencil.md) §1。

```
┌─ rail ─┬────────────────────────────────────────────────────────────────────┐
│        │  Playground                                              [▶ Run]   │
│  nav   │ ┌─ Compose ────┐ ┌─ Conversation ─────────┐ ┌─ Payload ─────────┐ │
│        │ │ Target        │ │ System [............] │ │ ▤ Request │ cURL  │ │
│  ▷ PG  │ │  [rule/provider│ │ ┌ user ────────────┐ │ │ POST /tingly/...  │ │
│        │ │   unified pick]│ │ │ …                │ │ │ headers…          │ │
│        │ │ Axes (全展开)  │ │ └──────────────────┘ │ │ {                 │ │
│        │ │  Shape Scope   │ │ ┌ assistant ───────┐ │ │   "model": …      │ │
│        │ │  Tool Vision   │ │ │ …                │ │ │   "messages": […] │ │
│        │ │  Thinking      │ │ └──────────────────┘ │ │ }            ⧉    │ │
│        │ │  Protocol      │ │ [+ turn] [templates] │ │ (debounced live)  │ │
│        │ │ Plugins overlay│ ├─ Result ─────────────┤ └───────────────────┘ │
│        │ │  (registry-    │ │ ✅ 850ms · 43 tok    │                       │
│        │ │   driven 三态) │ │ Journey (默认展开)   │                       │
│        │ └───────────────┘ │ Response / Raw JSON  │                       │
│        │                    └──────────────────────┘                       │
└────────┴────────────────────────────────────────────────────────────────────┘
```

布局要点：

- **左栏不再有 Advanced 折叠**。Probe dialog 把 Tool/Vision/Thinking/Protocol/Message 收进
  Advanced 是因为 80% 的诊断只碰 Shape/Scope；Playground 的存在理由恰恰是"所有旋钮可见、
  可叠加"，折叠反而违背页面使命。复用 `ProbeControls` 的 Axis / ExclusiveToggle / 滑杆原语
  （抽出为共享组件），但布局参数不同（无 Collapse）。
- **Payload 常驻右侧**，不是折叠在底部（probe dialog 的 cURL 位置）。它是本页第二主角：
  用户每拨一个旋钮，右侧 payload 实时（debounce 500ms）重建，"这个 flag 改了 body 的哪个
  字段"当场可见。构造走现有 `POST /api/v2/probe/curl`（construct-only，与执行共用同一
  param builders——两边**不可能**漂移，见 probe.md §cURL generation）。
- **Result 紧贴 Conversation 之下**（中栏）：发出的东西和它的结果在同一视野；Journey 默认
  **展开**（probe dialog 里默认收起）——来 Playground 的用户就是来看链路细节的。
- Run 按钮在页头右侧常驻；`⌘/Ctrl+Enter` 触发。

---

## 3. Target 模型

沿用 probe 的 target 语义，不发明新概念：

| target | 复用自 probe | Playground 行为 |
|--------|--------------|-----------------|
| `rule` | `E2ETargetRule` | 走 TB loopback `/tingly/{scenario}`，完整 middleware + 路由管线 |
| `provider`(+model) | `E2ETargetProvider` | 默认 loopback（synthetic rule pin），可切 Direct 对照 |
| `provider_config` | `E2ETargetProviderConfig` | **不纳入**。它服务于"未保存配置的连通性"（Connect AI 场景），Playground 玩的是已保存的对象；纳入只会引入第三种 target 心智 |

**统一 target picker**（ux-principles #2：消解模式选择）：不做 "先选 rule 还是 provider"
的两段式，而是一个可搜索的单选下拉，分组平铺：

```
  Rules        ├ Claude Code · cc-rule (claude_code)
               ├ Codex · codex-rule (codex)         ← 按 scenario 分组，含 profile
  Providers    ├ Kimi  ▸ kimi-k2-0905-preview       ← provider 行内二级选 model
               ├ OpenRouter ▸ …
```

选中即为 target；target 类型只是所选对象的属性，不是先要回答的问题。

**深链入口**（ux-principles #11：把物件交到下一步动作手上）：

- Probe dialog 标题栏加 "Open in Playground"：携带当前 target + axes + message 跳转——
  在弹窗里发现问题、去工作台深挖，是最自然的升级路径。
- Rule 卡片齿轮菜单、provider 卡片菜单同样加入口。
- URL 携带 target：`/playground?target=rule:{uuid}` / `?target=provider:{uuid}:{model}`，
  便于分享和回跳。URL 参数优先于 localStorage 恢复（§10）。

---

## 4. Axes：完全复用 probe 的正交轴

轴模型、可用性归约、默认值全部复用，零新语义：

| 轴 | 复用 | Playground 差异 |
|----|------|-----------------|
| Shape (stream) | `ProbeAxes.stream` | 无，默认 Stream |
| Scope (direct) | `scopeAvailable`（rule 锁 Through-TB） | Direct 时 **Plugins overlay 区整体禁用**（见 §5） |
| Tool | `ProbeAxes.tool` | 无 |
| Vision | `visionAvailable`（Google 禁用） | 无 |
| Thinking | 五档 ladder | 无 |
| Protocol | `protocolAvailability`（per-target 归约/锁定） | 无 |

实现上把 `probeConfig.ts` 的归约函数与 `ProbeControls` 的原语提炼为
`components/probe/` 内的共享模块，dialog 与 playground 各自组装布局——**一份轴逻辑，
两种排布**，避免 fork。

---

## 5. Plugins overlay：临时 flag 组合（本设计的核心新能力）

### 5.1 现状与缺口

今天 probe 测 flag 的唯一方式是 `X-Tingly-Probe-Rule` 加载**已落库**的 rule flags。
想验证 "开了 `use_max_completion_tokens` 之后 payload 变成什么样 / 上游还接不接受"，
只能先改 rule 保存 → probe → 再改回来。改动生产配置来做实验，既危险也违背
"done ≠ locked" 之外的另一面——**实验不应污染真实配置**。

### 5.2 请求语义：overlay，不是替换

`E2ERequest` 新增：

```go
// Flags is a per-request rule-flag overlay for playground-style testing.
// Only keys present in the JSON object are applied; they override the
// resolved (rule + scenario inherited) value for this one request. Keys and
// value types are validated against typ.RuleFlagRegistry(). Through-TB only.
Flags map[string]json.RawMessage `json:"flags,omitempty"`
```

- **只带用户显式设置的 key**（"出现即覆盖"）。`map[string]json.RawMessage` 保留了
  "哪些 key 出现过"的信息——typed struct 做不到区分零值与未设。
- key / 值类型按 `typ.RuleFlagRegistry()` 校验（registry 仍是唯一可信源，
  `ValidateE2ERequest` 中新增校验；未知 key、类型不符直接 400）。
- 语义是 **overlay**：没设置的 flag 走 rule + scenario 的正常解析继承。这让
  rule target 的实验读作"假如这条 rule 的 flags 是这样"，与用户心智一致。

### 5.3 传输与应用点

Through-TB 请求经 loopback，flag 解析发生在 TB 的 handler 内——overlay 必须搭 probe
header 便车（与 `X-Tingly-Probe-Service` / `X-Tingly-Probe-Rule` 同族）：

```
E2EProber → SDK client (probeHeaderRoundTripper)
    X-Tingly-Probe-Flags: base64url(JSON object)      ← 新 header
        → TB loopback handler
            → ResolveRuleFlagsWithScenario:
                 1. rule flags（含 cursor_compat_auto 折叠）
                 2. scenario 继承（or / override 语义）
              ➊  3. probe overlay 应用（本设计新增）
                 4. autoSetCleanHeaderFlag（协议转换自动项）
                 5. Claude OAuth 的 CleanHeader 抑制
            → X-Tingly-Applied-Flags 照常回显 → Result.AppliedFlags
```

**overlay 插在 ➌（scenario 继承之后、自动项/抑制之前）**，理由：

- overlay 要能盖过 rule + scenario 的任何解析结果——插在继承之前会被 `or` 语义
  （`SkipUsage || scenario`）重新合并回去，"关闭一个 scenario 默认开启的 flag" 就做不到。
- 但 **步骤 4/5 代表物理约束而非配置偏好**（billing header 必须到达 Claude OAuth 的计费
  后端），实验不可覆盖——overlay 之后仍然执行。用户在 payload/AppliedFlags 里会看到
  真实生效值（ux-principles #5：展示具体值），差异本身就是教育。
- header 不鉴权与其余 probe header 一致——probe 面本就是 admin-only surface
  （probe.md §Trade-offs），文档化即可；header 值大小按 registry 全量 flag 估算 < 1KB，
  无需分片。

**Direct 探测不支持 flags**（也不该支持）：flags 是 TB middleware 的行为，Direct 的存在
理由是"绕开 TB 做对照实验"（ux-principles #7）。请求层面：`direct=true` 且 `flags` 非空
→ 400；UI 层面：Scope 切到 Direct 时 Plugins 区整体禁用 + hint 说明原因。

### 5.4 UI：registry-driven 三态

完全复用 rule-flags 的前端资产：`GET /rule/flags/registry` + `flagHelpers.ts`
（`getFlagValue`/`setFlagValue`/`isFlagActive`/`flagDefault`），按 `FlagSpec.Category`
分组渲染，控件按 `spec.Type` 选择（bool→Switch、enum→Select、string→TextField、
int→number、service_ref→picker）——与 `FlagCatalogDialog` 同构，**新增 flag 零 Playground
改动**。

每个 flag 行是**三态**，而非简单开关：

| 态 | 呈现 | 语义 |
|----|------|------|
| inherited | muted，显示目标解析出的**具体值**（rule target 预载 rule.Flags + scenario 继承；provider target 为全默认） | 本次请求不干预 |
| overridden | 高亮边框 + 当前值 + 单项 ↺ reset | 出现在 overlay 里 |
| 分区级 | 顶部 "N overridden · Reset all" | 一眼看清实验偏离了基线多少 |

inherited 态展示的是**解析后的具体值**而不是 "默认" 字样（ux-principles #5）——
比如 CC rule 的 `clean_header` inherited 显示 "on (rule default)"。

> 前端预载基线仅为**展示**（读 rule.Flags + scenario flags 做浅合并即可）；生效值的
> 权威永远是响应回显的 `AppliedFlags`。两者不一致时（如 OAuth 抑制），以回显为准，
> UI 在 Journey 的 Flags 行并列展示，不试图在前端复刻全部后端逻辑。

---

## 6. Conversation：多轮消息编辑器

### 6.1 为什么必须是多轮

很多 flag / 转换行为**只在特定消息形态下发生**，单条 message 根本测不到：

| 要测的行为 | 需要的形态 |
|------------|-----------|
| `claude_code_compat` | 会话**中段**的 `role:"system"` 消息 |
| `cursor_compat` | 富文本（array content）user 消息 |
| `block_tools` | 请求携带 tools（Tool 轴已给）+ 多轮 |
| smart_compact / thinking 剥离 | assistant 带 thinking 的历史轮 |
| 各类转换保真 | 多轮 + 混合 role（harness 的 `flagBaseRequest` 正是为此做成多轮） |

工作台若只能发一句话，flags overlay 就是摆设——两者是同一条价值链。

### 6.2 请求模型

```go
// System sets the top-level system prompt (Anthropic `system` / OpenAI
// system message). Empty = probe's default echo instruction behavior.
System string `json:"system,omitempty"`

// Messages replaces the probe's built-in fixture with a custom conversation.
// Roles: "user" | "assistant" | "system" (system allowed mid-conversation —
// that non-standard shape is itself a test subject, see claude_code_compat).
// Empty = existing fixture + Message override behavior, unchanged.
Messages []ProbeMessage `json:"messages,omitempty"`

type ProbeMessage struct {
    Role string `json:"role"`
    Text string `json:"text"`
}
```

- **向后兼容**：`messages` 为空时行为与今天完全一致（fixture + `message` override）；
  probe dialog 不受影响。`messages` 与 `message` 同时出现时 `messages` 赢（校验层拒绝
  更好——二选一，避免猜）。
- V1 刻意收窄为 `role + text`：自定义 tool 定义、自定义图片、array content 编辑不做
  （见 §15）。富文本形态先靠模板注入（见下）。Tool / Vision 轴的 fixture 注入逻辑照常
  叠加在自定义 conversation 之上（tools 定义、canonical image 仍来自轴）。
- `internal/probe/helper.go` 的三个 param builders（OpenAI Chat / Responses / Anthropic）
  各自把 `[]ProbeMessage` 映射为 SDK 消息类型；Google 目标不支持 messages（与 vision
  同一台阶的能力缺口，校验期拒绝）。
- 末轮必须是 user（或 tool 轴的合成轮），校验给出明确错误。

### 6.3 编辑器 UI 与模板

- System 单独一格置顶；下方消息列表：每条 = role chip（点击轮换 user/assistant/system）
  + 多行文本 + 删除；`+ turn` 追加；拖拽排序 V1 不做（上下移按钮即可）。
- **Templates 下拉**（ux-principles #8：教育内嵌产品）：预置"教材式"形态一键填充——
  `Multi-turn`（基础三轮）、`Mid-conversation system`（claude_code_compat 的测试形态）、
  `Tool round-trip`（配合 Tool 轴）。模板名旁一句话说明它测什么。模板就是把 harness
  `flagBaseRequest` 的知识透给用户。

---

## 7. Payload 面板

- 两个 tab：**Request**（method + URL + headers 表 + pretty body，秘密仍以
  `$TB_API_KEY` / `$UPSTREAM_API_KEY` 占位）与 **cURL**（现有 `command`）。同一响应
  （`probe.CurlData`）渲染两种视图，无新端点。
- 配置变更 → debounce 500ms → `POST /api/v2/probe/curl`。构造失败（如校验错）时面板
  显示错误原因——**payload 面板同时兼任"配置是否合法"的即时反馈**。
- 每块可复制（CopyBlock 复用）；cURL caption 保留 key 替换提示。
- Run 后 payload 不变（同一 builder 构造，天然一致）；不做"回显实际请求"的第二份
  payload——单一来源，避免两份 JSON 让用户对不上。

---

## 8. Result 与 Run history

- 结果区复用 probe dialog 的四件套：StatusBar → Journey → Response → Raw JSON。
  从 `ProbeDialog.tsx` 提炼为共享组件（如 `components/probe/ResultSections.tsx`），
  dialog 与 playground 共用——避免两处维护 journey 字段映射。
- Playground 差异：Journey **默认展开**；Flags 行在 overlay 生效时并列展示
  `AppliedFlags`（权威）与 overlay 请求值，不一致处即为教育点（§5.4）。
- **Run history**（session 级）：结果区顶部一行 chips，最近 ~10 次 run：
  `✅ 850ms · stream · 2 flags` 。点击回看该次结果，且左栏/conversation 恢复为该次的
  请求配置（靠 Result 的 request-echo 字段 + 本地保存的请求快照）——"完成 ≠ 锁死"
  （ux-principles #10），对照两次实验是工作台的日常动作。不落库、刷新即清（V1）。

---

## 9. 导航与路由

- 一级 activity rail 项：`key: 'playground'`，`path: '/playground'`，icon 走
  `@/components/icons`（tabler `IconTestPipe` 经 `tablerMui()` 适配；`IconFlask` 已被
  VModel 用掉，避免视觉撞车）。位置放 Usage 之后——它与 Dashboard 同属"观察与验证"域。
- 页面 `React.lazy(() => import('./pages/playground/PlaygroundPage'))`（frontend/CLAUDE.md
  的 code-splitting 铁律）；**不从 page 文件导出任何共享状态**——picker 数据、共享轴原语
  都放独立模块。
- i18n：`en.ts` / `zh.ts` 增 `playground.*` 命名空间；复用 `probe.*` 已有的轴文案
  （同一概念同一词）。

---

## 10. 状态持久化

| 内容 | 存储 | 理由 |
|------|------|------|
| target + axes + flags overlay + conversation | `localStorage: tb.playground.state` | Playground 是**工作台**：离开再回来续着做是预期。这与 probe dialog 的"轴不持久化"决策**刻意相反**——probe 是诊断，默认必须可预测（probeConfig.ts 注释）；工作台的可预测性由"回来时和离开时一样"定义。两条决策各自成立，勿互相"统一" |
| URL `?target=` 深链 | — | 优先于 localStorage（显式意图 > 记忆） |
| Run history | 内存（session） | 见 §8；落库是 V2 的事 |

---

## 11. 后端改动清单

| # | 位置 | 改动 |
|---|------|------|
| 1 | `internal/probe/types.go` | `E2ERequest` 增 `Flags map[string]json.RawMessage`、`System`、`Messages []ProbeMessage`；`ValidateE2ERequest` 扩展（flags key/type 按 registry 校验、direct×flags 互斥、messages role/末轮校验、google 拒绝、`messages`×`message` 互斥） |
| 2 | `internal/typ/flag_registry.go` | 增按 key 校验值类型的 helper（`ValidateFlagValue(key, raw)`），registry 唯一可信源不变 |
| 3 | `internal/probe/e2e_probe.go` | loopback 路径上把 `Flags` 编码进 `X-Tingly-Probe-Flags`（base64url(JSON)），挂 probe headers |
| 4 | `internal/protocolserver/rule_flags.go` | `ResolveRuleFlagsWithScenario` 在 scenario 继承后应用 header overlay（解码 → registry 校验 → 逐 key set）；自动项/OAuth 抑制保持在 overlay 之后 |
| 5 | `internal/probe/helper.go` | 三个 param builders 接受自定义 conversation（`probeParams` 增 `System`/`Messages`），空时走现有 fixture 路径 |
| 6 | `internal/probe/curl.go` | 无逻辑改动（同 builders，自动跟上） |
| 7 | swagger | probe 路由的 request model 更新 → `task codegen`（backend openapi + 前端 client sdk）。codegen 前，前端按 CLAUDE.md 约定使用 placeholder |
| 8 | 测试 | `internal/protocoltest/`：新增 overlay 端到端 case（overlay 开 flag → `VirtualServer.LastRequest` 见效果；overlay 关 scenario 默认开的 flag → 不见效果；OAuth 抑制不可覆盖）；`probe` 包内 messages 构造单测 + curl 快照 |

## 12. 前端改动清单

| # | 位置 | 改动 |
|---|------|------|
| 1 | `pages/playground/PlaygroundPage.tsx` | 新页面（lazy），三栏布局编排 |
| 2 | `components/probe/` | 提炼共享件：轴原语（Axis/ExclusiveToggle/thinking 滑杆）、`ResultSections`、`CopyBlock`；`probeConfig.ts` 的归约函数直接复用 |
| 3 | `pages/playground/` 内部组件 | `TargetPicker`、`FlagOverlayPanel`（registry-driven 三态，复用 `flagHelpers.ts`）、`ConversationEditor`（含 templates）、`PayloadPanel` |
| 4 | `App.tsx` / `layout/useActivityItems.tsx` | lazy route + rail 项 |
| 5 | `services/api.ts` | 沿用 `runProbe`/`buildProbeCurl` 封装；新字段在 codegen 落地前以 placeholder 透传 |
| 6 | i18n | `playground.*` en/zh |
| 7 | 入口 | ProbeDialog "Open in Playground"、rule 齿轮菜单、provider 卡片菜单 |

## 13. 分期交付

三个可独立合并的阶段，每段结束都是可用状态：

1. **骨架页**（纯前端 + 现有 API）：三栏布局、target picker、全展开轴、单 message、
   payload 实时面板、result、run history。Plugins 区已渲染（inherited 只读展示），
   overlay 控件禁用 + hint "coming"。
2. **Flags overlay**（后端 #1–#4 + 前端 FlagOverlayPanel 激活 + codegen）——核心价值落地。
3. **Conversation**（后端 #5 + 前端 ConversationEditor + templates）。

每阶段用 `ui-preview` skill 截图验收布局。

---

## 14. 设计取舍

| 选项 | 已采纳 | 备择 | 取舍理由 |
|------|--------|------|----------|
| 独立一级页 vs 扩展 probe dialog | 一级页 | dialog 加 tab / "expert mode" | dialog 的价值是就地、轻、快；塞入 flags+conversation 会毁掉它的诊断心智，也放不下三栏。两者共享后端与结果组件，成本可控 |
| flags 传输 | 请求体 `flags` → probe header 中继 | loopback 请求体内嵌带外字段 | 与 `X-Tingly-Probe-Service/Rule` 同族同路径；请求体必须保持纯净的协议形态（它就是被测对象） |
| overlay（出现即覆盖） vs 全量替换 | overlay | UI 算好效果集整体替换 | 替换要求前端复刻 scenario 继承（or/override）与自动项逻辑——双实现必然漂移。overlay 让后端解析仍是唯一权威，前端只声明"我动了哪几个" |
| overlay 应用位置 | scenario 继承后、自动项/抑制前 | 最末尾（全覆盖） | 自动项与 OAuth 抑制是物理/计费约束，实验覆盖它们产出的是不可能存在于生产的假结果；差异经 AppliedFlags 回显，本身就是教育 |
| Direct × flags | 互斥（400 + UI 禁用） | 静默忽略 | flags 是 TB middleware 行为；静默忽略会让用户以为测了实际没测——最坏的一种假成功 |
| flags key 集合 | `map[string]json.RawMessage` + registry 校验 | typed `RuleFlags` 指针字段 | typed struct 无法区分"未设"与"设为零值"，而这正是 overlay 的核心语义；registry 校验保住类型安全 |
| conversation V1 范围 | role+text 多轮 + 模板 | 完整 content-block 编辑器 | 中段 system / 多轮已解锁最大一批测试场景；富文本/自定义 tools 的编辑器复杂度指数上升，先用模板顶住教育需求 |
| 消息形态自定义与 Tool/Vision 轴的关系 | 轴 fixture 叠加在 conversation 上 | conversation 全接管 | 轴保持正交（ux #4）：Tool 轴管 tools 定义，Vision 轴管图片，conversation 管文字轮次——各管一轴，组合自由 |
| 配置持久化 | localStorage 持久化 | 跟随 probe 的不持久化 | 诊断要默认可预测，工作台要延续上下文——两个页面的正确答案相反，显式写下避免"统一"冲动 |
| provider_config target | 不纳入 | 三种 target 全支持 | 它属于 Connect AI 的"保存前验证"流程；Playground 面向已保存对象，多一种 target 只添心智噪音 |
| Run history | session 内存 | 落库持久化 | 先验证"回看/对照"是不是真实高频动作，再决定值不值得一张表 |

---

## 15. 未做 / 后续可做

- **Variant 对比运行（matrix run）**：同一配置一键跑 A/B——flag on vs off、Through-TB vs
  Direct、两个 provider——结果并排 diff（payload diff + response diff）。这是"对照实验"
  的完全体，也是 playground 数据模型（配置快照 + run 结果）天然支持的方向。
- 自定义 tool 定义 / 自定义图片 / array content 编辑（conversation 编辑器 V2）。
- 命名预设（保存/分享一份 playground 配置；配合 URL 深链导出）。
- "导出为 harness case"：把当前配置生成 `protocoltest` flagCase 骨架——用户复现的问题
  一键变回归测试。
- Run history 落库 + 与 recording 子系统打通。
- Scenario 级 flag 的 overlay（当前 overlay 仅 rule 级 registry；scenario-only flag 如
  `smart_compact` 待 ScenarioFlagRegistry 建立后同机制接入，见 rule-flags.md §13）。

---

## 16. UX 原则对照（ux-principles.md 12 条 checklist）

| # | 原则 | 本设计的落点 |
|---|------|--------------|
| 1 | 按用户问题组织 IA | 三栏 = 我要发什么 / 实际发出什么 / 发生了什么（§1） |
| 2 | 消解模式选择 | 统一 target picker；进页即工作面，无向导 |
| 3 | 命名唯一 | Playground / Plugins / Probe 各指一物；轴词汇与 probe 完全共用 |
| 4 | 正交维度分轴 | 轴 × flags overlay × conversation 三个正交面板；轴内沿用 probe 拆轴成果 |
| 5 | 展示具体值 | inherited flag 显示解析后实际值；payload 展示真实 body；AppliedFlags 回显权威生效值 |
| 6 | 聪明默认 | 轴默认沿用 probe（Stream/Through-TB/primary protocol）；flags 默认全 inherited；conversation 默认 fixture |
| 7 | 诊断走真实链路 | 一切默认 loopback 生产路径；Direct 仅作对照且与 flags 互斥 |
| 8 | 教育内嵌 | conversation templates 即教材；overlay 与 AppliedFlags 的差异展示约束本身 |
| 9 | 降视觉噪声 | 主角是 payload 与 result；flags 未覆盖时 muted；journey 字段仍是等宽极简行 |
| 10 | 完成 ≠ 锁死 | run history 回看并恢复配置；页面状态持久化，随时回来续做 |
| 11 | 交付下一步物件 | cURL/payload 逐块可复制；probe→playground 深链带配置；（V2）导出 harness case |
| 12 | 副作用限定当前表面 | overlay 不落库、不触碰任何 rule/scenario 配置；run 不进 usage 统计之外的任何状态 |
