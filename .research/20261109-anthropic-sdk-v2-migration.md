---
date: 2026-11-09
---

# Research: anthropic-sdk-go v1 → v2 升级

## Summary

Submodule 已切至 `tingly-dev-v2.7.0`(v2.7.0 + 3 个 fork 补丁全部随行,无功能损失)。导入路径机械替换已完成并分两个 commit 提交;语义适配(角色联合类型、采样参数)按既定模式进行中。

## 决策(用户拍板,2026-11-09)

1. **保留老版本行为为默认**:`anthropic-version: 2023-06-01`(官方之外的三方兼容生态都还是老版本)。
2. **新建一组 Client V2**(`xxxV2` 命名)面向新 API 版本 `2026-10-21`;官方 Provider 实现 V2 支持,**通用版本实现 V2 备用**。
3. 被删参数(temperature/top_p/top_k)通过 `SetExtraFields`/`option.WithJSONSet` 以旧 flag 方式填充。
4. 认证头新旧交替(`x-api-key` vs `Authorization: Bearer`)。

## v1 → v2 差异与影响面

| v2 变更 | 影响点 | 处理 |
|---|---|---|
| 模块路径 `+/v2` | 全仓库 275 文件 | 机械替换(commit `0f5eabd7f`) |
| `MessageParam`/`BetaMessageParam` → 角色联合类型(无 `.Role`/`.Content`) | internal/protocol、token、round、request 转换器、afk、~434 处构造点(大半是测试) | shim:`internal/protocol/msgparam`(afk 有一份本地拷贝 `afk/msgparam`,因其不依赖根模块) |
| `Temperature`/`TopP`/`TopK` 从 params 删除 | thinking/reconcile.go、*_to_responses 转换器、afk/engine.go | params extras 机制:`req.ExtraFields()` 读 / `req.SetExtraFields()` 写 / `param.Omit` 删 |
| 默认 API 版本 `2026-10-21` | 所有 SDK egress | 老 Client 需显式 `anthropic-version: 2023-06-01`(claude_round_tripper.go 已有);V2 Client 组用新默认 |
| 默认 `Authorization: Bearer` | 通用 `WithAPIKey` 仍发 `x-api-key`;ClaudeClient 显式设头;kimi 用 `WithAuthToken` | 低风险,新旧交替待 Client V2 时统一 |
| TextCompletions 移除 | 无使用 | 无影响 |

## 进度

- [x] submodule → tingly-dev-v2.7.0 + .gitmodules/Taskfile/go.mod wiring(commit `839f87e32`)
- [x] /v2 导入路径重写(commit `0f5eabd7f`,纯机械,322+/322-)
- [x] `msgparam` shim 包(v1 + Beta 访问器/构造器)(commit `3b25f3613`)
- [x] thinking/reconcile.go:采样 reconcile 改走 extras(含 `param.Omit` 丢弃)
- [x] afk(engine/harness)、agentboot、projects 编译通过
- [x] root 模块全部包迁移(13 包,含 request 转换器、guardrails、toolengine、vision、vmodel)
- [x] 老 Client 显式锁定 2023-06-01(commit `3d420baac`)
- [x] 测试文件迁移(51 文件,go vet 全绿)(commit `0b4a94583`)
- [x] wire 兼容修复:x-api-key 认证头 + beta 出口 `?beta=true`(commit `def27dbea`)
- [x] 采样参数解码丢失修复:fork 补丁 apijson 保留 unknown key 到 extras(submodule `f108011`,root `ef22b3a6f`);Beta→V1 降级 guard 改为按字段声明判定
- [x] Client V2 组:`AnthropicClientV2` + `ClaudeClientV2`(2026-10-21 / Bearer / 剥离采样 extras;Claude V2 保留 `?beta=true`,属 CC wire 指纹)
- [x] **V2 暂不启用**(2026-11-10 决策):pool 一律走 classic,`TODO(v2-rollout)` 标注启用点;官方 API 目前仍接受旧 wire
- [x] 各 module 构建轮测:root/afk/agentboot/projects/remote/imbot/ai/swagger/gui-wails3 全过
- [x] harness `matrix --mode=all`:5647 用例,Fail 0(2 个迁移前已知 gap)
- [ ] fork 补丁 push 到 origin/tingly-dev-v2.7.0(需人工,含 apijson extras 补丁 f108011)
- [ ] 真机回归(用户后续完成):V2 路径、历史回放新限制、旧响应字段核对
- [ ] V2 启用时的通用 Provider 备用开关设计

## 风险与注意

- 反序列化成功 ≠ 兼容:旧响应字段可能被忽略、旧请求字段 v2 类型无法表达——需核对实际发送 JSON 与业务依赖的返回字段。
- 新版 API 对 assistant document、user 非空 citations 有新限制,重点测历史消息回放。
- Beta system 消息 content 是不同联合(text/tool-addition/tool-removal),shim 里只保留 text 往返。
