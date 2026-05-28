# Image API Extension Design

> Audience: tingly-box backend contributors working on `internal/imagegen`, `internal/server/openai_images*.go`, and `internal/client`.
> This document covers the roadmap for extending image-related API support beyond the current `POST /images/generations` surface.

---

## 1. 现状梳理

### 1.1 已支持的接口

| 端点 | 路由 | 厂商适配 |
|---|---|---|
| `POST /images/generations` | `/tingly/{scenario}/images/generations` | OpenAI-compat（SDK直通）、DashScope（异步任务轮询）、MiniMax（自定义同步）、Codex（Responses API image_generation tool） |

### 1.2 OpenAI 完整 Image API 现状

OpenAI Image API 共 3 个端点（2025–2026）：

| 端点 | 模型支持 | 格式 | 当前状态 |
|---|---|---|---|
| `POST /v1/images/generations` | dall-e-2, dall-e-3, gpt-image-1/1-mini/1.5/2, chatgpt-image-latest | JSON body | ✅ 已实现 |
| `POST /v1/images/edits` | dall-e-2, gpt-image-1/1-mini/1.5, chatgpt-image-latest | multipart/form-data | ❌ 未实现 |
| `POST /v1/images/variations` | dall-e-2 only | multipart/form-data | ❌ 未实现 |

OpenAI Responses API 的 `image_generation` **tool** 已通过 Codex 路径支持，也覆盖了 `action:"edit"` 的能力，但当前只暴露了 generation，未暴露 edit action。

### 1.3 主流图像厂商覆盖现状

| 厂商 | 接入状态 | 能力范围 |
|---|---|---|
| OpenAI（compat） | ✅ 接入，仅 generation | /images/generations |
| Codex（ChatGPT OAuth） | ✅ 接入，仅 generation | Responses API image_generation tool |
| DashScope（阿里） | ✅ 接入，仅 generation | 异步文生图 |
| MiniMax | ✅ 接入，仅 generation | 同步文生图 |
| Stability AI | ❌ 未接入 | Core / Ultra / SD3 文生图；Inpaint；Outpaint；Upscale；Remove BG |
| Ideogram | ❌ 未接入 | Generate；Edit（inpaint）；Remix；Upscale；Describe |
| Google Imagen（Gemini API） | ❌ 未接入 | Imagen 4 文生图；Gemini 内嵌图像生成（generateContent） |
| Fal.ai / FLUX | ❌ 未接入 | FLUX.1 dev/pro/schnell；Kontext（image-to-image）；Fill；Erase |

---

## 2. 扩展范围

按优先级分三个批次：

### Batch A — OpenAI API 面补全（高优）

补全 OpenAI 官方 Image API 的两个缺失端点，让 gateway 对外是完整的 OpenAI-compat proxy。

1. `POST /images/edits` — 图像编辑 / inpainting
2. `POST /images/variations` — 变体（dall-e-2 only）

### Batch B — Codex image edit（高优）

Codex 已通过 Responses API 支持 `action:"edit"`（`input_fidelity`、mask、image in context），但当前 `CodexClient.ImagesGenerate` 只构造 generation，需扩展以支持 edit 路径。

### Batch C — 新厂商适配（中优）

按能力复杂度由低到高：

1. **Google Imagen**：`generateImages()` 简单 JSON，优先级最高（Gemini API 已有部分接入基础）
2. **Stability AI**：multipart/form-data，能力最全面（inpaint / upscale / remove-bg）
3. **Ideogram**：JSON + multipart 混合，独特能力（text rendering、describe）
4. **Fal.ai / FLUX**：异步 job 模式，FLUX Kontext 是最强的 image-to-image 模型

---

## 3. 数据模型扩展

### 3.1 `imagegen.Request` 新增字段

