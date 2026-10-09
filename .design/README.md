# Design documents — topic index

这里按职责导航设计文档。文件内的“当前契约 / 实施状态 / 历史基线 / 提案”
应明确区分；索引的状态只概括范围，具体未完成项以各文档为准。

维护约定：

- 现行契约由主题正文维护；迁移前分析明确标成历史，不在历史段追改旧路径。
- 提案中的 schema、API 与未来能力不能当成已存在的实现。
- **所有 pencil 独立保存**，命名为 `主题.pencil.md`；UI 面板可用 `主题-panel.pencil.md`。
  正文解释契约和取舍，pencil 表达流程、状态或布局。双方互相链接，职责有交集时指定权威来源。
- 用 `internal/typ/flag_registry.go` 核对 flag 清单；新增 flag 同步正文与测试矩阵。
- 重命名或拆分时更新正文、README 与源码注释引用；Claude Code 的旧入口保留为索引。
- 文件路径缺失不总是偏误：历史路径、提案文件与 codegen 产物分别注明，勿伪装成现行代码地图。

## 独立 pencil 文档

| 文档 | 职责 | 配套说明 |
|---|---|---|
| [load-balancing.pencil.md](./load-balancing.pencil.md) | 选路总览、运行时状态和诊断面 | [tier-routing.md](./tier-routing.md) |
| [tier-routing.pencil.md](./tier-routing.pencil.md) | 跨请求 tier/breaker、恢复及与请求内 retry 的衔接 | [tier-routing.md](./tier-routing.md) |
| [request-failover.pencil.md](./request-failover.pencil.md) | 一次请求内 prologue、候选切换和每次重新变换 | [tier-routing.md](./tier-routing.md)、[请求管线](./protocol-stage-pipeline.md) |
| [session-affinity.pencil.md](./session-affinity.pencil.md) | strict TTL、分区 pin 与重新锁定时间线 | [tier-routing.md](./tier-routing.md) |
| [protocol-stage-pipeline.pencil.md](./protocol-stage-pipeline.pencil.md) | Source/Target 阶段装配与迁移图 | [protocol-stage-pipeline.md](./protocol-stage-pipeline.md) |
| [bench.pencil.md](./bench.pencil.md) | 测试台线框；被放弃的布局明确标注 | [bench.md](./bench.md) |
| [probe-panel.pencil.md](./probe-panel.pencil.md) | Probe 面板轴、打开状态与 cURL 流程 | [probe.md](./probe.md) |
| [imbot-output.pencil.md](./imbot-output.pencil.md) | 消息序列及回复归属；历史图与现行降级分开 | [imbot-output.md](./imbot-output.md) |
| [npm.pencil.md](./npm.pencil.md) | `tb gui` / `tb app` 与 `npx tingly-box-gui` 的启动、取回与缓存流程 | [npm.md](./npm.md) |

