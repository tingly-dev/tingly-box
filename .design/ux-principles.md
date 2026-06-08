# UX-First 原则

> 技术产品的核心仍然是产品本身。没人用的产品没有价值。
> 技术需要合理实现，但当 UX 与实现便利冲突时，让步的方向永远是让 UX 做得更彻底。
>
> 本文档不是设计风格指南，也不是组件清单。它是**做新功能/改旧功能时的判断标准**——从最近几周的实战工作（probe 重做、routing 教育系统、Connect AI 统一、版本更新对话框、Quick Start 改造、Smart Guide on Claude Code 等）中提炼出的、可复用于未来工作的原则。

---

## 一句话的判断标准

> **"用户此刻在想什么、想做什么、下一步要拿到什么？"**
>
> 把 UI 围绕这三个问题排版；把命名拉到一个词一个义；把决策替换成聪明的默认；把诊断接到真实链路；把下一步动作的物件直接交到手上。

---

## 1. 信息架构以"用户脑中的问题"组织，而不是后端分类 — Organize IA around user questions, not backend taxonomy

**典型案例**：Probe 重做。旧版按 mode 分类（simple/streaming/tool、direct/loopback）展示数据；新版重新组织为用户真正在问的三个问题——**"成功了吗 / 请求是怎么走的 / 返回了什么"**——于是出现 "请求旅程"（Rule → Flags → Routing → Provider→Model → Endpoint → Upstream URL）这个布局基元。

**指导原则**：任何诊断/详情/结果视图，先写出"用户此刻想知道的 3 个问题"，再让那 3 个问题成为分区标题。后端的字段分类不应直接外露。

---

## 2. 消解模式选择，把工作面直接打开 — Eliminate mode pickers; open the work surface directly

**典型案例**：
- Probe 取消 mode-picker 菜单，trigger 直接开 dialog，形态/范围在里面切换、就地 re-run。
- Import 从 toolbar 的独立按钮挪进 Connect AI 选择器。
- ConnectProviderDialog 统一所有连接入口。

**指导原则**：能"直接进入工作面"就不要"先选模式再进入"。让用户先看到东西，再在内部微调。每一次"选完才能开始"都是一次摩擦税。

---

## 3. 命名碰撞必须拆开，词汇必须全局统一 — Split name collisions; keep vocabulary globally consistent

**典型案例**：
- Probe 旧版 "Direct" 同时承担"流/非流"和"经过 TB/直连上游"两个含义 → 拆成 **形态（Nonstream/Stream）× 范围（经过 TB/直连上游）**。
- "Add API Key / Add New API Key / + Connect" → 全部统一为 **Connect AI**，跨页面、跨 i18n。

**指导原则**：一个词在产品中只能指一件事；一件事在产品中只能用一个词。一旦发现一个名字承载两个概念，**立刻拆**——这是后续所有混乱的源头。

---

## 4. 正交维度必须分轴呈现 — Separate orthogonal dimensions onto their own axes

**典型案例**：
- Probe 的形态 × 范围拆分。
- Smart Guide changedir 设计：**会话锚点 vs 逻辑 pwd 是两条独立的轴**，对应"shell 心智模型"——session 不变，pwd 自由漂移。

**指导原则**：发现一个旋钮在控制两件事，怀疑底层模型错了。先把轴拆清楚，UI 自然会变简单。

---

## 5. 展示"具体值"，而不是别名 — Show the concrete value, not the alias

**典型案例**：
- 自定义 UA 预设：主行展示**真实 UA 字符串（等宽字体）**，友好名作为 caption。
- TOFU 配对面板：展示完整 `/bind <code>` 而不是裸 code。
- Smart routing 引导示例：展示 `model: contains: claude` 这种真实条件，而不是空占位。

**指导原则**：如果用户最终要面对/复制/对照的是字面值，就直接给字面值。标签是辅助，不能替代实物。

---

## 6. 合理的默认值，优于多一个开关 — Smart defaults beat another toggle

**典型案例**：
- Affinity 在 Claude Code 等 agent 场景**默认开启**。
- Probe 默认 Stream 模式（更贴近生产）。
- 路由图自动滚动到 80% 位置（重要节点直接进入视野）。
- 版本检查 cache TTL 从 24h 降到 2h。

