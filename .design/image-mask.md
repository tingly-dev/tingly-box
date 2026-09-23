# Image Mask(生图 Playground 的局部重绘)

> 适用对象:tingly-box 前端贡献者(含一处可选的网关改动)。
> 描述 Image Playground 的 mask 能力:在参考图上涂出"允许模型改动的区域",
> 走 `/images/edits` 的 `mask` 字段做局部重绘(inpainting)。
> 关联文档:`ux-principles.md`(判断标准)、`imageedit.md`(edit 网关链路)、
> `sketch-canvas.md`(同一面板的画布输入)、`image-slice.md`(同一面板的后置切分)。
>
> **状态:前端已实现(方案 A);Codex 走 Responses 的那条(方案 D)按实验性实现,
> 待真实订阅验证。见 §8。**

---

## 1. mask 是什么

`POST /v1/images/edits` 的 `mask` 是一张 **PNG**,语义只有一条:

> **alpha = 0(完全透明)的像素 = 允许模型重画的区域;不透明的像素 = 保持原样。**

要点(都是 wire 事实,不是我们的选择):

| 事实 | 影响 |
|------|------|
| 只读 alpha 通道,RGB 被忽略 | 我们导出的 mask 可以是"纯黑 + 打洞",颜色无所谓 |
| 尺寸必须与 image **完全一致** | 画布 backing store 必须取参考图的原始像素,不能取 Playground 的 `Size` |
| 必须是 PNG 且真的带 alpha 通道 | `toBlob('image/png')` 天然满足,但合成时不能铺不透明底色 |
| 多图时 mask 作用于 **image 数组的第一张** | 只有第一张参考图能带 mask(§3.2) |
| 大小上限 4MB | 二值 alpha 的 PNG 压缩率极高,实际到不了 |
| outpainting(向外扩画布)是同一个机制 | 把原图放进更大的透明画布即可;本版不做(§7) |

**为什么值得做。** 现在 Playground 的 edit 是"把整张图连同 prompt 交给模型重画":
想换掉沙发,模型顺手把墙色、光线、人脸一起改了。mask 是这条链路上**唯一**能表达
"只动这一块"的输入——它不是锦上添花的画笔,而是补一个当前完全缺失的语义。

---

## 2. 链路现状:每一层通到哪了

```text
前端 client.images.edit({ image, mask })   ← openai-js 走 multipart,字段已有
    ↓
POST /tingly/imagegen/v1/images/edits
    ↓ parseImageEditMultipart              ← 已经读 form.File["mask"]
    ↓                                        (openai_image_edit.go:214)
    ↓ parseImageEditJSON                    ← ✗ 没有 mask(不对称,§6.2)
    ↓ ForwardOpenAIImageEdit → ImagesEdit
        ├─ OpenAIClient  → SDK 原样透传 ✓
        ├─ CodexClient   → 有 mask 走 Responses 工具(实验,§8.2),原生端点报错
        └─ Kimi/vmodel/DashScope/MiniMax → 整个 edits 面就不支持
```

结论:**对 OpenAI 兼容上游,后端零改动即可端到端跑通**。这正是这个功能能做到
"相对隔离"的物理基础——它是一个前端功能,后端那两处(§6.2)是顺手补齐的对称性,
不是前置条件。

> 未在本环境编译验证:`frontend/node_modules` 未安装。openai-js v6 的
> `ImageEditParams.mask?: Uploadable` 需要在实现时确认一次字段名。

### 2.1 Codex 原生协议核对:确认不支持 mask

不是从文档推的,是读的源码(openai/codex,`fc948f8`,2026-09-11):

| 事实 | 出处 |
|------|------|
| `ImageEditRequest` 的字段只有 `images / prompt / background / model / n / quality / size`——**没有 mask** | `codex-rs/codex-api/src/images.rs` |
| edit 就是把这个结构体整个 `to_value` 成 JSON body POST 到 `images/edits`,没有别处再塞字段 | `codex-rs/codex-api/src/endpoint/images.rs` |
| 模型能填的工具参数只有 `prompt / referenced_image_paths / num_last_images_to_include`,连区域概念都没有 | `codex-rs/ext/image-generation/src/tool.rs`(`ImagegenArgs`) |
| 整个 `codex-rs` 里与图像相关的 `mask` 一个都没有(grep 命中的全是 `CollaborationModeMask` 之类的同名无关物) | 全仓 grep |

所以"Codex 支持了就跟着加"这条路**现在走不通**:不是我们没接,是那条 wire 上
没有这个字段。这把 §3.10 从"可选的礼貌"变成了唯一正确的做法——mask 在 Codex
上不可能生效,静默丢弃就是让用户拿到一个无法解释的结果。

重新核对成本很低(上面四行 grep),Codex 哪天加上 `mask`,`buildCodexImageEditRequest`
里就是多一个字段 + 去掉那处报错。

#### 核过的一个猜想:mask 会不会混在 `images` 数组里传?