```go
// ImageInput represents a single image input (source image or mask).
// Exactly one of Data or URL should be set.
type ImageInput struct {
    Data     []byte // raw image bytes (PNG/JPEG/WebP)
    MimeType string // "image/png" | "image/jpeg" | "image/webp"
    URL      string // alternative to Data: https:// URL or data: URI
    Name     string // filename hint for multipart uploads
}

type Request struct {
    // === 已有字段 ===
    Model          string
    Prompt         string
    N              int
    Size           string
    Quality        string
    Style          string
    ResponseFormat string
    Background     string
    OutputFormat   string
    User           string
    Extra          map[string]any

    // === 新增字段 ===
    // Action distinguishes the operation type.
    // "generate" (default) | "edit" | "variation"
    Action string

    // Images holds the source image(s) for edit/variation requests.
    // For /images/edits with GPT image models: up to 16 images; for dall-e-2: exactly 1.
    // For /images/variations: exactly 1 image (must be square PNG < 4MB).
    Images []ImageInput

    // Mask is the inpainting mask for edit requests.
    // Black regions = areas to edit; white regions = preserve.
    // Required for dall-e-2 edits; optional for GPT image models.
    Mask *ImageInput

    // InputFidelity controls how closely the model preserves source image features.
    // "high" | "low" (default). Only for gpt-image-1 and later.
    InputFidelity string

    // Moderation controls content filtering sensitivity.
    // "auto" (default) | "low". Only for GPT image models.
    Moderation string
}
```

### 3.2 `imagegen.Client` 接口扩展

```go
type Client interface {
    // Generate produces images from a text prompt (current, action="generate").
    Generate(ctx context.Context, req *Request) (*Response, error)

    // Edit produces images by editing/inpainting a source image.
    // Vendors that do not natively support editing should return ErrEditUnsupported.
    Edit(ctx context.Context, req *Request) (*Response, error)

    // Variation produces variations of a source image (dall-e-2 only).
    // Vendors that do not support variations should return ErrVariationUnsupported.
    Variation(ctx context.Context, req *Request) (*Response, error)

    Provider() *typ.Provider
    Vendor() Vendor
    Close() error
}

var (
    ErrUnsupported         = errors.New("imagegen: provider does not support image generation")
    ErrEditUnsupported     = errors.New("imagegen: provider does not support image editing")
    ErrVariationUnsupported = errors.New("imagegen: provider does not support image variations")
)
```

默认实现：在 `imagegen.go` 中提供 `UnsupportedEdit` / `UnsupportedVariation` 嵌入结构，让已有适配器只需实现它们支持的方法：

```go
// UnsupportedEdit is an embed helper that returns ErrEditUnsupported.
type UnsupportedEdit struct{}
func (UnsupportedEdit) Edit(_ context.Context, _ *Request) (*Response, error) {
    return nil, ErrEditUnsupported
}

// UnsupportedVariation is an embed helper that returns ErrVariationUnsupported.
type UnsupportedVariation struct{}
func (UnsupportedVariation) Variation(_ context.Context, _ *Request) (*Response, error) {
    return nil, ErrVariationUnsupported
}
```

### 3.3 `OpenAIClientInterface` 扩展

```go
type OpenAIClientInterface interface {
    // ... 现有方法 ...

    // ImagesEdit forwards image edit/inpainting requests.
    // multipart/form-data: image(s) + optional mask + prompt params.
    ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error)

    // ImagesVariation forwards image variation requests (dall-e-2 only).
    // multipart/form-data: image + optional params.
    ImagesVariation(ctx context.Context, req openai.ImageVariationParams) (*openai.ImagesResponse, error)
}
```

**注意**：`openai.ImageEditParams` 和 `openai.ImageVariationParams` 是 openai-go SDK 的现有类型，含文件上传字段（`io.Reader`）。如果 SDK 版本未暴露这两个 params 类型，需在 `imagegen` 包内自定义对应结构体，镜像 OpenAI wire format。

---

## 4. HTTP 层新端点（Batch A）

### 4.1 路由注册

```go
// server_routes.go — 在 mixin group 里追加：
group.POST("/images/edits",     s.getModelAuthMiddleware(), s.HandleOpenAIImageEdit)
group.POST("/images/variations", s.getModelAuthMiddleware(), s.HandleOpenAIImageVariation)
```

两个端点与 `/images/generations` 共用同一 scenario 检查（`ScenarioImageGen` + `TransportImageGen`）。

### 4.2 `HandleOpenAIImageEdit`

Request 格式：`multipart/form-data`（OpenAI wire format）

```
image:   file (PNG/JPEG/WebP, ≤25MB for GPT models, ≤4MB for dall-e-2)
mask:    file (PNG, same size as image, optional for GPT models)
prompt:  string (required)
model:   string (required)
n:       int    (1–10, default 1)
size:    string (1024x1024 | 1024x1536 | 1536x1024 | 256x256 | 512x512)
quality: string (auto | low | medium | high)
response_format: string (url | b64_json) [dall-e-2 compat]
output_format:   string (png | jpeg | webp) [GPT image models]
background:      string (transparent | opaque | auto) [GPT image models]
```

