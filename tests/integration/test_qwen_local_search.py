#!/usr/bin/env python3
"""
Qwen-Plus Local Search 对比测试

对比：
1. 直接调用 Qwen API（可能使用模型原生搜索）
2. 通过 tingly-box 网关调用（使用本地工具拦截）
"""

import json
import time
import requests
from typing import Optional
from dataclasses import dataclass

# 配置
QWEN_API_KEY = "sk-bda8a779f42f4ebc81178e27a85a30b2"  # 需要填写
QWEN_BASE_URL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
QWEN_MODEL = "qwen-plus"

TBE_GATEWAY_URL = "http://127.0.0.1:12580/tingly/openai/v1"
TBE_API_KEY = "tingly-box-eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJjbGllbnRfaWQiOiJ0ZXN0LWNsaWVudCIsImV4cCI6MTc2NjU2ODc4MywiaWF0IjoxNzY2NDgyMzgzfQ.Dp5YAV2ibWe2pYaO9sP2nzTAPTGOgNQ9ykHfz1QNs9c"

PROXY_URL = "http://127.0.0.1:7897"


@dataclass
class TestResult:
    name: str
    response_time: float
    content: str
    tool_calls: list
    has_search_results: bool
    error: Optional[str] = None


def test_direct_qwen(query: str) -> TestResult:
    """直接调用 Qwen API"""
    print(f"\n>>> 直接调用 Qwen API: {query[:50]}...")

    start = time.time()
    try:
        resp = requests.post(
            f"{QWEN_BASE_URL}/chat/completions",
            headers={
                "Authorization": f"Bearer {QWEN_API_KEY}",
                "Content-Type": "application/json"
            },
            json={
                "model": QWEN_MODEL,
                "messages": [{"role": "user", "content": query}],
                "temperature": 0.7
            },
            timeout=60,
            proxies={"http": PROXY_URL, "https": PROXY_URL}
        )
        resp.raise_for_status()
        data = resp.json()

        elapsed = time.time() - start
        content = data["choices"][0]["message"].get("content", "")

        # 检测是否有搜索结果特征
        has_search = any(x in content.lower() for x in [
            "搜索", "search", "根据", "来源", "http", "www.",
            "网页", "网站", "链接"
        ])

        return TestResult(
            name="Direct Qwen",
            response_time=elapsed,
            content=content,
            tool_calls=[],
            has_search_results=has_search
        )
    except Exception as e:
        return TestResult(
            name="Direct Qwen",
            response_time=time.time() - start,
            content="",
            tool_calls=[],
            has_search_results=False,
            error=str(e)
        )


def test_tbe_gateway(query: str, use_tools: bool = True) -> TestResult:
    """通过 tingly-box 网关调用"""
    print(f"\n>>> 通过 tingly-box 网关: {query[:50]}...")

    tools = []
    if use_tools:
        tools = [{
            "type": "function",
            "function": {
                "name": "web_search",
                "description": "Search the web for current information",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "query": {"type": "string"},
                        "count": {"type": "integer"}
                    },
                    "required": ["query"]
                }
            }
        }]

    messages = [
        {"role": "system", "content": "You are a helpful assistant. Use web_search tool when you need current information."},
        {"role": "user", "content": query}
    ]

    start = time.time()
    try:
        resp = requests.post(
            f"{TBE_GATEWAY_URL}/chat/completions",
            headers={
                "Authorization": f"Bearer {TBE_API_KEY}",
                "Content-Type": "application/json"
            },
            json={
                "model": "qwen-plus",
                "messages": messages,
                "tools": tools if use_tools else None,
                "tool_choice": "auto" if use_tools else None,
                "temperature": 0.7
            },
            timeout=120
        )
        resp.raise_for_status()
        data = resp.json()

        elapsed = time.time() - start

        choice = data["choices"][0]
        message = choice.get("message", {})
        content = message.get("content", "")
        tool_calls = message.get("tool_calls", [])

        # 检测是否有搜索结果
        has_search = bool(tool_calls) or any(x in content.lower() for x in [
            "搜索", "search", "结果", "来源", "http", "www.", "网页"
        ])

        return TestResult(
            name=f"TBE Gateway {'(with tools)' if use_tools else '(no tools)'}",
            response_time=elapsed,
            content=content,
            tool_calls=tool_calls,
            has_search_results=has_search
        )
    except Exception as e:
        return TestResult(
            name=f"TBE Gateway {'(with tools)' if use_tools else '(no tools)'}",
            response_time=time.time() - start,
            content="",
            tool_calls=[],
            has_search_results=False,
            error=str(e)
        )


def print_comparison(query: str, results: list):
    """打印对比结果"""
    print("\n" + "=" * 80)
    print(f"查询: {query}")
    print("=" * 80)

    for r in results:
        print(f"\n【{r.name}】")
        print(f"  响应时间: {r.response_time:.2f}s")
        print(f"  搜索结果: {'✓ 有' if r.has_search_results else '✗ 无'}")
        print(f"  Tool Calls: {len(r.tool_calls) if r.tool_calls else 0}")
        if r.error:
            print(f"  错误: {r.error}")
        else:
            print(f"\n  响应内容:")
            print(f"  {r.content[:500]}..." if len(r.content) > 500 else f"  {r.content}")


def main():
    # 检查配置
    if False:
        print("警告: 请设置 QWEN_API_KEY")
        print("从 ~/.tingly-box/config.json 中复制 qwen provider 的 token")
        return

    if False:
        print("警告: 请设置 TBE_API_KEY")
        print("使用 tingly-box 的 user_token 或 provider token")
        return

    # 测试查询
    test_queries = [
        "2024年诺贝尔物理学奖得主是谁？",
        "Python 3.12 有哪些新特性？",
        "当前北京的天气怎么样？"
    ]

    print("=" * 80)
    print("Qwen-Plus Local Search 对比测试")
    print(f"直接调用: {QWEN_BASE_URL}")
    print(f"网关调用: {TBE_GATEWAY_URL}")
    print("=" * 80)

    for query in test_queries:
        results = []

        # 1. 直接调用 Qwen
        # results.append(test_direct_qwen(query))
        # time.sleep(1)

        # 2. 网关调用（不带 tools）- 测试模型原生能力
        results.append(test_tbe_gateway(query, use_tools=False))
        time.sleep(1)

        # 3. 网关调用（带 tools）- 测试本地搜索拦截
        results.append(test_tbe_gateway(query, use_tools=True))

        print_comparison(query, results)
        time.sleep(2)

    print("\n" + "=" * 80)
    print("测试完成")
    print("=" * 80)


if __name__ == "__main__":
    main()
