# protocol/transform 架构设计

> 描述 `internal/protocol/transform/` 与 `internal/protocol/ops/` 两个包的职责边界、
> 分层规则和扩展约定。新增或修改协议变换时以本文为准。

---

## 1. 包结构

```
internal/protocol/
  ops/           ← 协议级纯函数库；可被任意层调用
  transform/     ← Transform 接口实现；仅感知 TransformContext / 链路位置
  transform/ops/ ← 不存在；勿创建（见 §3）

internal/server/
  transform_*.go ← server-domain Transform；可读 ScenarioConfig / runtime.Runtime
```

**三层分工**：

| 层 | 包 | 职责 | 可依赖 |
|----|----|------|--------|
| 纯函数 op | `protocol/ops` | 字段级 mutation；不感知链路 | SDK 类型、`protocol/` 自身 |
| Protocol Transform | `protocol/transform` | type-switch 请求类型、调 op 或内联辅助 | `protocol/ops`、SDK 类型 |
| Server Transform | `server/transform_*.go` | 读配置 / runtime，组合 op | `protocol/ops`、`protocol/transform`、`typ`、`runtime` |

---

## 2. Transform 文件的标准结构

```go
type FooTransform struct { /* 构造期配置，无运行时状态 */ }

func NewFooTransform(...) *FooTransform { ... }
func (t *FooTransform) Name() string    { return "foo" }

func (t *FooTransform) Apply(ctx *TransformContext) error {
    switch req := ctx.Request.(type) {
    case *SomeShape:
        applyFooForSomeShape(req, ...)   // 本文件 unexported 函数，或 ops.ApplyFoo(...)
    }
    return nil
}
```

`Apply()` 只做 type-switch + 分发，不包含字段操作逻辑本身。

---

## 3. 纯函数的放置规则

写好一个字段 mutation 后，按以下规则决定放在哪里：

```
被 server-domain 或 2+ 个 Transform 调用  →  protocol/ops/（exported ApplyXxx）
只被本 Transform 文件使用                →  同文件 unexported 函数
```

**不要**为每个 Transform 单独创建 `ops/` 文件——`protocol/ops/` 是跨层共享 API，
不是 Transform 内部实现的存放地。

判断标准：函数签名里是否出现 `*TransformContext`、`*typ.ScenarioConfig`、
`*runtime.Runtime` 等非 SDK 类型。
- 不出现 → 可以是纯函数，按上面规则二选一。
- 出现 → 留在 Transform 或 Server Transform 里。

---

## 4. 现有文件状态

### 4.1 符合规范的 Transform ✅

| 文件 | 纯函数位置 |
|------|-----------|
| `tool_block.go` | 同文件 unexported（5 个 shape）|
| `claude_code_compat.go` | 同文件 unexported |
| `openai_max_tokens_rewrite.go` | 同文件 unexported |
| `openai_cursor_compat.go` | `protocol/ops`（`ApplyCursorCompatContentNormalization`，被 vendor 共用）|
| `vendor.go` | `protocol/ops`（model transform、codex、provider transforms）|

### 4.2 待整理的文件 ⚠️

| 文件 | 现状 | 建议方向 |
|------|------|---------|
| `consistency.go` | 711 行，4 shape × 4 类操作全内联 | 按 shape 拆为 `consistency_openai_chat.go` 等；可纯函数化的部分放 `protocol/ops/` |
| `base.go` | ~300 行，协议转换大开关 + 字段辅助混在一起 | 抽 `base_thinking.go` / `base_stop.go` 等辅助文件；主开关不拆 op |
| `rule_thinking.go` | 144 行，thinking budget 逻辑内联且 export 给 server-domain | 内部辅助改为 `protocol/ops/request_thinking.go` 纯函数；消除跨包 export 耦合 |
| `vendor.go` | Responses 路径仍有内联 Codex 字段处理 | 移入已有的 `protocol/ops/request_openai_codex.go` |

---

## 5. 不在范围内的事

- **不重命名** Transform 接口或 `chain.go` 的结构。
- **不合并** `protocol/transform/` 与 `server/transform_*.go`——协议层（SDK-only）与 server-domain 层的分离有意义。
- **不为 MCP / runtime 依赖**的 Transform ops 化——依赖 `*runtime.Runtime` 的 mutation 不满足纯函数语义。
- **不引入**"OpTransform 自动包装"之类的泛型 shell——现有抽象已足够，再加一层只增复杂度。

---

## 6. 依赖方向约束

```
protocol/ops        ← 不 import server/、typ/ 以外的内部包
protocol/transform  ← 不 import server/
server/transform_*  ← 可 import 以上两层
```

可用 `go vet` + `golang.org/x/tools/go/analysis/passes/slog`（或 `depguard`）做静态检查。
