# Bench — 高度可定制的端到端测试台

> 适用对象：tingly-box 前端 / 后端贡献者。
> 状态：**已实现（V1）**——页面 `frontend/src/pages/bench/`，后端字段见 §11。线框图见 [`bench.pencil.md`](./bench.pencil.md)。
> 前置阅读：[`probe.md`](./probe.md)、[`rule-flags.md`](./rule-flags.md)、[`ux-principles.md`](./ux-principles.md)。

---

## 1. 定位：它是什么、不是什么

TB 已有两类端到端验证能力：

| 已有能力 | 形态 | 回答的问题 | 局限 |
|----------|------|------------|------|
| Probe（E2E dialog） | 目标附着的弹窗诊断 | "这个 provider/rule 现在通吗、怎么走的、回了什么" | 固定 fixture、单条 message、只能测 rule **已落库**的 flags、次要轴折叠在 Advanced 里 |
| Rule-flag harness（`protocoltest`） | 后端测试套件 | "这个 flag 的行为契约还成立吗" | 开发者工具，用户不可达；fixture 固定 |

**Bench 是第三个**：一个一级页的测试工作台——"放大的 probe，高度可定制"。
它把 probe 的所有旋钮全部摊开常驻，加上三个 probe 没有的自由度：

1. **原始请求**——直接写一份 Anthropic Messages / OpenAI Chat / OpenAI Responses 形态的
   客户端请求发进去（§6），多轮、system、tools、图片、任意参数都是请求本身的一部分，
   不再只有预设请求 + 单条 message override；
2. **Flags overlay**——任选 rule flag 组合**临时**应用于本次请求，不落库、不改动任何 rule；
3. **Header 覆盖**——直接改最终发出的 header（§7），入站特征类的中间件也能触发。

所有维度可以自由叠加（轴 × flags × 请求任意组合），也可以只动一个旋钮做独立测试。

**它不是什么**：

- 不替代 Probe dialog。Probe 是"就地快速诊断"（在 provider 卡片 / rule 齿轮处一键打开，
  两次点击拿到结论）；Bench 是"离开原地、自由实验"。两者共享同一套后端（`/api/v2/probe*`）
  与同一套结果组件，但入口心智不同，都保留。
- 不是 chat 客户端。它发的是**一次**探测请求并解剖其全过程，没有会话状态、没有持续对话。
  （对话式试用是另一个产品面，不在本设计范围。）
- 命名全局唯一（ux-principles #3）：产品内 "Bench" 只指这个页面；这个页面只叫 Bench。
  > 为什么不叫 Playground：业界的 "playground"（OpenAI Playground、AI Studio）是"对着模型
  > 试 prompt"的聊天形态，与本页"发一次探测并解剖全过程"的心智相反；产品内 Image 场景页
  > 又已有真正 playground 语义的 "Image Playground" 卡片。也不叫 Develop：System → Develop
  > 是给 TB 自己的前端开发者用的工具页，受众不同。"Bench"（测试台）取 test bench 的工程
  > 含义——受控激励打进被测对象、观察输出。唯一歧义是 benchmark，靠中文标签与副标题压住。

### 三种粒度

Probe / Bench / Server 三者不是并列的三个工具，是同一件事在三种**粒度**下的样子：

| 粒度 | 载体 | 问的问题 | 词汇表 |
|------|------|----------|--------|
| 场景 | 探针（Probe） | "这类行为（工具调用/图片/thinking…）能不能存活" | 封闭、协议无关的一组断言 |
| 请求 | Bench | "发这个具体的东西，TB 会怎么处理" | 开放，一份具体的文档 |
| 协议 | Server | "字段怎么从一种 wire 形态映射到另一种" | `internal/protocol/request/*` 那套转换器 |

三者是一条投影链，不是三个孤立的抽屈：**探针的本质就是预设**——一组封闭的、协议无关
的断言；把探针materialize 成一份具体的请求，得到的正是 Bench 默认视图里的那份
**预设请求**（§6 用这个词，不再用"固定场景"/"fixture"）。所以 Bench 的默认视图长得
和 Probe 弹窗一模一样，不是"复用了 Probe 的组件"这种实现细节——它是同一件事：预设
请求就是探针在请求粒度上的投影。而 Bench 的**自定义请求**，是请求粒度上不背靠任何
探针的内容，就是它字面的样子。协议粒度的转换（Anthropic ↔ OpenAI Chat ↔ Responses）
是 Server 的职责，不属于 Bench——Bench 只管"在一个协议里写一份具体的东西"，不管"把
这份东西翻译到另一个协议还讲不讲得通"；真要测跨协议的场景保真度，那问题已经回到了
场景粒度，答案是探针（它的 Protocol 轴 + 其它场景轴组合起来，正是在跑真实转换代码、
观察场景保真度），不是 Bench 该操心的事（这也是 §6 里"切换协议 = 换一份新协议的起始
模板，而不是翻译现有 body"这个决定的根本原因，不只是图省事）。

Scope / Routing / Flags overlay / Header 覆盖不在这条投影链上——它们不在请求 body
里，是"Server 怎么处理这次运行"的配置，和请求内容长什么样无关，预设请求和自定义
请求两种模式都用得到（§7）。Stream 不在此列，见下——它在 body 里，是一个真参数。

