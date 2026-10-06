# Catalog Packs（pencil）

提案图解，对应 [model-data.md](./model-data.md)。**未实施**；与代码不一致时以代码和 model-data.md 为准。

## 1. 现状：两份数据，两套格式

```
 providers.json (供给)                      claude.models.json (能力)
 ┌───────────────────────────┐              ┌───────────────────────────┐
 │ anthropic-com   ──┐       │              │ claude-opus-5-5           │
 │ claude-code     ──┤       │              │ claude-sonnet-5-5   ...   │
 │ aws-bedrock     ──┼─ 都提供 Claude 模型   │ (dialects / efforts)      │
 │ gcp-vertex-claude─┤       │              └─────────────▲─────────────┘
 │ antigravity     ──┤       │                            │ 只按模型名查
 │ groq-com        ──┘       │              LookupClaudeThinkingCaps(model)
 │ kimi / qwen / ...         │              （运行时不知道 provider）
 └───────────────────────────┘
   各自的加载、校验、扩展方式 → 用户无法扩展任何一边
```

## 2. 不要这样合：把能力塞进 claude provider

```
 anthropic-com ──含能力──┐
 aws-bedrock ───引用────►│  ✗ 依赖方向错：能力是模型族属性，
 gcp-vertex ────引用────►│    与"谁提供"无关；查询时也拿不到 provider
```

## 3. 提案：统一容器格式（pack），不合并实体

```
 pack = { providers?: {...},  models?: [...] }      两个 section 都可选

 ┌ core pack ───────────┐  ┌ anthropic pack ──────────┐  ┌ 用户 pack ───────────┐
 │ providers: kimi,qwen…│  │ providers: anthropic-com,│  │ providers: my-gw     │
 │ models:    —         │  │   claude-code, bedrock…  │  │ models:    my-model  │
 └──────────┬───────────┘  │ models: claude-*  (能力) │  └──────────┬───────────┘
            │              └────────────┬─────────────┘             │
            └──────────────┬────────────┴───────────────────────────┘
                           ▼
                  ┌─────────────────┐   合并优先级：用户 > 远程 > 内嵌
                  │  Pack Loader    │   坏文件：跳过 + 在 UI/诊断提示，不阻塞启动
                  └───────┬─────────┘
            ┌─────────────┴─────────────┐
            ▼                           ▼
   providers 索引 (按 id)       model 能力索引 (最长 key 匹配)
   GetTemplate / GetAll…        LookupClaudeThinkingCaps  ← 消费方不变
```

## 4. 来源与校验

```
 内嵌 (go:embed) ─┐
 远程 GitHub ─────┼─► Pack Loader ─► CheckCatalogs ◄── harness catalog check -f <pack>
 ~/.tingly-box/   │                  （同一套规则）      （CI 与用户本地共用）
   catalog.d/*.json ┘
```

## 5. 与 flag / plugin 的关系

```
  声明式（pack，数据）            行为扩展（plugin，代码）
  ───────────────────            ─────────────────────
  openai_endpoint_mode           以后单独设计
  web_search_schema        ───►  pack 只引用 flag key
  api_style / 默认 flag 值       不在 pack 里放代码
  (= provider 级 flag 的默认值，见 provider-flags.md)
```

## 6. 分步

```
 ① pack schema + 多源加载器      行为不变，两个现有文件原样读取
 ② claude.models.json → pack 的 models；Claude 相关 provider 拆成 anthropic pack
 ③ 用户目录 catalog.d/
 ④ 对接 provider 级 flag
```

## 待定

- 用户 pack 能否覆盖内置 provider（如 anthropic-com 的 base URL）？倾向第一版只允许新增 + 覆盖能力表。
- 远程 registry 按 pack 下发，还是仍只下发 providers？
