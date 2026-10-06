# internal/catalog

模型相关的静态数据与加载器。改这里的数据前先读 `.design/model-data.md`（分层原因、字段取舍、完备性不变式）。

## 文件

| 文件 | 角色 |
|------|------|
| `providers.json` + `provider_catalog.go` | 供给注册表：谁在哪个端点、以什么限额提供哪些模型。 |
| `claude.models.json` + `claude_model_catalog.go` | Claude 能力目录：模型本身支持的 thinking 方言 / effort 档位。 |
| `claude.models.ref.json` | 核对用的 Anthropic `/v1/models` 镜像，不被 embed、不被任何代码读取。 |
| `catalog_check.go` | 上面两份数据的合法性校验，供测试和 `harness catalog check` 共用。 |
| `model_list.go` | 运行时从 provider API 拉取的模型列表缓存，与静态数据无关。 |

## 改数据的规则

- **新增 Claude 模型：先加 `claude.models.json`，再加 `providers.json`。** 反过来会被跨 catalog 检查拦住。
- `claude.models.json` 只允许 `id` 和 `reasoning`（`dialects` / `mandatory` / `supported_efforts`）；
  dialect 只能是 `budget|adaptive`，effort 只能是 `low|medium|high|xhigh|max`。不要为没有消费方的字段扩 schema。
- `providers.json` 的 map key 必须等于 `id`；非 OAuth、非云凭据的模板至少要有一个 base URL；OAuth 模板必须有 `oauth_provider`。
- `context` / `max_output` 写十进制整数（1M=1000000、256K=256000），不写 2 的幂。
- `sources`、`note`、`last_updated` 等元数据只给人看，Go 会丢弃，不要依赖它们。
- 没有可核实来源的数值不要填，宁可省略字段。

## 校验

改完任何一份 JSON 后必须通过：

```bash
go test ./internal/catalog/
go run ./cli/harness catalog check          # 校验内嵌的 catalog
go run ./cli/harness catalog check -f internal/catalog/providers.json -f internal/catalog/claude.models.json
```

CI 的 `harness-pr.yml` 在 `internal/catalog/**` 变化时会跑同一条 `catalog check`。

## 加新规则

规则写在 `catalog_check.go`（返回 `CheckIssue`，带 catalog 与 subject），并在 `catalog_check_test.go`
里同时覆盖"嵌入数据通过"和"构造坏数据被拦住"。不要在 `cli/harness` 里另写一份校验。
`providers.json` 不做未知字段检查——它本来就带只供阅读的元数据。