> 术语提醒：TB 代码里 `scenario` 另有一个具体含义（路由用的产品面家族，`rule.Scenario`、
> `/tingly/{scenario}` 入口），和这里说的"场景粒度"不是同一件事——后者是更宽泛的
> "一组协议无关的兼容性断言"。

### 四种归类：场景粒度内部也不是铁板一块

"探针的所有旋钮都是预设"这句话过粗——Tool/Vision 轴打开后注入的是**完全写死**的内容
（`internal/probe/helper.go` 里 `getVisionToolOpenAI()`/`getVisionToolAnthropic()` 返回
固定 schema，没有任何参数传入），没有"选哪个工具"这种取值空间，和 Thinking（真实的
effort/budget 取值）、Protocol（决定一切怎么写的坐标系）完全不是一回事。按"这东西到底
是什么"重新归类，不按"UI 上像不像轴"分：

| 归类 | 成员 | 特征 |
|------|------|------|
| 坐标系 | Protocol | 决定其它一切"写出来长什么样"，不是坐标系里的一个点，和别的归类不平级 |
| 真参数 | Stream、Thinking | body 里的具体字段，有真实取值范围，不含任何"内容" |
| 内容预设 · 片段级 | Tool、Vision、Message | 写死的一段东西，只有"塞不塞"这个开关，塞进去的内容不可调 |
| 内容预设 · 整体级 | Bench Templates | 同上，只是范围是整份 body 而不是一个片段 |
| 传输配置 | Scope、Routing、Flags overlay、Header 覆盖 | 不在 body 里，"Server 怎么处理这次运行"，和内容无关 |

**内容预设的两个粒度是同一种东西**：Tool 轴给的固定工具交换，和 Bench Templates 里
的 "Tool round-trip" 概念上是一回事，只是前者局部（一个片段）、后者整体（一份 body）。
这就是为什么 Bench 的"从哪开始"菜单（§6.3）能把两者列进同一份清单，不用发明第三种
机制——它们本来就是同一类，只是粒度不同。

Probe 弹窗和 Bench 的轴面板按这个分类可视化分组（`AxisGroup`，共享组件）：**参数**
（Thinking、Protocol）、**内容**（Tool、Vision、Message）。Shape（Stream）和 Scope
常驻不分组——常驻的理由是触碰频率，不是归类，它们恰好一个是真参数、一个是传输配置。

不完全正交的两处，已经在代码里处理，只是没点破：
1. **Protocol 收窄其它轴的值域**——不是取值互相干扰，是某些组合压根不存在（provider
   只讲 OpenAI 时 Anthropic Messages 不该出现；Direct 模式下 Protocol 被 provider 能力
   锁死）。`protocolAvailability`/`AxisAvailability` 一直在做这件事，是坐标系限定了
   点集，不是 bug。
2. **内容预设覆盖不到真实场景需要的空间**——例如"assistant 带 thinking 的历史轮"需要
   多轮对话 + thinking 内容同时出现在历史里，但 Thinking 轴只管这次请求要不要带
   thinking 配置，不管历史消息里有没有 thinking block。这正是要有 Templates、要有
   自定义请求这条退路的理由，不是这次发现的新问题。

### 与线上运转的差异边界

Through-TB 模式下，从请求进入 `/tingly/{scenario}` 入口起走的就是生产的 handler 链
（flag 解析、transform、rule 内路由、上游 client、响应转换、usage 记账）。与线上不同的只有
两端：

- **入站请求默认是合成的**（SDK 极简 fixture，而非 Claude Code / Codex / Cursor 真实发出的
  形态）。要复现真实客户端的形态，把它的请求体原样粘进 Request 面板（§6）——三种协议
  都收，TB 本来就是协议转换器，这正是它的主场。依赖入站 header 触发的中间件——
  `clean_header` 的 billing 块、UA 优先级——用 §7 的 header 覆盖补上对应特征。刻意不做
  "以某客户端身份发送"这类预设：那是过度设计，真实客户端的形态来自它真实发出的请求，
  而不是一份会漂移的模仿清单。
- **provider target 跳过规则选择**（合成规则钉住 service）。rule target 默认不跳过。

Bench 是"在真实管线里做受控实验"：请求可以是真的，但每次只发一发、并解剖全过程。

### 用户问题驱动的信息架构（ux-principles #1）

页面按用户脑中的三个问题分区，而不是按后端字段分类：

| 用户的问题 | 页面分区 |
|------------|----------|
| ① 我要发什么？ | 左栏 **Compose**（Target + Axes + Plugins overlay）+ 中栏 **Request** |
| ② 实际发出的是什么？ | 右栏 **Payload**（Request / cURL，实时构造） |
| ③ 发生了什么？ | 中栏 **Result**（Status → Journey → Response → Raw） |

---

## 2. 页面布局

三栏工作台（宽屏），窄屏时 Payload 栏下沉为底部折叠区。完整线框见
[`bench.pencil.md`](./bench.pencil.md) §1。

