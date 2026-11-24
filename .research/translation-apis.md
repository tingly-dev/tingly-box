# 翻译接口调研：定义、差异与能力交并集

首次调研：2026-10-25。整理及复核：2026-11-12（Asia/Shanghai）。

关联：[TEP-2254](https://github.com/tingly-dev/tingly-box/issues/2254)。设计见 [translation.md](../.design/translation.md)。

## 1. 范围和证据等级

目标是连接已经运行的自部署翻译模型、托管翻译模型和在线机器翻译 API。模型下载、GPU 部署和翻译质量排名不在本次范围。

- **文档定义**：官方接口或源码明确描述的字段/行为；不等于已调用成功。
- **模板示例**：官方建议通过提示词表达的意图；不等于确定性的结构或术语遵循保证。
- **网关可编排**：通过拆分请求、逐项调用等提供的能力；不属于后端原生能力。
- **未知**：资料不完整、依赖部署或未验证组合。路由不能把未知当作支持。

接口对照阶段没有部署模型或使用付费 API。2026-11-12 后续完成的腾讯本地模型实测另见 [smoke test](translation-smoke-test.md)，不改变本篇的文档证据等级。下列主要页面已复核；TokenHub 的完整正文仍未取得，明确保留该证据缺口。

## 2. 来源

| 编号 | 官方来源 | 本记录采用的范围 |
| --- | --- | --- |
| S1 | [DeepL Translate Text](https://developers.deepl.com/api-reference/translate/request-translation) | 文本输入、结果、上下文、术语库、标签处理 |
| S2 | [LibreTranslate Translate](https://docs.libretranslate.com/api/operations/translate/) | 标量/数组、源语言、格式、识别和候选译文 |
| S3 | [Google Basic v2](https://docs.cloud.google.com/translate/docs/reference/rest/v2/translate) | 请求和响应定义、HTML 默认值 |
| S4 | [Google Translating Text](https://docs.cloud.google.com/translate/docs/translate-text) | Advanced 文本数组、HTML/XML 边界 |
| S5 | [Azure Translator v3 Translate](https://learn.microsoft.com/en-us/azure/ai-services/translator/text-translation/reference/v3/translate) | 多文本、多目标、识别、音译和对齐 |
| S6 | [Qwen-MT API](https://www.alibabacloud.com/help/en/model-studio/qwen-mt-api) | user 消息、translation_options、词对、参考句、流式；复核页面更新于 2026-10-27 |
| S7 | [Qwen-MT-Uni API](https://help.aliyun.com/en/model-studio/qwen-mt-uni-api) | 独立 DashScope 接口、文本/文件、任务状态和业务结果 |
| S8 | [Hy-MT2 README](https://github.com/Tencent-Hunyuan/Hy-MT2/blob/main/README.md) | 自部署推理入口和版本对应的翻译模板 |
| S9 | [TokenHub 混元调用指南](https://cloud.tencent.com/document/product/1823/132252) | 官方检索结果中的 Chat Completions 与独立 translations 入口；完整契约待核实 |
| S10 | [旧 ChatTranslations](https://cloud.tencent.com/document/product/1729/113395) | 历史专用翻译协议和预下线状态 |
| S11 | [沉浸式翻译自定义接口](https://immersivetranslate.com/zh-Hans/docs/services/custom/) | 客户端 text_list 协议、占位符和语言编码 |
| S12 | [RFC 5646 / BCP 47](https://www.rfc-editor.org/rfc/rfc5646.html) | 语言、文字和地域标识 |
| S13 | [Browser Translator API](https://webmachinelearning.github.io/translation-api/) / [ITS 2.0](https://www.w3.org/TR/its/) | 2026-10-25 查阅的标准背景，未用于认定服务器 HTTP 契约 |
| S14 | [DeepL Usage and Limits](https://developers.deepl.com/docs/resources/usage-limits) | 2026-10-25 查阅的字符计量说明；配额上线前重新查询 |

引用指向活文档。实现时应固定 SDK/模型版本，并将原生请求、响应及错误保存为去敏 fixture；不要把本记录日期当成上游永久承诺。

## 3. 原生接口结构

以下仅摘录结构所需字段；鉴权材料由配置提供，示例不包含密钥。

| 接口 | 请求结构 | 返回结构 | 差异 |
| --- | --- | --- | --- |
| DeepL / S1 | `POST /v2/translate`；`text: string[]`、`source_lang?`、`target_lang` | `translations[]`，每项有 `text`，可含识别语言 | 固定数组；输入独立，顺序对应；省略源语言时识别 |
| LibreTranslate / S2 | `POST /translate`；`q: string 或 string[]`、`source`、`target`、`format` | `translatedText: string 或 string[]`，可含识别结果/候选译文 | 请求与响应随标量/数组切换；实例可以要求 body API key |
| Google Basic / S3 | `POST /language/translate/v2`；`q`、`source?`、`target`、`format` | `data.translations[].translatedText` | 默认 HTML；纯文本适配必须显式设置格式 |
| Google Advanced / S4 | 项目路径下的 `:translateText`；`contents[]`、源/目标语言、MIME 类型 | `translations[]` | 云资源名、地域和术语库资源属于平台配置 |
| Azure v3 / S5 | `POST /translate`；查询参数含版本、`from?`、一个或多个 `to`；body 是文本对象数组 | 输入项数组，每项内含目标译文数组 | 原生支持多目标及可选识别、音译、对齐 |
| Qwen-MT / S6 | Chat Completions；user 消息和 `translation_options` | assistant 文本及 token usage | 语言用英文全名；专用参数承载翻译意图 |
| Qwen-MT-Uni / S7 | DashScope multimodal-generation；`input.source_texts` 或 `input.fileUrl`，源/目标语言 | `output.Data.TranslatedTexts` 或文件地址 | 与 Qwen-MT chat 是不同协议；存在同步/异步及业务结果封装 |
| 自部署 Hy-MT2 / S8 | 推理服务器的消息结构；user 内容由对应版本的翻译模板生成 | 推理服务器的生成结果 | 模板需要语言全名；部署别名可以与模型原名不同 |
| 沉浸式翻译 / S11 | POST JSON；`source_lang`、`target_lang`、`text_list[]` | `translations[].text` 和可选识别语言 | 客户端兼容协议，不能直接代表上游模型协议 |

### 3.1 模型协议的特别之处

Qwen-MT 的官方定义只支持 user 消息，并要求专用翻译选项。词语对、参考句和领域提示是不同参数。流式方面，文档区分 plus 的累积内容与 flash/lite 的增量内容，不能统一直接 append。见 S6。

Hy-MT2 官方文档给出 vLLM、SGLang、llama.cpp 等部署说明，以及翻译、术语、风格、上下文和结构化内容的提示模板。模板表达的是模型意图；保持 JSON 键、HTML 或变量完整仍需输出验证。部署服务能力需要另外核实。见 S8。

Qwen-MT-Uni 原生支持文本字符串/数组以及文档、图片、音频文件。HTTP 成功或异步任务完成之后，还必须检查业务成功字段；文本输出保留输入标量/数组形态。它不能复用 Qwen-MT chat 的 envelope 解析器。见 S7。

### 3.2 资源、格式与计量

DeepL 的持久化术语库需要匹配语言对并明确源语言；请求内上下文不是待翻译文本。Qwen 的请求内词对和参考句不能直接视为该术语库 ID。见 S1、S6。

Google 明确支持文本和 HTML，但不承诺 XML 语义。Azure 的多目标/音译/对齐是额外输出维度。LibreTranslate 的可用语言还取决于实例安装的语言模型。见 S2、S4、S5。

token、UTF-8 字节、Unicode 字符和实际计费字符是不同量。S6/S7 提供 token usage；S14 描述字符计量。文档中没有 usage 不能记录成实测零，也不能凭文本长度制造实际账单。

## 4. 能力对照：原生、模板、未知

`N` = 文档中的原生能力；`P` = 模板表达；`?` = 本次未证实。没有列出的能力不等于明确不支持。

| 后端 | 原生有序文本数组 | 多目标 | 自动处理未知源语言 | 返回识别语言 | HTML/结构保持 |
| --- | --- | --- | --- | --- | --- |
| DeepL | N | ? | N | N | N：明确标签模式 |
| LibreTranslate | N | ? | N | N | N：HTML |
| Google Basic/Advanced 文本接口 | N | ? | N | N | N：HTML；XML 无承诺 |
| Azure v3 | N | N | N | N | N：HTML |
| Qwen-MT chat | ? | ? | N：source=auto | ? | ? |
| Qwen-MT-Uni | N | ? | N | ? | N：文件类型；不等于任意片段严格保持 |
| 自部署 Hy-MT2 | ? | ? | P：目标语言模板不要求填源语种 | ? | P：仍需验证 |

| 后端 | 请求内词对 | 持久化术语库 | 参考句/记忆 | 上下文、领域或风格 | 交付方式 |
| --- | --- | --- | --- | --- | --- |
| DeepL | ? | N | N：资源引用 | N：独立参数 | 完整文本响应 |
| LibreTranslate | ? | ? | ? | ? | 完整文本响应 |
| Google Advanced | ? | N | ? | 依模型/产品配置 | 文本同步；文件另有接口 |
| Azure v3 | N：dynamic dictionary 的特定编码 | ? | ? | N：定制类别；不等于自由风格指令 | 完整文本响应 |
| Qwen-MT chat | N | ? | N：请求内句对 | N：领域提示；不等于通用 context | 完整/流式，chunk 语义按模型不同 |
| Qwen-MT-Uni | N | ? | ? | N：领域提示 | 同步/异步任务；文本与文件 |
| 自部署 Hy-MT2 | P | ? | ? | P | 依推理服务器及具体版本 |

表内能力有前提，例如明确源语言、特定语言对或特定模型。`N` 也不能用来推导任意组合都成立。

## 5. “最大交”到底指什么

需要先固定比较的后端集合和输入范围。本记录讨论上述后端的文本翻译能力；不存在不受模型版本、实例安装和语言对影响的全球交集。

### 5.1 线上字段名的交集

没有可直接采用的共同 JSON 字段/路径集合：原文可以叫 q、text、contents 或 messages，译文也有不同 envelope。因此不能通过重命名一个字段就完成统一。

### 5.2 共同语义核心

在选定后端确实支持该语言对的前提下，共同操作是：**一段原文 + 目标语言 → 一段译文**。源语言可能来自请求、配置或后端推断，不同后端的控制和报告能力不同。

批量、多目标、检测置信度、HTML、术语、文件、异步、流式都不属于这份最保守的共同能力承诺。批量可以由网关逐项编排，但那是新增能力，必须标注成本和失败语义。

**能翻译未知语种、能检测出语种、能返回语种编码、能返回置信度是四件不同的事。**

### 5.3 路由集合的交集

若路由可选服务的有效能力集合为 C1…Cn，任何服务都能完成的能力集合为 `C_safe = C1 ∩ … ∩ Cn`。这里必须考虑具体输入类型、源/目标语言对、选项和配额限制；不能只做几枚布尔开关的相交。

## 6. “最大并”到底指什么

`C_available = C1 ∪ … ∪ Cn` 表示这些服务合起来曾提供的能力，包含以下轴：

| 轴 | 调研范围内的并集 |
| --- | --- |
| 输入 | 单文本、批量文本、结构化内容、文档、图片、音频 |
| 语言 | 明确源语种、未知源处理、识别报告、文字/地域变体、一个/多个目标 |
| 控制 | 内联词对、术语库引用、参考句/翻译记忆、背景上下文、领域、风格 |
| 保持 | HTML/XML 等各自格式能力、不可翻译片段、占位符、格式输出 |
| 输出 | 主译文、候选译文、音译、对齐/句界、翻译后文件 |
| 交付 | 同步完整响应、生成流、异步任务及查询 |
| 运维 | usage、请求 ID、限额、原生错误和业务状态 |

这是对已查来源的有界归纳，不是某个供应商的完整可选项清单，也不是一个后端能接收的“大 JSON”。例如某服务支持术语库、另一服务支持多目标，不能据此承诺“术语库 + 多目标”组合可执行。

适配或编排不能保持含义的选项应拒绝或列为未知。最大并集不是允许静默丢弃参数的理由。协议投影和组合约束在设计文档中定义。

## 7. 其他标准和证据缺口

- BCP 47 定义语言标识，适合作为外部语言命名基础；它不定义翻译请求或宣称某后端支持该语言。见 S12。
- 2026-10-25 查阅的 Browser Translator API 是浏览器 JS 提案；ITS 2.0 是本地化元数据。都不能充当跨厂商服务器翻译 HTTP 协议。见 S13。
- S9 的官方检索结果显示 Chat Completions 与独立 `/v1/api/translations`，但网页工具超过正文尺寸限制；2026-11-12 HTTP 回查仅取得脚本页面，完整鉴权/响应仍未核实。不可直接据此写适配器。
- S10 当前仍标为预下线，预计日期为 2026-12-21；这不影响自部署模型。旧接口仅作为历史对照。
- 百度文档壳、腾讯传统 TMT 的部分旧文本链接未取得完整当前定义，没有列为已验证实现规范。
- 待补：每个后端真实错误 fixture、目标语言/选项组合、源语言提示是否有效、usage 缺失行为、长文本和请求限制，以及模型/推理服务版本固定后的运行验证。

## 8. 事实与建议的边界

本文件支持的结论是：字段和能力确实不同；公共核心较小；能力并集需要有约束的适配和路由。采用固定数组、统一错误、默认路由以及分期实现，是 [设计文档](../.design/translation.md) 的建议，不是上游协议已有的共同标准。
