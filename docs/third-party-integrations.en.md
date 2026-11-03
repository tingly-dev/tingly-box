# Third-Party Integration Statement

[中文](third-party-integrations.zh.md) | [English](third-party-integrations.en.md)

Last updated: October 21, 2026

Tingly Box provides compatible connections to third-party models, clients and tools so users can carry out everyday work and use routing, request records, access controls and tool extensions within the scope permitted by upstream services. This statement explains the boundaries of those connections, including API Keys, cloud credentials, account sign-in, client configuration and tool connections.

## Connections and authorization

Tingly Box is an independent project. Unless explicitly stated otherwise in writing, connecting to a service or displaying its name or logo does not imply authorization, certification, partnership or endorsement by that service provider. Names, trademarks and related rights belong to their respective owners. The catalog label `official` means that the service provider operates the endpoint; it does not mean that Tingly Box has official approval.

Connect only accounts, keys, cloud resources and tools that you are authorized to use, and comply with the applicable service terms, plan scope, regional requirements, organization policies and usage limits. Completing sign-in only obtains the current credentials; it does not grant additional permissions. Subscription or promotional credits do not automatically include general-purpose API access rights.

Some account connections use client interfaces or protocols observed by the community rather than public APIs with a stability commitment to developers. Service providers may change sign-in flows, model catalogs, quotas, client channels and access policies. Compatibility does not guarantee that every client, model or feature will remain available, or that an account will not be restricted upstream.

Tingly Box connections are intended for authorized everyday use and capability extensions. Do not use them to register accounts in bulk, share accounts, resell credits whose resale is not permitted, or circumvent channel restrictions or account enforcement. When a channel is refused, an account is restricted or authorization is revoked, follow the upstream provider's requirements. Protocol conversion or a different connection method does not constitute upstream permission.

## Current integration scope

The following inventory describes the implementation at the time of this update. It lists connection entry points and does not promise availability or official authorization. Some entries may be unavailable depending on the version, account, region, plan or administrator settings. This statement also applies to custom services added by users.

### Account sign-in

| Connection | Scope and notes |
| --- | --- |
| Anthropic Claude Code | `claude_code`; permitted calls depend on Anthropic's terms and account entitlements |
| OpenAI Codex / ChatGPT | `codex`; separate from the OpenAI API Key billing entry point |
| Google Gemini CLI | `gemini`; separate from Gemini API Key and Vertex AI entry points |
| Google Antigravity | `antigravity`; experimental connection, subject to the account's actual permissions |
| Kimi Code | `kimi_code`; separate from the regular Moonshot model API entry point |
| xAI Grok | `xai`; availability depends on the xAI account and plan |
| ZCode / GLM Coding Plan | `zcode` (Z.ai) and `zcode_cn` (BigModel); accounts in the two regions are authorized separately |
| WorkBuddy / CodeBuddy | `workbuddy` (international) and `workbuddy_cn` (China); uses client sign-in interfaces, separate from WorkBuddy Open Platform application authorization; client channels may be restricted |
| Qwen Code | `qwen_code`; the preset entry point is currently disabled and does not imply a working sign-in flow |

The backend also contains iFlow and configurable OpenAI / Google / GitHub OAuth entries, as well as identity adapter code for Cursor / Copilot. These are not all sign-in entry points enabled by default; they require the corresponding implementation and configuration in the version in use. Mock is for testing and is not a third-party service.

### Model, cloud and local-service catalog

The table below covers all 74 preset entries in `internal/catalog/providers.json`. Regional and plan-specific entry points for the same service are listed separately. The catalog also includes some of the account sign-in entries above.