**没有证据,判定为否。**`ImageUrl` 的结构体只有 `image_url: String` 一个字段,没有
类型、角色、顺序约定可以把其中一张标成 mask;`images` 数组的每一项都来自用户给的
路径或会话里最近的图片(`request_for_call_args`);工具描述(`imagegen_description.md`)
通篇讲的是"改哪张图",一个字都没提区域或蒙版。也就是说:客户端没有把 mask 藏进去的
地方,模型也没有被告知可以这么做。

这个猜想指向的**技术**是真的存在的,只是叫另一个名字:把"要改的区域"画在参考图上
(圈出来/压暗)让模型自己读——visual prompting。它对任何支持 edits 的 provider 都
成立,但它是 prompt 层的示意,不是 alpha 通道的硬约束:模型可能连那个圈一起画进
结果里。它正是 `sketch-canvas.md` §6 里"在已有图片上标注"那条,见 §7 的降级路径。

### 2.2 Codex 上仍然存在的一条路:Responses 的 `input_image_mask`

原生 `images/edits` 没有 mask,但那不是 Codex 订阅唯一的出图面。**Responses API 的
hosted `image_generation` 工具带一个 `input_image_mask`**(SDK 里确有其物:
`responses.ToolImageGenerationParam.InputImageMask`,内含 `image_url`(base64)或
`file_id`,注释写的就是 "Optional mask for inpainting"),同一个工具上还有
`action: generate | edit | auto` 和 `input_fidelity`。

而我们的 `CodexClient.ImagesGenerate` **已经在走这条面**——
`buildImageGenerationResponsesRequest` 现在就在构造这个 tool,只是没有填
`InputImageMask`。所以"Codex 能不能做局部重绘"这个问题没有关闭,它变成了一个
可以做实验的具体问题,而不是一个协议事实。

挡在前面的未知只有一个:`imageedit.md` §1 断言"Responses surface 无法给该 tool 挂
reference image",这正是当初要为 edit 另开 endpoint 的理由。但公开 Responses API
的官方改图姿势恰恰是"消息里放 `input_image` + 工具 `action: edit`",所以这条断言
要么是 ChatGPT backend 的特殊限制,要么是当时的一个未验证假设。**它是这条路上唯一
需要先回答的问题。**

实验(不改产品代码就能做,建议在打开这条路之前先跑):

1. 对 `chatgpt.com/backend-api/codex/responses` 发一个 hosted `image_generation`
   请求,消息内容里带一张 `input_image`,工具设 `action: edit`,看后端接不接。
2. 接的话,同一请求加上 `input_image_mask.image_url`,看局部重绘是否真的只改
   涂过的区域。
3. 两步都过 → Codex 分支的 mask 从"报错"变成"换一条面走";任一步不过 →
   §3.10 的明确报错就是终局,并把失败结论写回这里,省得下次再猜一遍。

注意:codex-rs 自己**故意不挂** hosted `image_generation` 工具
(`core/tests/suite/responses_lite.rs` 直接断言它不在工具列表里),它走的是自己
客户端执行的 `image_gen.imagegen`。所以这条路是**我们的用法**,不是 Codex CLI 的
用法——它能不能成立只能由实验回答,不能从 codex 源码推出来。

再补一条把未知收窄的证据:**codex 自己大量往 `/codex/responses` 发 `input_image`
内容项**(用户贴图、`view_image` 工具输出、动态工具的图片返回,见
`codex-rs/protocol/src/models.rs` 的 `ContentItem::InputImage` 与它的十几处构造点)。
所以"这个 endpoint 收不到图片"在协议层面已经不成立;`imageedit.md` §1 那句断言
即便当初观察属实,也只可能是**hosted tool 与 input_image 的组合**没打通,而不是
整个 Responses 面收不到参考图。实验因此从"能不能挂图"收窄成"hosted tool 认不认
`action: edit` + `input_image_mask`"。

### 2.3 三条出图面的能力对照

| | OpenAI 兼容 `/v1/images/edits` | Codex 原生 `codex/images/edits` | Responses hosted `image_generation` |
|---|---|---|---|
| 编码 | multipart(SDK)/ JSON(我们的便捷编码) | JSON | JSON(SSE 响应) |
| 参考图 | `image` 可多张 | `images[].image_url`,≤5 | 消息里的 `input_image` 内容项 |
| **mask** | **`mask` 字段 ✓** | **✗ 协议无此字段** | **`input_image_mask{image_url\|file_id}` ✓** |
| 相关旋钮 | `input_fidelity`、`background`、`size`、`quality` | `background`、`size`、`quality`、`n` | `action: generate\|edit\|auto`、`input_fidelity`、`background`、`moderation` |
| Codex CLI 自己用不用 | — | **用**(`ImagesClient::edit`) | **不用**,且断言不挂(`responses_lite.rs`) |
| 我们现在的代码 | `OpenAIClient.ImagesEdit` 原样透传 | `CodexClient.ImagesEdit` 丢弃 mask | `CodexClient.ImagesGenerate` 建了 tool,没填 mask、没挂参考图 |
| mask 可行性 | 已通,零后端改动 | 不可能(除非上游加字段) | **未验证,值得实验** |

### 2.4 方案矩阵

