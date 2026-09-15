# 提示与帮助

路径：`/help`

![提示与帮助](../images/help-page.png)

位于活动栏（Activity Bar）图标列底部的灯泡入口——引导向导现在也会把没有任何 Provider 的全新安装引导到这里（`/onboarding` 会重定向至此）。页面副标题：「A few easy-to-miss but useful things.」

三张可折叠的手风琴卡片，彼此都是独立操作，而非线性教程中的某一步——展开一张不会收起其他两张：

## Desktop Shortcut（桌面快捷方式）

仅在当前平台支持时显示。一键创建桌面快捷方式，用于重启 Tingly-Box 并打开 Web UI——它不会自动更新或自动重启服务，创建快捷方式是它唯一做的事。卡片内含 **Create Shortcut** / **Recreate** 按钮。

## Providers（Provider 目录）

默认展开。内嵌与 **Connect AI** 选择器相同的 Provider 目录（搜索框 + 按连接类型分组的卡片——Custom、OAuth sign-in、Self-hosted、API key providers）——完整的选择器说明详见 [凭证管理 · 添加 Provider](./08-credentials.md#添加-provider-connect-ai-流程)。卡片内容超过固定高度后会在内部滚动，避免把另外两张卡片挤出屏幕。

## Routing & Tier Guides（路由与 Tier 引导）

三个按钮重新打开产品中其他位置内嵌的同一套图解引导，因此只需维护一份图表，而不是多份重复内容：
- **Direct routing guide**：与任意场景页面上每位用户首次访问时自动弹出的 [首次访问引导](./20-routing-rules.md#首次访问引导) 是同一个
- **Smart routing guide**：Smart 模式下的对应引导
- **Tier guide**：解释基于 Tier 的故障转移（T0/T1/T2 优先级与熔断机制）

---

## 相关页面

- [凭证管理](./08-credentials.md)
- [路由规则与插件](./20-routing-rules.md)
