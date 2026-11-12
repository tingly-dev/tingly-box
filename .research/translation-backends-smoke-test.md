# 翻译后端复验：通用 LLM、原生 MT 与传统服务

Date: 2026-11-12（Asia/Shanghai）。关联 [TEP-2254](https://github.com/tingly-dev/tingly-box/issues/2254)。
设计见 [后端接口](../.design/translation.md#12-后端接口与实现当前)，复现见 [SDK 示例](../sdk/python/examples/translation/README.md)。

## 实现边界

`Translator` 绑定选中的后端，单次翻译独立文本。LLM 与 MT 的区别是消息语义；
传统服务实现同一个操作，但直接使用 native HTTP，不伪装成 Chat。

- 通用模型无需 catalog 登记。普通 Chat / Responses / Anthropic 复用已有 attempt planner、
  transforms、协议转换、模型限额、池化 client 和 forwarding。不运行 agent 工具循环。
- 混元、Qwen-MT 的特殊语义由已有 catalog 描述；native MT 不经过通用 chat 变换。
- `api_style=libretranslate|deepl` 是既有 provider 协议字段的新取值。LB、规则、provider
  字段结构没有新增翻译配置。传统 service.model 为空；HTTP payload 不伪造 model。
- 传统服务字符计费与 token 独立。仅服务确实报告时返回 billed_characters；现有 usage
  DB 仍是 token 存储，不持久化传统字符计费，也不声称已实现统一成本统计。

## 新端口与真实模型

普通 tb 新端口 **12680**，独立配置 `work/translation-research/backends-tb`；
没有覆盖原实例配置，也不复用旧实验端口。token 只通过本次配置传入子进程，不记录在文档。

| 后端 | serving 端口 | 逻辑路由 | 上游部署名 |
| --- | --- | --- | --- |
| 本地混元 MT | 18681 | translate | tencent/Hy-MT2-1.8B |
| 本地普通指令 LLM | 18683 | translate-llm | local-llm（无 catalog 条目） |

两者均使用保留的 Python SDK serving 示例，Transformers 实际加载权重，CPU 4 线程、
bfloat16、greedy decoding、最多生成 128 tokens。普通 LLM 通过 `--generic-chat` 接受
system/user 消息；混元保留 user-only 原生模板。

- 混元：`tencent/Hy-MT2-1.8B`，revision `9a341cd1b679d3efd23b46e847b01745a71ed792`。
- 普通 LLM：`Qwen/Qwen2.5-0.5B-Instruct`，revision `7ae557604adf67be50417f59c2c2f167def9a775`。
- Python 3.12.14；torch 2.15.0+cpu；transformers 5.22.2；Go 1.26.8；SDK editable 安装本分支。
- 普通 Qwen 是本地通用模型，**不是 hosted Qwen-MT**；没有调用云推理或付费翻译 API。

两组 provider/rule 都由 `setup.py` 经正常管理 API 创建，然后运行 SDK `client.py`。
请求实际经过鉴权、选路、适配、HTTP、真实模型、完成状态校验及统一响应：

| 实际后端 | 输入 | 目标 | 实际输出 |
| --- | --- | --- | --- |
| Hy-MT2-1.8B | Hello, world! | zh-Hans | 你好，世界！ |
| Hy-MT2-1.8B | Save changes | zh-Hans | 保存更改 |
| Hy-MT2-1.8B | 今天天气真好。 | en | The weather is really nice today. |
| Qwen2.5-0.5B-Instruct / local-llm | Hello, world! | zh-Hans | 你好，世界！ |
| Qwen2.5-0.5B-Instruct / local-llm | Save changes | zh-Hans | 保存更改 |
| Qwen2.5-0.5B-Instruct / local-llm | 今天天气真好。 | en | Today it is really nice outside. |

每个模型的两个英文输入属于同一 batch，结果按输入顺序返回。小模型仅用于低资源
连通性验证；以上不是质量、抗注入、覆盖语种或性能 benchmark，也不构成生产选型建议。

## 协议与回归检查

- Go：protocol/translation、protocolserver、client、config、loadbalance、provider、rule
  相关包回归通过，ai 全模块测试通过。
- Python SDK：正常 tb binary 下全部 **60 项**测试通过，无跳过；包括普通 LLM、native MT、
  LibreTranslate 和 DeepL 经管理 API 配置后的 SDK 往返。传统服务使用本地 HTTP fixtures，
  并非真实 LibreTranslate 推理或 DeepL 账号质量验证。
- 覆盖：generic LLM 无 catalog、Chat / Responses / Anthropic 转换、native user-only/Qwen
  options、有序 batch、传统服务→LLM 混合 failover 且不重放已完成项、鉴权/team 隔离。
- 传统客户端覆盖 native body/key/header、缺失用量保留未知、字符计费独立、非 2xx /
  业务错误 / 空结果 / 数量不符、取消及禁止重定向，不把可能回显密钥的错误 body 暴露给客户端。
- 特殊语种变体按明确映射筛选；LibreTranslate 实际语言包和供应商限额仍可能导致 native
  4xx，不假装已经完成动态语言能力发现。现有回退策略不会任意重试全部 4xx。

复验命令见 SDK README。普通 LLM 语言能力仍是 best-effort；工具调用、拒答和截断
会失败，不能包装成译文成功。Google-native、OAuth、多字段凭据、vmodel、smart-routing
分区、HTML/术语/文件/流式、新 UI 及传统字符计费持久化仍待后续设计。