| 方案 | 覆盖面 | 改动量 | 依赖 | 结论 |
|------|--------|--------|------|------|
| **A. 前端 mask → multipart** | OpenAI 及一切兼容上游 | 前端 2 个新文件 + 约 60 行(§6.1),后端 **0** | 无 | **主线,先做** |
| **B. JSON 便捷编码补 `mask`** | 程序化调用方 | 后端约 15 行 + 单测(`parseImageEditJSON`) | 无 | 顺手做,补对称性 |
| **C. Codex 带 mask 时明确报错** | Codex | 后端约 5 行 + 单测(`buildCodexImageEditRequest`) | 无 | 与 A 同批;D 若成立则退成兜底 |
| **D. Codex 走 Responses + `input_image_mask`** | Codex 订阅 | 后端较大:`ImagesEdit` 在 mask 存在时分流到 Responses、把参考图作为 `input_image` 挂进消息、填 `InputImageMask`、复用已有的 `parseImageGenerationStream` | **实验 E1/E2 通过** | 实验说了算,不提前投入 |

A/B/C 三条互不依赖,可以一次做完;D 是独立的后续,它落地后 C 的报错只在 D 也失败时
才触发。**不做的**:按 provider 能力在前端隐藏 mask 入口(违反 `imageedit.md` §6 的
"provider 能力是网关的事"),以及把带 mask 的请求静默降级成整图 edit。

### 2.5 实验(D 的前置)

跑法沿用现成的 opt-in e2e 模式:`internal/client/codex_e2e_test.go` 靠
`CODEX_ACCESS_TOKEN` / `CODEX_ACCOUNT_ID` 决定跑还是 skip,直接复用 `CodexClient`
的 transport,OAuth、header、path 重写全部免费。

| 步 | 问题 | 请求 | 通过判据 | 不通过的含义 |
|----|------|------|----------|--------------|
| **E1** | hosted tool 能不能拿消息里的参考图改图(先不管 mask) | `/codex/responses`:消息含 `input_image` + "把 X 换成 Y",tool `image_generation` 带 `action: edit` | 回来的 `image_generation_call` 结果明显基于那张参考图,而不是凭空新生成 | `imageedit.md` §1 的断言成立 → **D 死**,C 是终局 |
| **E2** | 它认不认 `input_image_mask` | E1 的请求 + `input_image_mask.image_url`(base64 PNG) | 只有涂过的区域变,其余像素基本不动 | 后端忽略 mask → D 退化成"另一种整图 edit",不如原生 edits,**不值得做** |
| **E3** | 值不值得切 | 同一 prompt/图,D 与原生 edits 对照 | 边缘、保真、耗时可接受 | 质量明显更差 → 保留原生 edits 做无 mask 路径,D 只在有 mask 时启用 |

三步都只发请求、不改产品代码。E1 失败就把结论写回 §2.2,省得下次再猜一遍;E1/E2
通过再评估 D 的实现成本。

---

## 3. 核心决策

### 3.1 mask 是参考图的一个属性,不是新的来源、也不是模式

`ReferenceImage` 多一个可选字段,除此之外提交链路不变:

```ts
interface ReferenceImage {
    file; previewUrl; source: 'upload' | 'sketch'; layers?; width?; height?;
    mask?: {
        file: File;          // 导出的 alpha PNG,直接进请求
        previewUrl: string;  // 缩略图上叠加的可视化(涂过的区域)
        strokes: Stroke[];   // 可再进入的原料(§3.7)
    };
}
```

备选是做成"Mask 模式"或"第四种参考图来源"。**否决**,与 `sketch-canvas.md` §2.1
同一条理由:mask 回答的是"这张图哪里能改",不是"图从哪来";它依附于一张具体的
参考图而存在,离开那张图没有意义。给它开模式等于把两条正交轴拧到一个旋钮上
(原则 4)。端点推导也完全不动:有没有 mask 都还是 `edits`。

### 3.2 只有第一张参考图能带 mask,并且把这件事做成结构而不是提示

wire 只认第一张。与其加一个"mask 作用于哪张图"的选择器(一个用户没法拒绝的
错误答案),不如让 UI 只在 index 0 的缩略图上长出 Mask 按钮:**够不着的东西
不需要解释**。

- mask 挂在图上,所以删掉首图 = mask 一起走;第二张递补成首张时**不会继承**一个
  不属于它的 mask。
- 参考图**可以拖拽重排**(上游后加的能力)。规则:mask 跟着它那张图走,不会因为
  换了位置就丢;但只有排在第一位的那张会被发送。所以一张带 mask 的图被拖离队首
  时,不是静默失效——缩略图上的色块压暗、按钮退回普通态,提示行多一句"只有第一
  张的 mask 会被发送"。**已经画的东西不丢**,拖回去就继续生效(原则 10);而
  "现在会不会生效"在两个地方写着,不留给用户猜。

### 3.3 涂的是"要改的地方",不是"要保留的地方"

用户脑子里的动作是"我要换掉这块"。所以画布上涂出来的区域**高亮**(半透明暖色
覆盖),导出时**反转成 alpha = 0**。alpha 语义是 wire 的事,不外露。