| Catalog ID | Service | Connection / status |
| --- | --- | --- |
| `aws-bedrock` | AWS Bedrock | AWS credentials / Bedrock Key |
| `gcp-vertex-claude` | Vertex — Claude | GCP service account |
| `gcp-vertex-gemini` | Vertex — Gemini | GCP service account |
| `azure-openai` | Azure OpenAI | Azure Key |
| `openai-com` | OpenAI | API Key / compatible API |
| `anthropic-com` | Anthropic | API Key / compatible API |
| `claude-code` | Anthropic Claude Code (OAuth) | Account sign-in |
| `codex` | ChatGPT Code (OAuth) | Account sign-in |
| `googleapis-com` | Google Gemini | API Key / compatible API |
| `gemini-cli` | Google Gemini CLI (OAuth) | Account sign-in |
| `antigravity` | Antigravity (Google OAuth) | Account sign-in |
| `x-ai` | xAI | API Key / compatible API |
| `deepseek-com` | Deepseek | API Key / compatible API |
| `mistral-ai` | Mistral | API Key / compatible API |
| `dashscope-cn` | Aliyun Dashscope (CN) | API Key / compatible API |
| `dashscope-intl` | Alibaba Dashscope | API Key / compatible API |
| `dashscope-cn-coding` | Qwen Coding Plan (CN) | API Key / compatible API |
| `dashscope-intl-coding` | Qwen Coding Plan | API Key / compatible API |
| `dashscope-cn-token` | Qwen Token Plan (CN) | API Key / compatible API |
| `dashscope-intl-token` | Qwen Token Plan | API Key / compatible API |
| `qwen-code` | Qwen Code (OAuth) | Account sign-in; currently disabled |
| `kimi-code` | Kimi Code (OAuth) | Account sign-in |
| `modelscope-cn` | ModelScope (CN) | API Key / compatible API |
| `minimaxi-com` | Minimax (CN) | API Key / compatible API |
| `minimax-io` | Minimax | API Key / compatible API |
| `z-ai` | Z.ai | API Key / compatible API |
| `z-ai-coding` | Z.ai Coding Plan | API Key / compatible API |
| `bigmodel-cn` | Bigmodel (CN) | API Key / compatible API |
| `bigmodel-cn-coding` | Bigmodel GLM Coding Plan (CN) | API Key / compatible API |
| `moonshot-ai` | Kimi (Moonshot AI) | API Key / compatible API |
| `moonshot-cn` | Kimi (Moonshot AI) (CN) | API Key / compatible API |
| `kimi-com-coding` | Kimi Code | API Key / compatible API |
| `kimi-ai-coding` | Kimi Code (Global) | API Key / compatible API |
| `volces-com` | Volcengine Ark (CN) | API Key / compatible API |
| `volces-com-coding` | Volcengine Ark Coding Plan (CN) | API Key / compatible API |
| `baidubce-com` | Baidu Qianfan (CN) | API Key / compatible API |
| `baidubce-com-coding` | Baidu Qianfan Coding Plan (CN) | API Key / compatible API |
| `tencent-com` | Tencent Hunyuan | API Key / compatible API |
| `xf-yun-com` | iFlytek Spark (CN) | API Key / compatible API |
| `baichuan-com` | Baichuan AI (CN) | API Key / compatible API |
| `lingyiwanwu-com` | 01.AI (CN) | API Key / compatible API |
| `stepfun-com` | Stepfun (CN) | API Key / compatible API |
| `stepfun-com-coding` | Step Plan (CN) | API Key / compatible API |
| `stepfun-ai` | Stepfun | API Key / compatible API |
| `stepfun-ai-coding` | Step Plan | API Key / compatible API |
| `siliconflow-cn` | SiliconFlow (CN) | API Key / compatible API |
| `siliconflow-com` | SiliconFlow | API Key / compatible API |
| `xiaomimimo-com` | Xiaomi Mimo | API Key / compatible API |
| `xiaomimimo-com-coding` | Mimo Coding Plan (CN) | API Key / compatible API |
| `xiaomimimo-sgp-coding` | Mimo Coding Plan (Singapore) | API Key / compatible API |
| `xiaomimimo-ams-coding` | Mimo Coding Plan (Europe) | API Key / compatible API |
| `groq-com` | Groq | API Key / compatible API |
| `together-xyz` | Together AI | API Key / compatible API |
| `fireworks-ai` | Fireworks AI | API Key / compatible API |
| `cerebras-ai` | Cerebras | API Key / compatible API |
| `perplexity-ai` | Perplexity | API Key / compatible API |
| `cohere-com` | Cohere | API Key / compatible API |
| `nvidia-com` | NVIDIA NIM | API Key / compatible API |
| `novita-ai` | Novita AI | API Key / compatible API |
| `deepinfra-com` | DeepInfra | API Key / compatible API |
| `hyperbolic-xyz` | Hyperbolic | API Key / compatible API |
| `runcrate-ai` | Runcrate | API Key / compatible API |
| `vercel-ai-gateway` | Vercel AI Gateway | API Key / compatible API |
| `cloudflare-ai-gateway` | Cloudflare AI Gateway | API Key / compatible API |
| `typesafe-ai` | TypeSafe AI (Jev) | API Key / compatible API |
| `openrouter-ai` | OpenRouter | API Key / compatible API |
| `opencode-ai` | OpenCode Zen | API Key / compatible API |
| `opencode-go` | OpenCode Go | API Key / compatible API |
| `ollama` | Ollama | Local / self-hosted API |
| `lm-studio` | LM Studio | Local / self-hosted API |
| `localai` | LocalAI | Local / self-hosted API |
| `jan` | Jan | Local / self-hosted API |
| `vllm` | vLLM | Local / self-hosted API |
| `sglang` | SGLang | Local / self-hosted API |
| `lemonade` | AMD Lemonade | Local / self-hosted API |

