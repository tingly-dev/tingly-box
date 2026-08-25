# internal/protocol 与 internal/client 的边界

Status: 惯例约定（无迁移动作，新代码按此执行；存量逐步收敛）
Date: 2026-08-25

## 动机

`internal/protocol` 和 `internal/client` 在 vendor quirk 上有真实的重叠：同一个需求
（例如"ChatGPT backend 不支持 `temperature`"）既可以写进
`protocol/transform/vendor.go`（类型化改），也可以写进 client 的 round tripper
（字节级改）。`transform/vendor.go` 中曾以注释互相让路
（`// MENTION: no need to do transform here, the codex client will handle this`），
说明缺少一条明确的判定规则。本文固定这条规则。

**不合并**这两个包：依赖是单向的（client → protocol），protocol 的 ~200 个转换
文件全部可纯函数式单测、不碰 HTTP；合并会把它们和 auth / 连接管理 / SDK 封装
搅在一起，得不偿失。

## 两包各自的职责

- **`internal/protocol`** — wire 级翻译层。APIType 之间的类型化转换
  （request / stream / nonstream 矩阵）、transform chain
  （Base / Consistency / Vendor）、assembler、SSE 帧、token 记账。
  操作对象是 SDK 的类型化请求（`anthropic.MessageNewParams`、
  `openai.ChatCompletionNewParams` 等）。**不碰 `http.Request`。**
- **`internal/client`** — 每个 provider 的 SDK client 封装 + HTTP 传输层。
  proxy、连接池、超时、logging round tripper，以及各家的 round tripper
  （Codex 路径重写与字段过滤、Kimi CLI 伪装头、Claude beta flag 白名单、
  Code Assist envelope）。

一句话：**protocol 回答"请求长什么样"，client 回答"怎么把它送出去"。**

## 边界判定规则

按"修改发生在 SDK 序列化之前还是之后"切分。判定时只问一句：
**这个改动需要 `http.Request` 吗？需要就是 client，不需要就是 protocol。**

| 改动的性质 | 归属 |
| --- | --- |
| 能在类型化请求对象上表达（SDK struct 有这个字段） | **protocol**（VendorTransform / `request/` 转换器） |
| SDK 类型表达不了：HTTP header、URL path、SDK 强制序列化的字段、响应格式的字节级重写、envelope 包装 | **client**（round tripper） |
| 跨 provider 的协议语义（APIType shape 转换、一致性规则） | **protocol** |
| 与"如何连上这个特定 endpoint"有关：auth、伪装、路径、重试、连接 | **client** |

新增 vendor quirk 时的顺序本身就是边界：**先走类型化路径（VendorTransform）；
只有 SDK 表达不了时，才允许开 round tripper。**

## 灰色地带：字节级 quirk 的归属模式

有些 quirk 必须在字节层做（round tripper 里），但它的转换逻辑本身不依赖
`http.Request`。这类逻辑的归属遵循 `protocol/ops` 已有的模式：

> **`protocol/ops` 拥有"改什么"（纯函数、可单测），
> client 的 round tripper 拥有"何时/在哪调用它"（transport hook）。**

`protocol/ops` 中的 `request_openai_codex.go`、`request_openai_kimi.go`、
`request_anthropic_claude_code_compat.go` 即是此模式；`client/claude_client.go`
import `protocol/ops` 调用它们，round tripper 退化为薄接线层。

存量收敛方向（非强制，顺手时做）：round tripper 中不依赖 `http.Request`
的纯字节转换函数（如 Codex round tripper 的 `filterField` gjson/sjson 字段过滤）
逐步下沉到 `protocol/ops`；round tripper 只保留 header / path / timing。

## PR review 检查项

- 新的 vendor quirk 是否先尝试了 VendorTransform？开 round tripper 的理由
  是否是"SDK 类型表达不了"？
- round tripper 里新增的转换逻辑是否依赖 `http.Request`？不依赖的部分
  是否放在了 `protocol/ops`？
- 是否引入了 protocol → client 的反向依赖？（禁止；现状仅 manual test 例外）