处理流程：
1. 解析 multipart form，提取文件和字段
2. 构造 `imagegen.Request{Action: "edit", Images: [...], Mask: ...}`
3. 路由选取 provider + service（复用 `SelectServiceForImageGeneration`）
4. 分发到 client：
   - OpenAI-compat → `wrapper.ImagesEdit(ctx, openaiEditParams)`
   - Codex → `CodexClient.ImagesEdit(ctx, ...)` → Responses API with `action:"edit"`
   - DashScope → native edit if supported, else `ErrEditUnsupported` → 400
   - MiniMax → native edit if supported, else `ErrEditUnsupported` → 400
5. 返回 `openai.ImagesResponse` JSON（与 generations 一致）

### 4.3 `HandleOpenAIImageVariation`

Request 格式：`multipart/form-data`

```
image:   file (PNG, square, ≤4MB, required)
model:   string (default dall-e-2, only dall-e-2 supported)
n:       int    (1–10)
size:    string (256x256 | 512x512 | 1024x1024)
response_format: string (url | b64_json)
```

处理流程同 edit，调用 `wrapper.ImagesVariation(ctx, ...)`；非 dall-e-2 model 直接返回 400。

---

## 5. Codex image edit via Responses API（Batch B）

### 5.1 OpenAI Responses API image_generation tool 的 action 参数

```json
{
  "tools": [{
    "type": "image_generation",
    "size": "1024x1024",
    "quality": "high",
    "background": "transparent",
    "output_format": "png",
    "input_fidelity": "high",
    "action": "edit"   // "generate" | "edit" | "auto"
  }],
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": [
        {
          "type": "input_image",
          "image_url": "data:image/png;base64,..."  // source image
        },
        {
          "type": "input_image",
          "image_url": "data:image/png;base64,..."  // mask image (optional)
        },
        {
          "type": "input_text",
          "text": "remove the background"           // edit prompt
        }
      ]
    }
  ]
}
```

### 5.2 `CodexClient.ImagesEdit` 实现要点

```go
func (c *CodexClient) ImagesEdit(ctx context.Context, req openai.ImageEditParams) (*openai.ImagesResponse, error) {
    // 1. 将 req.Image(s) 读为 base64 data URI
    // 2. 如有 Mask，同样转 base64
    // 3. 构造 Responses API request：
    //    - tool: image_generation with action:"edit"
    //    - input: [input_image(source), input_image(mask?), input_text(prompt)]
    // 4. c.OpenAIClient.ResponsesNewStreaming(ctx, req)
    // 5. parseImageGenerationStream — 复用现有解析逻辑
}
```

**关键转换**：source image 和 mask image 都以 `input_image` content part 传入；mask 放在 source 之后（Responses API 用位置区分）。

---

## 6. 新厂商适配器（Batch C）

### 6.1 Stability AI (`VendorStabilityAI`)

**检测**：`host == "api.stability.ai"`

**能力矩阵**：

| 功能 | 端点 | Request 格式 |
|---|---|---|
| 文生图 Core | `POST /v2beta/stable-image/generate/core` | multipart/form-data |
| 文生图 Ultra | `POST /v2beta/stable-image/generate/ultra` | multipart/form-data |
| 文生图 SD3 | `POST /v2beta/stable-image/generate/sd3` | multipart/form-data |
| Inpainting | `POST /v2beta/stable-image/edit/inpaint` | multipart/form-data |
| Outpainting | `POST /v2beta/stable-image/edit/outpaint` | multipart/form-data |
| Remove Background | `POST /v2beta/stable-image/edit/remove-background` | multipart/form-data |
| Upscale (Conservative) | `POST /v2beta/stable-image/upscale/conservative` | multipart/form-data |
| Upscale (Creative) | `POST /v2beta/stable-image/upscale/creative` | multipart/form-data（异步） |

**Auth**：`Authorization: Bearer {api_key}`

**Response**：
- `Accept: image/*` → 直接返回图像二进制
- `Accept: application/json` → `{ artifact: "<base64>" }`

**规范化映射（Generate）**：

```
Request.Size    → aspect_ratio（1024x1024→"1:1"；1792x1024→"16:9"；…，无法整除则报错）
Request.Prompt  → prompt
Request.Quality → style_preset（hd→"photographic"；其他不映射）
Request.Model   → 决定选择 core / ultra / sd3 端点
Request.N       → 不支持 n>1（Stability API 每次只生成 1 张，N>1 需客户端循环调用）
```

