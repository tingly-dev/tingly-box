# Tingly Box

**Tingly Box** is a high‑performance desktop LLM proxy designed for personal or local use, providing a **unified OpenAI‑compatible API** for hundreds of model providers. It enables centralized credential management, routing, and load balancing with low latency.

> Think of Tingly Box as a *model gateway* between your applications and multiple LLM vendors.

---

![Tingly Box Web UI Demo](https://raw.githubusercontent.com/andreasfoo/images/main/uPic/2025%2012%2022%2014%2045.gif)

## ✨ Key Features

- **Unified API** – Single OpenAI‑compatible endpoint for many providers
- **Load Balancing** – Distribute traffic across multiple API tokens using routing tactics
- **Auto API Translation** – Seamlessly translate between OpenAI‑ and Anthropic‑style APIs
- **High Performance** – Additional latency typically **< 1ms**
- **JWT Authentication** – Separate user tokens and model tokens
- **Web Management UI** – Visual provider, routing, and model management

---

## 🚀 Quick Start

### Install

**From npm (recommended)**

```bash
# install and run
npx tingly-box@latest start
```

**From source code**

*Requires: Go 1.21+, Node.js 18+, pnpm, task, openapi-generator-cli*

```bash
# Install dependencies
# - Go: https://go.dev/doc/install
# - Node.js: https://nodejs.org/
# - pnpm: npm install -g pnpm
# - task: https://taskfile.dev/installation/
# - openapi-generator-cli: npm install @openapitools/openapi-generator-cli -g

# Build CLI binary
task go:build

# Build with frontend
task cli:build

# Run with gui
task start
```

**From Docker**

```bash
# Build Docker image
docker build -t tingly-box:latest .

# Run container
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v $(pwd)/data/.tingly-box:/app/.tingly-box \
  -v $(pwd)/data/logs:/app/logs \
  -v $(pwd)/data/memory:/app/memory \
  tingly-box:latest
```



---

## **🔌 Use with OpenAI SDK or Claude Code**

**Python OpenAI SDK**

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-tingly-model-token",
    base_url="http://localhost:12580/openai/v1"
)

response = client.chat.completions.create(
# To pass litellm model name validation, use "gpt-3.5-turbo"
    model="tingly",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response)
```

**Claude Code**

```bash
# Settings file (~/.claude/settings.json)
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "{content after tingly token cmd 'Current API Key from Global Config'}",
    "ANTHROPIC_BASE_URL": "http://localhost:12580/anthropic",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "tingly",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "tingly",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "tingly",
    "ANTHROPIC_MODEL": "tingly"
  }
}
```

> Tingly Box proxies requests transparently for SDKs and CLI tools.

---



## 🖥 Web Management UI

```bash
tingly-box start
```

---

## 📚 Documentation

**[User Manual](https://chatgpt.com/c/docs/user_manual.md)** – Installation, configuration, and operational guide

------

## **🧩 Philosophy**

- **One endpoint, many providers** – Consolidates multiple providers behind a single API with minimal configuration.
- **Seamless integration** – Works with SDKs and CLI tools with minimal setup.

------

## **🤝 How to Contribute**

We welcome contributions! Follow these steps, inspired by popular open-source repositories:

1. **Fork the repository** – Click the “Fork” button on GitHub.

2. **Clone your fork**

   ```bash
   git clone https://github.com/your-username/tingly-box.git
   cd tingly-box
   ```

3. **Create a new branch**

   ```bash
   git checkout -b feature/my-new-feature
   ```

4. **Make your changes** – Follow existing code style and add tests if applicable.

5. **Run tests**

   ```bash
   task test
   ```

6. **Commit your changes**

   ```bash
   git commit -m "Add concise description of your change"
   ```

7. **Push your branch**

   ```bash
   git push origin feature/my-new-feature
   ```

8. **Open a Pull Request** – Go to the GitHub repository and open a PR against `main`.

------

Apache-2.0 License · © Tingly
