#!/usr/bin/env python3
"""
Local Search 效果对比测试脚本

对比两种方式：
1. 直接调用 LLM API（如 OpenAI/Qwen）- 可能使用模型原生搜索
2. 通过 tingly-box 网关调用 - 使用本地工具拦截

验证 local search 是否正常工作，以及结果差异。
"""

import json
import os
import sys
import time
import argparse
from typing import Optional, Dict, Any
from dataclasses import dataclass
from datetime import datetime

import requests


@dataclass
class TestResult:
    """测试结果数据类"""
    name: str
    response_time: float
    content: str
    tool_calls: list
    has_search_results: bool
    error: Optional[str] = None


class LLMTester:
    """LLM 测试基类"""

    def __init__(self, name: str, api_key: str, base_url: str, model: str):
        self.name = name
        self.api_key = api_key
        self.base_url = base_url.rstrip('/')
        self.model = model
        self.headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }

    def chat_completion(self, messages: list, tools: Optional[list] = None) -> TestResult:
        """发送聊天完成请求"""
        start_time = time.time()

        payload = {
            "model": self.model,
            "messages": messages,
            "temperature": 0.7,
        }

        if tools:
            payload["tools"] = tools
            payload["tool_choice"] = "auto"

        try:
            response = requests.post(
                f"{self.base_url}/chat/completions",
                headers=self.headers,
                json=payload,
                timeout=120
            )
            response.raise_for_status()
            data = response.json()

            response_time = time.time() - start_time

            # 提取内容
            content = ""
            tool_calls = []

            if "choices" in data and len(data["choices"]) > 0:
                choice = data["choices"][0]
                message = choice.get("message", {})
                content = message.get("content", "")
                tool_calls = message.get("tool_calls", [])

            # 检查是否有搜索结果特征
            has_search = self._detect_search_results(content, tool_calls)

            return TestResult(
                name=self.name,
                response_time=response_time,
                content=content,
                tool_calls=tool_calls,
                has_search_results=has_search
            )

        except Exception as e:
            return TestResult(
                name=self.name,
                response_time=time.time() - start_time,
                content="",
                tool_calls=[],
                has_search_results=False,
                error=str(e)
            )

    def _detect_search_results(self, content: str, tool_calls: list) -> bool:
        """检测响应中是否包含搜索结果特征"""
        # 检查内容中的搜索特征
        search_indicators = [
            "搜索结果", "search result", "found", "根据搜索",
            "以下是", "相关网页", "来源:", "URL:", "http"
        ]

        content_lower = content.lower()
        for indicator in search_indicators:
            if indicator.lower() in content_lower:
                return True

        # 检查 tool_calls
        for tc in tool_calls:
            if isinstance(tc, dict):
                function = tc.get("function", {})
                name = function.get("name", "")
                if any(x in name.lower() for x in ["search", "web", "fetch"]):
                    return True

        return False


class TinglBoxTester(LLMTester):
    """tingly-box 网关测试器"""

    def __init__(self, api_key: str, base_url: str, model: str):
        super().__init__("tingly-box-gateway", api_key, base_url, model)

    def test_with_local_search(self, query: str) -> TestResult:
        """测试本地搜索功能"""
        # 定义 web_search 工具
        tools = [{
            "type": "function",
            "function": {
                "name": "web_search",
                "description": "Search the web for current information",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "query": {"type": "string", "description": "Search query"},
                        "count": {"type": "integer", "description": "Number of results"}
                    },
                    "required": ["query"]
                }
            }
        }]

        messages = [
            {"role": "system", "content": "You are a helpful assistant. Use web_search tool when you need current information."},
            {"role": "user", "content": query}
        ]

        return self.chat_completion(messages, tools)

    def test_with_local_fetch(self, url: str) -> TestResult:
        """测试本地网页获取功能"""
        tools = [{
            "type": "function",
            "function": {
                "name": "web_fetch",
                "description": "Fetch content from a URL",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "url": {"type": "string", "description": "URL to fetch"}
                    },
                    "required": ["url"]
                }
            }
        }]

        messages = [
            {"role": "system", "content": "You are a helpful assistant. Use web_fetch tool to read webpage content."},
            {"role": "user", "content": f"Please fetch and summarize the content from: {url}"}
        ]

        return self.chat_completion(messages, tools)


class DirectLLMTester(LLMTester):
    """直接调用 LLM API 测试器"""

    def __init__(self, provider: str, api_key: str, base_url: str, model: str):
        super().__init__(f"direct-{provider}", api_key, base_url, model)
        self.provider = provider

    def test_native_search(self, query: str) -> TestResult:
        """测试模型原生搜索能力"""
        messages = [
            {"role": "user", "content": query}
        ]

        # 对于支持原生搜索的模型，直接提问即可
        # 模型会自己决定是否使用搜索工具
        return self.chat_completion(messages)


