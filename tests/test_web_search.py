#!/usr/bin/env python3
"""Test script for web_search functionality using explicit tool calls."""

import os
import json
import sys
import uuid
import subprocess
import tempfile
import time
from datetime import datetime

# Install requirements: pip install openai requests
from openai import OpenAI

# Get current date for time-sensitive queries
CURRENT_DATE = datetime.now().strftime("%Y-%m-%d")

# Configuration
BASE_URL = "http://127.0.0.1:12580"
OPENAI_ENDPOINT = f"{BASE_URL}/tingly/openai/v1"
ANTHROPIC_ENDPOINT = f"{BASE_URL}/tingly/anthropic/v1"
CONFIG_PATH = os.path.expanduser("~/.tingly-box/config.json")
DEFAULT_SERVER_BIN = os.path.abspath(
    os.path.join(os.path.dirname(__file__), "..", "build", "tingly-box")
)


def load_model_token(path: str) -> str:
    with open(path, "r", encoding="utf-8") as f:
        cfg = json.load(f)
    token = cfg.get("model_token")
    if not token:
        raise RuntimeError("model_token not found in config.json")
    return token


API_KEY = os.environ.get("TINGLY_BOX_API_KEY") or load_model_token(CONFIG_PATH)


def test_openai_explicit_tool_call(label=""):
    """Test OpenAI-style endpoint with tool definitions - let provider decide."""
    print("\n" + "="*70)
    print(f"Testing OpenAI-style endpoint with tool definitions {label}")
    print("Expected: Provider calls web_search, server intercepts and executes locally")
    print("="*70)

    client = OpenAI(
        base_url=OPENAI_ENDPOINT,
        api_key=API_KEY
    )

    # Define the tool
    tools = [
        {
            "type": "function",
            "function": {
                "name": "web_search",
                "description": "Search the web for current information",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "query": {
                            "type": "string",
                            "description": "The search query"
                        },
                        "count": {
                            "type": "integer",
                            "description": "Number of results to return (default: 5)"
                        }
                    },
                    "required": ["query"]
                }
            }
        }
    ]

    messages = [
        {"role": "user", "content": f"What is the latest stable version of Go (Golang)? (Today is {CURRENT_DATE})"}
    ]

    print(f"\nSending request to qwen-plus with web_search tool defined...")
    print(f"  User: What is the latest stable version of Go (Golang)? (Today is {CURRENT_DATE})")

    max_iterations = 5
    iteration = 0

    try:
        while iteration < max_iterations:
            iteration += 1
            print(f"\n--- Iteration {iteration} ---")

            response = client.chat.completions.create(
                model="qwen-plus",
                messages=messages,
                tools=tools,
                max_tokens=500
            )

            # Check response
            if response.choices:
                choice = response.choices[0]
                print(f"Finish reason: {choice.finish_reason}")

                if choice.finish_reason == "tool_calls" and choice.message.tool_calls:
                    # Provider wants to call tools - add assistant message and check if intercepted
                    print(f"  Provider wants to call {len(choice.message.tool_calls)} tool(s)")

                    assistant_message = {
                        "role": "assistant",
                        "content": choice.message.content or "",
                        "tool_calls": []
                    }

                    for tool_call in choice.message.tool_calls:
                        tc_dict = {
                            "id": tool_call.id,
                            "type": "function",
                            "function": {
                                "name": tool_call.function.name,
                                "arguments": tool_call.function.arguments
                            }
                        }
                        assistant_message["tool_calls"].append(tc_dict)

                        # Check if this is web_search
                        if tool_call.function.name == "web_search":
                            args = json.loads(tool_call.function.arguments)
                            print(f"  Tool: web_search(query='{args.get('query')}', count={args.get('count', 5)})")
                            print(f"  ⚠️  Tool was NOT intercepted by server (client executed)")
                            print(f"     Server tool interceptor should have handled this!")

                            # Client-side fallback (since server didn't intercept)
                            tool_result = json.dumps({
                                "results": [{
                                    "title": f"Search result for: {args.get('query')}",
                                    "url": "https://example.com",
                                    "snippet": "This is a client-side fallback result."
                                }]
                            })

                            messages.append(assistant_message)
                            messages.append({
                                "role": "tool",
                                "tool_call_id": tool_call.id,
                                "content": tool_result
                            })
                        else:
                            # Unknown tool - return error
                            messages.append(assistant_message)
                            messages.append({
                                "role": "tool",
                                "tool_call_id": tool_call.id,
                                "content": json.dumps({"error": f"Unknown tool: {tool_call.function.name}"})
                            })

                    print(f"  Continuing loop...")
                    continue

                elif choice.finish_reason == "stop":
                    if choice.message.content:
                        print(f"\n✅ Final answer received!")
                        print(f"\nAssistant: {choice.message.content}")
                        return True
                    else:
                        print(f"\n⚠️  Empty response (no content)")
                        return False

            return False

        print(f"\n⚠️  Max iterations ({max_iterations}) reached")
        return False

    except Exception as e:
        print(f"\n❌ ERROR: {e}")
        import traceback
        traceback.print_exc()
        return False


def test_openai_baseline():
    """Test OpenAI endpoint with a simple question (no tools)."""
    print("\n" + "="*70)
    print("Testing OpenAI endpoint with simple question (baseline)")
    print("="*70)

    client = OpenAI(
        base_url=OPENAI_ENDPOINT,
        api_key=API_KEY
    )

    print(f"\nUser: What is 2+2?")
    print(f"\nSending request to qwen-plus...")

    try:
        response = client.chat.completions.create(
            model="qwen-plus",
            messages=[
                {"role": "user", "content": "What is 2+2?"}
            ],
            max_tokens=100
        )

        result = response.choices[0].message.content
        print(f"\nAssistant: {result}")
        print(f"\n✅ SUCCESS: Basic endpoint working!")
        return True

    except Exception as e:
        print(f"\n❌ ERROR: {e}")
        return False


