# Understand: Task Runtime

**Date**: 2026-07-15
**Scope**: `internal/task`, `internal/data/db/task_store.go`, `agentboot`, server/frontend integration seams

## Summary

仓库已经拥有 Task v0 所需的多数底层积木，但它们尚未形成产品：

- `internal/task` 已实现持久任务、延时调度、取消、重试与 workspace 级串行能力；
- `agentboot` 已实现可取消的 agent process supervisor、结构化事件流和人工 approval/ask 回调；
- Claude driver 已支持固定 workspace、显式 session ID 和 resume；
- Codex 只有配置能力，没有 `agentboot` execution driver；
- Task manager 当前没有 server/API/UI wiring。

## Most Important Design Consequence

不需要另建通用 automation 引擎。最短生产路径是：

```text
HTTP/UI
  → internal/task.Manager
  → agent Task Handler
  → agentboot Claude/Codex worker
  → structured outcome
  → internal/task 状态迁移及下次 ScheduledAt
```

`internal/task` 当前唯一必须先改的抽象，是“handler 成功必然 succeeded”。Task supervisor 需要允许一次执行返回：完成、等待下次唤醒、等待人工输入。

## Existing Assets to Reuse

1. `tasks` 表与 GORM store。
2. `ScheduledAt` 作为下一次具体唤醒时间。
3. `SerializationKey` 作为 canonical workspace identity。
4. `Payload` 保存 Task definition；`Result` 保存最新 outcome。
5. `agentboot.ExecutionHandle` 的 event/control/cancel/wait 接口。
6. Claude `SessionID + Resume + ProjectPath` 支持。

## Required New Work

1. 扩展 Task 状态和 handler outcome，支持 `sleeping` 与 `needs_input`。
2. 增加 agent Task payload/result 类型及 handler。
3. 增加 Codex agentboot driver/transport。
4. 在 server lifecycle 启停 manager 并注册 API。
5. 新增实验开关、Task 页面及 API placeholder。

## Risk Notes

- native session 和 workspace 必须一一映射，禁止同一 session 并发 resume。
- 人工问题不能阻塞 server goroutine；应持久化为 `needs_input` 后结束本轮进程。
- 自动 retry 只处理启动失败、暂时性 IO/CLI 错误；agent 主动 `continue` 属于下一轮 wake-up，不消耗 retry attempt。
- v0 不应解析或搬运完整 native transcript。