反过来做(涂=保护)在实现上更直白,但它要求用户先把 OpenAI 的 alpha 约定装进
脑子,再在每一笔上做一次反向翻译。底部一句话把契约说清楚就够:
**"Painted areas are what the model may change."**(原则 8:教育内嵌,但不替用户
翻译两遍。)

`Invert` 是工具栏一等按钮,不是隐藏项:"把背景换掉"的自然操作是涂主体再反转,
少了它用户要沿着轮廓涂外面一整圈。

### 3.4 画布尺寸跟随参考图,不跟随 `Size`

与草图画布(跟随输出 `Size`,见 `sketch-canvas.md` §2.2)**相反**,而且必须相反:
mask 的尺寸约束来自 image 本身,差一个像素就是 400。标题栏把具体值写出来
(`1024 × 1536 px · matches reference`,原则 5),用户不需要也不能在这里选尺寸。

参考图的像素尺寸在加入时已经读过(`ReferenceImage.width/height`),不用再读一次。

### 3.5 橡皮是真的擦,不是刷白

草图画布上橡皮 = 画白色(白底不透明,`sketch-canvas.md` §2.3)。mask 画布上这条
不成立:底是**透明**的,刷白会造出一片不透明区域,语义正好反了。所以 mask 的
橡皮走 `destination-out`,重放时同样。

这是两个画布唯一一处真正的行为分歧,也是它们不该合并成一个对话框的证据之一(§4)。

### 3.6 第一版的工具:笔 / 橡皮 / 清空 / 反转 / 撤销

不做的:羽化(模型本来就会重新合成边缘,一个我们调不准的旋钮)、矩形与套索
(笔加粗一档就覆盖了大部分场景)、魔棒与自动分割(§7)。

笔宽沿用 `brushWidthFor` 的相对定义,所以同一档在 512 和 1792 的图上视觉粗细一致。

### 3.7 可再进入:mask 存笔画,不存位图

`mask.strokes` 与草图同构(点串 + 粗细 + 笔/橡皮),重开即可继续涂、可逐笔撤销
(原则 10,`sketch-canvas.md` §3.3 已经为这条付过一次学费,不再重犯)。代价是几个
数组,比第二张全尺寸 PNG 还省。

### 3.8 prompt 的提示语跟着场景切换

首图带 mask 时,placeholder 换成"描述涂出来的区域应该变成什么"。与草图同一条
(原则 8):提示语跟着心智变,**但不往 prompt 里塞任何句子**——prompt 是用户的东西
(`image-slice.md` §2)。

### 3.9 历史卡片把 mask 记进元信息行

run 上记一个 `maskUsed: boolean`,元信息行写 `images/edits · mask`,source 缩略图
叠加当时的 mask 预览。理由与 `imageedit.md` §6 把端点写出来完全一样:看着结果就
知道该调哪个接口、多传哪个字段(原则 11)。

### 3.10 provider 不支持时不静默降级

前端**不做任何 provider 判断**(沿用 `imageedit.md` §6)。但网关侧现在的行为是
`CodexClient` 把 mask 丢掉、只打一行 debug log——用户涂了一块,拿回来的是整张
重画,而且没有任何地方告诉他为什么。Codex 原生协议确认没有 mask 字段(§2.1),
所以这里没有"接上去"的选项,只有"说清楚"。这与"不允许把 edit 静默降级成
generation"是同一条原则:**带 mask 的请求落到 Codex 上应当明确报错**(§6.2),
而不是退化成一次用户没要求的整图重画。

---

## 4. 为什么不做进 SketchCanvasDialog

两个对话框表面上是同一件事(在画布上涂),底下四项全不同:

| | Sketch | Mask |
|---|---|---|
| 尺寸基准 | Playground 的 `Size` | 参考图的原始像素 |
| 底 | 不透明白底 | 透明(且必须保持透明) |
| 橡皮 | 画白色 | `destination-out` |
| 产物 | 一张新的参考图 | 挂在既有参考图上的 alpha 通道 |

合并意味着让 sketch 对话框长出一个模式开关,恰好是原则 2 和 4 各违一次。复用
落在**下面那层**:`utils/sketchCanvas.ts` 的 `Stroke` / `StrokeHistory` /
`toCanvasPoint` / `fitWithin` / `brushWidthFor` 被 mask 编辑器只读引用,一行不改。

顺带:`sketch-canvas.md` §6 里"在已有图片上标注(圈出要改的区域)"那条未做项,
由本功能**吸收掉**——当时挂起的理由就是"它和 edit 的 mask 语义重叠",现在直接做
成 mask 本身,那条待办可以划掉。

---

## 5. 交互脚本

1. 拖一张照片进参考图区 → 缩略图(已有行为)。
2. **第一张**缩略图右下角出现 Mask 按钮(与草图的画笔按钮同一个位置与尺寸语汇)。
3. 点开对话框:原图铺满工作区,涂过的区域实时高亮;工具栏 Pen / Eraser / 粗细 /
   Invert / Clear / Undo;标题栏 `1024 × 1536 px · matches reference`;底部
   "Painted areas are what the model may change."