```
┌─ rail ─┬────────────────────────────────────────────────────────────────────┐
│        │  Bench                                              [▶ Run]   │
│  nav   │ ┌─ Compose ────┐ ┌─ Request ──────────────┐ ┌─ Payload ─────────┐ │
│        │ │ Target        │ │ preset: message [...]  │ │ ▤ Request │ cURL  │ │
│  ▷ PG  │ │  [rule/provider│ │   or                   │ │ POST /tingly/...  │ │
│        │ │   unified pick]│ │ custom: [Anthropic ▾]  │ │ headers…  [+ hdr] │ │
│        │ │ Axes (全展开)  │ │ {                      │ │ {                 │ │
│        │ │  Shape Scope   │ │   "messages": […]      │ │   "model": …      │ │
│        │ │  Tool Vision   │ │ }                      │ │   "messages": […] │ │
│        │ │  Thinking      │ │ [templates ▾]          │ │ }         [Edit]⧉ │ │
│        │ │  Protocol      │ │ [back to preset]        │ │ (debounced live)  │ │
│        │ │ Plugins overlay│ ├─ Result ─────────────┤ └───────────────────┘ │
│        │ │  (registry-    │ │ ✅ 850ms · 43 tok    │                       │
│        │ │   driven 三态) │ │ Journey (默认展开)   │                       │
│        │ └───────────────┘ │ Response / Raw JSON  │                       │
│        │                    └──────────────────────┘                       │
└────────┴────────────────────────────────────────────────────────────────────┘
```

布局要点：

- **左栏不再有 Advanced 折叠**。Probe dialog 把 Tool/Vision/Thinking/Protocol/Message 收进
  Advanced 是因为 80% 的诊断只碰 Shape/Scope；Bench 的存在理由恰恰是"所有旋钮可见、
  可叠加"，折叠反而违背页面使命。复用 `ProbeControls` 的 Axis / ExclusiveToggle / 滑杆原语
  （抽出为共享组件），但布局参数不同（无 Collapse）。
- **Payload 常驻右侧**，不是折叠在底部（probe dialog 的 cURL 位置）。它是本页第二主角：
  用户每拨一个旋钮，右侧 payload 实时（debounce 500ms）重建，"这个轴改了 body 的哪个
  字段"当场可见。构造走现有 `POST /api/v2/probe/curl`（construct-only，与执行共用同一
  param builders——两边**不可能**漂移，见 probe.md §cURL generation）。
  注意 Through-TB 时 payload 是 TB 回环入口收到的**入站**请求（被测的协议形态）；flag
  在这之后、TB 内部才生效，所以 flag 的效果不体现在这份 body 里，而体现在响应回显的
  `AppliedFlags`（Journey 的 Flags 行）。面板文案明说这一点。
- **Result 紧贴 Request 之下**（中栏）：发出的东西和它的结果在同一视野；Journey 默认
  **展开**（probe dialog 里默认收起）——来 Bench 的用户就是来看链路细节的。
- Run 按钮在页头右侧常驻；`⌘/Ctrl+Enter` 触发。

---

## 3. Target 模型

沿用 probe 的 target 语义，不发明新概念：

| target | 复用自 probe | Bench 行为 |
|--------|--------------|-----------------|
| `rule` | `E2ETargetRule` | 走 TB loopback `/tingly/{scenario}`，完整 middleware + 路由管线。默认**全链路**：只发 `request_model`，TB 像对真实客户端一样匹配规则（Journey 显示实际命中的规则，与所选不同时给出提示）；可切**钉住规则**（`routing: "pinned"` → `X-Tingly-Probe-Rule`），只跳过匹配这一步 |
| `provider`(+model) | `E2ETargetProvider` | 默认 loopback（`X-Tingly-Probe-Service` 合成规则：跳过规则选择、保留全部中间件，即"近似直连"），可切 Direct 完全绕开 TB 对照 |
| `provider_config` | `E2ETargetProviderConfig` | **不纳入**。它服务于"未保存配置的连通性"（Connect AI 场景），Bench 玩的是已保存的对象；纳入只会引入第三种 target 心智 |

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

- Probe dialog 标题栏加 "Open in Bench"：携带当前 target + axes + message 跳转——
  在弹窗里发现问题、去工作台深挖，是最自然的升级路径。
- Rule 卡片齿轮菜单、provider 卡片菜单同样加入口。
- URL 携带 target：`/bench?target=rule:{uuid}` / `?target=provider:{uuid}:{model}`，
  便于分享和回跳。URL 参数优先于 localStorage 恢复（§10）。

---

## 4. Axes：完全复用 probe 的正交轴

轴模型、可用性归约、默认值全部复用，零新语义：

| 轴 | 复用 | Bench 差异 |
|----|------|-----------------|
| Shape (stream) | `ProbeAxes.stream` | 无，默认 Stream |
| Scope (direct) | `scopeAvailable`（rule 锁 Through-TB） | Direct 时 **Plugins overlay 区整体禁用**（见 §5） |
| Tool | `ProbeAxes.tool` | 无 |
| Vision | `visionAvailable`（Google 禁用） | 无 |
| Thinking | 五档 ladder | 无 |
| Protocol | `protocolAvailability`（per-target 归约/锁定） | 无 |

