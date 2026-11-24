# Vision 与翻译的当前结构对照

Date: 2026-11-12（Asia/Shanghai）。基于本分支源码阅读，不是新增运行时验证。
对应讨论：[翻译设计](../.design/translation.md#13-与-vision-对照后的待收敛点尚未实施)。

## Vision proxy：模型完成一种业务能力

- [Service](../internal/vision/visionproxy/service.go) 统一 Resolve / Apply，
  启动时由 NewServiceFromPool 注入共享 pool 与 provider resolver。
- [VisionClient](../internal/vision/visionproxy/vision_proxy_client.go) 暴露 Describe；
  production adapter 根据已经选择的 provider/model 取现有池化 SDK client。
  处理器只依赖小接口，测试可以替换 fake，不依赖 HTTP handler。
- 普通视觉模型不需要逐模型 catalog 白名单。提示词、token budget、结果提取属于
  看图能力的实现。OpenAI 分支还会收集流式输出，兼容只支持流式的上游。
- 这条能力调用不等同于完整聊天/agent 入口。它没有复制 HTTP chat handler，
  也没有把全部 chat transforms 和工具循环作为必经路径。
- 当前实现直接分派 OpenAI / Anthropic；这不证明所有 provider 协议已经覆盖。

## Imagegen：client 隐藏供应商差异

- [OpenAIClient.ImagesGenerate](../internal/client/openai.go) 是调用入口：
  OpenAI-compatible 走现有 SDK；特殊供应商才进入 imagegen adapter。
- [imagegen](../internal/vision/imagegen/imagegen.go) 定义 Request / Response / Client，
  是不 import internal/client 的叶子包，避免循环依赖。
- [New](../internal/vision/imagegen/registry.go) 仅构造特殊 native adapter；
  不把所有供应商重新包装一遍。
- [DetectVendor](../internal/vision/imagegen/vendor.go) 根据 host / OAuth issuer
  识别接口差异，没有为每种生图供应商新增全局 APIStyle。
- 供应商存在自定义同步、异步提交/轮询等形态，仍归一为生成操作。

## 与当前翻译实现的差异

- Translator 的单项输入/输出已经独立于 batch，但
  [translation_backend.go](../internal/protocolserver/translation_backend.go)
  仍在 protocolserver 中构造实现、捕获 gin.Context，并展开 Chat / Responses /
  Anthropic 调用；接口的外形与实际依赖边界尚不一致。
- 通用 LLM 目前复用完整 attempt transforms，其中还包括 chat 场景的 MCP 整形。
  它没有执行 agent 工具循环，但“复用传输与转换”和“继承聊天业务策略”尚未清楚分开。
- Native MT / traditional HTTP 的转换已经有小函数；特殊适配不需要决定 LB 数据结构。
- 新增 libretranslate / deepl APIStyle 值是显式选择 native HTTP 的现行实现，
  尚不能从 vision 的 host 识别推导出它们应一律删除：本地 LibreTranslate、反代
  和自定义域名不携带可靠的 vendor host 特征。

## 不能直接移植的行为

- Vision proxy 允许部分描述，并在失败时剥图为 marker；翻译不应把截断或 marker
  当作成功译文。
- Imagegen 的 Usage 结构把未报告用量表达成零；翻译仍需区分未知、实测 token 与
  实测字符计费，不能复制该语义。
- Host 识别可以帮助已知托管端点，不能代替任意自部署服务的显式协议声明。
- Vision proxy 的 rule/scenario 配置优先级服务于辅助看图，不意味着独立 /translate
  需要新增同名的 scenario/rule 开关或另一个服务引用配置。