4. Apply → 缩略图上出现 mask 角标与半透明覆盖预览;再点 Mask 按钮回到同一块涂层。
5. prompt placeholder 变成"描述涂出来的区域应该变成什么"。
6. Generate → `images/edits`,multipart 里多一个 `mask` 字段。
7. 结果卡片元信息行 `images/edits · mask`。不满意 → 已有的 **Use as reference** 把
   结果推回参考图 → 再涂一次。mask 让这个循环第一次真正闭合(原则 10、11)。

键盘:`Ctrl/Cmd+Z` 撤销(对话框打开期间),`Esc` 关闭。指针交互沿用草图那套
(Pointer Events + `setPointerCapture` + `getCoalescedEvents` + `touch-action: none`)。

---

## 6. 改动清单(隔离边界)

### 6.1 前端

| 文件 | 改动 | 量级 |
|------|------|------|
| `frontend/src/utils/maskCanvas.ts` | **新增**:笔画 → alpha PNG 合成、反转、尺寸校验、预览着色。纯逻辑,带单测 | 新文件 |
| `frontend/src/pages/scenario/components/MaskEditorDialog.tsx` | **新增**:单层 canvas + 工具栏 + undo + 导出 | 新文件 |
| `frontend/src/utils/sketchCanvas.ts` | 只读复用,**不改** | 0 |
| `ImageGenPlaygroundCard.tsx` | `ReferenceImage.mask` 字段;首图缩略图一个按钮 + 角标;`runGeneration` 里 `mask: request.sources[0]?.mask?.file`;run 元信息一行;placeholder 分支 | ~60 行 |

不碰:端点推导、SketchCanvasDialog、ImageSliceDialog、`playgroundSession` 的既有形状
(mask 是 `File`,与参考图同样不进会话持久化)、swagger(网关路由不在 `/api/v1` 管理面)。

### 6.2 网关(可选,与前端解耦,可以后做)

| 改动 | 理由 |
|------|------|
| `parseImageEditJSON` 补 `mask` 字段(data URL / 裸 base64,复用 `decodeInlineImage`) | multipart 支持而 JSON 不支持是不对称,不是设计 |
| `CodexClient.ImagesEdit` 遇到 mask 明确报错,而不是 debug 丢弃 | §3.10;原生协议没有这个字段(§2.1),静默丢弃让用户拿到一次他没要求的整图重画 |

两者都不阻塞前端:OpenAI 兼容上游走的是 multipart + `OpenAIClient`,已经是通的。

### 6.3 测试

- `maskCanvas` 单测:笔画→alpha(涂过=0,未涂=255)、反转幂等、橡皮真的擦出不透明、
  尺寸与源图一致、空 mask 不产出文件。
- canvas 渲染仍然不进 jsdom:走 `.claude/skills/ui-preview` 的真实浏览器链路验证
  (加图 → 涂 → 反转 → 撤销 → Apply → 重开笔画还在 → Generate)。
- 后端若做 §6.2:JSON mask 解析用例 + Codex 报错用例。

---

## 7. 未做 / 后续

- **降级路径:标注式 mask(visual prompting)**。provider 收不到 alpha mask 时
  (Codex,以及 §2.2 的实验若不通过),把要改的区域直接画在参考图上仍然能给模型
  一个指向。它与本功能共用同一块涂层数据,导出时不是抽 alpha 而是把涂过的区域
  压暗/描边合成进图片。**不做进第一版**:它的成功率取决于模型,而且有"圈被画进
  结果里"的失败模式,先把真 mask 这条做扎实,再决定要不要给不支持的 provider 补
  这个替身。
- **outpainting(向外扩画布)**:同一个 `mask` 机制,但需要一套"把原图放进更大画布
  并选择扩展方向"的 UI,是另一个功能。
- **魔棒 / 自动分割**:SAM 之类能在浏览器里跑,挡在前面的是几 MB 模型怎么带——与
  `sketch-canvas.md` §6"从照片提取姿势"同一个待决问题,应该一起决定,不该顺手加。
- **羽化边缘**:等实际用下来确认模型的接缝确实差,再做。
- **每张参考图各自的 mask**:wire 不支持,除非上游改。
- **参考图重排**:重排落地时再定 mask 的跟随规则(§3.2)。

---

## 8. 实现状态

### 8.1 本分支(前端 + 网关直通)