实现上把 `probeConfig.ts` 的归约函数与 `ProbeControls` 的原语提炼为
`components/probe/` 内的共享模块，dialog 与 bench 各自组装布局——**一份轴逻辑，
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
// Flags is a per-request rule-flag overlay for bench-style testing.
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
→ 400；UI 层面：Scope 切到 Direct 时 Plugins 区整体禁用 + hint 说明原因。由于 rule
target 锁定 Through-TB（§4），这个禁用态只会出现在 provider target 上。

header 的安全边界与 `X-Tingly-Probe-Rule` 相同：任何持网关 key 的客户端都可以在自己的
请求上带它来改变本次请求的 flags（不落库、只影响自己），属于 probe.md 已记录的 admin-only
probe surface；未来若要收紧，三个 probe header 一起做。

### 5.4 UI：registry-driven 三态

完全复用 rule-flags 的前端资产：`GET /rule/flags/registry` + `flagHelpers.ts`
（`getFlagValue`/`setFlagValue`/`isFlagActive`/`flagDefault`），按 `FlagSpec.Category`
分组渲染，控件按 `spec.Type` 选择（bool→Switch、enum→Select、string→TextField、
int→number、service_ref→picker）——与 `FlagCatalogDialog` 同构，**新增 flag 零 Bench
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

## 6. Request：写你自己的客户端请求（三种协议）

Request 面板做两件不同的事，故意不把它们合并成一件：**预设请求**（默认视图，
和 Probe 弹窗共用同一套轴——Tool/Vision/Thinking/Protocol/Message，同一批
builder——它就是探针在请求粒度上的投影，见 §1"三种粒度"）回答"TB 已知的兼容
性矩阵，对这个目标现在还成立吗"；**自定义请求**（写你自己的完整请求，不背靠
任何探针）回答"我发这个具体的东西，TB 会怎么处理"。前者故意固化——它的价值
就是"小、快、可反复用同一套维度验证任意目标"，不该随手加轴；后者故意不受
限——任何轴表达不了的形态，直接在协议原生的 JSON 里写。

这条边界曾经含混过：早期实现里自定义请求只**禁用**部分轴而不是彻底不显示，
Protocol 下拉在自定义请求模式下还能"切换"却只改标签不改 body，这类半耦合正
是混乱的来源——两边都不是完整的自己。现在的规则很简单：**一份请求 body 只有
一个作者**，要么是预设请求的 builder，要么是你写的 JSON，从不"部分借用"对方。
进入自定义请求是一个单向动作（"Write the request yourself"），不是一个可以来
回切的 tab——回去的唯一方式是重新开始，因为自定义 JSON 没法自动逆推回轴的状
态，假装可以双向切换只会制造一种不存在的对称感。

**这个自由度只属于 Bench，不属于 Probe 弹窗。** Probe 弹窗永远只产出预设请求
（轴 + Message 覆盖，这就是它自己），没有自定义请求编辑器、没有 flags overlay、没有 header
覆盖、没有 routing pin——弹窗的价值是"就地、两次点击拿结论"，塞入任何这些都会
稀释它。弹窗唯一新增的出口是"在 Bench 中打开"：带着当前 target/axes/message 跳
到 Bench 的默认视图（和弹窗长得一样），自定义请求这道门只在 Bench 页面里才有。
这是设计约束，不是待办——复杂操作不会因为"顺手"就加回弹窗。

### 6.1 为什么必须能自己写请求

很多 flag / 转换行为**只在特定请求形态下发生**，单条 message 根本测不到：

| 要测的行为 | 需要的形态 |
|------------|-----------|
| `claude_code_compat` | 会话**中段**的 `role:"system"` 消息 |
| `cursor_compat` | 富文本（array content）user 消息 |
| `block_tools` | 请求携带 tools + tool_use / tool_result 轮 |
| 多模态转换 | 带图片块的 user 消息 |
| smart_compact / thinking 剥离 | assistant 带 thinking 的历史轮 |
| 各类转换保真 | 多轮 + 混合 role（harness 的 `flagBaseRequest` 正是为此做成多轮） |

工作台若只能发一句话，flags overlay 就是摆设——两者是同一条价值链。

### 6.2 不自定义消息结构：直接收三种协议的原始请求

早期草案定义过一个 `ProbeMessage{role,text}` 的中间结构。它被否掉了：自造一个消息载体
既表达不了 tools / 图片 / content block，又得在三个 builder 里各写一遍映射，还永远比真实
客户端少一截。TB 是协议转换器，三种 wire 形态它本来就都会解——所以请求载体直接用协议
本身：

```go
// Request is a raw client request body in RequestProtocol's shape
// (Anthropic Messages / OpenAI Chat / OpenAI Responses). It replaces the
// probe's synthesized fixture entirely: message, tool, vision and thinking
// knobs are rejected alongside it. `model` is filled by the probe.
Request         json.RawMessage `json:"request,omitempty"`
RequestProtocol ProbeProtocol   `json:"request_protocol,omitempty"`
```

