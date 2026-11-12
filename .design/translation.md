# 统一 /translate：交集核心、并集扩展与首版契约

Status: Prototype implemented：通用 LLM、原生 MT、LibreTranslate / DeepL 后端；并集扩展仍为 Proposed。
Date: 2026-11-12（Asia/Shanghai）。
Tracking: [TEP-2254](https://github.com/tingly-dev/tingly-box/issues/2254)。
Evidence: [接口调研](../.research/translation-apis.md)。

## 1. 已确认与待决事项

已确认：以 `/translate` 为统一入口，接入自部署混元 MT、千问翻译及在线翻译服务；调研在 `.research/`，设计在 `.design/`。模型部署由用户在网关之外完成。

首版采用第 4 节的固定数组契约，默认路由别名 `translate`。后端统一实现 `Translator`：通用 LLM 不需要 catalog 登记；catalog 只选择混元、Qwen-MT 的特殊语义。传统服务直接使用原生 HTTP API，不包装成 Chat、不伪造上游模型。LB Service、规则及 provider 字段结构保持不变；沿既有 `api_style` 增加 `libretranslate` / `deepl` 协议值。

Python SDK 示例集中于 [示例目录](../sdk/python/examples/translation/README.md)，通过正常 tb 和管理 API 配置，网关新端口 12680；模型示例端口 18681 / 18683。当前后端设计以第 12 节为准，第 11 节保留第一次收敛的历史。

当前支持普通 API-key/no-key 的 Chat / Responses / Anthropic 模型路径、native MT 的 OpenAI serving 路径和两种传统 HTTP 服务。smart-routing 分区、Google-native、OAuth、多字段凭据、vmodel、HTML/术语/流式/文件及新 UI 仍未支持。32 项、单项 32 KiB、总 body 256 KiB。详见 [后端实测记录](../.research/translation-backends-smoke-test.md)。

## 2. 三层结构

1. **客户端入口**：Tingly `/translate`，以及有明确需要时的 DeepL、LibreTranslate、沉浸式翻译兼容入口。
2. **翻译操作**：标准化输入、语言、控制和结果；在这里做能力匹配、批量编排和配置允许的 failover。
3. **后端适配**：Qwen 原生参数、对应版本的 Hunyuan 模板、在线 HTTP API 的 envelope、凭据、usage 和错误。

复用现有鉴权、team scope、配置和可用的路由底座。专用翻译不必经过通用 chat 消息链；需要按后端重新构造原生请求。模板与 serving transport 是两个轴，不根据部署别名猜模型家族。能力归 catalog，原生请求适配归 `internal/protocol/translation`，连接与鉴权仍由现有 client/forwarding 承担。

遵循 [protocol/client 边界](protocol-client-boundary.md)：纯语义和类型转换与 URL、鉴权、连接、代理、超时分开。当前 [模型路由](../internal/protocolserver/routes.go) 和 [鉴权范围](../internal/middleware/auth.go) 是实现时必须保持的边界。

## 3. 最大交：共同语义骨架

以下是**概念表达**，不作为第二套对外 API。前提是已经选定支持目标语言对的服务；源语言信息按该服务要求解析，不隐含所有服务都支持自动识别。

```json
{
  "text": "Hello",
  "target_lang": "zh-Hans"
}
```

```json
{
  "text": "你好"
}
```

语义为 `Translate(text, target, resolved source information) → translated text`。若后端需要明确源语言，就必须先取得请求/配置中的值，无法解析则拒绝。

这个共同核心不包括 batch、源语种报告、置信度、术语、HTML、文件、流式和异步。也没有共同的原生字段名可直接复用。

## 4. 推荐对外 v1：交集核心 + 网关的批量编排

`POST /translate`，路径相对于已配置且已鉴权的模型服务 base URL。
实际挂载沿用 `/tingly/:scenario/translate` 和 `/tingly/:scenario/v1/translate`；不要另开绕过 team scope 的根入口。

### 请求

```json
{
  "model": "translate",
  "text": ["Hello, world!", "Save changes"],
  "source_lang": "auto",
  "target_lang": "zh-Hans"
}
```

| 字段 | 建议契约 |
| --- | --- |
| `text` | 必填、非空 `string[]`；单条也用数组；空字符串项拒绝；空白、换行与项边界保持 |
| `target_lang` | 必填，一个 BCP 47 语言标识 |
| `source_lang` | 可选；省略等价于 `auto`；仅路由到已确认可以自动处理源语言的服务；指定源语言时不承诺每个模型都能强制约束其判断 |
| `model` | 可选，Tingly 逻辑路由名；建议默认 `translate`；未配置则明确报错，不随意挑选模型 |

JSON UTF-8、纯文本、单目标、同步完整响应。暂不接受 `messages`、上游地址、密钥、温度或任意透传 body；这些不属于基础翻译意图。未知字段明确报错。

批量是网关承诺：原生支持有序数组时直接适配，否则有界逐项执行。它不是所有后端的原生交集。先不把独立项拼成一个带分隔符的长提示，再猜测输出边界。

### 响应

```json
{
  "model": "translate",
  "translations": [
    {"text": "你好，世界！"},
    {"text": "保存更改"}
  ]
}
```

成功时结果数量等于输入数量、顺序一致，每项文本必须完整。服务确实返回源语种时，才附加该项 `detected_source_language`。不制造识别结果或置信度。`model` 回显逻辑路由名，不暴露凭据或内部 provider 信息。

### 为什么取这个形状

借鉴 DeepL 的固定数组结构，降低调用方分支数量，并与 Google 的批量语义一致。Tingly 保持自己的 BCP 47、鉴权和路由定义，不宣称完全兼容 DeepL。LibreTranslate 的 q/translatedText 和沉浸式翻译的 text_list 可以在兼容入口做薄转换。

语言标识合法与后端支持该语言对分别检查；保留文字和地域区别。不得为了路由成功把繁体目标变成简体。客户端历史别名只能通过有依据的兼容映射处理。

## 5. 最大并：有约束的可扩展操作模型

“最大并”描述调研范围内所有能力的表达空间。它不表示任意服务支持全部字段，也不表示任意选项组合都成立。

### 概念类型结构

```text
TranslationOperation {
  model?: logical route alias
  input: TextInput | DocumentInput | ImageInput | AudioInput
  languages: { source: language | auto, targets: language[] }
  controls?: TranslationControls
  delivery: { execution: sync | async, response: complete | stream }
}

TextInput       = { kind: text, items: [{ id, text }], format: text | html | xml | markdown | json }
DocumentInput   = { kind: document, artifact_id, media_type }
ImageInput      = { kind: image, artifact_id, media_type }
AudioInput      = { kind: audio, artifact_id, media_type }

TranslationControls {
  context?: untranslated background
  terms?: [{ source, target }]
  glossary_ref?: gateway-managed glossary reference
  translation_memory?: [{ source, target }]
  domain_hint?: domain description
  style?: { formality?, instructions? }
  preserve?: { placeholders?, do_not_translate?, structural_validation? }
  alternatives?: requested count
  annotations?: { alignment?, sentence_boundaries?, transliteration? }
}
```

`format`、`style` 等列举的是候选表达维度。某值存在于这个类型空间不代表已证明支持。音译、参考句、风格指令不能靠字段名相似就等同转换。文档/图片/音频先用带授权的 artifact 引用表达，不提前承诺任意 URL 抓取。

### 并集示意请求

以下用于讨论扩展结构，**不能发送给 v1**。示例只组合部分能力；实际执行前仍需匹配同一个后端或明确的编排实现。

```json
{
  "model": "translate",
  "input": {
    "kind": "text",
    "items": [{"id": "p1", "text": "Hello, {{name}}!"}],
    "format": "text"
  },
  "languages": {"source": "en", "targets": ["zh-Hans", "ja"]},
  "controls": {
    "context": "Greeting in an application",
    "preserve": {"placeholders": ["{{name}}"]}
  },
  "delivery": {"execution": "sync", "response": "complete"}
}
```

这里的上下文和占位符可表达共享意图。词对等语言相关控制还需按语言对绑定，或拒绝歧义组合；上例没有混入只适用于某个目标的词表。

结果模型按输入 ID 与目标语言二维对应：文本结果含 text，可选识别/候选/对齐；文件结果含授权 artifact 引用。异步提交返回 job ID，之后查询结果；流式返回带输入/目标 ID 的事件。同步与异步是执行轴，完整与流式是响应轴，二者分别约束。

文档、图片、音频和任务 API 应在各自生命周期被设计后发布，避免把文件状态、生成 chunk 和普通文本结果塞进一个含义模糊的响应。

### 扩展控制的语义边界

| 控制 | 必须保留的差别 |
| --- | --- |
| 内联词对 / 术语库 | 前者是请求数据，后者是资源引用；外部 provider ID 不能跨服务通用 |
| 背景上下文 / 待翻译文本 | context 不应出现在译文中，也不自动把其他 batch 项视为上下文 |
| 翻译记忆 / 词表 | 句子级参考与术语替换不是同一能力 |
| 领域 / 风格 | domain hint、定制模型类别、formal 等参数不天然等价 |
| 文本 / 标记 / 文件 | 文件格式支持不自动意味着支持 HTML 片段或严格 JSON 模式 |
| 模型流 / 稳定译文 | 累积快照可能修订此前内容；不能总是转为追加 delta |

对多目标语言，后续可采用按目标绑定 controls 的结构；在该问题解决前，对外 v1 坚持一个目标语言。

## 6. 能力交集、并集和匹配

```text
C_safe      = intersection(effective capabilities of eligible services)
C_available = union(effective capabilities of eligible services)
eligible(request) = services that satisfy every request constraint
```

`C_available` 中存在两个特性，不意味着任一服务能同时完成它们。能力描述须包含语言对、输入类型、选项组合、长度限制、模型版本及证据。

建议每个能力标为 `native`、`gateway`、`unsupported` 或 `unknown`。前两种分别表示原生实现与已验证的网关编排；未知和明确不支持在诊断中区分，但都不能满足请求的必需能力。提示词表达另外记录 `mechanism=prompt` 和输出验证要求，不伪装成强保证。

举例：服务 A 支持术语库，服务 B 支持多目标，二者并集不是“术语库 + 多目标”可执行承诺；可以由明确实现的按目标拆分补齐，否则拒绝。

能力的示意描述（不是已发布查询端点）：

```json
{
  "capabilities": {
    "text_translation": "native",
    "ordered_batch": "gateway",
    "auto_source": "native",
    "reported_source_language": "unknown",
    "html": "unsupported"
  },
  "constraints": {
    "input_formats": ["text"],
    "language_pairs": [{"source": "en", "target": "zh-Hans"}]
  }
}
```

route 级展示：公共保证来自有效服务的交集；可选择能力来自并集并明确约束。实际请求先筛选兼容服务，再负载选择；failover 后重新做原生适配。不得静默丢弃 terms、简繁体或格式要求。

## 7. 批量、错误和 usage

- 首版成功响应始终完整。任一项失败时返回非 2xx 和已知的失败项索引；不返回缩短的成功数组。
- 上游调用不具备事务性：失败前可能已消费额度。保留已完成项，重试/failover 不重放它们；不宣称撤销、exactly-once 或通用幂等。客户端重新提交整个请求可能再次计费。
- 建议错误 envelope：`error: { type, code, message, input_index? }`。索引从 0 开始。
- 400：输入或语言/选项错误；413：网关请求上限；401/403：调用方鉴权；429：限流/配额；502：上游鉴权、异常或不完整结果；503：暂时无可用服务；504：超时。更细分类由 code 表达。合法请求但无兼容服务时给明确 code，不与瞬时故障混淆。
- 模型拒答、截断、工具调用、空结果、原生业务失败和数量不匹配，都不能包装成译文成功。上游状态/错误码以去敏诊断保留。
- 不承诺自动分割一条超长文本。长度校验按字节、字符或模型 token 分开；网关和后端限制分别表达。
- usage 按实际尝试和实际来源记账，分别记录 token 与 billed characters。跨模型重试的不同 tokenizer 数据不当作同一模型的可比测量。缺失记未知，不记实测零。
- 配置允许的本地/在线 fallback 才执行；增加在线 provider 不等于自动允许本地规则外发。

## 8. 实施顺序和验证

1. 讨论并确定对外 v1、小型内部操作类型、能力/错误语义、默认路由配置。
2. 首个完整切片覆盖自部署 Hunyuan/HY-MT、Qwen-MT，以及 LibreTranslate 或 DeepL 的非 chat 服务。
3. 验证同一调用契约切换后端、team 鉴权、有序批量、配置允许的 failover 和取消。
4. 按实际客户端需求增加兼容入口；再按独立设计扩展 controls、流式和文件任务。

首版检查包含原生 MT 请求、鉴权/team 隔离、语言边界、批量、逐项 failover、不完整输出和取消。通过 Python Transformers 加载固定版本的 Hy-MT2-1.8B，并完成真实 `/translate` 往返。Qwen-MT 仅做 mock 协议检查，未调用真实云账号。流式/文件等后续功能仍需对应验证；这次真实模型 smoke check 不构成质量 benchmark。

## 9. UX 原则检查

- 入口直接完成“选路由 → 输入原文/目标 → 得到译文”，不给用户先选底层 wire protocol。
- model 路由、部署模型、鉴权和翻译能力分别命名、配置；默认值不掩盖缺失配置。
- 给可复制的完整示例和真实失败原因；诊断走实际翻译路径。
- 高级能力按需展开、保留再次执行入口；扩展效果限定于当前请求。

## 10. 待决事项

- 实验反馈后冻结 v1 的 `text[]`、默认别名及能力查询呈现。
- 供给侧部署别名→模型族绑定尚未实现；需要沿 provider/model 扩展设计补齐，而不是扩展 LB。smart-routing 分区能力过滤仍待实现。
- 多目标下 terms、context 和术语资源的语言对绑定；默认不做有损降级。
- 文件任务的资源授权、生命周期和结果保存，以及流式快照/修订事件契约。
- 上线前确定实际 byte/item/token 限制；文件中没有借用某一服务的限额作为全网关标准。

## 11. 实现收敛（2026-11-12）

撤销早期 `loadbalance.Service.TranslationBackend` 与独立 Go demo gateway。
模型能力归 catalog；协议形状归 `internal/protocol/translation`；选路、连接、
凭据与 failover 复用现有组件。示例仅存于 SDK，并通过管理 API 配置正常 tb。
不增加第二套服务启动/鉴权链，也不把未实施的 provider extensions 当成现成机制。

当前模型 ID 采用 catalog 精确匹配。公共 `model` 路由别名仍可自由配置；
上游部署别名→模型族绑定暂不支持，未知模型在调用前拒绝。这个限制明确替代
早期任意部署别名的实验承诺，后续应在供给侧模型扩展中解决。

## 12. 后端接口与实现（当前）

`Translator` 绑定一次路由选中的后端，暴露 `Supports(source, target)` 与
`Translate(ctx, TextInput) (TextOutput, error)`。TextInput 只有原文和语言；
TextOutput 包含译文、可选的实际检测结果/字符计费及可空 token 用量。
批量、选路、逐项 failover、鉴权/team scope 在接口之外；每次回退重新选择实现。

- 通用 LLM：构造翻译指令与独立 user 文本，复用现有 attempt plan、transform、
  输出限制、协议转换、池化 client 与 forwarding；无需逐模型登记，不运行 agent 工具循环。
- 原生 MT：catalog 选择特殊模板/translation_options。避免通用 chat 变换注入不支持的
  system 消息；调用仍走原有模型 client/forwarding。未登记的部署别名按普通 LLM 处理，
  不猜测 MT 家族；需要原生语义时必须使用精确 catalog ID。
- 传统服务：`api_style=libretranslate|deepl` 声明原生 wire 协议；连接、代理、规则
  headers、超时复用现有传输设施。服务无上游模型，现有 service.model 可为空，HTTP
  请求里不携带 model。API key 来自 provider，LibreTranslate 放 body，DeepL 用专用头。

没有新增 TranslationBackend LB 字段，也不引入第二套 credential、路由或 agent 系统。
角色分离仍是 catalog 管特殊模型能力、protocol 管语义、client 管传输。
传统服务可配置在 T0、LLM 在 T1，回退只使用明确配置且语种兼容的候选。

实际 token 可空；供应商若报告 billed_characters，则作为响应的可选独立字段保留，
不把本地字符长度冒充计费量或换成 token。现有 usage DB 仍按模型 token 存储，
未实现传统服务的持久化计费和成本计算，不能宣称成本统计已经统一。

检测语言仅在服务实际报告时返回并规范化为 BCP 47。传统 HTTP 业务失败、非 2xx、
缺失/数量不符/空输出均报错；错误不透出可能回显密钥的 vendor body，禁止跟随重定向。
通用 LLM 支持合法语言标识不等于保证该模型掌握所有语言；质量、格式与抗注入能力
仍需另行验证，拒答/工具调用/截断不能包装成成功。

## 13. 与 Vision 对照后的待收敛点（尚未实施）

源码事实与限制见 [结构对照](../.research/translation-vision-architecture.md)。
本节为讨论方向，不改变第 12 节描述的当前代码与实测范围。

- 保留 backend-neutral Translator，但让实现不再捕获 gin.Context。HTTP handler
  保留解析、鉴权、规则选择、batch/failover 和写回；能力调用以普通 context 与
  类型化输入/输出表达，由启动装配或明确工厂注入依赖。
- 参照 visionproxy 的模型能力调用：翻译指令与译文校验属于翻译服务，模型调用复用
  pool、已有协议转换和 transport。需要先识别可提取的非 HTTP 调用入口，避免在
  translation handler 继续扩张多协议分发，也不要默认继承完整聊天/agent 策略。
- 参照 imagegen 的特殊适配：普通模型走已有设施，native MT / traditional 服务
  仅封装真实差异。能力模块保持叶子依赖；不能让 client 与能力服务相互 import。
- 单独审视供给侧协议选择：APIStyle 用于整体 wire 协议还是特定操作差异，需按
  provider 的真实接口决定。保留任意自部署/反代可显式声明的路径，不用 host 猜测
  取代配置，也不引入新的 LB 字段或泛化配置袋。
- 不在这次讨论中创建跨 vision/translation 的万能能力框架，不迁移已有 vision。
  翻译的完整性与未知用量语义继续保留，不能照搬视觉代理的降级策略。
