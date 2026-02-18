<p align="center">

[//]: # (  <img src="docs/banner.png" alt="Tingly Box Banner" width="100%" />)
</p>

![Tingly Box Web UI Demo](./docs/images/hero.png)

<h1 align="center">Tingly Box</h1>

<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#key-features">Features</a> •
  <a href="#use-with-openai-sdk-or-claude-code">Usage</a> •
  <a href="#documentation">Documentation</a> •
  <a href="https://github.com/tingly-dev/tingly-box/issues">Issues</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go" alt="Go Version" />
  <img src="https://img.shields.io/badge/License-MPL%202.0-brightgreen.svg" alt="License" />
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey" alt="Platform" />
</p>

Tingly Box decides **which model to call, when to compress context, and how to route requests for maximum efficiency**, offering **secure, reliable, and customizable functional extensions**.

![Tingly Box Web UI Demo](./docs/images/output.gif)


## Key Features

- **Unified API** – One mixin endpoint to rule them all, use what you like - OpenAI / Anthropic / Google
- **Smart Routing, Not Just Load Balancing** – Intelligently route requests across models and tokens based on cost, speed, or custom policies, not simple load balancing
- **Smart Context Compression** – (Coming soon) Automatically distill context to its essential parts: sharper relevance, lower cost, and faster responses
- **Auto API Translation** – Seamlessly bridge OpenAI, Anthropic, Google, and other API dialects—no code changes needed  
- **Blazing Fast** – Adds typically **< 1ms** of overhead—so you get flexibility without latency tax  
- **Flexible Auth** – Support for both API keys and OAuth (e.g., Claude.ai), so you can use your existing quotas anywhere  
- **Visual Control Panel** – Intuitive UI to manage providers, routes, aliases, and models at a glance
- **Client Side Usage Stats** - Track token consumption, latency, cost estimates, and model selection per request—directly from your client

## Quick Start

### Install

**From npm (recommended)**

```bash
# Install and run (auto service migration without any args)
npx tingly-box@latest
```

> if any trouble, please check tingly-box process and port 12580 and confirm to kill them.

**From source code**

*Requires: Go 1.21+, Node.js 18+, pnpm, task, openapi-generator-cli*

```bash
# Install dependencies
# - Go: https://go.dev/doc/install
# - Node.js: https://nodejs.org/
# - pnpm: `npm install -g pnpm`
# - task: https://taskfile.dev/installation/, or `go install github.com/go-task/task/v3/cmd/task@latest`
# - openapi-generator-cli: `npm install @openapitools/openapi-generator-cli -g`

git submodule update --init --recursive

# Build with frontend
task build

# Build GUI binary via wails3
task wails:build
```

**From Docker (Github)**

```bash
mkdir tingly-data
docker run -d \
  --name tingly-box \
  -p 12580:12580 \
  -v `pwd`/tingly-data:/home/tingly/.tingly-box \
  ghcr.io/tingly-dev/tingly-box
```

## **Use with IDE, CLI, SDK and any AI application**

**Tool Integration**

- Claude Code
- OpenCode
- Xcode
- Gemini
- ……

Any application is ready to use.

**OpenAI SDK**

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-tingly-model-token",
    base_url="http://localhost:12580/tingly/openai/v1"
)

response = client.chat.completions.create(
    model="tingly-gpt",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response)
```

**Anthropic SDK**

```python
from anthropic import Anthropic

client = Anthropic(
    api_key="your-tingly-model-token",
    base_url="http://localhost:12580/tingly/anthropic"
)

response = client.messages.create(
    model="tingly",
    max_tokens=1024,
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response)
```

> Tingly Box proxies requests transparently for SDKs and CLI tools.

**Using OAuth Providers**

You can also add OAuth providers (like Claude Code) and use your existing quota in any OpenAI-compatible tool:

```bash
# 1. Add Claude Code via OAuth in Web UI (http://localhost:12580)
# 2. Configure your tool with Tingly Box endpoint
```


Requests route through your OAuth-authorized provider, using your existing Claude Code quota instead of requiring a separate API key.

This works with any tool that supports OpenAI-compatible endpoints: Cherry Studio, VS Code extensions, or custom AI agents.



## Web Management UI

```bash
npx tingly-box@latest
```


## Documentation

**[User Manual](./docs/user-manual.md)** – Installation, configuration, and operational guide
**[Guardrails](./docs/guardrails.md)** – Content-aware filtering and rule configuration


## **Philosophy**

- **One endpoint, many providers** – Consolidates multiple providers behind a single API with minimal configuration.
- **Seamless integration** – Works with SDKs and CLI tools with minimal setup.


## **How to Contribute**

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



## Support

| Telegram    | Wechat |
| :--------: | :-------: |
| <img width="196" height="196" alt="image" src="https://github.com/user-attachments/assets/56022b70-97da-498f-bf83-822a11668fa7" /> | <img width="196" height="196" alt="image" src="https://github.com/user-attachments/assets/8a285ffa-bb2d-47db-8e5b-3645ce9eddd9" /> |
| https://t.me/+V1sqeajw1pYwMzU1 | http://chv.ckcoa5.cn/t/OSFb |


## Contributors

<a href="https://github.com/tingly-dev/tingly-box/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=tingly-dev/tingly-box" />
</a>

<br />
<br />
Eearly contributor badges are given to following contributors:

<br />

<img width="128" height="128" alt="image" src="https://github.com/user-attachments/assets/18730cd4-5e04-4840-9ef7-eab5cb418032" />
<img width="128" height="128" alt="image" src="https://github.com/user-attachments/assets/2df1c253-94f8-4cef-b6b7-9fef11ac9ecc" />




---

Mozilla Public License Version 2.0 · © Tingly Dev