**`stabilityClient` 实现**：
- `Generate` → 选择对应端点，POST multipart，解析 base64 artifact
- `Edit` → 根据 `Request.Mask` 存在与否选 inpaint，否则 remove-background
- `Variation` → 返回 `ErrVariationUnsupported`
- Upscale Creative 是异步（返回 `id`，需 `GET /v2beta/stable-image/upscale/creative/result/{id}` 轮询），单独走 polling loop

**Vendor 检测追加**：

```go
// vendor.go
case strings.Contains(host, "api.stability.ai"):
    return VendorStabilityAI
```

### 6.2 Ideogram (`VendorIdeogram`)

**检测**：`host == "api.ideogram.ai"`

**能力矩阵**：

| 功能 | 端点 | Request 格式 | Response |
|---|---|---|---|
| 文生图 | `POST /generate` (v2) 或 `POST /v1/ideogram-v3/generate` (v3) | JSON | JSON with image URLs |
| 编辑（inpaint） | `POST /edit` | multipart/form-data | JSON |
| Remix（image-to-image） | `POST /remix` (v2) 或 `POST /remix-v3` (v3) | multipart/form-data | JSON |
| Upscale | `POST /upscale` | multipart/form-data | JSON |
| Describe（vision） | `POST /describe` | multipart/form-data | JSON captions |

**Auth**：`Api-Key: {api_key}`

**规范化映射（Generate v3）**：

```
Request.Prompt    → generation_request.prompt
Request.N         → generation_request.num_samples (default 1, max 8)
Request.Size      → generation_request.aspect_ratio（WxH → 枚举"ASPECT_1_1","ASPECT_16_9"...）
Request.Style     → generation_request.style_type（vivid→"REALISTIC"；natural→"GENERAL"）
Request.Model     → 版本选择（"V_2"/"V_2_TURBO"/"V_3"）
Request.Moderation → magic_prompt_option（"ON"/"OFF"/"AUTO"）
```

**生成 v2 vs v3**：通过 `Request.Model` 前缀判断（model 包含 "v3" 则走 v3 路径）。

**Describe 暴露**：为了让 gateway 支持 "图像描述" 能力，可以在 `imagegen.Response` 中增加 `Descriptions []string` 字段（用于 describe-only 请求），或作为 `Image.RevisedPrompt` 填充。这是扩展能力，Phase 1 先 stub 留接口。

### 6.3 Google Imagen (`VendorGoogleImagen`)

**检测**：`host == "generativelanguage.googleapis.com"` 且 model 包含 "imagen"

**注意**：当前 `VendorOpenAICompat` 已覆盖 `googleapis.com` 的 Gemini compat 路径（`/v1beta/openai/` 前缀），但原生 Imagen 走 `predict` 端点，不是 compat 路径，需要单独适配。

**能力矩阵**：

| 功能 | 端点 | 方式 |
|---|---|---|
| 文生图（Imagen 4） | `POST /v1beta/models/imagen-4.0-generate-001:predict` | JSON |
| 文生图（Imagen 4 Fast） | `POST /v1beta/models/imagen-4.0-fast-generate-001:predict` | JSON |
| Gemini 内嵌图像生成 | `POST /v1beta/models/gemini-2.5-flash:generateContent` | JSON，responseModalities:["IMAGE"] |

**Auth**：`?key={api_key}` 或 `Authorization: Bearer {oauth_token}`（取决于 provider 配置）

**Imagen 4 request/response**：

```json
// Request
{
  "instances": [{ "prompt": "..." }],
  "parameters": {
    "sampleCount": 1,
    "aspectRatio": "1:1",
    "outputMimeType": "image/png"
  }
}

// Response
{
  "predictions": [
    { "bytesBase64Encoded": "<base64>", "mimeType": "image/png" }
  ]
}
```

**规范化映射**：

```
Request.Prompt      → instances[0].prompt
Request.N           → parameters.sampleCount
Request.Size        → parameters.aspectRatio（WxH → "W:H" GCD 化简）
Request.OutputFormat → parameters.outputMimeType
```

**Vendor 检测逻辑**：

```go
// 在 DetectVendor 中，先于 VendorOpenAICompat 检查：
case strings.Contains(host, "generativelanguage.googleapis.com") && isImagenModel(provider_model):
    return VendorGoogleImagen
```