### Clients and tool extensions

Client configuration entry points include Claude Code, Claude Desktop, Codex, Gemini CLI, OpenCode, Pi, DeepSeek Harness (DSH), WorkBuddy, Xcode, VS Code and Cursor. Protocol entry points include OpenAI, Anthropic, Google Gemini and their compatible protocols. The upstream capabilities available to each client depend on both sides' actual protocol support and upstream authorization.

Tool connections include user-configured MCP services and skills, as well as messaging adapters for Telegram, Discord, Slack, Feishu / Lark, DingTalk, WeCom, Weixin, QQ and WhatsApp. Each platform's bot, account, organization approval and data access requirements still apply. The existence of adapter code does not mean that it is enabled or authorized in the current deployment.

## Credentials and data

Use Tingly Box on a trusted device or deployment. The server needs to store and use configured credentials, which may include access tokens, refresh tokens, API Keys and cloud authentication materials. Do not disclose sensitive information in configurations, exports, backups or logs. With a remote deployment, credentials and records reside in that deployment; they should not be treated as existing only in the browser's local environment.

Requests are sent to the selected model or tool services according to your configuration. Routing fallback may also send a request to another service in the rule. Tool execution may access additional external resources depending on user configuration and authorization. Confirm that data, source code and personal information may be sent to the relevant services, and configure appropriate access controls and log retention. Third-party data processing is governed by each provider's own privacy policy and agreements.

## Service changes and troubleshooting

Connection capabilities depend on the current release and actual upstream responses. When a request fails, distinguish expired credentials, incompatible request formats, rate limits, channel refusals and account restrictions. Signing in again or switching models does not necessarily resolve a channel or account restriction. Account enforcement and appeals should be handled through the service provider's official channels.

When reporting a compatibility problem, you may provide the Tingly Box version, connection region, model, HTTP status, upstream error code and a redacted error description. Do not submit tokens, keys, complete configurations or request content containing private data.

This statement supplements the explanation of third-party integration boundaries. It does not replace the project license, applicable agreements or third-party service terms. The inventory is based on account configuration and adapter code, the model catalog, scenario registration and messaging platform adapters. Maintainers should update this inventory when adding or removing integrations.
