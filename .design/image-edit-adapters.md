# Image Edit 适配器(xAI / 千帆 / DashScope)

> 适用对象:tingly-box 后端贡献者。
> 描述 `/images/edits` 在"不说 OpenAI multipart 协议"的 vendor 上怎么落地。
> 关联文档:`imageedit.md`(edit 网关链路与 Codex)、`image-mask.md` §9(各 vendor
> 的 mask / n 核对与来源)。

---

## 1. 为什么需要

`OpenAIClient.ImagesEdit` 原来只有两种结局:OpenAI 兼容上游走 SDK 的 multipart,
DashScope / MiniMax 直接报"不支持"。`image-mask.md` §9 的核对说明这不够:

| vendor | 生成 | 改图 | 原来发生什么 |
|---|---|---|---|
| xAI | OpenAI 兼容 | `/v1/images/edits` **只收 JSON** | SDK 发 multipart,上游拒绝 |
| 千帆 v2 | OpenAI 兼容 | `/v2/images/edits` **JSON**,mask **白=改** | 同上;即便发过去,mask 语义也是反的 |
| DashScope | 原生异步(已有适配器) | 万相 `image2image`(带 mask)/ qwen-image `multimodal-generation` | 明确报错,能改图的 vendor 被挡在门外 |

这三家生成都已经能用,改图却要么打不通,要么打通了也会把用户保护的区域改掉。

## 2. 结构

```text
OpenAIClient.ImagesEdit
    ├─ MiniMax                  → 明确报错(没有改图面)
    ├─ xAI / 千帆 / DashScope   → imagegen.EditRequestFromOpenAI → NewEditor(provider).Edit
    └─ 其余                      → SDK multipart(OpenAI / Azure / DeepInfra / ...)
```

- `EditRequest`(`internal/vision/imagegen/edit.go`)把 OpenAI 请求里的 reader
  **一次读进内存**:各家都要 data URL,千帆 / 万相还要把 mask 转一遍,single-use
  reader 做不到。
- vendor 仍按 host 识别(`DetectVendor`):`api.x.ai` → xAI,`qianfan.baidubce.com`
  → 千帆。两者的**生成**继续走 SDK,只有改图分流——识别它们只为改图面。
- 适配器共用 `postJSON`(`http.go`):非 200 时把上游 body 带进错误,那里才有
  用户真正需要的原因(尺寸、内容审核、配额)。DashScope 的文生图提交也改走它。

## 3. 三条关键决策

### 3.1 mask 必须翻转,不能透传

OpenAI 只读 alpha:**透明 = 改**。千帆(`ernie-irag-edit`)和万相
(`description_edit_with_mask`)只读亮度:**白 = 改,黑 = 留**。原样透传等于把
用户保护的区域交给模型重画,方向正好反了。`whiteEditMask`(`mask.go`)按 alpha
阈值(0x80)生成同尺寸灰度 PNG;尺寸不变是两家的硬要求,也正好是 Playground 的
mask 本来就满足的。

### 3.2 做不到 mask 就明确报错

与 `image-mask.md` §3.10 同一条:mask 被静默丢掉,用户拿回的是一次他没要求的整图重画。

| 情况 | 行为 |
|---|---|
| xAI 带 mask | 报错:xAI 改图没有 mask 参数 |
| 千帆非 `ernie-irag-edit` 带 mask | 报错,指向 `ernie-irag-edit` |
| DashScope qwen-image 带 mask | 报错,指向 `wanx2.1-imageedit` |
| `ernie-irag-edit` / 万相 多张参考图 | 报错:只编辑单张 |

`ernie-irag-edit` 不带 mask 时发 `feature: variation`——它唯一不需要 mask 的改法;
带 mask 时发 `repaint`(prompt 说涂过的地方该是什么)。

### 3.3 结果 URL 拉回成 base64

千帆和 DashScope 只返回 **24 小时有效**的 URL。原样交出去,Playground 会话里的图
(存 IndexedDB)一天后全部失效,网关的 `persistImages` 也只落盘 base64。所以除非
调用方**明确**要 `response_format: url`,适配器都把 URL 拉回来转成 base64
(`inlineResultURLs`,单张上限 32MB)。xAI 直接请求 `b64_json`,省一次往返。

## 4. 请求映射

| | xAI | 千帆 | DashScope 万相 | DashScope qwen-image |
|---|---|---|---|---|
| 端点 | `{base}/images/edits` | `{base}/images/edits` | `/api/v1/services/aigc/image2image/image-synthesis`(异步,轮询 `/api/v1/tasks/{id}`) | `/api/v1/services/aigc/multimodal-generation/generation`(同步) |
| 参考图 | 1 张 `image{url}`,多张 `images[]`(≤5) | 1 张字符串,多张数组(qwen ≤3) | `base_image_url`,1 张 | `content[{image}...]`,1–3 张 |
| mask | ✗ 报错 | `mask`(白=改)+ `feature: repaint` | `mask_image_url`(白=改)+ `function: description_edit_with_mask` | ✗ 报错 |
| n | `n` | `n`(qwen 只出 1) | `parameters.n` | `parameters.n` |
| size | `WxH` 约分后命中 `aspect_ratio` 枚举才发,否则跟随第一张输入 | 原样 `size` | 不发 | `W*H` |
| 模型识别 | — | `ernie-irag-edit*` 走 mask 分支 | `wan*imageedit*` | 其余 |

图片一律以 data URL 内联(三家都收;千帆文档明确要求 `data:image/...;base64,` 前缀)。

## 5. 测试

`internal/vision/imagegen/edit_test.go`:mask 翻转方向与尺寸;vendor 识别;
`EditRequestFromOpenAI`;三家的请求体构造(含各自的报错分支);以及 httptest 端到端:
xAI 同步返回、千帆 URL 被拉回成 base64 且显式 `url` 时保留、万相提交 → 轮询 → 拉回、
qwen-image 同步。

未覆盖:真实上游。host 识别让 `OpenAIClient` 的分发无法指向测试服务器,分发只由
适配器侧的用例间接覆盖。

## 6. 未做

- 千帆 `ernie-irag-edit` 的 `erase`(纯擦除,不需要 prompt):与"按 prompt 改涂过的
  区域"是不同的动作,Playground 没有入口,暂不映射。
- 万相的其它 `function`(风格化、扩图、超分、线稿上色……):不是 `/images/edits`
  的语义,不从这个入口暴露。
- MiniMax:只有 `subject_reference`,没有改图面,仍明确报错。
- 超过上游张数上限(xAI 5、qwen 3)时只让上游报错,不在网关截断。