| 部分 | 位置 | 说明 |
|------|------|------|
| mask 合成 / 反转 / 导出(纯逻辑 + 单测) | `frontend/packages/vision/src/maskCanvas.ts`(+ `.test.ts`) | 涂 = 改,导出时反转成 alpha 0;反转档由同一份笔画换个方向合成。放在 `@tingly/vision` 里,与 `sketchCanvas` 同一层:只碰 canvas/Blob,不碰 MUI/i18n |
| 编辑器 | `frontend/src/pages/scenario/components/MaskEditorDialog.tsx` | 笔 / 橡皮 / 粗细 / Invert / Clear / Undo / Remove;画布取参考图原始像素;透明底,橡皮走 `destination-out` |
| 面板接线 | `ImageGenPlaygroundCard.tsx`、`ImageGenPlayground.types.ts` | 缩略图上的 Mask 按钮与色块(含被拖离队首后的失效态,§3.2)、prompt 提示语切换、`images.edit({ mask })`、历史卡片 `images/edits · mask`;`ReferenceMask` 整个存进 run,所以重试和"把这次请求放回面板"拿回的都是**可继续编辑**的 mask |
| JSON 便捷编码收 `mask` | `internal/protocolserver/openai_image_edit.go` | 与 multipart 对齐,同样只收 data URL / 裸 base64 |

网关这一侧**只做直通**:multipart 早就解析 `mask`,OpenAI 兼容 client 原样透传,
所以对 OpenAI 及兼容上游,这条链路是通的。

单测:`maskCanvas.test.ts`(合成方向,用记录型 ctx 断言每一笔的 composite)、
`openai_image_edit_test.go`(JSON mask)。画布交互按惯例走真实浏览器验证:加图 →
涂 → 反转 → Apply → 缩略图角标 → 重开笔画还在。

### 8.2 Codex 那条(**本分支**:实验性,未验证)

§2.2 的 Responses 路线(方案 D)在这里,和它一起的还有"原生端点遇到 mask 明确
报错"。它与 §8.1 分开成两个分支,是因为 §2.5 那三个实验还没跑:在真实订阅上确认
hosted tool 认不认 `action: edit` 和 `input_image_mask` 之前,不把一条猜出来的
链路压在一个已经能用的功能下面。

| 改动 | 位置 |
|------|------|
| Responses 请求构造(参考图作为 `input_image` 进消息,mask 作为工具的 `input_image_mask`,`action: edit`)+ 路由决策 + 单测 | `internal/client/codex_images_responses.go`(+ `_test.go`) |
| `ImagesEdit` 按有没有 mask 分流;原生端点遇到 mask 明确报错(不再 debug 丢弃) | `internal/client/codex_images.go` |

响应解析复用 generation 那条(`parseImageGenerationStream`),因为 hosted tool 两
种用法发的是同一串事件。

### 8.3 路由开关与测试脚本

```
TINGLY_CODEX_IMAGE_EDIT_ROUTE=            # 不设:有 mask 走 Responses,没 mask 走原生
TINGLY_CODEX_IMAGE_EDIT_ROUTE=responses   # E1/E3:无 mask 也走 Responses
TINGLY_CODEX_IMAGE_EDIT_ROUTE=native      # 钉死原生(带 mask 时明确报错)
```

不设即产品行为:只有 mask 这一个功能性理由会离开已验证的原生端点。实验结束后这个
变量应该消失。

E1(hosted tool 认不认参考图,先不带 mask):

```bash
TINGLY_CODEX_IMAGE_EDIT_ROUTE=responses tingly-box serve
curl -s http://127.0.0.1:PORT/tingly/imagegen/v1/images/edits \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"image":"data:image/png;base64,...","prompt":"把沙发换成绿色天鹅绒","model":"<codex model>"}'
```

E2(带 mask,默认路由即可):Playground 里涂一块再 Generate,或 JSON 里多带一个
`"mask":"data:image/png;base64,..."`。判据是**只有涂过的区域变**。

三种结果对应三条路:两步都过 → 去掉实验标记、收敛默认、合并;E2 不过(mask 被
忽略)→ 退回原生,只留下那条明确报错;E1 不过 → `imageedit.md` §1 的断言成立,
写回 §2.2 结案,本分支只剩报错值得保留。

链路在日志里:`[Codex] Using Responses image_generation tool for image edit
(experimental), model: ..., mask: true`。

### 8.4 仍未做

- 前端不按 provider 隐藏 mask 入口(沿用 `imageedit.md` §6:能力是网关的事)。
- 失败信息仍是通用的请求错误通知,没有"这个 provider 不支持 mask"的专门措辞。
- §7 的羽化 / 自动分割 / outpainting 全部未动。

---

## 9. 各出图 vendor 的核对:mask 与多张图(n)

mask 落地后逐个 vendor 过了一遍"要不要跟着改"。分发点是
`OpenAIClientInterface.ImagesEdit / ImagesGenerate`(`imageedit.md` §2)。下表按官方
文档核对(2026-09-23),来源见 §9.5;标 ? 的是文档没写清、未证实。