- 后端用 SDK 自己的 decoder 解析（`anthropic.MessageNewParams` /
  `openai.ChatCompletionNewParams` / `responses.ResponseNewParams`，Responses 先过
  `protocol.PreprocessInputData`，与生产 handler 同一路径），解析结果原样交给现有
  param builder 短路发出——探测只补 `model`（Anthropic 缺省 `max_tokens`、流式时
  chat 的 `stream_options.include_usage`）。请求体是什么形态，发出去就是什么形态。
- **向后兼容**：`request` 为空时行为与今天完全一致（预设请求 + `message` override）；
  probe dialog 不受影响。`request` 与 `message` / tool / vision / thinking 轴互斥
  （校验拒绝，不猜）：这些轴都是"合成预设请求的旋钮"，请求既然是你写的，就整份归你。
- 协议一致性由校验守住：provider target 的 `protocol` 必须等于 `request_protocol`；
  rule target 的 scenario 家族（`ScenarioEndpoint`）必须与之相符。跨协议的"错投"
  不在 Bench 的范围（它测的是 TB 的转换，不是 TB 的 400）。
- 这一层单独成 PR（"accept a raw client request in any of the three protocols"），
  Bench 叠在它之上。详见 probe.md "Raw client requests"。

### 6.3 编辑器 UI：一份"从哪开始"菜单，不是三处分散的入口

早期实现里，"决定自定义请求的 body 从哪来"这件事散落在三处：进门按钮（文案随
`seedBody` 是否存在在 "Write the request yourself" / "Edit the builder's request"
之间切换，靠文案隐晦地告诉用户点了会拿到什么）、进门之后的第二个同名按钮（把
body 重新同步成预设请求现在的样子）、以及一个独立的 Templates 菜单。三处入口、
两个不同的出现时机，服务的却是同一件事。

`StartingPointMenu`（`pages/bench/RequestEditor.tsx`）把它们收成一份菜单，两个
触发点共用：

- **门**（预设请求模式）：按钮文案固定为 **Write the request yourself**，不再随
  `seedBody` 变化。点开是同一份菜单。
- **Change starting point**（自定义请求模式内）：已经在自定义视图里想换起点时，
  打开同一份菜单，替换掉原来分开的"Edit the builder's request"按钮和"Templates"
  按钮。

菜单内容两处完全一致（§1"四种归类"——它们都是内容预设，粒度不同而已）：

| 选项 | 是什么 |
|------|--------|
| Copy the preset request | 预设请求（探针）现在会发出的样子；`seedBody` 存在才显示 |
| Blank | 该协议形态下的一份空请求（`{messages:[]}`/`{input:[]}`），不是隐式回退到第一个模板 |
| Multi-turn / Tool round-trip / Image / Mid-conversation system | 按协议各备的"教材式"整体内容预设（ux-principles #8），Anthropic 独有 Mid-conversation system（claude_code_compat 的测试形态） |

选中任何一项都是**整体替换**，不做确认弹窗——这个页面的受众本来就习惯"重来"这个
动作，和"切换协议 = 重新开始"（下一条）是同一套语义。

`seedBody`（"Copy the preset request"用到的值）必须在**两个触发点都准确**：门那
一侧好办（此刻确实没有自定义请求，Payload 面板显示的就是预设请求）；但已经进了
自定义视图之后，Payload 面板显示的是**当前的自定义请求**，不能再拿来当"预设请求
现在的样子"——那样"Copy the preset request"会变成把自己抄一遍的空操作。所以
`BenchPage.tsx` 维护了第二条独立的、`raw:null` 构造的 curl 请求（`presetPreviewCurl`），
只在自定义请求处于活跃状态时才发起，预设请求模式下直接复用已有的 curl 结果，不重复
请求。

- **切换协议 = 重新开始，不是重新贴标签**：协议下拉的 onChange 把 body 换成新
  协议的起始模板，而不是只改 `request_protocol` 这个字段——旧实现只改标签、
  不改 body，会产出一份标签和内容对不上的请求，这是已修的 bug，不是特性。
- Tool / Vision / Thinking / Protocol 这几个轴在自定义请求模式下**不渲染**，不是
  变灰禁用——它们不是这个视图的旋钮，压根不适用（`BenchAxes` 的 `rawMode` 直接
  跳过这几个 `AxisGroup`，不是给它们传 `disabled`）。Shape（Stream）/ Scope /
  Routing 保持可见：Scope/Routing 是传输配置，Stream 是 body 里的真参数，三者的
  共同点只是"和内容无关"，预设请求和自定义请求两种模式都用得到（§1"四种归类"）。
- 切换 target 时清空自定义请求（它是针对旧 target 协议写的）。

---

## 7. Payload 面板：可看，也可改

- 两个 tab：**Request**（method + URL + headers 表 + pretty body，秘密仍以
  `$TB_API_KEY` / `$UPSTREAM_API_KEY` 占位）与 **cURL**（现有 `command`）。同一响应
  （`probe.CurlData`）渲染两种视图，无新端点。
- 配置变更 → debounce 500ms → `POST /api/v2/probe/curl`。构造失败（如校验错）时面板
  显示错误原因——**payload 面板同时兼任"配置是否合法"的即时反馈**。