**指导原则**：能用一个聪明的默认覆盖 80% 用户的场景，就不要做成配置项。配置项不是友好——它是**把判断成本转嫁给用户**。

---

## 7. 诊断要走真实链路；"假路径"只为回答一个特定问题 — Diagnostics traverse the real path; fake paths exist only to answer a specific diff question

**典型案例**：Probe 重做的关键决策——所有 probe 走 TB 自己的 loopback `/tingly/{scenario}`，让 flags / smart routing / load balancer 全部按生产路径执行。**Direct 模式**保留下来，但角色被重新定义——不是"省事的旁路"，而是**"用来区分故障在 TB 内还是上游"的对照实验**。

**指导原则**：诊断/调试工具必须走生产代码路径。如果非要有"快路径/直连路径"，它的存在理由必须是回答一个具体的差分问题，而不是"实现起来简单"。

---

## 8. 教育要内嵌产品，用可视化和交互式步骤 — Embed education in the product with visuals and interactive steps

**典型案例**：
- EntryGuideDialog：Direct vs Smart 模式的 6 步交互引导 + 路由图示。
- TierGuideDialog：垂直 stepper + 悬停注释。
- 路由图自动滚动、tooltip 跨主题统一、节点 action 按钮可见性。

**指导原则**：核心概念（路由模式、tier、scenario、smart rule）需要**产品内的视觉教材**。不要假设用户读文档，也不要靠"用了几次就懂了"——给一张图、一个 stepper、一个 hover。

---

## 9. 降低视觉噪声，让主体成为视觉锚点 — Reduce visual noise; let the subject be the visual anchor

**典型案例**：Provider 协议 tag / 角标（OpenAI / Anthropic / Key / OAuth / CN / Global）原本太抢眼 → 改为淡化的文本标签，让 **provider name 成为视觉锚点**。

**指导原则**：每一块区域问一次"谁是主角？"——主角是主体名字，元信息要让位。视觉层级混乱比信息不全更伤体验。

---

## 10. 完成 ≠ 锁死：保留再进入与可逆性 — Done ≠ locked: preserve re-entry and reversibility

**典型案例**：
- Quick Start 步骤完成后**仍然可展开**，可以重新复制安装命令。
- Step 4 加 **Skip** 按钮——不想跑就跳过，不阻塞主流程。
- Probe dialog 内部可重新运行。

**指导原则**：不要因为"完成了"就把工具藏起来。用户会回来复制、复查、重做。"完成"是状态，不是终结。

---

## 11. 显式提供"下一步动作所需的物件" — Hand over the artifact needed for the next action

**典型案例**：
- 版本更新对话框：直接给 `npx` / bundle / docker 命令 + copy 按钮；release URL 改到 GitHub releases（用户能 act 的地方），而非 npm 包页面。
- 检测到 OpenAI 协议 + 缺 `/v1`：常驻 tooltip 提示。
- TOFU：直接给完整命令。

**指导原则**：不要"通知 + 让用户去想下一步"。把下一步的具体物件（命令、链接、配对串）直接放进同一个视野。"informed"是低标准，"enabled"才是。

---

## 12. 副作用要限定在用户当前所处的表面 — Scope side effects to the surface the user is currently on

**典型案例**：`ui: should only scroll in guiding graph.`——滚动只发生在引导图本身，不冒泡到外层页面。同理，probe 控件作用域在 dialog 内、scenario flag 的 setter 收敛到一个 factory。

**指导原则**：用户在哪儿，作用域就到哪儿。任何"我点了 A，B 也跟着动"的行为必须有强理由。

---

## 用法

做新功能或评审 PR 时，把这 12 条当作 checklist 过一遍：

- [ ] 视图分区是按"用户的问题"组织的吗？
- [ ] 用户能直接进入工作面，还是被先要求选模式？
- [ ] 这里的每个名词在全局只有一个含义吗？
- [ ] 这个旋钮是不是在控制两件事？
- [ ] 用户要面对的是别名还是真实值？
- [ ] 默认值覆盖了 80% 的场景吗？
- [ ] 诊断/调试走的是生产路径吗？
- [ ] 核心概念有内嵌的视觉教材吗？
- [ ] 这块区域的"主角"足够突出吗？
- [ ] 完成之后用户还能回来再用吗？
- [ ] 用户拿到的是"通知"还是"下一步的物件"？
- [ ] 副作用是否被限定在当前可见的表面？
