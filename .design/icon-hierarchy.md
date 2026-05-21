# Icon Hierarchy

UI 中使用了多个层面的 icon，来自不同的库，承担不同的职责。

---

## Icon 来源（库）

| 库 | 用途 | 为什么 |
|---|---|---|
| `@mui/icons-material` | 通用 UI 操作图标 | 与 MUI 组件系统无缝集成，颜色/尺寸遵循 theme |
| `@tabler/icons-react` | 导航与插件特性图标 | 线条风格更细腻，适合侧边栏等对视觉密度要求高的场景 |
| `@lobehub/icons-static-svg` | AI 提供商品牌 logo | 专门维护 50+ LLM 提供商的官方 SVG，避免手工维护 |
| 自定义 SVG (`src/assets/icons/`) | 企业通讯工具 logo | Lobehub 未覆盖的 IM 平台（钉钉、飞书、微信等）单独维护 |

---

## Icon 层面（Hierarchy）

### Layer 1 — Navigation（导航层）

- **位置**：Activity Bar、Sidebar
- **库**：Brand Icons + Tabler Icons
- **尺寸**：20px
- **文件**：`src/layout/Layout.tsx`, `src/layout/Sidebar.tsx`
- **为什么**：导航 icon 代表功能入口，需要高辨识度且与品牌风格一致；Tabler 的线条风格在小尺寸下仍清晰。

```tsx
<IconPlus size={20} />        // @tabler/icons-react
<Claude size={20} />          // BrandIcons
```

---

### Layer 2 — Page / Dialog Header（页面/对话框标题层）

- **位置**：Dialog 标题区、页面区块标题
- **库**：`@mui/icons-material`
- **尺寸**：默认 24px，有时加 `sx={{ fontSize: 20 }}`
- **语义色**：`color="warning"` / `color="info"`
- **文件**：`src/components/ForceAddConfirmDialog.tsx`
- **为什么**：标题层 icon 传达操作的严重性（警告/信息），必须使用语义色以符合可访问性要求。

```tsx
<WarningAmber color="warning" />
<Info color="info" />
```

---

### Layer 3 — Component Control（组件控制层）

- **位置**：表格展开/收起、折叠面板
- **库**：`@mui/icons-material`
- **尺寸**：默认 24px
- **文件**：`src/components/SmartRoutingLogViewer.tsx`
- **为什么**：这类 icon 是交互控件的视觉提示，需要与 MUI 组件的尺寸系统对齐。

```tsx
<KeyboardArrowDownIcon />
<KeyboardArrowUpIcon />
<ExpandMore />
<ExpandLess />
```

---

### Layer 4 — Action Button（操作按钮层）

- **位置**：Button 的 `startIcon` / `endIcon`
- **库**：`@mui/icons-material`
- **尺寸**：`fontSize="small"`（20px）
- **文件**：`src/components/ApiKeyModal.tsx`, `src/components/ApiKeyTable.tsx`
- **为什么**：按钮 icon 辅助文字标签，传达操作意图（复制、删除、编辑），`small` 尺寸避免喧宾夺主。

```tsx
<Button startIcon={<ContentCopy fontSize="small" />}>Copy</Button>
<Button startIcon={<DeleteIcon fontSize="small" />} color="error">Delete</Button>
```

---

### Layer 5 — Form Field Adornment（表单字段装饰层）

- **位置**：TextField `InputAdornment`
- **库**：`@mui/icons-material`
- **尺寸**：`fontSize="small"`
- **文件**：`src/components/model-select/ModelsPanel.tsx`, `src/components/ConnectProviderDialog.tsx`
- **为什么**：字段装饰 icon 提示输入类型（搜索、密钥可见性），不参与交互时保持低视觉权重。

```tsx
<InputAdornment position="start">
  <SearchIcon fontSize="small" />
</InputAdornment>
```

---

### Layer 6 — Status Indicator（状态指示层）

- **位置**：健康检查、探针结果、连接状态
- **库**：`@mui/icons-material`
- **尺寸**：`fontSize="small"` 或固定 24px
- **语义色**：`color="success"` / `color="error"`
- **文件**：`src/components/HealthIndicator.tsx`, `src/components/ProbeModal.tsx`
- **为什么**：状态 icon 是系统反馈的第一视觉信号，颜色语义（绿/红）必须清晰且一致。

