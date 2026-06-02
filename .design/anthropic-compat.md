# Anthropic Compatibility Mode

## 背景

部分客户端（如使用较新特性的 SDK 版本）会在 `messages` 数组中混入 `role: "system"` 的消息条目，而非使用 Anthropic API 规定的顶层 `system` 参数。这是非标准用法：官方 Anthropic API 的 `messages` 只允许 `user` / `assistant` 两种 role。

当这类请求被转发到严格遵守规范的第三方 Anthropic 兼容 Provider 时，Provider 会因为不认识 `system` role 而直接报错，导致请求失败。

## 解决方案

引入 **Anthropic 兼容模式**（`anthropic_compat`），在请求转发前将 `messages` 数组中所有 `role == "system"` 的条目重写为 `role: "user"`，从而让第三方 Provider 能够正常接受请求。

## 架构

### 标志分层

| 层 | 字段 | 类型 | 语义 |
|----|------|------|------|
| ScenarioFlags | `AnthropicCompat bool` (json: `anthropic_compat`) | bool | 场景级开关，自动注入到该场景下所有 rule |
| RuleFlags | `AnthropicCompat bool` (json: `anthropic_compat`) | bool | rule 级开关，手动配置单条 rule |

场景 flag 通过 `resolveRuleFlagsWithScenario` 注入：
```go
flags.AnthropicCompat = flags.AnthropicCompat || scenarioConfig.Flags.AnthropicCompat
```

### Transform 链路

属于 **Type 1b-pre**（pre-Base Transform），作用于 inbound Anthropic 请求形态：

```
AnthropicCompatTransform  →  BaseTransform  →  ...  →  upstream
       ↑
  仅对 *anthropic.MessageNewParams 和
       *anthropic.BetaMessageNewParams 生效；
  其他类型 type-switch 直接 no-op
```

实现分两层：

- **Op 原语**（`internal/protocol/transform/ops/request_anthropic_compat.go`）
  - `ApplyAnthropicCompatRoleRewrite(*anthropic.MessageNewParams)` 
  - `ApplyAnthropicBetaCompatRoleRewrite(*anthropic.BetaMessageNewParams)`
  - 纯函数，无 rule / chain 感知

- **Transform**（`internal/protocol/transform/anthropic_compat.go`）
  - `AnthropicCompatTransform`，type-switch 后调用对应 op
  - 注册点：`internal/server/rule_flags.go::rulePreBaseTransforms`

### 为什么是 pre-Base

该变换必须在 BaseTransform（协议转换）之前执行：pre-Base 看到的是客户端原始发来的 Anthropic 形态，`messages[].role` 字段仍是客户端设置的原始值。若放到 post-Base，在 Anthropic→OpenAI 路径下，BaseTransform 已将 messages 转换为 OpenAI 格式，原始 role 字段语义已变，变换将失效。

## 使用场景

在场景配置（ScenarioConfig.Flags）中启用 `anthropic_compat: true`，即可对该场景下所有 rule 生效；也可以在单条 rule 的 `flags` 中手动启用，仅对该 rule 生效。

典型适用场景：
- 转发目标是严格 Anthropic 规范的第三方 Provider（如 Bedrock、Vertex、第三方代理）
- 客户端 SDK 版本较新，会在 messages 中写入 system role

## 与 CleanHeader 的对比

| | `clean_header` | `anthropic_compat` |
|-|---------------|-------------------|
| 作用对象 | `system` 参数中的计费 header 块 | `messages` 数组中的 system role 条目 |
| 触发方式 | 场景 flag 注入 + billing scenario 自动启用 | 场景 flag 注入，无自动推导 |
| 协议层位置 | pre-Base | pre-Base |