- 每块可复制；cURL caption 保留 key 替换提示。Through-TB 的 cURL 现在携带探测头
  （`X-Tingly-Probe-Rule` / `X-Tingly-Probe-Service` / `X-Tingly-Probe-Flags`），否则
  复制出来手动执行会按普通流量路由，复现不了这次探测。

### 7.1 Header 覆盖；body 的"编辑"就是进入 raw 模式

只读的 payload 意味着"能改的都得先变成旋钮"，对一个叫"工作台"的页面这是真实缺口。
两类可改的东西走两条路，不做第三种机制：

- **Body**：点 Edit 不是打开一个 diff/覆盖层，而是把当前 body 原样搬进中栏 Request 面板
  进入 raw 模式（§6.3）。早期草案里"顶层 key diff → `body_overrides` → sjson 重写"的
  设计被撤掉：既然整份请求都能写，一个"改几个 key"的第二套机制只是重复，且它改的是
  SDK 序列化之后的 body，与 Request 面板里"你写的就是发出的"这条原则打架。
- **Header**：`headers: {name: value}`，空值 = 移除。后端由 ctx 携带的
  `probeHeaderOverridesRoundTripper`（探测客户端 transport 最内层）在请求离开进程前应用；
  `/probe/curl` 对同一份 header 表应用等价镜像（`applyCurlHeaderOverrides`），面板与实际
  请求不可能不一致。UI 上被覆盖的 header 行高亮、可单个 ✕ 或整体 Reset；切换 target 时
  清空。
- 校验：header 名不能含空白或冒号。带 raw 请求 / flags / header 覆盖的探测不进 endpoint
  能力缓存（`E2ERequest.Customized()`）。

## 8. Result 与 Run history

- 结果区复用 probe dialog 的四件套：StatusBar → Journey → Response → Raw JSON。
  从 `ProbeDialog.tsx` 提炼为共享组件（如 `components/probe/ResultSections.tsx`），
  dialog 与 bench 共用——避免两处维护 journey 字段映射。
- Bench 差异：Journey **默认展开**；Flags 行在 overlay 生效时并列展示
  `AppliedFlags`（权威）与 overlay 请求值，不一致处即为教育点（§5.4）。
- **Run history**（session 级）：结果区顶部一行 chips，最近 ~10 次 run：
  `✅ 850ms · stream · 2 flags` 。点击回看该次结果，且左栏/request 恢复为该次的
  请求配置（靠 Result 的 request-echo 字段 + 本地保存的请求快照）——"完成 ≠ 锁死"
  （ux-principles #10），对照两次实验是工作台的日常动作。不落库、刷新即清（V1）。

---

## 9. 导航与路由

- 一级 activity rail 项：`key: 'bench'`，`path: '/bench'`，icon 走
  `@/components/icons`（tabler `IconTestPipe` 经 `tablerMui()` 适配；`IconFlask` 已被
  VModel 用掉，避免视觉撞车）。位置放 Usage 之后——它与 Dashboard 同属"观察与验证"域。
- 页面 `React.lazy(() => import('./pages/bench/BenchPage'))`（frontend/CLAUDE.md
  的 code-splitting 铁律）；**不从 page 文件导出任何共享状态**——picker 数据、共享轴原语
  都放独立模块。
- i18n：`en.ts` / `zh.ts` 增 `bench.*` 命名空间；复用 `probe.*` 已有的轴文案
  （同一概念同一词）。

---

## 10. 状态持久化

| 内容 | 存储 | 理由 |
|------|------|------|
| target + axes + flags overlay + raw request + headers | `localStorage: tb.bench.state` | Bench 是**工作台**：离开再回来续着做是预期。这与 probe dialog 的"轴不持久化"决策**刻意相反**——probe 是诊断，默认必须可预测（probeConfig.ts 注释）；工作台的可预测性由"回来时和离开时一样"定义。两条决策各自成立，勿互相"统一" |
| URL `?target=` 深链 | — | 优先于 localStorage（显式意图 > 记忆） |
| Run history | 内存（session） | 见 §8；落库是 V2 的事 |

---

## 11. 后端改动清单（已实现）

前置 PR（"accept a raw client request in any of the three protocols"）：`E2ERequest.Request` /
`RequestProtocol`、`parseRawRequest`、三个 builder 的短路、校验（协议一致、与 fixture 轴互斥）、
`request_shaping_test.go`。本页在其上叠加：