```tsx
<CheckCircle color="success" fontSize="small" />
<Error color="error" fontSize="small" />
```

---

### Layer 7 — Alert / Message（提示消息层）

- **位置**：Alert 组件内的自定义 icon
- **库**：`@mui/icons-material`
- **尺寸**：`fontSize="small"`
- **文件**：`src/components/OAuthDialog.tsx`, `src/components/OAuthDetailDialog.tsx`
- **为什么**：Alert 默认 icon 有时不符合场景语义（如 OAuth 成功需要 Launch icon），自定义 icon 增强信息表达。

```tsx
<Alert severity="success" icon={<Launch fontSize="small" />}>
<Alert severity="info" icon={<Info fontSize="small" />}>
```

---

### Layer 8 — Table Row Action（表格行操作层）

- **位置**：表格行内的 `IconButton`
- **库**：`@mui/icons-material`
- **尺寸**：默认 24px
- **语义色**：删除用 `color="error"`，其他默认
- **文件**：`src/components/ApiKeyTable.tsx`
- **为什么**：行操作 icon 需要足够的点击区域（通过 `IconButton` 保证），同时不破坏表格行的视觉节奏。

```tsx
<IconButton onClick={handleEdit}><EditIcon /></IconButton>
<IconButton onClick={handleDelete} color="error"><DeleteIcon /></IconButton>
<IconButton><MoreVert /></IconButton>
```

---

### Layer 9 — Brand / Provider（品牌提供商层）

- **位置**：提供商列表、模型选择器、连接对话框
- **库**：Lobehub SVG + 自定义 SVG（通过 `BrandIcons.tsx` 统一封装）
- **尺寸**：20–32px
- **主题适配**：灰度/反色 filter 根据 `theme.palette.mode` 切换
- **文件**：`src/components/BrandIcons.tsx`, `src/components/ProviderIcon.tsx`
- **为什么**：提供商 logo 是品牌识别的核心，需要与普通 UI icon 分开管理；`createBrandIcon` 工厂统一处理 dark/light 模式下的滤镜。

```tsx
// BrandIcons.tsx
const createBrandIcon = (src, alt, defaultGrayscale, monochrome) => ...

// ProviderIcon.tsx — 50+ 提供商映射
const iconMap: Record<string, FC<BrandIconProps>> = {
  'openai': OpenAI,
  'anthropic': Anthropic,
  ...
}
```

---

### Layer 10 — Empty State / Placeholder（空状态占位层）

- **位置**：空列表、引导用户添加第一项内容
- **库**：`@mui/icons-material`
- **尺寸**：28–60px（刻意放大）
- **颜色**：`color: 'text.secondary'`（低调）
- **文件**：`src/components/RoutingGraph.tsx`
- **为什么**：空状态 icon 是装饰性的，用大尺寸引导视线，用低对比度避免与 CTA 竞争注意力。

```tsx
<AddIcon sx={{ fontSize: 40, color: 'text.secondary' }} />
```

---

## 尺寸与颜色规范速查

| 场景 | 尺寸 | 颜色 |
|---|---|---|
| 导航、品牌 | 20px | 由 theme primary / 品牌色决定 |
| 标题、组件控制 | 24px（默认） | 语义色或继承 |
| 按钮、表单、状态、提示 | `small`（20px） | 语义色（success/error/warning/info） |
| 行操作 | 24px（默认） | 默认或 error |
| 空状态 | 28–60px | `text.secondary` |

---

## 为什么使用多个层面的 icon

1. **视觉权重分级**：不同层面的 icon 尺寸不同，用户视线自然先落在大 icon（空状态引导）→ 中 icon（导航、标题）→ 小 icon（操作、状态），形成清晰的视觉层次。

2. **语义与交互分离**：品牌 icon 纯展示，操作 icon 触发行为，状态 icon 反馈结果——三类职责不混用，降低认知负担。

3. **库选型合理性**：MUI 负责通用 UI，Tabler 负责高密度导航场景，Lobehub 负责 LLM 生态品牌，自定义 SVG 兜底——各司其职，避免单一库的覆盖盲区。

4. **主题一致性**：所有 icon 通过 MUI theme 的颜色系统（`color="success"` 等）或 `BrandIcons` 的 filter 逻辑统一适配 dark/light 模式。
