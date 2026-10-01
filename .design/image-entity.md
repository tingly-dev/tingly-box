# 图像设定（Image Entities）

> 状态：**前端原型，未接入后端**。数据是 `frontend/src/pages/image/entities/mockEntities.ts`
> 里的 mock，存在模块内存里；生成请求本身没有改动，不会带上设定的参考图。

## 1. 是什么

把"每次都要重新描述 / 重新拖参考图"的输入存成一个有名字的对象，在 prompt 里用
`@名称` 引用。生成结果是一次性的产物，设定是长期留着的输入。

MVP 只有两种：

| kind | 固定什么 | `@` 怎么展开 |
|---|---|---|
| character（角色） | 画面里是谁/是什么 | 原地展开为 `名称（标准描述）`，句子保持通顺 |
| style（风格） | 画面怎么画 | mention 本身去掉，描述追加到 prompt 末尾（风格描述的是整张图） |

Environment 和 Scene 延后：Scene 是其他设定的组合，不是第五种类型，等这两种把组合规则跑通再说。

## 2. 入口（对照 ux-principles）

- **用**：Playground 的 prompt 里输入 `@`，或者点 prompt 下方的「@ 设定」按钮。
- **建**：大多数设定来自一次满意的结果，所以入口放在结果上，即 lightbox 里的「存为设定」。
  `@` 选择器里没有匹配时，也可以直接「新建设定 "xxx"」，保存后补全那个 mention。
- **管**：Image › 设定库（`/image/entities`）。

fal 的 Media Library 里同一组设定出现了三处（侧栏、右栏、tab），这里刻意只保留一个管理入口。

## 3. 组合规则（`composeEntities.ts`）

- 一次 run 的参考图上限是 `MAX_EDIT_REFERENCE_IMAGES`（5）。用户手动附加的参考图优先，
  剩下的名额才给设定用。
- 名额按 mention 顺序**轮转分配**：每个设定先拿到它的第一张图，然后才有人拿第二张。
  设定内部的图片按用户排的顺序取。
- 名额不够、发生截断时，这是一个没人做过的默认选择，所以要**说出来**：在面板里提示
  "林夏 共 4 张，这次只带前 2 张"，并给出「调整顺序」的入口。不能悄悄截断。
- 「实际发送给模型的 prompt」可以展开查看，展示的是具体值而不是别名。

## 4. 接入时要补的

- 后端存储和 API（swagger + `task codegen`），替换 `entityStore.ts`。
- 生成时真正把分配到的参考图带上（generations → 有参考图时走 edits）。
- run 记录里存下这次用了哪些设定（`RunSourceStrip` 显示）。
- `uses` 计数。
