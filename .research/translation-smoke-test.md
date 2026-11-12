# 统一翻译首版：腾讯 1.8B 真实模型实测

Date: 2026-11-12（Asia/Shanghai）。关联：[TEP-2254](https://github.com/tingly-dev/tingly-box/issues/2254)。

## 环境与版本

- 模型：[`tencent/Hy-MT2-1.8B`](https://huggingface.co/tencent/Hy-MT2-1.8B)，本次选择最小参数规模 1.8B 的普通权重版本，不是更大的 7B/30B。
- 固定 revision：`9a341cd1b679d3efd23b46e847b01745a71ed792`。权重缓存约 3.9 GiB，不提交权重。
- CPU 4 核、16 GiB 内存，无 GPU；PyTorch 推理线程设为 4，bfloat16，greedy decoding，最多生成 128 tokens。
- Python 3.12.14；torch `2.15.0+cpu`、transformers `5.22.2`、accelerate `1.15.0`；仓库 Python SDK `tingly` `0.1.0`（editable 安装本分支）。模型服务使用 SDK 的 stdlib HTTP server，无额外 Web serving 框架。
- Go `1.26.8`。接口实现与 [复现示例](../sdk/python/examples/translation/README.md) 在同一分支；实验依赖和下载只保存在 `work/`。

## 实际路径

Python Transformers `AutoTokenizer` / `AutoModelForCausalLM` 真正加载固定模型权重。[保留的模型服务](../sdk/python/examples/translation/hunyuan_server.py) 用 `tingly.Server.openai_chat` 提供 loopback OpenAI serving endpoint，早期实验部署名使用 `local-mt`，通过 service 专用字段选择模板；此结构已撤销。当前 SDK serving 默认使用精确模型 ID `tencent/Hy-MT2-1.8B`，模板选择复用 catalog，不修改 LB/规则/provider schema。

请求经过模型 token 鉴权、`/tingly/custom/v1/translate`、规则/后端选择、原生 MT 模板、Python 推理、完成状态验证，最后回到统一数组响应。没有拿直接 Python 推理结果替代网关验证。

以下早期记录运行了三种入口（当时的独立 Go demo 已删除，不能作为当前复现步骤）：生产路由 handler 的 opt-in live test、独立运行的 loopback demo gateway + 真实 HTTP curl、[保留的 Python SDK client 示例](../sdk/python/examples/translation/client.py) 通过同一 demo gateway 的真实 HTTP 请求。

## 观察结果

| 输入 | 目标 | 实际返回 | 观察入口 |
| --- | --- | --- | --- |
| Hello, world! | zh-Hans | 你好，世界！ | live gateway test，和下一项属于同一 batch |
| Save changes | zh-Hans | 保存更改 | live gateway test，结果保持顺序 |
| 今天天气真好。 | en | The weather is really nice today. | live gateway test |
| Good morning. | zh-Hans | 早上好。 | 独立 demo gateway 的真实 HTTP 请求 |
| Hello, world! / Save changes | zh-Hans | 你好，世界！ / 保存更改 | `Client.translate()` 的双项 batch → gateway → `tingly.Server` → 真实模型 |

初次前 3 项 live test 约 7.68 秒；换成最终保留的 `tingly.Server` 模型服务后，重跑约 7.73 秒，译文一致。最终 SDK client 双项 batch 的模型日志报告生成 5、3 tokens，用时约 4.14、1.82 秒。这些是本次短文本观察，包含运行条件影响，不是吞吐/延迟 benchmark。

早期真实 HTTP 请求和响应（译文结果保留，接线方式以上述最新说明为准）：

```json
{
  "text": ["Good morning."],
  "source_lang": "en",
  "target_lang": "zh-Hans"
}
```

```json
{
  "model": "translate",
  "translations": [{"text": "早上好。"}]
}
```

## 自动检查及复现

针对性检查覆盖原生 user-only 消息、Qwen 专用参数、语言变体边界、鉴权/team 隔离、批量顺序、不重放已完成项的混元→Qwen failover、不完整结果、body 限制和 HTTP 取消。

```bash
go test ./internal/protocol/translation ./internal/protocolserver -run '^TestTranslation' -count=1
```

Go 回归通过：`internal/protocol/translation`、`internal/protocolserver`、`internal/config`、`internal/loadbalance`、`internal/server/module/rule`。

Python SDK：按 `task gen:py:quota` 的现有流程生成 quota models 后，`python -m unittest discover -s sdk/python/tests` 共 57 项，48 项通过、9 项需要独立 tb binary 的既有 E2E 检查按条件跳过。新增检查验证翻译 body/鉴权、scenario/model 参数及失败索引保留；既有测试仍有 socket ResourceWarning，未影响测试结果。

当前 SDK 调用（先 editable 安装 `sdk/python`，启动模型服务与正常 tb，再运行 setup.py 并设置网关 token）：

```bash
python sdk/python/examples/translation/client.py --target zh-Hans 'Hello, world!' 'Save changes'
```

启动 [SDK 模型服务与正常 tb](../sdk/python/examples/translation/README.md) 后：

```bash
TINGLY_TRANSLATION_LIVE_BASE_URL=http://127.0.0.1:18081 \
  go test ./internal/protocolserver -run '^TestTranslationLiveHunyuan$' -v -count=1
```

重构后规则 Service 不再新增字段；管理面 schema 与基线一致，仍通过实际 Swagger 命令及 `pnpm gen:api` 校验生成。模型服务面 `/tingly/...` 不属于管理面 OpenAPI 生成器；接口契约及使用步骤在示例文档中。

## 边界

- 证明了固定 Tencent 权重、Python SDK 和统一入口的真实连通；不是翻译质量、安全性、格式保持或语言覆盖 benchmark。
- Qwen-MT 原生参数和跨后端 failover 为 mock-provider 检查，未用真实 Qwen 账号验证；没有调用付费翻译服务。
- 官方 config 含 rope 相关键告警，但此次模型加载、生成和 HTTP 往返成功；没有改模型 config 消除告警。
- 模型没有报告检测语种，网关没有伪造该字段或置信度。
- 缺失 token usage 时不记录成功的 token 测量；现有详细 usage 存储仍不是通用可空单位模型，此版本没有新增字符计费后端。
- 网关取消会传递给上游 HTTP 请求；Python 示例中已经开始的 CPU generation 不会强制中断，最多继续到其生成 token 上限。
- 本版本没有 UI adapter selector、smart-routing 分区、通用 chat fallback、原生 DeepL/LibreTranslate、HTML/术语/流式或文件任务。

## 收敛后重新验证：正常 tb + SDK 示例

2026-11-12，在删除独立 Go demo 与 LB 专用字段后重新执行：

- 构建普通 `cli/tingly-box`，在独立 `work/` 配置目录启动；此次端口 12581。
- 用 SDK 目录中的 `setup.py` 经现有管理 API 创建 provider 与普通 `custom` 规则；不含翻译专用路由字段。使用该实例独立的 UserToken / ModelToken，未记录凭据。
- SDK 模型服务以 `tencent/Hy-MT2-1.8B` 为 served model ID，加载上述固定 revision 的真实本地权重。
- `client.py --target zh-Hans 'Hello, world!' 'Save changes'` 实际返回 `{"model":"translate","translations":[{"text":"你好，世界！"},{"text":"保存更改"}]}`。
- Go catalog、protocol/translation、protocolserver、config、loadbalance、rule 回归通过；`harness catalog check` 通过。
- Python SDK 使用本次构建的正常 tb binary 执行全部 58 项测试，全部通过、无跳过。新增翻译 E2E 复用既有正常 tb 启动/配置 harness；该项用 SDK stub 验证协议，真实权重验证为上面的独立 HTTP 调用。
- Swagger 与前端声明重新生成后，管理面 OpenAPI 与基线一致；LB Service 也与基线一致。

当前仅精确支持 catalog 中的两个模型 ID。公共路由别名仍可自由配置；任意上游部署别名暂不支持，新增检查确认其在调用上游前被拒绝。Qwen 云实测按用户后续决定不执行，仍只保留 mock 协议验证。