| # | 位置 | 改动 |
|---|------|------|
| 1 | `internal/probe/types.go` | `E2ERequest` 增 `Flags typ.FlagOverlay`、`Headers map[string]string`、`Routing`（natural 默认 / pinned → `X-Tingly-Probe-Rule`）；`ValidateE2ERequest` 扩展（flags 按 registry 校验、direct×flags 互斥、header 名合法、routing 取值）；`Customized()` = raw request ∨ flags ∨ headers，让定制请求绕过能力缓存 |
| 2 | `internal/typ/flag_overlay.go` | `FlagOverlay`、`ValidateFlagOverlay`（含 `multi_enum`）、`ApplyFlagOverlay`（经 JSON 形态合并，显式零值可以清掉已开的 flag）、`ProbeFlagsHeader` 及 base64url 编解码。registry 仍是唯一可信源 |
| 3 | `internal/probe/e2e_probe.go` | 回环路径把 `Flags` 编进 `X-Tingly-Probe-Flags`；pinned 时 rule target 发 `X-Tingly-Probe-Rule`；`Headers` 经 ctx 交给 header round tripper（最内层）；capture 客户端同样套用 |
| 4 | `internal/client/probe_rewrite.go` | `WithProbeHeaderOverrides` / `probeHeaderOverridesRoundTripper` / `ApplyHeaderOverrides`：请求离开进程前设/删 header；只做 header，不碰 body |
| 5 | `internal/protocolserver/rule_flags.go` | `ResolveRuleFlagsWithScenario` 在 scenario 继承后、自动项/OAuth 抑制前应用 header overlay（`applyProbeFlagOverlay`；解码失败记 warn 并忽略） |
| 6 | `internal/probe/curl.go` | 应用 header 覆盖；Through-TB 的 curl 带探测头 |
| 7 | swagger / codegen | `openapi.json` 与前端 `schema.d.ts` 已重新生成 |
| 8 | 测试 | `typ/flag_overlay_test.go`、`client/probe_rewrite_test.go`、`probe/bench_test.go`（校验 + routing + Customized + curl header 覆盖）、`protocolserver/rule_flags_overlay_test.go` |

## 12. 前端改动清单（已实现）

| # | 位置 | 改动 |
|---|------|------|
| 1 | `pages/bench/BenchPage.tsx` | 页面（lazy），三栏布局编排、run history、⌘/Ctrl+Enter、localStorage 持久化、深链消费 |
| 2 | `pages/bench/benchLink.ts` | URL 契约（`?target=rule:{uuid}&scenario=` / `?target=provider:{uuid}&model=` + 轴参数）；独立小模块，供 ProbeDialog 的 "Open in Bench" 使用而不拖入页面 chunk |
| 3 | `pages/bench/benchState.ts` | 状态模型（含 `raw: {protocol, body}`）、`parseRawBody`、`buildProbeRequest`（Run 与 payload 面板共用的唯一请求构造；raw 模式下产出 `request`/`request_protocol` 并丢弃 fixture 轴）、run 标签 |
| 4 | `components/probe/AxisPrimitives.tsx` / `ResultSections.tsx` | 从 ProbeControls / ProbeDialog 提炼的共享原语（Axis、`AxisGroup`——Parameters/Content 分组，与 PluginsPanel 同一套 overline+分割线样式、ExclusiveToggle、ThinkingSlider；StatusBar、Journey、CollapsibleSection、CopyBlock）。Journey 增 `showFlags` / `flagsExtra`。`ProbeControls` 与 `BenchAxes` 都改用 `AxisGroup` 按"四种归类"（§1）分组，不再是一个扁平列表 |
| 5 | `pages/bench/` 内部组件 | `TargetPicker`（统一目标选择）、`BenchAxes`（全展开轴、Parameters/Content 分组，自定义请求模式下归属该模式的 `AxisGroup` 整块不渲染而非禁用）、`PluginsPanel`（registry-driven 三态）、`RequestEditor`（预设/自定义两态，`StartingPointMenu` 统一"从哪开始"——门与"Change starting point"共用同一份菜单，见 §6.3）、`PayloadPanel`（只读 body + Edit→自定义、header 覆盖）、`RunHistory` |
| 6 | `App.tsx` / `layout/useActivityItems.tsx` / `components/icons` | lazy route、rail 项（Usage 之后）、`TestPipe` 图标 |
| 7 | `services/api.ts` | `getAllRules`（不带 scenario 即全部规则） |
| 8 | i18n | `bench.*` en/zh；`probe.openInBench`；`layout.bench` |
| 9 | 入口 | ProbeDialog 标题栏 "Open in Bench"（带 target + 轴 + message） |
| 10 | 测试 | `pages/bench/bench.test.ts`（深链往返、请求构造、raw 请求解析与互斥） |

## 13. 分期交付

原计划三个阶段一次落地（V1 已含全部三段）。当时的划分保留作参考：

1. **骨架页**（纯前端 + 现有 API）：三栏布局、target picker、全展开轴、单 message、
   payload 实时面板、result、run history。Plugins 区已渲染（inherited 只读展示），
   overlay 控件禁用 + hint "coming"。
2. **Flags overlay**（后端 #1–#4 + 前端 FlagOverlayPanel 激活 + codegen）——核心价值落地。
3. **Raw request**（前置 PR 的 `request`/`request_protocol` + 前端 RequestEditor + templates）。

每阶段用 `ui-preview` skill 截图验收布局。

---

## 14. 设计取舍

