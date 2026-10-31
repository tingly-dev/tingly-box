# 第三方接入声明

[中文](third-party-integrations.zh.md) | [English](third-party-integrations.en.md)

更新日期：2026 年 10 月 21 日

Tingly Box 提供第三方模型、客户端和工具的兼容接入，帮助用户在上游服务允许的范围内完成日常工作，并使用路由、调用记录、权限控制和工具扩展等能力。本文说明这些接入的适用边界，覆盖 API Key、云平台凭据、账号登录、客户端配置及工具连接等方式。

## 接入与授权

Tingly Box 是独立项目。除非另有明确书面说明，接入某项服务、展示其名称或标识，不表示获得该服务提供方的授权、认证、合作关系或推荐。名称、商标及相关权益归各自权利人所有。目录中的 `official` 表示接口由服务提供方运营，不表示 Tingly Box 获得官方认可。

请仅连接自己有权使用的账号、密钥、云资源及工具，并遵守相应服务的条款、套餐范围、地区要求、组织策略和调用限制。完成登录只表示取得了当前凭据，不代表获得额外使用权限；订阅或赠送额度也不自动等同于通用 API 调用权。

部分账号接入使用客户端接口或社区观察到的协议，并非对开发者承诺稳定的开放 API。服务提供方可能调整登录、模型目录、配额、客户端渠道和访问策略。兼容接入不保证所有客户端、模型或功能始终可用，也不保证账号不受上游限制。

Tingly Box 的接入用途是用户获准的常规使用及能力扩展。请勿利用接入批量注册或共享账号、转售不允许转售的额度、规避渠道限制或账号处置。出现渠道拒绝、账号受限或授权撤销时，应按上游要求处理；格式转换或更换接入方式不构成上游许可。

## 当前接入范围

以下是本次文档更新时的实现清单，列出接入入口，不构成可用性或官方授权承诺。版本、账号、地区、套餐和管理员设置可能使其中部分入口不可用。新增的自定义服务同样适用本声明。

### 账号登录

| 接入 | 范围与说明 |
| --- | --- |
| Anthropic Claude Code | `claude_code`；是否允许相关调用，以 Anthropic 的条款和账号权益为准 |
| OpenAI Codex / ChatGPT | `codex`；与 OpenAI API Key 计费入口分开 |
| Google Gemini CLI | `gemini`；与 Gemini API Key、Vertex AI 入口分开 |
| Google Antigravity | `antigravity`；实验性接入，以实际账号权限为准 |
| Kimi Code | `kimi_code`；与 Moonshot 常规模型 API 入口分开 |
| xAI Grok | `xai`；可用范围由 xAI 的账号和套餐决定 |
| ZCode / GLM Coding Plan | `zcode`（Z.ai）、`zcode_cn`（智谱 BigModel）；两地账号分别授权 |
| WorkBuddy / CodeBuddy | `workbuddy`（国际版）、`workbuddy_cn`（国内版）；使用客户端登录接口，与 WorkBuddy 开放平台应用授权分开；客户端渠道可能受限 |
| Qwen Code | `qwen_code`；当前预置入口已停用，不表示可以正常登录 |

后端另有 iFlow 及 OpenAI / Google / GitHub 可配置 OAuth 项，以及 Cursor / Copilot 等身份适配代码；它们不是全部默认开放的登录入口，需要对应版本的实现和配置。测试用 Mock 不属于第三方服务。

### 模型、云平台与本地服务目录

下表覆盖 `internal/catalog/providers.json` 的全部 74 个预置项；同一服务的地区和套餐入口分别列出。该目录也包含上表中的部分账号登录项。