## 架构与持久化

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [backend-organization.md](./backend-organization.md) | Backend Organization | 步骤 1–6 已落地；后续 backlog |
| [protocol-server.md](./protocol-server.md) | ProtocolServer — 模型服务面从 internal/server 独立 | 设计说明；局部实施状态见正文 |
| [protocol-client-boundary.md](./protocol-client-boundary.md) | internal/protocol 与 internal/client 的边界 | 设计说明；局部实施状态见正文 |
| [protocol-transform.md](./protocol-transform.md) | protocol/transform 架构设计 | 设计说明；局部实施状态见正文 |
| [usecase-layer.md](./usecase-layer.md) | internal/usecase | 设计说明；局部实施状态见正文 |
| [db.md](./db.md) | Database Layer (internal/db) | 设计说明；局部实施状态见正文 |
| [config-migration.md](./config-migration.md) | Config Migration Pipeline | 设计说明；局部实施状态见正文 |
| [rule-storage.md](./rule-storage.md) | Rule 存储迁移：config.json → SQLite | 设计说明；局部实施状态见正文 |
| [rule-uuid.md](./rule-uuid.md) | Rule UUID Conventions | 设计说明；局部实施状态见正文 |
| [remote-storage.md](./remote-storage.md) | Remote 存储重设计 | 设计说明；局部实施状态见正文 |
## 协议、路由与请求能力

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [protocol-stage.md](./protocol-stage.md) | Protocol Stage | 设计说明；局部实施状态见正文 |
| [protocol-stage-pipeline.md](./protocol-stage-pipeline.md) | Protocol Stage 请求管线 | 设计说明；局部实施状态见正文 |
| [protocol-responses.md](./protocol-responses.md) | Responses Protocol Contract | 设计说明；局部实施状态见正文 |
| [stream-converter-pipeline.md](./stream-converter-pipeline.md) | Stream Converter Pipeline | 设计说明；局部实施状态见正文 |
| [multimodal-content.md](./multimodal-content.md) | Multimodal content — protocol catalog & conversion contract | 设计说明；局部实施状态见正文 |
| [anthropic-sdk-stream-tool-name-restoration.md](./anthropic-sdk-stream-tool-name-restoration.md) | Restoring tool names in Anthropic streaming responses | 设计说明；局部实施状态见正文 |
| [openai-endpoint-routing.md](./openai-endpoint-routing.md) | OpenAI Endpoint Routing 设计 | 设计说明；局部实施状态见正文 |
| [tier-routing.md](./tier-routing.md) | Tier-Based Service Routing | 设计说明；局部实施状态见正文 |
| [rule-flags.md](./rule-flags.md) | Rule Flags 设计与实操 | 设计说明；局部实施状态见正文 |
| [provider-flags.md](./provider-flags.md) | Provider Extensions & Provider/Model/Rule Flags | rule headers 已实现；provider/model 提案 |
| [user-agent.md](./user-agent.md) | User-Agent 处理与优先级 | 设计说明；局部实施状态见正文 |
| [vision-proxy.md](./vision-proxy.md) | Vision Proxy | 设计说明；局部实施状态见正文 |
| [model-data.md](./model-data.md) | 模型数据分层 | 设计说明；局部实施状态见正文 |
| [model-list.md](./model-list.md) | Model List 获取 / 缓存 / 兜底 设计 | 设计说明；局部实施状态见正文 |
| [dual-provider.md](./dual-provider.md) | Dual Provider | 设计说明；局部实施状态见正文 |
## 接入、凭证与配额

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [connect-ai-flow.md](./connect-ai-flow.md) | Connect AI Flow | 设计说明；局部实施状态见正文 |
| [oauth.md](./oauth.md) | OAuth | 设计说明；局部实施状态见正文 |
| [third-party-credentials.md](./third-party-credentials.md) | Third-Party Cloud Credentials (AWS Bedrock / GCP Vertex / Azure OpenAI) | 设计说明；局部实施状态见正文 |
| [kimi-config.md](./kimi-config.md) | Kimi Code OAuth & Client Round Trip: Design and Decisions | 设计说明；局部实施状态见正文 |
| [zcode-oauth.md](./zcode-oauth.md) | ZCode OAuth (GLM Coding Plan) | 设计说明；局部实施状态见正文 |
| [deepseek.md](./deepseek.md) | DeepSeek | 设计说明；局部实施状态见正文 |
| [codex.md](./codex.md) | Codex as a Client | 设计说明；局部实施状态见正文 |
| [codex-auth.md](./codex-auth.md) | Codex Auth Modes: gateway, direct ChatGPT, and hybrid | 设计说明；局部实施状态见正文 |
| [codex-config.md](./codex-config.md) | Codex Quick Config: Design and Decisions | 设计说明；局部实施状态见正文 |
| [claude-code.md](./claude-code.md) | Claude Code — design index | 入口索引；会话与 OAuth 兼容层独立 |
| [claude-code-config.md](./claude-code-config.md) | Claude Code Quick Config: Design and Decisions | 设计说明；局部实施状态见正文 |
| [cursor.md](./cursor.md) | Cursor Scenario — the public-URL constraint | 设计说明；局部实施状态见正文 |
| [opencode-session.md](./opencode-session.md) | OpenCode Zen session header | 设计说明；局部实施状态见正文 |
| [opencode-quota.md](./opencode-quota.md) | OpenCode（Zen / Go）配额方案 | 设计说明；局部实施状态见正文 |
| [quota-semantics.md](./quota-semantics.md) | Quota 语义归一 | 设计说明；局部实施状态见正文 |
| [quota-relay.md](./quota-relay.md) | Quota 中继：端侧看到中央的 quota | 设计说明；局部实施状态见正文 |
| [team.md](./team.md) | Team and Sharing Key Authorization | 设计说明；局部实施状态见正文 |
| [security.md](./security.md) | Security: Design Decisions | 设计说明；局部实施状态见正文 |
## Agent、Bot 与会话

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [afk.md](./afk.md) | AFK — Agent Framework Kit | 设计说明；局部实施状态见正文 |
| [agentboot-refactor.md](./agentboot-refactor.md) | Agentboot Refactor | 已完成的重构记录；后续决定见附录 |
| [claude-code-session.md](./claude-code-session.md) | Claude Code: from one-shot processes to a persistent stream session | P0–P2 完成；默认关闭；P3 待做 |
| [claude-code-oauth-compat.md](./claude-code-oauth-compat.md) | Claude Code OAuth compatibility — wire identity and version profiles | 已实现 profile；限制见 §B7 |
| [bot-arch.md](./bot-arch.md) | Bot Architecture — resource, channel, consumers (+ naming) | 设计说明；局部实施状态见正文 |
| [bot-capability-access-control.md](./bot-capability-access-control.md) | Bot Capability Access Control — Bot、Direct Chat、Group 与 Actor | 资源/授权核心已实现；验收项按正文核对 |
| [bot-interaction-api.md](./bot-interaction-api.md) | Bot Interaction Interface — auth + open two-kind API (notify / interactive) | 通用 API 已实现；旧 hook HTTP 鉴权待迁移 |
| [bot-panic-isolation.md](./bot-panic-isolation.md) | Bot Panic Isolation — recover at the trust boundary | 设计说明；局部实施状态见正文 |
| [imbot-sync.md](./imbot-sync.md) | ImBot State Sync: Event-Driven with a Reconcile Backstop | 设计说明；局部实施状态见正文 |
| [imbot-output.md](./imbot-output.md) | ImBot 输出预估 — Predicted Per-Platform Chat Output, and the Noise Problem | 当前矩阵 + 已标注的修复前推演 |
| [im-feishu.md](./im-feishu.md) | Feishu/Lark: IM Service Built Directly, Not via the Fat SDK Client | 设计说明；局部实施状态见正文 |
| [remote-cc-profile.md](./remote-cc-profile.md) | Remote @cc Profile Selection | 设计说明；局部实施状态见正文 |
| [desk.md](./desk.md) | Desk (MVP) | 设计说明；局部实施状态见正文 |
| [smart-guide-on-claude-code.md](./smart-guide-on-claude-code.md) | Smart Guide on Claude Code (retiring tingly-agentscope) | 已废弃方案；现行 runtime 见 afk |
| [smart-guide-react-anthropic-sdk.md](./smart-guide-react-anthropic-sdk.md) | Smart Guide: in-house ReAct loop on the official Anthropic SDK | 已交付的起源记录；现行设计见 afk |
## Image 与图形工具

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [image-edit.md](./image-edit.md) | Image Edit(/images/edits) | 设计说明；局部实施状态见正文 |
| [image-edit-adapters.md](./image-edit-adapters.md) | Image Edit 适配器(xAI / 千帆 / DashScope) | 设计说明；局部实施状态见正文 |
| [image-layout.md](./image-layout.md) | Image 顶级入口(Playground + Image API) | 设计说明；局部实施状态见正文 |
| [image-mask.md](./image-mask.md) | Image Mask(生图 Playground 的局部重绘) | 设计说明；局部实施状态见正文 |
| [image-slice.md](./image-slice.md) | Image Slice(生图 Playground 的切分下载) | 设计说明；局部实施状态见正文 |
| [playground-run-reentry.md](./playground-run-reentry.md) | Playground 运行卡片的重入(Re-entry) | 设计说明；局部实施状态见正文 |
| [sketch-canvas.md](./sketch-canvas.md) | Sketch Canvas(生图 Playground 的草图与人偶输入) | 设计说明；局部实施状态见正文 |
| [mannequin-standard.md](./mannequin-standard.md) | 人偶标准(Mannequin Standard) | 设计说明；局部实施状态见正文 |
| [pose-from-image.md](./pose-from-image.md) | 从一张图片得到姿势 | 关键点换算已实现；模型接入/交付待决定 |
## 观测与测试

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [usage-tracking.md](./usage-tracking.md) | Usage & Token Tracking | 设计说明；局部实施状态见正文 |
| [usage-analytics.md](./usage-analytics.md) | Usage Analytics | 设计说明；局部实施状态见正文 |
| [ttft.md](./ttft.md) | TTFT（Time To First Token）记录 | 设计说明；局部实施状态见正文 |
| [logging.md](./logging.md) | 日志系统：架构与上游错误分类 | 设计说明；局部实施状态见正文 |
| [recording.md](./recording.md) | Recording 梳理:意图、现状与整合方向 | Phase 1/1.5/2 完成；wire/EventTap 待做 |
| [otel.md](./otel.md) | OTel 可观测性设计 — internal/otel | 设计说明；局部实施状态见正文 |
| [probe.md](./probe.md) | Probe Subsystem | 设计说明；局部实施状态见正文 |
| [bench.md](./bench.md) | Bench — 高度可定制的端到端测试台 | 设计说明；局部实施状态见正文 |
| [test-infrastructure.md](./test-infrastructure.md) | Test Infrastructure | 设计说明；局部实施状态见正文 |
| [harness-matrix.md](./harness-matrix.md) | Harness Matrix | 设计说明；局部实施状态见正文 |
| [harness-duo.md](./harness-duo.md) | Harness Duo — two-process memory & protocol verification | 设计说明；局部实施状态见正文 |
| [harness-remote.md](./harness-remote.md) | Harness Remote — in-process IM chat e2e for remote/control/remoteagent | 设计说明；局部实施状态见正文 |
| [harness-agent-testing.md](./harness-agent-testing.md) | Runbook — tingly-box 阶段验收基线 | 设计说明；局部实施状态见正文 |
| [harness-npm.md](./harness-npm.md) | Harness npm — 用本地 registry 和虚拟版本号，从当前源码演练整条 npm 发布与安装链路 | 脚本 `build/npx/harness-npm.sh`；CI 里只有手动触发的 `harness-npm.yml`（Linux、macOS、Windows 三平台） |
| [rule-flag-testing.md](./rule-flag-testing.md) | Rule-Flag Behavior Testing | 设计说明；局部实施状态见正文 |
| [vmodel.md](./vmodel.md) | vmodel — design index | 入口索引 |
| [vmodel-benchmark.md](./vmodel-benchmark.md) | vmodel as a shared real-world benchmark | Phase 1–3 已完成 |
| [vmodel-sequence.md](./vmodel-sequence.md) | Virtual sequence models | 设计说明；局部实施状态见正文 |
| [vmodel-transport.md](./vmodel-transport.md) | vmodel transport — dispatch virtual providers over HTTP, not in-memory | 内存 listener 已实现；unix socket 未做 |
| [python-sdk.md](./python-sdk.md) | Python SDK (tingly) — v1 framework | 设计说明；局部实施状态见正文 |
## 产品、前端与宿主