def print_comparison(direct_result: TestResult, gateway_result: TestResult, query: str):
    """打印对比结果"""
    print("\n" + "=" * 80)
    print(f"查询: {query}")
    print("=" * 80)

    print(f"\n{'直接调用 LLM':<40} | {'通过 tingly-box 网关':<40}")
    print("-" * 80)

    # 响应时间
    print(f"响应时间: {direct_result.response_time:.2f}s{'':<28} | "
          f"响应时间: {gateway_result.response_time:.2f}s")

    # 是否有搜索结果
    direct_search = "✓ 有" if direct_result.has_search_results else "✗ 无"
    gateway_search = "✓ 有" if gateway_result.has_search_results else "✗ 无"
    print(f"搜索结果: {direct_search:<35} | 搜索结果: {gateway_search}")

    # Tool calls
    direct_tools = len(direct_result.tool_calls)
    gateway_tools = len(gateway_result.tool_calls)
    print(f"Tool Calls: {direct_tools:<34} | Tool Calls: {gateway_tools}")

    # 错误信息
    if direct_result.error:
        print(f"错误: {direct_result.error[:35]:<35} | ", end="")
    else:
        print(f"{'':<40} | ", end="")

    if gateway_result.error:
        print(f"错误: {gateway_result.error[:35]}")
    else:
        print()

    print("\n" + "-" * 80)
    print("直接调用 LLM 响应内容:")
    print("-" * 80)
    print(direct_result.content[:500] if direct_result.content else "(无内容)")
    if len(direct_result.content) > 500:
        print("... (truncated)")

    print("\n" + "-" * 80)
    print("tingly-box 网关响应内容:")
    print("-" * 80)
    print(gateway_result.content[:500] if gateway_result.content else "(无内容)")
    if len(gateway_result.content) > 500:
        print("... (truncated)")

    print("\n")


def test_search_queries(tester_direct: DirectLLMTester, tester_gateway: TinglBoxTester, queries: list):
    """测试多个搜索查询"""
    print("\n" + "=" * 80)
    print("搜索功能对比测试")
    print("=" * 80)

    for query in queries:
        print(f"\n>>> 测试查询: {query}")

        # 直接调用
        print("  正在调用直接 LLM...")
        direct_result = tester_direct.test_native_search(query)

        # 通过网关
        print("  正在调用 tingly-box 网关...")
        gateway_result = tester_gateway.test_with_local_search(query)

        # 打印对比
        print_comparison(direct_result, gateway_result, query)

        # 短暂延迟避免 rate limit
        time.sleep(1)


def test_fetch_urls(tester_gateway: TinglBoxTester, urls: list):
    """测试网页获取功能"""
    print("\n" + "=" * 80)
    print("网页获取功能测试 (仅网关)")
    print("=" * 80)

    for url in urls:
        print(f"\n>>> 测试 URL: {url}")

        result = tester_gateway.test_with_local_fetch(url)

        print(f"响应时间: {result.response_time:.2f}s")
        print(f"Tool Calls: {len(result.tool_calls)}")
        print(f"是否有搜索结果特征: {'✓ 是' if result.has_search_results else '✗ 否'}")

        if result.error:
            print(f"错误: {result.error}")
        else:
            print("\n响应内容:")
            print("-" * 80)
            print(result.content[:800] if result.content else "(无内容)")
            if len(result.content) > 800:
                print("... (truncated)")

        time.sleep(1)


def main():
    parser = argparse.ArgumentParser(description="Local Search 效果对比测试")
    parser.add_argument("--direct-api-key", help="直接调用 LLM 的 API Key")
    parser.add_argument("--direct-base-url", default="https://api.openai.com/v1",
                        help="直接调用 LLM 的 Base URL")
    parser.add_argument("--direct-model", default="gpt-3.5-turbo",
                        help="直接调用的模型名称")
    parser.add_argument("--gateway-api-key", required=True,
                        help="tingly-box 网关的 API Key")
    parser.add_argument("--gateway-url", default="http://127.0.0.1:12580/v1",
                        help="tingly-box 网关地址")
    parser.add_argument("--gateway-model", default="qwen",
                        help="网关使用的模型名称")
    parser.add_argument("--test-search", action="store_true",
                        help="测试搜索功能")
    parser.add_argument("--test-fetch", action="store_true",
                        help="测试网页获取功能")
    parser.add_argument("--test-all", action="store_true",
                        help="测试所有功能")

    args = parser.parse_args()

    if not any([args.test_search, args.test_fetch, args.test_all]):
        args.test_all = True

    # 初始化测试器
    tester_gateway = TinglBoxTester(
        api_key=args.gateway_api_key,
        base_url=args.gateway_url,
        model=args.gateway_model
    )

    print("\n" + "=" * 80)
    print("Local Search 对比测试")
    print(f"时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 80)

    # 测试搜索功能
    if args.test_search or args.test_all:
        if not args.direct_api_key:
            print("警告: 未提供 --direct-api-key，跳过直接调用 LLM 的对比测试")
            print("仅测试 tingly-box 网关的本地搜索...")

            search_queries = [
                "What are the latest features in Python 3.12?",
                "Who won the Nobel Prize in Physics 2024?",
                "What is the current weather in Beijing?"
            ]

            for query in search_queries:
                print(f"\n>>> 测试查询: {query}")
                result = tester_gateway.test_with_local_search(query)
                print(f"响应时间: {result.response_time:.2f}s")
                print(f"Tool Calls: {len(result.tool_calls)}")
                print(f"是否有搜索结果: {'✓ 是' if result.has_search_results else '✗ 否'}")
                if result.error:
                    print(f"错误: {result.error}")
                else:
                    print(f"\n响应内容:\n{result.content[:500]}...")
                time.sleep(1)
        else:
            tester_direct = DirectLLMTester(
                provider="openai",
                api_key=args.direct_api_key,
                base_url=args.direct_base_url,
                model=args.direct_model
            )

            search_queries = [
                "What are the latest features in Python 3.12?",
                "Who won the Nobel Prize in Physics 2024?",
                "What is the current weather in Beijing?"
            ]

            test_search_queries(tester_direct, tester_gateway, search_queries)

    # 测试网页获取功能
    if args.test_fetch or args.test_all:
        test_urls = [
            "https://example.com",
            "https://httpbin.org/html"
        ]

        test_fetch_urls(tester_gateway, test_urls)

    print("\n" + "=" * 80)
    print("测试完成")
    print("=" * 80)


if __name__ == "__main__":
    main()
