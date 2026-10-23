# Image（图像）

路径：`/image/playground`（默认落点，也是 `/image` 的目标）· `/image/profile/:id`（每个 Profile 一个页面）· `/image/library` · `/image/api`（旧的 `/agent/image` 与 `/agent/imagegen` 会重定向到这里）

![Image Playground](../images/image-playground.png)

**Image** 是左侧导航栏中独立的一级入口。它的侧边栏自上而下：

1. **Playground**（副标题 `default`）——日常工作台
2. 每个 **Profile**（封面缩略图 + 名称）各占一行，以及 **New profile**——与 Claude Code 的 Profile 同样的形态
3. 分隔线，然后是 **Library**——你保留下来的图片和提示词片段
4. **Image API**——接入客户端的页面（Base URL、Quick Start、模型规则），排在最后，因为只有要给自己的应用接入的人才需要它

工作面在前，共享的东西在后。Image 默认显示；[Power-ups 菜单](./02-scenario-overview.md#power-ups齿轮背后的菜单)里的 **Image** 开关可以隐藏或显示整个入口。

> Profile、Library 以及 Playground 的会话（生成记录和导入的图片）都保存在**当前浏览器的 IndexedDB** 里——刷新不丢，但不会跟随你到另一个浏览器或机器。生成的图片*文件*保存在服务器的输出目录（页面上会显示，并带复制按钮），删除某条记录、某个 Profile 或某个 Library 条目都不会删掉这些文件。

---

## Image API

![Image API](../images/image-api.png)

只需配置一次的页面——标准的 Agent 页面（[Claude Code · 页面结构](./03-scenario-claude-code.md#页面结构)）：

1. **页头**：`Image API`，带状态行、**Requests / Usage / Log**、**Try in Playground**，以及实心的 **Quick Start**——一个对话框（*Image API Quick Start*），给出可直接复制的 `images/generations` 或 `images/edits` 调用：把 base64 响应解码成图片文件；模型令牌可从 `GET /api/v1/token` 获取
2. **连接卡**：Base URL（`/tingly/imagegen`）和 API Key，带复制按钮和 **Local | Docker**
3. **Plugins 卡**：只有 **Record**
4. **Image Model Rules**（可折叠）：图像生成/编辑模型的路由规则——**New Model**、**Connect AI**、**Test All**

---

## Image Playground

交互式工作台。它没有自己的规则——读取的是与 Image API 相同的 `imagegen` 规则，每次运行都走客户端会用到的真实 `/tingly/imagegen` 路径。右上角的 **Use from your app** 按钮会跳到 Image API（*Call these models from your own app through the Image API*）。

在大屏幕上它是一个占满高度的**工作台**，而不是一个可滚动的普通页面：左侧是固定宽度的控制列，右侧结果面板占据剩余全部宽度（最低高度 600px，再矮时改为页面滚动，而不是把 prompt 挤扁）。

### 控制区（左侧）

- **Reference images（参考图）**——可选；可拖放到虚线框中，也可点击 **Browse** 浏览、**Paste** 从剪贴板粘贴，打开 **Sketch** 画布，或从 **Library** 里选（可多选，不超过上限），最多 **5** 张（PNG/JPEG/WebP）。缩略图可拖拽重新排序（或用方向键移动），每张图还可以携带：
  - 一个 **mask（蒙版）**（画笔图标）——见下文 [Mask 工具](#mask-工具)；只有**第一张**图的 mask 会真正被发送，如果被蒙版的图不再是第一张，参考图行会明确提示这一点
  - 一份 **sketch（草图）**（如果来自草图工具）——可重新打开继续编辑，因为"完成"并不等于"锁定"
- **Model**——你选的是 **provider · model**（例如 `OpenAI · gpt-image-1`），而不是规则名。列表末尾的 **Choose another model…** 会打开与规则编辑器相同的模型选择器（[Model Select](./21-model-select.md)）。后台会找到或创建对应的 `imagegen` 规则，所以 API 客户端可以调用同一个名字；你自己建的多服务规则会按名字列在 *Your rules* 下。还没选时显示 *Choose a model to start*
- **Prompt**——多行文本框，随列高自动撑满剩余空间；提供**插入片段**按钮（来自 [Library](#library)，插在光标处；菜单底部有 *Manage snippets…*）、"打开文本文件作为 prompt"按钮，以及展开为更大编辑器的按钮。占位文字会根据当前状态自动切换（蒙版编辑、草图编辑、基于参考图编辑，或普通新建生成）
- **Size** / **Quality**（auto/low/medium/high/standard）/ **N**（数量，1–10）
- **Generate** 按钮——文案随状态变化："Generate"、有参考图时的 "Generate from N images"，或批量生成进行中时的 "Generate another · N running"。在 Prompt 框中按 `⌘/Ctrl + Enter` 可直接提交。

### Mask 工具

![Mask Editor](../images/image-mask-editor.png)

在**第一张**参考图上涂抹模型可以修改的区域；未涂抹的区域保持不变，Prompt 描述涂抹区域应该出现什么内容。

- **工具**：Paint（画笔）/ Erase（橡皮），另有 **Invert**（反转选区）和 **Clear**（清空）
- 画布尺寸与参考图的真实像素尺寸完全一致
- Mask 可以随时编辑或移除，且在参考图被重新排序时依然跟随该图；只有当这张图本身被移除时 mask 才会一并消失

### Sketch 工具

一个按请求配置的 Size 尺寸绘制的自由画布——Pen/Eraser、笔刷粗细、颜色、撤销——用于画一个粗略草图，再由 Prompt 描述它应变成什么样。其中附带一个**人偶姿势**附加工具：拖动人偶的关节即可摆出姿势（内置 Standing、Walking、Sitting、Kneeling、Waving 等预设，以及从正面到俯视共六种镜头角度），无需手绘即可描述角色的姿态。

### 结果面板（右侧）

- 一条**时间线**，汇总本次会话中的所有内容——生成结果与导入的图片混排，按从旧到新排列——只保留最近的若干条，更早的条目折叠进末尾的一个"查看全部"格子中
- 空状态：*Generated images appear here*，带 **Import** 和 **From clipboard**（用于在不生成的情况下裁剪、切分或蒙版某张图），下方是 `Generated images are saved to: <输出目录>` 和复制按钮
- 每次生成显示它的 Prompt，并带 **Copy prompt**、**Edit this request**（把这次运行的参考图、Prompt、模型和设置重新载入面板）、**Save this request as a profile**（调色板图标——见 [Profile](#profile)）和 **Remove this generation**；进行中的可以**取消**，失败的可以**重试**。移除记录绝不会删除已写入输出目录的文件
- 每张图片（结果、参考图或导入图）都会在同一个共享的**查看器（lightbox）**中打开：复制 Prompt、下载、**Use as reference**（重新作为参考图输入，无需离开页面即可连续编辑）、**Split into tiles**、在适用时编辑草图/蒙版，以及 **Keep in library**（保留后变成 *In the library*）
- 图片也可以直接**导入**（拖放/粘贴/浏览）作为一等的工作图片，享有与生成结果相同的查看器与工具

### Overview（分页画廊）

![Image Overview](../images/image-gallery.png)

上方的结果条回答的是"现在正在发生什么"；从结果条打开的 **All session images** 回答的是"我二十张图之前生成的那张在哪"——本次会话的全部图片都在一个页面里，按最新在前排列，并提供：

- 在 Prompt 和文件名中**搜索**
- **分页**，每页 24 格
- 每个格子：缩略图、编辑结果角落最多 3 个**来源图徽标**（点击可打开来源图）、hover 操作（预览、用作参考图、重试失败项、移除），以及两行说明文字（Prompt 或文件名；model/size/quality 或导入图的尺寸/大小）
- **Clear session** 一次性清空所有内容（需确认）——这是唯一能看到"总共有多少可以清空"的地方

### 切分为方格

通过可拖拽、可缩放的方框和可调节的间隙，将结果切成均匀的行×列网格——用于制作贴纸表、联系表或精灵图（spritesheet）。点击某格可将其排除，随后可下载单个方格，或将剩余方格打包为 ZIP 下载。

### 多图与蒙版编辑

单次编辑请求最多可附带 5 张参考图——这是该场景所路由到的各家 Provider 的通用上限。每张图理论上都可以独立携带 mask，但底层 API 只对**第一张**图应用 mask，因此只有第一张图的 mask 会被发送；当被蒙版的图被拖出第一位时，参考图行会明确提示这一点。

---

## Profile

![图像 Profile](../images/image-profile.png)

**Profile** 把一项持续的工作固定下来：它的**参考图**（由你选，不超过 5 张的上限）、**提示词**，以及 **model / size / quality / N**。它的页面就是已经就位的 Playground。改动**随改随存**——不存在"未保存"状态——第一次打开某个 Profile 时，标题下会有一行说明（*A profile keeps its references, prompts and settings. Changes save as you go.*——点 **Got it** 关闭）。

- **创建**：侧边栏的 **New profile** 会新建一个空的并打开它，名称处于待输入状态；或者当某次结果满意时，点那次运行上的 **Save this request as a profile**（调色板图标），把它的参考图、Prompt、模型和设置变成一个 Profile——这次生成也会复制进去。页头的 **Save as profile** 则用面板里当前的内容创建（只有存在可保留内容时才显示）
- **一个 Prompt 框，多条提示词**：框上方的一排标签（`Prompt 1`、`Prompt 2`……以及 **New prompt**）用来切换正在编辑的那一条；双击标签可重命名，悬停在当前标签上可删除（至少保留一条）。输入在停顿片刻、切换标签或离开页面时保存
- **历史按 Profile 隔离**：每个页面只显示自己的生成记录和导入的图片，**Clear session** 只清当前这个。页头有 **Use from your app** 和 **Delete profile**（需确认；已生成的文件留在输出目录，Profile 的参考图、提示词、设置和历史会被删除）
- Profile 保存在当前浏览器——见顶部的说明

---

## Library

![图像素材库](../images/image-library.png)

**Library**（`/image/library`）是一个收藏架：区别于*正在使用*的（Profile、参考图行）和*发生过*的（会话历史，可清空），它放的是你想*留下*的。页头显示 `Stored in: <输出目录>`，有两个标签页：

- **Images**：你在查看器里用 **Keep in library** 留下的图片（不会有任何图片被隐式加入；同一张图留两次不会重复）。点击某张可以重命名或删除（*Deletes the image and its file. Profiles that already use it keep their copy.*）
- **Snippets**：你反复使用的提示词片段——一种质感、一种镜头、一套配色。**New snippet** 填写名称和文本；可在此编辑或删除。在 Playground 的 Prompt 框里用片段按钮插入（需要时会补一个逗号）

使用素材库里的图片是**拷贝**——拷进参考图行（**Library** 按钮，可多选，不超过上限）或某个 Profile——所以删除素材库条目绝不会改变已经用上它的 Profile。

![片段](../images/image-library-snippets.png)

---

## 接入方式

```bash
curl <tingly-box-imagegen-url>/images/generations \
  -H "Authorization: Bearer <api-key>" \
  -H "Content-Type: application/json" \
  -d '{"model": "dall-e-3", "prompt": "a cute cat", "n": 1, "size": "1024x1024"}'
```

---

## 相关页面

- [场景总览](./02-scenario-overview.md)
- [Custom / Embed](./06-scenario-special.md)
- [用量面板](./11-dashboard.md)
- [Model Select](./21-model-select.md)