| vendor | `/images/edits` | mask | n | 对我们的含义 |
|---|---|---|---|---|
| OpenAI(gpt-image-*、dall-e-2) | multipart / JSON | ✓ 作用于**第一张**;gpt-image 上是**软约束**("entirely prompt-based … may not follow its exact shape") | 1–10(dall-e-3 仅 1) | 透传即正确 |
| Azure OpenAI | multipart,同 OpenAI | ✓ 同 OpenAI | 1–10 | 透传即正确 |
| DeepInfra | multipart,OpenAI 形状 | ✓ alpha=0 | 1–4 | 透传即正确 |
| xAI | **只收 JSON**,SDK 的 multipart 明确不支持 | 未文档化 | ≤10 | 我们的 multipart 发过去会失败;要支持得转 JSON(另开) |
| StepFun | multipart(step-image-edit-2) | 未文档化 | 仅 1 | 能透传;n 由 shortfall 槽位呈现 |
| 百度千帆 v2 | **JSON** | ✓ `ernie-irag-edit`,但**白=改、黑=留**(与 OpenAI alpha 相反) | 1–4 | 需要转 JSON + 反转 mask(另开) |
| DashScope | compat 模式**没有** edits(改图走 generations 的 `image` 字段) | 原生 `wanx2.1-imageedit` 的 `description_edit_with_mask`,**白=改**,收 data URL | compat ≤6 | 现状:适配器明确拒绝 edits,不会静默吃 mask |
| 火山 Seedream、硅基、ModelScope、Gemini compat、Together、OpenRouter | **没有** `/images/edits`(改图走 generations + image 字段,或各自的接口) | 无 | 火山无 `n`(`sequential_image_generation` + `max_images`);其余各异 | 请求会被上游 404 / 拒绝,不会静默吃 mask |
| 智谱、MiniMax | 无改图 | 无 | 智谱 ?;MiniMax 1–9 | 不变 |
| Kimi / vmodel | 不支持 | — | — | 不变 |
| **Codex** | 原生 JSON `images/edits` / Responses 工具 | 走 Responses(实验,§8.2);原生端点见 §9.4 | **一次一张** | **n 在网关扇出**(§9.1) |

结论:

- **mask**:透传就正确的只有 OpenAI / Azure / DeepInfra。其余大多数 compat 上游
  **根本没有 `/images/edits`**,请求会被拒,而不是 mask 被静默吃掉——原先担心的
  "静默忽略"风险比预想小。真正能接 mask 但协议不同的是千帆和 DashScope 万相
  (JSON + 白=改的黑白图),需要各自的适配器,不在本分支。
- **即便在 OpenAI 上,gpt-image 的 mask 也是软约束**。编辑器底部那句
  "Painted areas are what the model may change" 措辞是对的(may,不是 will)。
- **n**:Codex 之外,StepFun / qwen-image 等也是一次一张。现在由卡片上的空槽位
  如实呈现(§9.2);要把扇出推广到这些 vendor,是同一个 `fanOutCodexImages` 的泛化,
  等有需求再做。

### 9.1 Codex:n 张图 = n 次单图调用,并行后合并

Codex 的三条出图面都是一次一张:Responses 的 `image_generation` 工具没有 `n` 字段,
一次只产出一个 `image_generation_call`;原生 `images/edits` 的 schema 里虽然有 `n`,
但 Codex CLI 从来不传(`n: None`),"一次一张"是唯一被验证过的形状。原来的行为是
`n` 打一行 debug log 然后只回一张。

做法(`internal/client/codex_images_fanout.go`):

- `ImagesGenerate`、原生 `ImagesEdit`、Responses 版 `ImagesEdit` 三条路径都经过同一个
  `fanOutCodexImages(ctx, n, one)`。请求体**只构造一次**——参考图的 `io.Reader` 只能
  读一次,读成 data URL 之后每次调用复用同一份 body。原生端点的 `n` 字段不再上线。
- 并发窗口 `codexMaxParallelImageCalls = 4`:每一次都是订阅上的一整次出图,无上限
  并发主要换来的是限流。
- 结果按调用顺序合并 `data[]`,`usage` 逐项相加,`created/size/...` 取第一份。
- **部分失败返回成功的那几张**:已经出好、已经计费的图不该因为另一次调用被限流而
  一起丢掉;整次请求的超时在后面几波还没跑完时触发,同样保留已完成的。只有全部
  失败才是错误。部分失败在网关打一行 warn,列出每一次的原因。

为什么在网关而不是前端扇出:能力差异是网关的事(`imageedit.md` §6),Playground
以外的调用方也拿到正确的 n 张;而前端拆成 n 个请求,在 OpenAI 这类上游会让参考图的
输入 token 被计 n 次。代价是结果一次性回来——逐张出现留给 images 流式(§9.3)。

### 9.2 前端:槽位网格

`GenerationRunCard` 从 run 开始就按 n 画出槽位(`runGridLayout`,`imageGenSession.ts`):

- **最多两行**,列数 = ⌈n / 行数⌉,卡片随列数变宽,但**不超过结果条本身的宽度**
  ——一张要在内部横向滚动的卡,就是 n 张没法同时比较的图。
- **pending 与完成共用同一个布局**:pending 时每个槽位是虚线框 + 转圈,落地后图片
  填进对应槽位,卡片形状不跳。
- 没有图的槽位在完成后保持虚线框,写 "No image returned"——缺在哪一格一眼可见,
  不需要单独的提示行(provider 封顶 n、Codex 扇出部分失败、上游忽略 n,都走这里)。