| 文档 | 内容 | 状态 / 阅读边界 |
|---|---|---|
| [ux-principles.md](./ux-principles.md) | UX-First 原则 | 设计说明；局部实施状态见正文 |
| [onboarding-ux.md](./onboarding-ux.md) | Onboarding UX — guidance for first-time users | 设计说明；局部实施状态见正文 |
| [ui-redesign.md](./ui-redesign.md) | UI 重设计 — 决策、实施状态与后续 | 部分落地；有撤回及后续项 |
| [agent-page-redesign.md](./agent-page-redesign.md) | Agent 页重设计 — 模板、配置状态与实施边界 | A–D 落地；E 部分撤回；Profile 暂缓 |
| [ui-flow-analysis.md](./ui-flow-analysis.md) | UI 动线分析 — 发现、人工打标反馈与实现结论 | 第 1 轮已落地；其余项见正文 |
| [refactor-frontend-2026-09.md](./refactor-frontend-2026-09.md) | Frontend Refactor 2026-09 — Plan & Survey Reports (merged) | 已完成的历史审计与交付记录 |
| [icon-hierarchy.md](./icon-hierarchy.md) | Icon Hierarchy | 设计说明；局部实施状态见正文 |
| [theme.md](./theme.md) | Theme system notes | 设计说明；局部实施状态见正文 |
| [gui-host-bridge.md](./gui-host-bridge.md) | Host bridge: one frontend for the browser tab and the desktop window | 设计说明；局部实施状态见正文 |
| [gui-packaging.md](./gui-packaging.md) | Desktop GUI packaging | 设计说明；局部实施状态见正文 |
| [host-binding.md](./host-binding.md) | Host Binding: Design and Decision | 设计说明；局部实施状态见正文 |
| [runtime-port-file.md](./runtime-port-file.md) | Runtime Port File | 设计说明；局部实施状态见正文 |
| [cli-entry-semantics.md](./cli-entry-semantics.md) | CLI entry semantics: npx vs installed CLI, daemon default | 设计说明；局部实施状态见正文 |
| [npm.md](./npm.md) | npm distribution | 设计说明；局部实施状态见正文 |
| [npm-ci.md](./npm-ci.md) | 新增 npm 包：占位发布 → `npm trust` → CI 接手 | 操作流程；命令已按 npm 12 文档核对 |
| [shortcut.md](./shortcut.md) | Desktop Shortcut: Design and Decisions | 设计说明；局部实施状态见正文 |
| [tui.md](./tui.md) | Federated TUI: Design and Decisions | 设计说明；局部实施状态见正文 |
