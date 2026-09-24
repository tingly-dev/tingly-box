# Image 顶级入口(Playground + Image API)

> 适用对象:tingly-box 前端贡献者。
> 描述生图从 Agent 下的一个 scenario 页,拆成 activity bar 上独立的 **Image**
> 入口的决定与接线。
> 关联文档:`ux-principles.md`(判断标准)、`imageedit.md`(网关链路)、
> `sketch-canvas.md` / `image-mask.md` / `image-slice.md` /
> `playground-run-reentry.md`(Playground 内的各项能力)。

---

## 1. 背景

`/agent/image` 原来一页装了两件事:

| | Image API(imagegen scenario) | Playground |
|---|---|---|
| 用户此刻在问 | "我的客户端怎么接 `/tingly/imagegen`?路由到哪个模型?" | "我要出一张图" |
| 内容 | Base URL、Quick Start、模型规则 | 生成/改图、参考图、草图+人偶、mask、切分/GIF、gallery、重入 |
| 频率 | 配一次 | 天天用 |

Playground 长到约 7k 行、5 份专属设计文档之后,仍然是 scenario 页 `CardGrid`
里 Base URL 卡片下面的第二张卡。用户每次都要越过接入配置才能到工作面(原则 2),
同一个页面在侧栏叫 "Image"、在标题叫 "Image API"(原则 3),两个不同的问题被
按后端分类放在一起(原则 1)。

## 2. 决定

- activity bar 新增 **Image**,两个子项:
  - **Playground** — `/image/playground`,也是 `/image` 与点击 rail 图标的落点(天天用的那个优先)。
  - **Image API** — `/image/api`,即原 `/agent/image`(imagegen scenario:Base URL、Quick Start、规则)。
- 名字统一为 **Image API**:侧栏、Agent 概览卡、页面标题都是这一个词。
- Gallery / 草图 / mask / 切分仍是 Playground 内的对话框(原则 12),不升级为子导航。

## 3. 保留的东西

- **Playground 走真实链路**(原则 7):它没有自己的规则,读取 imagegen scenario 的
  规则,请求走 `/tingly/imagegen`。拆的是入口,不是链路。
- **Agent 侧的入口保留**:Agent 侧栏与概览里的 Image API 仍在,指向 `/image/api`。
  侧栏行带 `match: () => false`,落到 `/image/api` 时选中的是 Image rail,而不是
  Agent(`Layout.tsx` 按"哪个 activity 的子项匹配当前路径"决定高亮,先匹配先得)。
- **一个开关**:Agent 里隐藏 `imagegen` 这个 scenario,会同时隐藏 Image rail
  (`useActivityItems.tsx`)。"我不用生图"只有一个开关,仍是原来那个全局的
  `scenario.hiddenScenarios`。
- **旧路径**:`/agent/image`、`/agent/imagegen` → `/image/api`;
  `/agent/playground` → `/image/playground`。

## 4. 两页之间的物件(原则 11)

- Image API 页头有 "Try in Playground" 按钮。
- Playground 没有可用模型时,提示条直接带 "Add a model" 按钮跳到 Image API,
  而不是一句"去下面添加规则"。

## 5. 代码位置

| 路径 | 内容 |
|---|---|
| `frontend/src/pages/image/ImagePlaygroundPage.tsx` | Playground 页:只加载 imagegen 规则,不等 provider,不出 skeleton |
| `frontend/src/pages/image/ImageApiPage.tsx` | 原 `UseImageGenPage`,去掉 Playground 卡 |
| `frontend/src/pages/image/components/` | 原 `pages/scenario/components/` 下全部 image 专属组件、hooks 与 `imageGenSession` 测试 |
| `frontend/src/layout/useActivityItems.tsx` | Image rail 项与 Agent 侧的快捷行 |
| `frontend/src/App.tsx` | 新路由与旧路径重定向 |

## 6. 整屏工作台(lg 及以上)

Playground 独占一页之后,不再是"页面里的一张卡",而是占满内容区的工作台:

- **页面给高度,面板分高度。** `ImagePlaygroundPage` 在 lg 上取内容区的 100% 高
  度(最低 600px,再矮就让内容区滚动,而不是把 prompt 挤扁)。卡片、网格、两侧面
  板依次 `height: 100%`。原来两侧共用的 `PLAYGROUND_PANEL_HEIGHT` 常量随之删除:两
  侧都填满同一行,天然对齐。
- **左窄右宽。** 控制列固定 360–420px;结果区拿走剩下的全部宽度——用户盯着看的是
  结果。多出来的纵向空间在左边给 prompt(它本来就是"拿剩余高度"的那个元素)。
- **结果卡随高度变大,且保持方形槽位。** 结果条是一个 size container
  (`container-type: size`),卡片的 flex-basis 用 `cqh` 按条的高度算出让每个槽位
  为方形的宽度(`imageGenSession.ts` 的 `stripCardBasis`):多出来的高度变成更大的
  图,而不是图片两侧的留白。算出的宽度不小于原来调好的宽度(矮屏上与之前完全一
  样),不大于结果条本身。
- **lg 以下不变。** md 及以下仍是上下堆叠、整页滚动,结果区固定 320px 高。
