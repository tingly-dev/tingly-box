# Image Edit(/images/edits)

> 适用对象:tingly-box 后端贡献者。
> 描述 image edit(改图)网关能力的设计:入站协议、vendor 分发、
> 以及 Codex(ChatGPT 订阅)原生 images 协议的适配。
> 关联文档:imagegen scenario 见 `../internal/vision/imagegen/imagegen.go`
> 包注释;Codex 请求链路见 `codex-auth.md` / `codex-config.md`。

---

## 1. 背景:为什么需要单独一条协议

image generation 早已就位(`/images/generations` surface),但
**edit(基于已有图片改图)** 一直缺失。补齐它的难点不在
OpenAI 兼容侧——SDK 直接支持 multipart `/v1/images/edits`——而在 Codex
(ChatGPT OAuth 订阅)侧:ChatGPT backend **不支持**公开的 multipart
edits 协议。

通过阅读 openai/codex 源码确认(而非猜测),Codex CLI 自身的改图链路是:

```text
模型调用 image_gen.imagegen (namespaced tool)
    ↓ 客户端接管(不是让 /responses 自己执行)
POST {base}/codex/images/edits        ← 独立 endpoint
    JSON: {images:[{image_url:"data:image/png;base64,..."}],
           prompt, model:"gpt-image-2",
           background/quality/size:"auto", n?}
    ↓
{created, data:[{b64_json}], background?, quality?, size?, usage}
```

源码依据(openai/codex,codex-rs):

| 事实 | 出处 |
|------|------|
| `POST images/generations` / `images/edits`,JSON body | `codex-api/src/endpoint/images.rs` |
| `ImageEditRequest{images[].image_url, prompt, model, background?, n?, quality?, size?}` | `codex-api/src/images.rs` |
| data URL 形式的 reference image;响应 `data[].b64_json` | 同上 + endpoint 单测 |
| 默认 model `gpt-image-2`;背景/质量/尺寸默认 `auto`;最多 5 张 reference | `ext/image-generation/src/tool.rs` |
| 请求头 `x-codex-image-turn-id`(响应头 `x-codex-imagegen-request-id`) | `ext/image-generation/src/backend.rs` / `endpoint/images.rs` |
| base URL `https://chatgpt.com/backend-api/codex` | `model-provider-info/src/lib.rs` |
| quality 枚举只有 low/medium/high/auto(无 standard/hd) | `codex-api/src/images.rs` |