但 model 信息此时不在 provider 上（在 Request 里），所以改为在 `New(provider, model string)` 中判断：

```go
case VendorOpenAICompat:
    if strings.Contains(host, "generativelanguage.googleapis.com") && isImagenModel(model) {
        return newGoogleImagenClient(provider)
    }
    return nil, ErrUnsupported  // openai-compat 由 OpenAIClient 原生处理，不从 imagegen.New 走
```

实际上更干净的方案：`DetectVendor` 接受 `model string` 参数（breaking change，需同步修改 `registry.go` 的 `New` 调用）。

### 6.4 Fal.ai / FLUX (`VendorFalAI`)

**检测**：`host == "api.fal.ai"` 或 `host == "fal.run"`

**能力矩阵**：

| 功能 | 端点模式 | 方式 |
|---|---|---|
| 文生图（FLUX.1 dev/pro/schnell/max） | `POST /v1/{model-id}` | JSON，同步或异步 |
| Image-to-image（Kontext） | `POST /v1/fal-ai/flux-pro/kontext` | JSON |
| Fill（inpainting） | `POST /v1/fal-ai/flux-pro/v1/fill` | JSON |
| Erase | `POST /v1/fal-ai/flux-pro/v1/erase` | JSON |
| FLUX 2 Edit | `POST /v1/fal-ai/flux-2/edit` | JSON |

**Auth**：`Authorization: Key {api_key}`

**异步模式**：Fal.ai 通过 `sync_mode: true` 参数启用同步返回（适合短生成）；默认异步时返回 `{ request_id }` 需要轮询 `GET /v1/queue/requests/{request_id}/status`，超过 polling timeout 后 fetch result：`GET /v1/queue/requests/{request_id}/response`。

**规范化映射**：

```
Request.Prompt        → prompt
Request.Size          → image_size（"1024x1024" → { width:1024, height:1024 }）
Request.N             → num_images
Request.Images[0].URL → image_url（for i2i）
Request.Mask.URL      → mask_url（for fill）
sync_mode             → true（gateway 控制）
```

**Request.Model → endpoint 映射**（在 `falClient.endpointForModel` 中维护）：

```go
var falModelEndpoints = map[string]string{
    "flux-dev":            "/v1/fal-ai/flux/dev",
    "flux-pro":            "/v1/fal-ai/flux-pro",
    "flux-schnell":        "/v1/fal-ai/flux/schnell",
    "flux-pro/kontext":    "/v1/fal-ai/flux-pro/kontext",
    "flux-2-dev":          "/v1/fal-ai/flux-2/dev",
    "flux-2-pro":          "/v1/fal-ai/flux-2/pro",
    // ...
}
```

---

## 7. `imagegen.Request` 工厂函数扩展

### 7.1 `RequestFromOpenAIEdit`

```go
// RequestFromOpenAIEdit constructs a normalized Request from an ImageEditParams.
// Images and Mask are read eagerly from the io.Reader fields; callers must not
// close the readers before this returns.
func RequestFromOpenAIEdit(p *openai.ImageEditParams) (*Request, error) {
    // read p.Image file(s), p.Mask file → []byte
    // set Action = "edit"
}
```

### 7.2 `RequestFromOpenAIVariation`

```go
func RequestFromOpenAIVariation(p *openai.ImageVariationParams) (*Request, error) {
    // read p.Image file → []byte
    // set Action = "variation"
}
```

---

## 8. Vendor 路由决策表（扩展后）

在 `imagegen.New(provider, model string)` 中：

| Vendor | Generate | Edit | Variation |
|---|---|---|---|
| OpenAI-compat | ✅ via SDK（不走 imagegen） | ✅ via SDK ImagesEdit | ✅ via SDK ImagesVariation |
| Codex | ✅ Responses API | ✅ Responses API action:edit | ❌ ErrVariationUnsupported |
| DashScope | ✅ 异步 submit-poll | ❌ Phase 1 stub | ❌ |
| MiniMax | ✅ 同步自定义 | ❌ Phase 1 stub | ❌ |
| Stability AI | ✅ core/ultra/sd3 | ✅ inpaint/remove-bg | ❌ |
| Ideogram | ✅ v2/v3 generate | ✅ edit（inpaint） | ❌（Remix 作为扩展） |
| Google Imagen | ✅ imagen-4 predict | ❌ Phase 1 stub | ❌ |
| Fal.ai / FLUX | ✅ flux-* sync | ✅ fill/kontext/erase | ❌ |

