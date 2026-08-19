# 模型数据分层

模型相关的静态数据有两条正交的轴,分别有各自的文件与维护主体,不要互相塞:

| 层 | 位置 | 回答的问题 | 维护方式 |
|----|------|-----------|---------|
| **能力目录(catalog)** | `internal/protocol/catalog/<vendor>.models.json` + `<vendor>.go` | 这个模型本身能做什么。能力是**模型族属性**,与哪个 provider 提供无关,每个模型只声明一次。当前只覆盖 thinking/reasoning(其余能力用不上就不建模)。 | 人工策展,新模型发布时更新 JSON,不改代码。 |
| **供给注册表(providers)** | `internal/data/providers.json` | 谁在哪个端点、以什么限额提供哪些模型(base_url、context、max_output、deprecated)。这些**可能**因 provider 而异,所以挂在 provider 条目下。 | 人工策展。 |

运行时从 provider API 拉取的模型列表(`ModelListManager` / DB)是第三层缓存,不在本文件讨论范围。

## Catalog 使用规则

- **只建模消费方实际用到的字段,不镜像 vendor API 的全量 capabilities 响应。**
  早期版本整体复制了 Anthropic `/v1/models` 的 capabilities 结构(batch/citations/
  code_execution/context_management/image_input/pdf_input/structured_outputs…),
  但代码只读 thinking 和 effort 两块 —— 其余是没人消费的死重量,已删除。加字段前
  先确认有真实消费方。
- Schema 直接采用 OpenRouter 模型列表的 `reasoning` 块命名(`supported_efforts` 等),
  而非 Anthropic `capabilities.effort.<level>.supported` 的嵌套布尔树:
  ```json
  { "id": "claude-opus-4-7", "reasoning": { "dialects": ["adaptive"], "supported_efforts": ["low","medium","high","xhigh","max"] } }
  ```
  `supported_efforts` = `output_config.effort` 支持的档位列表,省略即无 effort 支持;
  没有 `reasoning` 块的模型 = 完全不支持 extended thinking。
  `dialects`(`"budget"` = 接受 `thinking.type=enabled`+`budget_tokens`;`"adaptive"` =
  接受 `thinking.type=adaptive`)是 OpenRouter schema 里没有的一个字段 —— OpenRouter
  的调用方从不接触 wire 协议差异,由 OpenRouter 自己的代理层吸收;而这个包**就是**
  面向 Anthropic 的那层代理,必须知道该发哪种原始请求形状,所以补了这一个字段,其余
  照抄 OpenRouter 命名。
  `mandatory` / `default_enabled` / `default_effort`(OpenRouter schema 里有)目前**未采用**——
  没有可核实的官方数据支撑每个模型的取值,填 false/编造属于捏造事实,等有可靠来源
  (或代码出现真实消费方)再补。
- 每个 vendor 一对文件:`claude.models.json`(数据)+ `claude.go`(加载与查询,如
  `catalog.LookupClaudeThinkingCaps`)。openai / gemini 需要能力判定时按同样模式扩展,
  字段集合由各自实际消费方决定,不必与 claude 的 schema 一致。
- `claude.models.snapshot.json`:未经改动的 Anthropic `/v1/models` 响应镜像(**不是**
  运行时数据,不被任何代码 embed/读取),作为核对 `claude.models.json` 的事实来源。
  新增/修订条目时先查这份快照;它会随新模型发布而过期(当前只覆盖 19 个模型中的
  10 个),按需人工从线上 API 刷新,不必每次改动都同步。
- 查询按"完整 id + 去日期 family 名"双索引、最长 key 优先做子串匹配,所以裸名
  (`claude-opus-4-5`)、带日期 id、云厂商修饰名(`us.anthropic.…-v1:0`、`…@20251001`)都能解析。
- **完备性不变式**:`catalog/completeness_test.go` 断言 providers.json 中出现的每个 Claude
  模型 id 去除已知云厂商修饰后必须与 catalog 的完整 id 或去日期 family **精确相等**。
  完备性检查不复用运行时的宽松子串查询,避免新模型误借旧 family 的能力。加新模型先加
  catalog,再加 providers.json,否则测试失败。
- 不在 catalog 中的模型(第三方代理模型、比快照新的发布)由消费方给保守兜底
  (thinking 场景:budget-only、剥离 effort,见 `ops.anthropicModelThinkingCaps`)。

## 消费方

- `internal/protocol/ops/request_anthropic_model.go`:按模型能力对 thinking 三方言
  (adaptive / enabled+budget / output_config.effort)做 vendor 阶段互转与钳制。
- effort↔budget 的统一阶梯在 `internal/protocol/thinking`(见 `.design/rule-flags.md`
  的 `thinking_effort` 行),catalog 只负责"模型支持哪些方言/档位"。
- OpenAI 兼容出口没有按模型的 catalog,粒度是按 provider host 白名单
  (`ops.supportsExplicitPromptCache`)。不在白名单上的 host 一律保守收窄:
  reasoning_effort 收窄到 low/medium/high(`ops.genericEffortTiers`),
  prompt-cache 字段整体剥离。与上面 Claude 兜底同一条原则的另一份实现——
  "未验证的能力,一律折叠回已验证的公共子集",不是两套设计。

## 注意

- opus-4-7 / opus-4-8 / sonnet-5 / opus-5 / fable-5 的 effort 档位按 Anthropic
  官方 effort 文档维护；这些模型明确支持 `xhigh`，运行时不得提前折叠为 `high`。
  thinking dialect 仍需按官方模型 capabilities 持续核对。