以上结论在 2026-09-04 以 openai/codex commit
[`9d253c88`](https://github.com/openai/codex/commit/9d253c885cb7cc48aeb749a82e31e2070e14f73e)
重新核验。该版本的 `ImageGenerationRequest` 明确定义了 `prompt`、`background?`、
`model`、`n?`、`quality?`、`size?`；`ImagesClient::generate` 明确向相对路径
`images/generations` 发起 JSON POST；image-generation backend 则统一附加
`x-codex-image-turn-id`。这不是根据公开 OpenAI Images API 形状推测出来的适配。

关键结论:**Codex generation 与 edit 都走原生 JSON images endpoint；edit
额外使用 data URL，而不是公开 API 的 multipart**。generation 将 OpenAI 入站参数
转换为 `{prompt, model, background, n?, quality, size}`，其中 `n` 仅在调用方
显式设置时发送；原生响应的完整 `data[]` 按原顺序直接返回。

---

## 2. 分层设计

与 generation 完全同构,每层只加一个对称成员:

```text
POST /tingly/{scenario}/v1/images/edits            ← routes.go(mixin group)
    ↓ HandleOpenAIImageEdit                        ← 入站解析 + rule/service 选择
    ↓ forwarding.ForwardOpenAIImageEdit            ← 薄转发
    ↓ OpenAIClientInterface.ImagesEdit             ← vendor 分发点
        ├─ OpenAIClient  → SDK multipart /v1/images/edits(OpenAI 兼容上游)
        ├─ CodexClient   → JSON POST images/edits(原生协议,见 §3)
        ├─ KimiClient    → ErrKimiNotSupported
        ├─ vmodel        → not supported
        └─ DashScope/MiniMax → 明确报错(适配器无 edit surface)
```

scenario/transport 不需要新注册:`TransportImageGen` 覆盖整个
`/images/*` 面,新路由挂在同一个 mixin group 上(canonical home 仍是
`imagegen` scenario)。tracing 声明 operation 为 `image_edit`。

### 2.1 入站双编码

`HandleOpenAIImageEdit` 接受两种 Content-Type:

| 编码 | 场景 | image 形态 |
|------|------|-----------|
| `multipart/form-data` | 官方 SDK / curl -F(标准 wire 格式) | 文件字段 `image`(可重复)或 `image[]`,可带 `mask` |
| `application/json` | 程序化调用;把上一次 generation 的 `b64_json` 直接链回来改图 | `image` 为单个或数组的 data URL / 裸 base64 字符串 |

JSON 侧**拒绝** http(s) 远程 URL——网关不代为抓取任意 URL(SSRF),只
解码 inline 内容。两种编码解析到同一个 `openai.ImageEditParams`,后续
链路无分支。

### 2.2 持久化

edit 结果与 generation 共用 `persistImages`(configDir/image/YYYYMMDD/,
best-effort),sidecar 元数据多一行 `Operation: edit` 以便区分。

---

## 3. Codex 适配细节(决策注解)

### 3.1 复用 SDK http 管道,而不是另起裸 http.Client

`CodexClient.ImagesEdit` 通过 `openai.Client.Post(ctx, "images/edits", ...)`
发送,body 是本包定义的 `codexImageEditRequest`(镜像 codex-rs
`ImageEditRequest`)。这样 OAuth bearer、Account-ID header、超时、logging、
session-bound transport 全部免费继承——代价是 `codexRoundTripper` 必须
认识 images 端点(§3.2)。

输入图(SDK union 里的 `io.Reader`)在这里被读出、`http.DetectContentType`
嗅探、编码成 `data:{mime};base64,...`,与 Codex CLI 的行为一致(它也是
读本地文件转 data URL)。

### 3.2 codexRoundTripper 的 images 特例

RoundTripper 原本按"一切都是 Responses SSE"设计:强制注入
`stream:true/store:false`、拒绝非 SSE 的 200 响应。images 端点是普通
JSON 请求/响应,二者都会把它打死。所以:

- `rewriteCodexPath`/`rewriteCodexAPIPath` 现在返回 `(newPath,
  codexProtocol)` 二元组,而不是只返回路径。`codexProtocol` 是
  `codexProtocolResponsesSSE` / `codexProtocolPlainJSON` 两个值之一,path
  重写这一步就把"这条路径说哪种协议"判定完了;`RoundTrip` 只 switch 这
  一个值,不再用子串匹配从已重写的路径里反推协议(最初实现里
  `isCodexImagesPath` 就是这种反推,已删除——分类应该和重写同时产生,不
  是重写完之后再猜一次)。
- path 重写覆盖:`/backend-api/images/*` → `/backend-api/codex/images/*`
  (SDK base 是 `https://chatgpt.com/backend-api`,相对路径 `images/edits`
  落在前者);
- `codexProtocolPlainJSON` 命中时:跳过 body 过滤、跳过 `OpenAI-Beta:
  responses=experimental`、跳过 SSE 校验,JSON 响应原样透传;
- 非 200 错误处理保持共用。

> 加第三种 Codex 原生 JSON 端点时,只需要在 `rewriteCodexAPIPath` 里加
> 一个 case 返回 `codexProtocolPlainJSON`——不需要再碰 `RoundTrip` 内部
> 任何一处调用点。

### 3.3 参数归一

| OpenAI edit 参数 | Codex wire | 处理 |
|------|------|------|
| `quality: standard`/`hd` | 枚举无此值 | 归一为 `medium`/`high`(`normalizeCodexImageQuality`,generation 与 edit 两条路径共用同一份映射,定义于 `codex_images.go`) |
| `background`/`size` 未设 | — | 填 `auto`(Codex CLI 的默认) |
| `n` | `n?` | 原样透传(wire schema 支持,虽然 CLI 自己不传) |
| `mask` / `response_format` / `output_format` / `output_compression` / `input_fidelity` | 无 | 丢弃 + debug log |
| 超过 5 张 reference | 后端硬限 | 只 log 不截断——让后端明确报错,不静默变更语义 |

`x-codex-image-turn-id` 每次请求生成新 uuid——网关没有 Codex 的 turn
概念,fresh id 是合理的替身。

#### `n` 的安全默认值

Codex 官方 `ImageGenerationRequest` 把 `n` 定义为可选 `u64`，但官方
image-generation tool 构造 generation 和 edit 请求时都固定使用 `n: None`，
且其 tool 参数没有向模型暴露生成数量。tingly-box 因此采用相同默认行为：
调用方未设置 `n` 时不合成 `n: 1`，而是完全省略该字段，让 Codex backend
应用自身默认值。只有 OpenAI-compatible 入站请求显式携带 `n` 时才透传。

当前官方源码没有声明 generation `n` 的最大值（5 只适用于 edit reference
images），因此网关不臆造一个可能与 backend 漂移的上限；实际模型、账号和
配额限制由 Codex backend 统一拒绝。若产品需要成本护栏，应在 provider/rule
策略层增加可配置上限，而不是在 Codex wire adapter 中静默改写数量。

### 3.4 generation 原生 endpoint 与多图语义

`CodexClient.ImagesGenerate` 调用相对路径 `images/generations`，经
RoundTripper 重写为 `/backend-api/codex/images/generations`。请求与 edit 共用
quality/default 归一逻辑，并附带 `x-codex-image-turn-id`。显式 `n`（包括
`n: 2`）原样进入 Codex wire；未设置时省略。响应直接反序列化为
`openai.ImagesResponse`，除空 `data` 报错外不截断、不重建，因此上游
`data[]` 的数量、顺序和内容完整返回。

---

## 4. 关键文件索引

| 功能 | 文件 |
|------|------|
| Codex 原生 generation/edit 协议(types + request/response + data URL 转换) | `internal/client/codex_images.go` |
| RoundTripper images 特例 + path 重写 | `internal/client/codex_round_tripper.go` |
| 接口成员 + OpenAI 兼容实现 | `internal/client/openai.go` |
| Kimi / vmodel 的 not-supported 存根 | `internal/client/kimi_client.go`、`vmodel/client/openai.go` |
| 入站 handler(multipart + JSON 解析、校验) | `internal/protocolserver/openai_image_edit.go` |
| 持久化共用核心(`persistImages`) | `internal/protocolserver/openai_image.go` |
| 转发器 | `internal/protocolserver/forwarding/openai.go` |
| 路由注册 | `internal/protocolserver/routes.go` |
| 启动横幅 endpoint 打印 | `internal/server/server_lifecycle.go` |

---

## 5. 测试

| 层 | 用例 |
|----|------|
| Codex 请求构造 | 单图→data URL;多图+options;quality standard→medium/hd→high;n 透传;无图报错(`codex_images_test.go`) |
| RoundTripper | images path 重写 + 协议分类(`codexProtocol`);JSON body 不被注入 stream/store;JSON 200 透传;非 200 报错(`codex_images_test.go`) |
| 入站解析 | multipart `image`/`image[]`/字段;JSON data URL/裸 base64/数组;拒绝远程 URL;必填校验(`openai_image_edit_test.go`) |
| decodeInlineImage | 声明 mime / 嗅探 mime / 非 base64 data URL / 非法 base64 |
| 持久化 | edit sidecar 带 `Operation: edit`;generation 原测试不回归 |
| quality 归一 | `normalizeCodexImageQuality` 覆盖 standard/hd/high/low/auto/空 |

尚未覆盖(需真实 ChatGPT 订阅):对 `chatgpt.com/backend-api/codex/images/edits`
的端到端请求。上线前建议用 `codex_e2e_test.go` 的模式补一个 opt-in e2e。

### 5.1 `/simplify` 复审

提交后跑了一遍 `/simplify`(reuse / simplification / efficiency /
altitude 四个独立 agent)。已采纳:

- `decodeInlineImage` 改为复用 `internal/protocol/request.ParseImageURLToAnthropicSource`
  做 data URL 拆分,不再手写一遍 `data:`/`;base64,`/逗号解析。
- edit 路径的 quality 归一(原 `codexImageQuality`)与 generation 路径合并成
  `normalizeCodexImageQuality`——发现并修复了两者已经出现的漂移(edit
  路径此前漏了 `hd→high`)。
- `HandleOpenAIImageGeneration`/`HandleOpenAIImageEdit` 尾部的
  marshal→unmarshal→map→`c.JSON` 三次序列化去掉,直接
  `c.JSON(http.StatusOK, resp)`。
- `persistImageGeneration`/`persistImageEdit` 的 sidecar 文本拼装合并成
  共用的 `buildImagePersistMeta(imageMetaInfo{...})`。
- `parseImageEditMultipart` 去掉中间的匿名 struct 切片;`readMultipartFile`
  改用 `io.ReadAll`。
- `codexRoundTripper` 的协议判定从"重写路径后再子串匹配"改成"重写时直接
  返回分类"(见 §3.2)。

评估后跳过(记录理由,不是遗漏):

- **JSON 便捷编码下 base64 的 decode→encode 往返**(efficiency 发现
  #2):调用方传 data URL/裸 base64 时,`decodeInlineImage` 先解码成
  `[]byte`塞进 `openai.File`(实现 SDK 的 `io.Reader` 字段),
  `CodexClient.ImagesEdit` 再整体读出来重新 base64。要去掉这次往返需要
  绕开 SDK 的 `ImageEditParamsImageUnion`(它的字段类型就是
  `io.Reader`),把原始 base64 字符串一路串到 Codex 分支,单独给 Codex
  开一条不经过 SDK 类型的路径。收益(省一次内存拷贝)相对这条改动的复杂度
  不成比例,暂不做。
- **`HandleOpenAIImageGeneration`/`HandleOpenAIImageEdit` 整体合并**
  (simplification 发现 #1):两个 handler 在 scenario 校验→rule→service
  选择→tracking→forward→响应这段确实高度相似,但要素化需要 Go 泛型
  跨两个不同的 SDK 具体类型(`ImageGenerateParams`/`ImageEditParams`)取
  字段,SDK 类型本身没有存取器方法,只能靠调用方传 closure 把每个字段揉
  出来——这会把重复从"两个函数体"变成"两个函数各自的 closure 参数列
  表",可读性未必更好,而且这个仓库里其它并列 handler(chat/embeddings/
  responses/messages)也都是各自独立成篇,没有这种跨端点泛型合并的先
  例。保持现状,不引入这个仓库唯一一处的泛型 handler 抽象。
  为一次性收益引入这里唯一一处的泛型 handler 抽象不划算,跳过。
- **`readerToDataURL` 里对已知大小 reader 的 `io.ReadAll`**、**5 张
  reference image 并行读取**、**编辑结果落盘的同步 I/O**(efficiency 发
  现 #3 及两条 minor):量级有限(单请求路径、≤5 张图、已有的 generation
  持久化就是同步落盘的既有约定),收益不足以再引入并发/类型断言的复杂
  度,跳过。

---

## 6. 前端 / 后续

- 前端 imagegen playground 目前只有 generation;edit 的 UI(选图 + prompt)
  是后续工作,API 已就绪(`/tingly/imagegen/v1/images/edits`,JSON 编码对
  前端最友好)。
- 这些网关路由不在 swagger 管理范围内(swagger 只覆盖 `/api/v1` 管理面),
  无需 codegen。