| 目录 ID | 服务 | 接入方式 / 状态 |
| --- | --- | --- |
| `aws-bedrock` | AWS Bedrock | AWS 凭据 / Bedrock Key |
| `gcp-vertex-claude` | Vertex — Claude | GCP 服务账号 |
| `gcp-vertex-gemini` | Vertex — Gemini | GCP 服务账号 |
| `azure-openai` | Azure OpenAI | Azure Key |
| `openai-com` | OpenAI | API Key / 兼容接口 |
| `anthropic-com` | Anthropic | API Key / 兼容接口 |
| `claude-code` | Anthropic Claude Code (OAuth) | 账号登录 |
| `codex` | ChatGPT Code (OAuth) | 账号登录 |
| `googleapis-com` | Google Gemini | API Key / 兼容接口 |
| `gemini-cli` | Google Gemini CLI (OAuth) | 账号登录 |
| `antigravity` | Antigravity (Google OAuth) | 账号登录 |
| `x-ai` | xAI | API Key / 兼容接口 |
| `deepseek-com` | Deepseek | API Key / 兼容接口 |
| `mistral-ai` | Mistral | API Key / 兼容接口 |
| `dashscope-cn` | Aliyun Dashscope (CN) | API Key / 兼容接口 |
| `dashscope-intl` | Alibaba Dashscope | API Key / 兼容接口 |
| `dashscope-cn-coding` | Qwen Coding Plan (CN) | API Key / 兼容接口 |
| `dashscope-intl-coding` | Qwen Coding Plan | API Key / 兼容接口 |
| `dashscope-cn-token` | Qwen Token Plan (CN) | API Key / 兼容接口 |
| `dashscope-intl-token` | Qwen Token Plan | API Key / 兼容接口 |
| `qwen-code` | Qwen Code (OAuth) | 账号登录；当前停用 |
| `kimi-code` | Kimi Code (OAuth) | 账号登录 |
| `modelscope-cn` | ModelScope (CN) | API Key / 兼容接口 |
| `minimaxi-com` | Minimax (CN) | API Key / 兼容接口 |
| `minimax-io` | Minimax | API Key / 兼容接口 |
| `z-ai` | Z.ai | API Key / 兼容接口 |
| `z-ai-coding` | Z.ai Coding Plan | API Key / 兼容接口 |
| `bigmodel-cn` | Bigmodel (CN) | API Key / 兼容接口 |
| `bigmodel-cn-coding` | Bigmodel GLM Coding Plan (CN) | API Key / 兼容接口 |
| `moonshot-ai` | Kimi (Moonshot AI) | API Key / 兼容接口 |
| `moonshot-cn` | Kimi (Moonshot AI) (CN) | API Key / 兼容接口 |
| `kimi-com-coding` | Kimi Code | API Key / 兼容接口 |
| `kimi-ai-coding` | Kimi Code (Global) | API Key / 兼容接口 |
| `volces-com` | Volcengine Ark (CN) | API Key / 兼容接口 |
| `volces-com-coding` | Volcengine Ark Coding Plan (CN) | API Key / 兼容接口 |
| `baidubce-com` | Baidu Qianfan (CN) | API Key / 兼容接口 |
| `baidubce-com-coding` | Baidu Qianfan Coding Plan (CN) | API Key / 兼容接口 |
| `tencent-com` | Tencent Hunyuan | API Key / 兼容接口 |
| `xf-yun-com` | iFlytek Spark (CN) | API Key / 兼容接口 |
| `baichuan-com` | Baichuan AI (CN) | API Key / 兼容接口 |
| `lingyiwanwu-com` | 01.AI (CN) | API Key / 兼容接口 |
| `stepfun-com` | Stepfun (CN) | API Key / 兼容接口 |
| `stepfun-com-coding` | Step Plan (CN) | API Key / 兼容接口 |
| `stepfun-ai` | Stepfun | API Key / 兼容接口 |
| `stepfun-ai-coding` | Step Plan | API Key / 兼容接口 |
| `siliconflow-cn` | SiliconFlow (CN) | API Key / 兼容接口 |
| `siliconflow-com` | SiliconFlow | API Key / 兼容接口 |
| `xiaomimimo-com` | Xiaomi Mimo | API Key / 兼容接口 |
| `xiaomimimo-com-coding` | Mimo Coding Plan (CN) | API Key / 兼容接口 |
| `xiaomimimo-sgp-coding` | Mimo Coding Plan (Singapore) | API Key / 兼容接口 |
| `xiaomimimo-ams-coding` | Mimo Coding Plan (Europe) | API Key / 兼容接口 |
| `groq-com` | Groq | API Key / 兼容接口 |
| `together-xyz` | Together AI | API Key / 兼容接口 |
| `fireworks-ai` | Fireworks AI | API Key / 兼容接口 |
| `cerebras-ai` | Cerebras | API Key / 兼容接口 |
| `perplexity-ai` | Perplexity | API Key / 兼容接口 |
| `cohere-com` | Cohere | API Key / 兼容接口 |
| `nvidia-com` | NVIDIA NIM | API Key / 兼容接口 |
| `novita-ai` | Novita AI | API Key / 兼容接口 |
| `deepinfra-com` | DeepInfra | API Key / 兼容接口 |
| `hyperbolic-xyz` | Hyperbolic | API Key / 兼容接口 |
| `runcrate-ai` | Runcrate | API Key / 兼容接口 |
| `vercel-ai-gateway` | Vercel AI Gateway | API Key / 兼容接口 |
| `cloudflare-ai-gateway` | Cloudflare AI Gateway | API Key / 兼容接口 |
| `typesafe-ai` | TypeSafe AI (Jev) | API Key / 兼容接口 |
| `openrouter-ai` | OpenRouter | API Key / 兼容接口 |
| `opencode-ai` | OpenCode Zen | API Key / 兼容接口 |
| `opencode-go` | OpenCode Go | API Key / 兼容接口 |
| `ollama` | Ollama | 本地 / 自托管接口 |
| `lm-studio` | LM Studio | 本地 / 自托管接口 |
| `localai` | LocalAI | 本地 / 自托管接口 |
| `jan` | Jan | 本地 / 自托管接口 |
| `vllm` | vLLM | 本地 / 自托管接口 |
| `sglang` | SGLang | 本地 / 自托管接口 |
| `lemonade` | AMD Lemonade | 本地 / 自托管接口 |