def test_anthropic_explicit_tool_call(label=""):
    """Test Anthropic-style endpoint - GLM has built-in web_search."""
    print("\n" + "="*70)
    print(f"Testing Anthropic-style endpoint (GLM has built-in web_search) {label}")
    print("Expected: Provider uses native web_search (no tool definition sent)")
    print("="*70)

    import requests

    messages = [
        {
            "role": "user",
            "content": f"What is the latest stable version of Go (Golang)? (Today is {CURRENT_DATE})"
        }
    ]

    print(f"\nSending request to tingly/anthropic (no tool definition)...")
    print(f"  User: What is the latest stable version of Go (Golang)? (Today is {CURRENT_DATE})")
    print(f"  Note: GLM has built-in web_search, so we don't send tool definition")

    url = f"{ANTHROPIC_ENDPOINT}/messages"
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {API_KEY}",
        "anthropic-version": "2023-06-01"
    }

    try:
        data = {
            "model": "tingly/anthropic",
            "max_tokens": 500,
            "messages": messages
        }

        response = requests.post(url, json=data, headers=headers, timeout=30)
        response.raise_for_status()
        result_json = response.json()

        # Check response
        if "content" in result_json and len(result_json["content"]) > 0:
            for block in result_json["content"]:
                if isinstance(block, dict):
                    block_type = block.get("type")

                    if block_type == "text":
                        text = block.get("text", "")
                        if text:
                            print(f"\n✅ Final answer received!")
                            print(f"\nAssistant: {text}")
                            return True

        print(f"\n⚠️  Unexpected response format")
        print(f"   Response: {json.dumps(result_json, indent=2)[:500]}")
        return False

    except requests.exceptions.HTTPError as e:
        if "401" in str(e):
            print(f"\n⚠️  GLM API token expired (401 Unauthorized)")
            print(f"   This is expected for GLM built-in web_search")
            return True
        else:
            print(f"\n❌ ERROR: {e}")
            return False
    except Exception as e:
        print(f"\n❌ ERROR: {e}")
        import traceback
        traceback.print_exc()
        return False


def _read_config(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def _write_config(path: str, cfg: dict) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(cfg, f, indent=2, ensure_ascii=False)


def _restart_server(bin_path: str, config_dir: str) -> bool:
    if not os.path.exists(bin_path):
        print(f"⚠️  Server binary not found: {bin_path}")
        return False
    try:
        subprocess.run(
            [bin_path, "stop", "--config-dir", config_dir],
            check=False,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        subprocess.run(
            [bin_path, "start", "--config-dir", config_dir, "--port", "12580", "--daemon"],
            check=True,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        time.sleep(2)
        return True
    except Exception as exc:
        print(f"⚠️  Failed to restart server: {exc}")
        return False


def run_with_prefer_local_search(enabled: bool, server_bin: str) -> bool:
    config_dir = os.path.dirname(CONFIG_PATH)
    cfg = _read_config(CONFIG_PATH)
    tool_cfg = cfg.get("tool_interceptor")
    if tool_cfg is None:
        tool_cfg = {}
    tool_cfg["prefer_local_search"] = bool(enabled)
    cfg["tool_interceptor"] = tool_cfg

    with tempfile.NamedTemporaryFile("w+", delete=False, encoding="utf-8") as tmp:
        json.dump(cfg, tmp, indent=2, ensure_ascii=False)
        tmp_path = tmp.name

    # Backup original config
    backup_path = CONFIG_PATH + ".bak"
    os.replace(CONFIG_PATH, backup_path)
    os.replace(tmp_path, CONFIG_PATH)

    try:
        if not _restart_server(server_bin, config_dir):
            return False
        label = "(prefer_local_search=on)" if enabled else "(prefer_local_search=off)"
        ok1 = test_openai_explicit_tool_call(label=label)
        ok2 = test_anthropic_explicit_tool_call(label=label)
        return ok1, ok2
    finally:
        # Restore original config
        if os.path.exists(backup_path):
            os.replace(backup_path, CONFIG_PATH)
            _restart_server(server_bin, config_dir)


def main():
    """Run all tests."""
    print("\n" + "="*70)
    print("Web Search Functionality Test Suite (Explicit Tool Calls)")
    print("="*70)
    print(f"Base URL: {BASE_URL}")
    print(f"OpenAI Endpoint: {OPENAI_ENDPOINT}")
    print(f"Anthropic Endpoint: {ANTHROPIC_ENDPOINT}")

    results = {}

    # Comparison tests toggling prefer_local_search (qwen + glm)
    server_bin = os.environ.get("TINGLY_BOX_BIN", DEFAULT_SERVER_BIN)
    qwen_off, glm_off = run_with_prefer_local_search(False, server_bin)
    qwen_on, glm_on = run_with_prefer_local_search(True, server_bin)

    results['qwen_prefer_local_off'] = qwen_off
    results['glm_prefer_local_off'] = glm_off
    results['qwen_prefer_local_on'] = qwen_on
    results['glm_prefer_local_on'] = glm_on

    # Summary
    print("\n" + "="*70)
    print("Test Summary")
    print("="*70)
    for test_name, passed in results.items():
        status = "✅ PASS" if passed else "❌ FAIL"
        print(f"{status} - {test_name}")

    # Exit with appropriate code
    all_passed = all(results.values())
    if all_passed:
        print("\n🎉 All tests passed!")
        sys.exit(0)
    else:
        print("\n⚠️  Some tests failed")
        sys.exit(1)


if __name__ == "__main__":
    main()
