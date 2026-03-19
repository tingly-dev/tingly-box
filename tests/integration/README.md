# Local Search 对比测试

这个目录包含用于验证 tingly-box 本地搜索和网页获取功能的测试。

## 文件说明

| 文件 | 说明 |
|------|------|
| `tool_interceptor_test.go` | Go 集成测试，测试拦截器核心功能 |
| `test_local_search_comparison.py` | Python 对比测试脚本，对比直接调用 LLM 和通过网关的效果 |

## 测试类型

### 1. Go 集成测试

测试拦截器的核心功能：
- 本地搜索功能（通过 DuckDuckGo）
- 本地网页获取功能
- 软打开模式（默认）：模型有原生工具则不拦截
- 硬打开模式：总是使用本地工具

```bash
# 运行所有集成测试
go test ./tests/integration/... -v

# 只测试拦截器功能
go test ./tests/integration/... -v -run "TestToolInterceptor"

# 只测试模式切换
go test ./tests/integration/... -v -run "TestToolInterceptorModes"
```

### 2. Python 对比测试

对比直接调用 LLM API 和通过 tingly-box 网关的效果：
- 直接调用：可能使用模型原生搜索工具
- 网关调用：使用本地工具拦截

#### 安装依赖

```bash
pip install requests
```

#### 使用方法

**基本用法（只测试网关）**

```bash
python tests/integration/test_local_search_comparison.py \
  --gateway-api-key "your-tingly-box-api-key" \
  --gateway-url "http://127.0.0.1:12580/v1" \
  --gateway-model "qwen"
```

**对比测试（推荐）**

```bash
python tests/integration/test_local_search_comparison.py \
  --direct-api-key "your-openai-api-key" \
  --direct-base-url "https://api.openai.com/v1" \
  --direct-model "gpt-3.5-turbo" \
  --gateway-api-key "your-tingly-box-api-key" \
  --gateway-url "http://127.0.0.1:12580/v1" \
  --gateway-model "qwen" \
  --test-all
```

**参数说明**

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--direct-api-key` | 直接调用 LLM 的 API Key | - |
| `--direct-base-url` | 直接调用 LLM 的 Base URL | https://api.openai.com/v1 |
| `--direct-model` | 直接调用的模型名称 | gpt-3.5-turbo |
| `--gateway-api-key` | tingly-box 网关的 API Key | **必填** |
| `--gateway-url` | tingly-box 网关地址 | http://127.0.0.1:12580/v1 |
| `--gateway-model` | 网关使用的模型名称 | qwen |
| `--test-search` | 只测试搜索功能 | - |
| `--test-fetch` | 只测试网页获取功能 | - |
| `--test-all` | 测试所有功能 | 默认 |

## 预期结果

### 软打开模式（默认）

当 `prefer_local_search: false`（默认）：
- 如果模型有原生 `web_search` 工具（如 glm-4），使用模型原生工具
- 如果模型没有原生工具，使用本地 DuckDuckGo 搜索

### 硬打开模式

当 `prefer_local_search: true`：
- 无论模型是否有原生工具，总是使用本地搜索

## 验证要点

1. **搜索功能**
   - 响应中包含搜索结果
   - 结果包含标题、URL、摘要
   - 响应时间合理（< 30s）

2. **网页获取功能**
   - 成功获取网页内容
   - 内容经过 readability 提取（纯文本）
   - 响应时间合理（< 10s）

3. **代理配置**
   - 如果网络受限，配置 `proxy_url: http://127.0.0.1:7897`
   - 测试脚本会自动使用代理

## 故障排查

### 搜索超时

```
Search failed: search failed (network error): context deadline exceeded
```

- 检查代理是否运行：`curl -x http://127.0.0.1:7897 https://api.duckduckgo.com`
- 检查 tingly-box 配置中的 `proxy_url` 是否正确

### 网关返回错误

- 检查 tingly-box 是否运行：`curl http://127.0.0.1:12580/v1/models`
- 检查 API Key 是否正确
- 检查模型名称是否正确配置

### 模型不使用搜索工具

- 确保请求中包含 `tools` 参数
- 确保 `tool_choice` 设置为 `"auto"`
- 某些模型可能需要明确的提示词才会使用工具