### 客户端与工具扩展

客户端配置入口包括 Claude Code、Claude Desktop、Codex、Gemini CLI、OpenCode、Pi、DeepSeek Harness（DSH）、WorkBuddy、Xcode、VS Code 和 Cursor。协议入口包括 OpenAI、Anthropic、Google Gemini 及相应兼容协议；每个客户端能使用哪些上游能力，由双方实际支持的协议和上游授权共同决定。

工具接入包括用户配置的 MCP 服务与技能，以及 Telegram、Discord、Slack、飞书 / Lark、钉钉、企业微信、微信、QQ 和 WhatsApp 等消息平台适配。各平台的机器人、账号、组织审批及数据访问要求仍然适用；存在适配代码不等于当前部署已经启用或完成授权。

## 凭据与数据

请在可信的设备或部署中使用 Tingly Box。服务端需要保存并使用已配置的凭据，可能包括访问令牌、刷新令牌、API Key 和云平台认证材料；不要公开配置、导出文件、备份或日志中的敏感内容。远程部署时，凭据与记录位于该部署中，不能将其视为仅存在于浏览器本机。

请求会按配置发送至所选模型或工具服务；路由回退也可能把请求交给规则中另一家服务。工具执行可能进一步访问外部资源，具体取决于用户配置和授权。请先确认数据、源代码和个人信息是否允许发送到相关服务，并设置适当的访问控制与日志保留范围。第三方的数据处理适用其自身的隐私政策和协议。

## 服务变化与问题处理

接入能力以当前发行版及实际响应为准。遇到失败时，请区分凭据过期、请求格式不兼容、限流、渠道拒绝和账号限制；重复登录或换模型不一定能解决渠道或账号限制。账号处置和申诉应通过服务提供方的官方渠道完成。

报告兼容性问题时，可以提供 Tingly Box 版本、接入区域、模型、HTTP 状态、上游错误码及脱敏后的错误说明。请勿提交令牌、密钥、完整配置或包含隐私数据的请求内容。

本文补充说明第三方接入边界，不替代项目许可证、适用协议或第三方服务条款。清单来源：账号配置与适配代码、模型目录、场景注册及消息平台适配；维护者新增或移除接入时，应同步更新本清单。
