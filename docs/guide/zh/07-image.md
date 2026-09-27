# Image（图像）

路径：`/image/playground`（默认落点，也是 `/image` 的目标）· `/image/api`（配置页；旧的 `/agent/image` 与 `/agent/imagegen` 会重定向到这里）

![Image Playground](../images/image-playground.png)

**Image** 现在是左侧 Activity Bar 中独立的一级入口，包含两个页面：**Playground**（日常工作台）和 **Image API**（Base URL、Quick Start 与模型规则——只需配置一次）。它原来只是挂在某个 Agent 场景页上的一张卡片；由于使用频率远高于一次性的配置页面，把"客户端怎么接入"和"我现在要出一张图"这两个问题混在一起会让两者都更难回答，因此它被拆成独立的导航入口。Image 默认在侧边栏中可见（[场景总览](./02-scenario-overview.md) 上的眼睛图标仍可控制其显隐，因为可见性依旧由底层的 `imagegen` 场景 id 驱动）。

---

## Image API

![Image API](../images/image-api.png)

只需配置一次的页面——与其他场景页面结构相同：

1. **Image API 配置卡**：Base URL（`/tingly/imagegen`）和 API Key，均带复制按钮；**Quick Start** 按钮（curl 示例，一键复制）；**Try in Playground** 按钮
2. **Image Model Rules**（可折叠）：图像生成/编辑模型的路由规则

---

## Image Playground

交互式工作台。它没有自己的规则——读取的是 Image API 页面上配置的同一个 `imagegen` 场景规则，每次运行都走客户端会用到的真实 `/tingly/imagegen` 路径。

在大屏幕上它是一个占满高度的**工作台**，而不是一个可滚动的普通页面：左侧是固定宽度（360–420px）的控制列，右侧结果面板占据剩余全部宽度，两者都撑满内容区高度（最低 600px，再矮时改为页面滚动，而不是把 prompt 挤扁）。

### 控制区（左侧）

- **Reference images（参考图）**——可选；可拖放到虚线框中，也可点击 **Browse** 浏览、**Paste** 从剪贴板粘贴，或打开 **Sketch** 画布，最多 **5** 张（PNG/JPEG/WebP）。缩略图可拖拽重新排序（或用方向键移动），每张图还可以携带：
  - 一个 **mask（蒙版）**（画笔图标）——见下文 [Mask 工具](#mask-工具)；只有**第一张**图的 mask 会真正被发送，如果被蒙版的图不再是第一张，参考图行会明确提示这一点
  - 一份 **sketch（草图）**（如果来自草图工具）——可重新打开继续编辑，因为"完成"并不等于"锁定"
- **Model** 下拉框，从 Image Model Rules 中读取
- **Prompt**——多行文本框，随列高自动撑满剩余空间；提供复制按钮、"打开文本/prompt 文件作为 prompt"按钮，以及展开为更大编辑对话框的按钮。占位文字会根据当前状态自动切换（蒙版编辑、草图编辑、基于参考图编辑，或普通新建生成）
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

- 一条**时间线**，汇总本次会话中的所有内容——生成结果与导入的图片混排，按从旧到新排列——只保留最近的若干条，更早的条目折叠进末尾的一个"查看全部"格子中，而不是继续留在行内可滚动
- 每张图片（结果、参考图或导入图）都会在同一个共享的**统一查看器（lightbox）**中打开：显示 Prompt、Model、Size/Quality、复制 Prompt、下载、**Edit this image**（将其重新作为参考图输入，无需离开页面即可连续编辑），以及 **Split into tiles**
- 正在进行的生成可以**取消**；失败的生成可以**重试**；任意条目都可以从会话中**移除**（确认对话框会说明已写入磁盘的文件不受影响）
- 图片也可以直接**导入**（拖放/粘贴/浏览）作为一等的工作图片，享有与生成结果相同的查看器与工具，而不仅仅是请求参数

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
- [用量看板](./11-dashboard.md)
