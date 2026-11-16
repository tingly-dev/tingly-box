# Antigravity Code Assist 适配设计

Tingly Box 通过 Google 格式客户端连接 Antigravity OAuth 服务。本设计规定
账号配置、请求封装、流式响应与失败处理的行为，供实现维护和回归验收使用。

## 范围与状态

当前实现包含 CLI 配置修复、Code Assist 请求适配和响应完整性修复。
本地客户端与 CLI 测试覆盖接口契约；真实账号的权限、模型可用性与配额尚未验证。
账号级模型发现、配额查询、元数据端点回退及多轮思维状态属于后续工作。

本文以接口和验收条件表达需求，避免依赖某个外部项目的代码结构、算法组织或命名。
前期研究曾接触外部实现资料；重写设计文档不构成严格人员隔离的 clean-room 证明。

## 账号配置

Antigravity 账号使用以下配置：

| 字段 | 契约 |
| --- | --- |
| `api_base` | `https://cloudcode-pa.googleapis.com` |
| `api_style` | `google` |
| OAuth issuer | `antigravity` |
| OAuth metadata `project_id` | 内容生成所需的 Code Assist 项目标识 |

CLI 和 Web 登录创建的账号应使用一致的服务地址与协议类型。
CLI 保存账号时必须保留 Google 协议类型；该要求也适用于 Gemini CLI OAuth。
保存流程须保留令牌及发现的项目元数据。

已有账号不自动迁移。修复其地址或协议类型时保留 provider UUID，避免破坏路由引用。
缺少 `project_id` 的账号应重新认证；生成调用须在联系服务端前给出可操作的错误。

## 请求契约

生成调用从 Google SDK 的模型路径转换到对应的内部操作路径：

| SDK 操作 | 服务端路径 |
| --- | --- |
| `generateContent` | `/v1internal:generateContent` |
| `streamGenerateContent` | `/v1internal:streamGenerateContent` |

保留 URL 查询参数。请求体必须是 JSON 对象，不能在解析失败后继续发送已被消耗的请求体。
封装字段如下：

| 字段 | 契约 |
| --- | --- |
| `project` | 账号的项目标识 |
| `model` | 从生成请求路径取得的模型标识 |
| `request` | SDK 请求对象，移除其中的顶层 `model` 字段 |
| `requestId` | `agent/<conversation>/<unix milliseconds>/<unique ID>/<message count>` |
| `userAgent` | `antigravity` |
| `requestType` | 按下表选择，可省略 |

`requestType` 的选择以请求内容为依据：

| 条件，按优先级排列 | 值 |
| --- | --- |
| 输出模态包含 IMAGE，忽略大小写 | `image_gen` |
| 存在非空工具定义，或历史消息包含函数调用或函数结果 | `agent` |
| 其余请求 | 省略，使用服务端默认行为 |

同一 provider 的同一显式会话使用稳定的 conversation 标识；不同 provider 或会话相互隔离。
该标识不包含原始会话值或客户端 IP。空会话和仅有 IP 回退的请求不共享稳定会话标识。
每次生成请求仍须有独立的完整 request ID，避免会话稳定性变成请求 ID 复用。
该 conversation 标识不等同于服务端的多轮状态管理能力。

请求内容中的大整数、小数、工具结果和扩展字段须保持 JSON 值的精度。
适配器不得通过浮点数中转任意 JSON 数字。

## HTTP 行为

遵守 Go `http.RoundTripper` 的调用约定：复制调用方的 URL 和 headers 后修改，
不改变 SDK 持有的原始对象；正确关闭消费过的请求体，并提供与出站请求体一致的
`GetBody` 和 `ContentLength`。

SDK API key 转换为 Bearer 认证后从 headers 移除。没有 SDK key 时保留已有 Bearer 认证。
清除 SDK 客户端标识、quota-project header 和失效的显式长度字段；保留业务 headers。
生成请求的服务端 User-Agent 与当前项目发现流程保持一致，不能据此推断客户端版本最新。

## 响应与流式处理

成功的普通 JSON 响应提取顶层 `response` 字段；没有该字段或解析失败时原样传递。
更新响应体后同步长度属性与 header。

成功的流式响应按完整 SSE 行处理，不能把一次网络读取当作一条完整消息。
只展开 `data:` 行中的 JSON `response` 封装；保留注释、事件字段、空行、结束标记、
无封装 JSON 和无法解析的数据。

读取行为必须满足：

- 任意网络分片和 UTF-8 字符边界都不改变最终输出。
- 保留 LF 或 CRLF 行尾，不额外插入换行。
- 支持超过普通扫描器默认大小的单条事件。
- 最后一行即使没有换行也能输出；同次读取附带错误时，先交付已读数据再报告错误。
- 消费方使用任意缓冲区大小时，输出和结束语义一致。

流式变换后无法预知总长度，应清除原响应长度。
非成功响应不按 SSE 展开；保留状态码、错误体与 `Retry-After`，交给现有策略层处理。
适配器内不额外增加账号轮换或重试循环。

SSE 和 JSON 展开由 Antigravity 与 Gemini CLI 共享，修改时须覆盖两条路径的回归。

## 验收与验证边界

```sh
go test ./internal/client ./internal/command -count=1
```

现有 HTTP 测试服务器需要本地网络权限。验收至少覆盖：

| 场景 | 判定条件 |
| --- | --- |
| CLI 创建 Antigravity 和 Gemini 账号 | 保存 Google 协议，保留 issuer 和项目元数据 |
| 请求封装与回放 | 路径、查询参数、JSON 内容和长度正确；调用方对象不被修改 |
| 普通、工具和图像请求 | 按请求内容选择 request type |
| 会话标识 | 显式会话稳定且隔离，IP 回退不形成共享会话 |
| 非法或不完整配置 | 错误请求不发送到服务端 |
| SSE 网络分片 | 遍历样本的每个切分点，并覆盖逐字节读取 |
| SSE 边界与异常 | UTF-8、CRLF、大事件、无换行末行和终止错误均保持内容 |
| JSON 数字 | 大于整数安全浮点范围的值和长小数不丢失精度 |
| 服务端错误 | 错误内容与重试提示保持可用 |

本地回归只能证明适配器满足上述契约。真实账号验证需单独记录授权环境、测试输入、
脱敏的请求与响应及预期结果，不将单元测试通过表述为服务端兼容性保证。

## 后续设计输入

新增协议行为应先形成可审阅的接口证据，包括服务方公开规范、获授权的黑盒观测、
脱敏测试样本，以及能够重现的输入和输出。规格应分开标明观察事实、产品选择和待验证假设。
若后续要求严格的 clean-room 流程，须由未接触受限实现材料的实施者仅依据批准的规格工作，
保留输入清单和独立验证记录；不能用删除引用或改写术语替代该隔离过程。