---

## 9. Swagger / API 文档

两个新端点需要在 backend Swagger 定义中注册（参照 `internal/server/` 中现有的 swagger 标注格式）：

```go
// openai_images_edit.go
apiV1.POST("/images/edits", imageAPI.HandleOpenAIImageEdit,
    swagger.WithTags("images"),
    swagger.WithDescription("Creates an edited or extended image given source image(s) and a prompt"),
    swagger.WithRequestModel(OpenAIImageEditRequest{}),  // custom multipart model
    swagger.WithResponseModel(openai.ImagesResponse{}))

// openai_images_variation.go
apiV1.POST("/images/variations", imageAPI.HandleOpenAIImageVariation,
    swagger.WithTags("images"),
    swagger.WithDescription("Creates a variation of a given image (dall-e-2 only)"),
    swagger.WithRequestModel(OpenAIImageVariationRequest{}),
    swagger.WithResponseModel(openai.ImagesResponse{}))
```

前端 API client SDK 由 swagger codegen 生成，暂用 placeholder 函数即可。

---

## 10. 实现顺序建议

```
Phase 1（Batch A + B）：
  1. 扩展 imagegen.Request（新字段）+ imagegen.Client 接口（Edit/Variation + helper embeds）
  2. OpenAIClientInterface 增加 ImagesEdit / ImagesVariation
  3. OpenAIClient 实现（SDK 直通）
  4. CodexClient.ImagesEdit（Responses API action:edit）
  5. HandleOpenAIImageEdit / HandleOpenAIImageVariation HTTP handler
  6. 路由注册 + Swagger
  7. 现有 DashScope / MiniMax 实现 UnsupportedEdit / UnsupportedVariation embed

Phase 2（Batch C — 按厂商优先级）：
  8. Google Imagen adapter（stabilityClient 最简单入手）
  9. Stability AI adapter（能力最全面，multipart 多端点）
  10. Ideogram adapter（JSON + multipart 混合）
  11. Fal.ai adapter（异步 job polling，FLUX 模型映射表）

Provider Template 新增（providers.json）：
  - stability-ai（api.stability.ai）
  - ideogram（api.ideogram.ai）
  - fal-ai（api.fal.ai）
  每个 template 设 openai_endpoint_mode: "" （Chat 默认），imagegen 走各自 native adapter
```

---

## 11. 关键文件列表

| 文件 | 变更类型 |
|---|---|
| `internal/imagegen/imagegen.go` | 扩展 Request（新字段）、Client 接口（Edit/Variation）、工厂函数 |
| `internal/imagegen/vendor.go` | 增加 VendorStabilityAI / VendorIdeogram / VendorGoogleImagen / VendorFalAI 常量和检测逻辑 |
| `internal/imagegen/registry.go` | New() 扩展新 vendor case；DetectVendor 接受 model 参数（可选） |
| `internal/imagegen/stability.go` | **新建** Stability AI adapter |
| `internal/imagegen/ideogram.go` | **新建** Ideogram adapter |
| `internal/imagegen/googleimagen.go` | **新建** Google Imagen adapter |
| `internal/imagegen/falai.go` | **新建** Fal.ai / FLUX adapter |
| `internal/client/openai.go` | OpenAIClientInterface 增加 ImagesEdit / ImagesVariation；OpenAIClient 实现 |
| `internal/client/codex_client.go` | 实现 ImagesEdit（Responses API action:edit） |
| `internal/server/openai_images_edit.go` | **新建** HandleOpenAIImageEdit handler |
| `internal/server/openai_images_variation.go` | **新建** HandleOpenAIImageVariation handler |
| `internal/server/forwarding/openai.go` | 增加 ForwardOpenAIImageEdit / ForwardOpenAIImageVariation |
| `internal/server/server_routes.go` | 注册两个新路由 |
| `internal/data/providers.json` | 新增 stability-ai / ideogram / fal-ai template |

---

## 12. 不在本文档范围

- 视频生成 API（Stable Video Diffusion、Sora、Runway）—— 独立设计
- 图像理解 / vision（读图，不产图）—— 已通过 chat completions 的 image_url 支持
- Ideogram Reframe / Replace Background / Remove Background 等高级编辑 —— Phase 3
- Replicate.com 作为代理聚合器的接入 —— 评估中
- 前端图像生成 UI 组件 —— 独立设计
