# Image Mask(生图 Playground 的局部重绘)

> 适用对象:tingly-box 前端贡献者(含一处可选的网关改动)。
> 描述 Image Playground 的 mask 能力:在参考图上涂出"允许模型改动的区域",
> 走 `/images/edits` 的 `mask` 字段做局部重绘(inpainting)。
> 关联文档:`ux-principles.md`(判断标准)、`imageedit.md`(edit 网关链路)、
> `sketch-canvas.md`(同一面板的画布输入)、`image-slice.md`(同一面板的后置切分)。
>
> **状态:设计,未实现。** 本文先把"是什么 / 怎么进 / 怎么交互 / 隔离边界"定下来。

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
        ├─ CodexClient   → 丢弃 + debug log ⚠(codex_images.go:137)
        └─ Kimi/vmodel/DashScope/MiniMax → 整个 edits 面就不支持
```

结论:**对 OpenAI 兼容上游,后端零改动即可端到端跑通**。这正是这个功能能做到
"相对隔离"的物理基础——它是一个前端功能,后端那两处(§6.2)是顺手补齐的对称性,
不是前置条件。

> 未在本环境编译验证:`frontend/node_modules` 未安装。openai-js v6 的
> `ImageEditParams.mask?: Uploadable` 需要在实现时确认一次字段名。

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
- 参考图目前不可重排。真要加重排,规则是 mask 跟着它那张图走、离开首位即失效并
  提示——那是重排功能的账,不在本版。

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
重画,而且没有任何地方告诉他为什么。这与"不允许把 edit 静默降级成 generation"
是同一条原则,建议改成明确报错(§6.2)。

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
| `CodexClient.ImagesEdit` 遇到 mask 明确报错,而不是 debug 丢弃 | §3.10;静默丢弃让用户拿到一个无法解释的结果 |

两者都不阻塞前端:OpenAI 兼容上游走的是 multipart + `OpenAIClient`,已经是通的。

### 6.3 测试

- `maskCanvas` 单测:笔画→alpha(涂过=0,未涂=255)、反转幂等、橡皮真的擦出不透明、
  尺寸与源图一致、空 mask 不产出文件。
- canvas 渲染仍然不进 jsdom:走 `.claude/skills/ui-preview` 的真实浏览器链路验证
  (加图 → 涂 → 反转 → 撤销 → Apply → 重开笔画还在 → Generate)。
- 后端若做 §6.2:JSON mask 解析用例 + Codex 报错用例。

---

## 7. 未做 / 后续

- **outpainting(向外扩画布)**:同一个 `mask` 机制,但需要一套"把原图放进更大画布
  并选择扩展方向"的 UI,是另一个功能。
- **魔棒 / 自动分割**:SAM 之类能在浏览器里跑,挡在前面的是几 MB 模型怎么带——与
  `sketch-canvas.md` §6"从照片提取姿势"同一个待决问题,应该一起决定,不该顺手加。
- **羽化边缘**:等实际用下来确认模型的接缝确实差,再做。
- **每张参考图各自的 mask**:wire 不支持,除非上游改。
- **参考图重排**:重排落地时再定 mask 的跟随规则(§3.2)。