| 选项 | 已采纳 | 备择 | 取舍理由 |
|------|--------|------|----------|
| 独立一级页 vs 扩展 probe dialog | 一级页 | dialog 加 tab / "expert mode" | dialog 的价值是就地、轻、快；塞入 flags+raw request 会毁掉它的诊断心智，也放不下三栏。两者共享后端与结果组件，成本可控 |
| flags 传输 | 请求体 `flags` → probe header 中继 | loopback 请求体内嵌带外字段 | 与 `X-Tingly-Probe-Service/Rule` 同族同路径；请求体必须保持纯净的协议形态（它就是被测对象） |
| overlay（出现即覆盖） vs 全量替换 | overlay | UI 算好效果集整体替换 | 替换要求前端复刻 scenario 继承（or/override）与自动项逻辑——双实现必然漂移。overlay 让后端解析仍是唯一权威，前端只声明"我动了哪几个" |
| overlay 应用位置 | scenario 继承后、自动项/抑制前 | 最末尾（全覆盖） | 自动项与 OAuth 抑制是物理/计费约束，实验覆盖它们产出的是不可能存在于生产的假结果；差异经 AppliedFlags 回显，本身就是教育 |
| Direct × flags | 互斥（400 + UI 禁用） | 静默忽略 | flags 是 TB middleware 行为；静默忽略会让用户以为测了实际没测——最坏的一种假成功 |
| flags key 集合 | `map[string]json.RawMessage` + registry 校验 | typed `RuleFlags` 指针字段 | typed struct 无法区分"未设"与"设为零值"，而这正是 overlay 的核心语义；registry 校验保住类型安全 |
| 请求载体 | 三种协议的原始请求体（SDK decoder 解析） | 自定义 `ProbeMessage{role,text}` / 结构化多轮编辑器 | 自造载体表达不了 tools/图片/content block，还要在三个 builder 各写映射；协议本身就是最完整的载体，TB 本来就会解三种形态。编辑器换成 JSON 文本域 + 模板，复杂度反而更低 |
| raw request 与 fixture 轴的关系 | 互斥：raw 时 Tool/Vision/Thinking/message 归请求 | 轴 fixture 叠加在 raw 请求上 | 叠加意味着后端要往用户写的请求里注入 tools/图片——"你写的就是发出的"被破坏，且注入规则本身又是一套要维护的映射 |
| body 细粒度修改 | Edit → 进入 raw 模式 | 顶层 key diff → `body_overrides` sjson 重写 | 整份都能写时，"改几个 key"是重复机制；且它改的是序列化后的 body，与 Request 面板的单一来源打架。header 覆盖保留，因为 header 不属于请求体 |
| 配置持久化 | localStorage 持久化 | 跟随 probe 的不持久化 | 诊断要默认可预测，工作台要延续上下文——两个页面的正确答案相反，显式写下避免"统一"冲动 |
| provider_config target | 不纳入 | 三种 target 全支持 | 它属于 Connect AI 的"保存前验证"流程；Bench 面向已保存对象，多一种 target 只添心智噪音 |
| Run history | session 内存 | 落库持久化 | 先验证"回看/对照"是不是真实高频动作，再决定值不值得一张表 |

---

## 15. 未做 / 后续可做

- **Variant 对比运行（matrix run）**：同一配置一键跑 A/B——flag on vs off、Through-TB vs
  Direct、两个 provider——结果并排 diff（payload diff + response diff）。这是"对照实验"
  的完全体，也是 bench 数据模型（配置快照 + run 结果）天然支持的方向。
- **字节级 raw 模式**：原样发送任意 body（非 JSON、跨协议错投、故意写坏的字段），绕开
  SDK decoder；结果退化为原始文本 / SSE dump。当前 raw request 走 SDK 解析，所以能测的是
  "合法请求的转换"，测不了"TB 对畸形输入的 400"。
- 从 recording 一键载入 raw request（录制 → Bench 重放）。
- Google 协议的 raw request（目前 provider 若只说 google，raw 模式不可用）。
- Run history 落库前先补一个"配置来自哪次 run"的可视标记（当前只高亮 chip）。
- 命名预设（保存/分享一份 bench 配置；配合 URL 深链导出）。
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
| 3 | 命名唯一 | Bench / Plugins / Probe 各指一物；轴词汇与 probe 完全共用 |
| 4 | 正交维度分轴 | 轴 × flags overlay × request 三个正交面板；轴内沿用 probe 拆轴成果；raw 模式下归请求的轴明确禁用而非静默忽略 |
| 5 | 展示具体值 | inherited flag 显示解析后实际值；payload 展示真实 body；AppliedFlags 回显权威生效值 |
| 6 | 聪明默认 | 轴默认沿用 probe（Stream/Through-TB/primary protocol）；flags 默认全 inherited；request 默认 fixture |
| 7 | 诊断走真实链路 | 一切默认 loopback 生产路径；Direct 仅作对照且与 flags 互斥 |
| 8 | 教育内嵌 | 按协议的 request templates 即教材；overlay 与 AppliedFlags 的差异展示约束本身 |
| 9 | 降视觉噪声 | 主角是 payload 与 result；flags 未覆盖时 muted；journey 字段仍是等宽极简行 |
| 10 | 完成 ≠ 锁死 | run history 回看并恢复配置；页面状态持久化，随时回来续做 |
| 11 | 交付下一步物件 | cURL/payload 逐块可复制；probe→bench 深链带配置；（V2）导出 harness case |
| 12 | 副作用限定当前表面 | overlay 不落库、不触碰任何 rule/scenario 配置；run 不进 usage 统计之外的任何状态 |
