# Playground（图像生成测试台）

路径：`/agent/playground`

Playground 是一个交互式的图像生成测试界面，让你无需写代码即可测试图像生成 API 的效果。

---

![Playground](../images/playground.png)

## 页面结构

### 参数面板

位于页面左侧或顶部，包含以下控制项：

| 参数 | 说明 |
|------|------|
| **Model** | 下拉选择，从已配置的 ImageGen 转发规则中自动读取可用模型 |
| **Size** | 图像尺寸：256×256 / 512×512 / 1024×1024 / 1024×1792 / 1792×1024 |
| **Quality** | 图像质量：auto / low / medium / high / standard |
| **N** | 生成数量，范围 1–10 |
| **Prompt** | 多行文本框，输入图像描述 |

### 生成与结果区

- **Generate 按钮**：提交生成请求（未选择模型或未填写 Prompt 时禁用）
- 生成中显示加载旋转图标
- 生成完成后，结果图片以网格方式展示在页面中
- 每张生成图片可直接在浏览器中查看或右键保存

---

## 使用前提

Playground 需要先在 [ImageGen 场景](./06-scenario-special.md) 中配置至少一条转发规则，Model 下拉列表才会有可选项。

---

## 使用流程

1. 前往 `/agent/imagegen`，确认已配置图像生成转发规则
2. 点击页面上的 **Open Playground** 按钮，或直接访问 `/agent/playground`
3. 选择模型和参数
4. 在 Prompt 框中输入描述，如：`a watercolor painting of a mountain lake at sunset`
5. 点击 **Generate**，等待图片生成
6. 查看结果，调整参数后可继续测试

---

## 相关页面

- [ImageGen 场景](./06-scenario-special.md)
- [场景总览](./02-scenario-overview.md)