- 列数 > 2 时去掉角上的放大徽标(小格子放不下,点击和 hover 遮罩本来就能放大)。
- 元信息行在 n > 1 时写 `· n=4`(原则 5)。
- 大图预览的缩略图条:原来只在有参考图时出现,现在**同一次有多张输出**也出现,
  挑图时可以在它们之间切换。条分成两块放在对角:参考图(Original)在左上,生成图
  (Generated)在右下——请求就是这个读序,进去的在前、出来的在后;两块同时出现时
  各占一半高度、各自滚动。←/→ 仍然按"参考图 → 生成图"一条线走。
- **参考图在哪里出现,就能在哪里点开大图**,打开的是同一个 lightbox:面板的参考图行
  (之前就能点,现在 lightbox 左上多一块 Reference 条,多张参考图之间可以 ←/→)、
  run 卡片的来源缩略图(之前就能点,补上与其他图一致的 hover 放大提示)、Overview
  里每张图左上的来源角标(之前是纯展示,现在每个小图都能点开,落到带 Original /
  Generated 两块条的同一个 lightbox)。
- **mask 在大图里叠加显示**:带 mask 的参考图(面板里的,或某次 run 的第一张来源图)
  在 lightbox 里叠一层与缩略图同样的着色预览;右上角控制按钮里多一个画笔按钮切换
  显示/隐藏,默认显示,在 filmstrip 里切换时保持。mask 与原图像素尺寸完全一致,两层
  用同样的 `scale-down` 铺满同一个盒子就逐像素对齐,不需要测量。

mock 后端的 prompt 带 `[partial]` 时只返回一半(向上取整),用来验证缺图槽位,
与已有的 `[fail]` / `[slow]` 同一套约定。

### 9.3 仍然开着的

- **逐张出现**:OpenAI images API 有 `stream: true`(SDK 已有 `GenerateStreaming` /
  `EditStreaming`,openai-js 也支持)。网关对 Codex 扇出可以每完成一次推一个
  completed 事件,原生支持的上游透传;前端改成收到一张填一格。槽位网格已经是它
  需要的形状,到时只改"填"的时机。
- **千帆 / DashScope 万相 的 mask**:协议不同(JSON、白=改),各自需要 edit 适配器。
- **xAI edits**:只收 JSON,我们的 multipart 发不过去,需要转换。
- `parseImageGenerationStream` 原来把 `partial_image` 的 base64 **拼接**;官方文档确认
  每个 partial 都是一张完整预览图。已改成:done 事件的 `result` 优先(不看 status——
  第三方观察到上游会把已完成的 call 留在 `generating`),没有再退回最后一个 partial。

### 9.4 值得跑的一个实验:Codex 原生端点会不会收 `mask`

openai/codex 源码里 `ImageEditRequest` 没有 mask 字段(§2.1 不变)。但公开 Images API
的 JSON 编码本身就有 `mask: {image_url | file_id}`,而第三方代理 CLIProxyAPI 对
gpt-image-2 系列直接把 `mask` 映射成 `mask.image_url` 发到 `codex/images/edits`(同时
透传 `n`)。这说明 ChatGPT backend 的 images 端点**可能**就是公开 API 的同一个实现,
只是 Codex CLI 没用到这个字段。没有找到"确实生效"的证据。

如果成立,它比 §8.2 的 Responses 路线简单得多(不换面、不换事件解析),n 也可能原生
就能用。实验 E0:在原生 JSON body 里加 `"mask":{"image_url":"data:image/png;base64,..."}`,
看是 400、被忽略还是局部重绘;同时试一次 `n: 2`。结果写回这里。

### 9.5 来源

- OpenAI 图像指南 / Images API 参考:developers.openai.com/api/docs/guides/image-generation、
  developers.openai.com/api/reference/resources/images
- Azure:learn.microsoft.com/en-us/azure/foundry/openai/how-to/dall-e
- DeepInfra:docs.deepinfra.com/api-reference/image-generation/openai-images-edits
- xAI:docs.x.ai/developers/model-capabilities/images/editing
- Gemini compat:ai.google.dev/gemini-api/docs/openai;Together:docs.together.ai/reference/post-images-generations;
  OpenRouter:openrouter.ai/docs/features/multimodal/image-generation
- DashScope:help.aliyun.com/zh/model-studio/qwen-image-generation-and-editing-api-reference、
  help.aliyun.com/zh/model-studio/wanx-image-edit-api-reference
- 火山方舟:docs.volcengine.com/docs/82379/1541523;硅基:api-docs.siliconflow.cn/docs/api/images-generations-post
- 智谱:docs.bigmodel.cn(图像生成 API);StepFun:platform.stepfun.com/docs/api-reference/images/image
- MiniMax:platform.minimax.cn/docs/api-reference/image-generation-i2i
- 千帆:cloud.baidu.com/doc/qianfan-api/s/8m7u6un8a、cloud.baidu.com/doc/qianfan-api/s/Rm9m76ekf
- Codex:github.com/openai/codex(codex-rs/codex-api/src/images.rs、ext/image-generation/src/tool.rs);
  CLIProxyAPI:github.com/router-for-me/CLIProxyAPI(internal/runtime/executor/codex_openai_images.go,issue #4273)
