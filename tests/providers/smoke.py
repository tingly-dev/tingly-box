"""
Smoke test suite for provider API testing.
"""

import time
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime

from .config import TestConfig, Provider, APIStyle
from .client import (
    BaseProviderClient,
    OpenAIClient,
    AnthropicClient,
    GoogleClient,
    ProxyClient,
    ChatRequest,
    ChatMessage,
    TestResult,
)


@dataclass
class SmokeTestResult:
    """Result of a smoke test."""
    provider_name: str
    api_style: str
    test_type: str
    passed: bool
    message: str
    duration_ms: float
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())
    details: dict = field(default_factory=dict)
    error: Optional[str] = None


@dataclass
class SmokeTestSuiteResult:
    """Aggregate result of all smoke tests."""
    suite_name: str
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    results: list[SmokeTestResult] = field(default_factory=list)
    duration_ms: float = 0.0
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    @property
    def success_rate(self) -> float:
        if self.total_tests == 0:
            return 0.0
        return (self.passed / self.total_tests) * 100

    def add_result(self, result: SmokeTestResult):
        self.results.append(result)
        self.total_tests += 1
        if result.passed:
            self.passed += 1
        else:
            self.failed += 1


class SmokeTestSuite:
    """Smoke test suite for provider API testing."""

    def __init__(self, config: TestConfig, verbose: bool = False):
        self.config = config
        self.verbose = verbose

    def _print(self, msg: str):
        if self.verbose:
            print(f"  [SMOKE] {msg}")

    def _create_client(self, provider: Provider) -> BaseProviderClient:
        """Create appropriate client for provider."""
        if provider.api_style == APIStyle.OPENAI:
            return OpenAIClient(
                name=provider.name,
                api_base=provider.api_base,
                token=provider.token,
                proxy_url=provider.proxy_url or None,
                timeout=provider.timeout,
            )
        elif provider.api_style == APIStyle.ANTHROPIC:
            return AnthropicClient(
                name=provider.name,
                api_base=provider.api_base,
                token=provider.token,
                proxy_url=provider.proxy_url or None,
                timeout=provider.timeout,
            )
        elif provider.api_style == APIStyle.GOOGLE:
            return GoogleClient(
                name=provider.name,
                api_base=provider.api_base,
                token=provider.token,
                proxy_url=provider.proxy_url or None,
                timeout=provider.timeout,
            )
        else:
            raise ValueError(f"Unknown API style: {provider.api_style}")

    def test_provider_model_fetch(self, provider: Provider) -> list[SmokeTestResult]:
        """Test model fetching for a provider."""
        results = []
        self._print(f"Testing model fetch for {provider.name}")

        try:
            client = self._create_client(provider)
            result = client.list_models()

            smoke_result = SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="list_models",
                passed=result.success,
                message=result.message,
                duration_ms=result.duration_ms,
                details=result.data or {},
                error=result.error,
            )
            results.append(smoke_result)

        except Exception as e:
            results.append(SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="list_models",
                passed=False,
                message="Exception during test",
                duration_ms=0,
                error=str(e),
            ))

        return results

    def test_provider_chat(
        self,
        provider: Provider,
        prompt: Optional[str] = None,
        model: Optional[str] = None,
    ) -> list[SmokeTestResult]:
        """Test chat completion for a provider."""
        results = []

        test_prompt = prompt or self.config.test_prompt
        test_model = model or self._get_test_model(provider)

        if not test_model:
            return [SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions",
                passed=False,
                message="No test model available",
                duration_ms=0,
                error="No model specified or available",
            )]

        self._print(f"Testing chat for {provider.name} with model {test_model}")

        try:
            client = self._create_client(provider)
            request = ChatRequest(
                model=test_model,
                messages=[ChatMessage(role="user", content=test_prompt)],
                temperature=0.7,
                max_tokens=100,
            )

            result = client.chat_completions(request)

            smoke_result = SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions",
                passed=result.success,
                message=result.message,
                duration_ms=result.duration_ms,
                details=result.data or {},
                error=result.error,
            )
            results.append(smoke_result)

        except Exception as e:
            results.append(SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions",
                passed=False,
                message="Exception during test",
                duration_ms=0,
                error=str(e),
            ))

        return results

    def test_provider_chat_with_system(
        self,
        provider: Provider,
        system_prompt: str = "You are a helpful assistant.",
        user_prompt: Optional[str] = None,
        model: Optional[str] = None,
    ) -> list[SmokeTestResult]:
        """Test chat completion with system message."""
        test_prompt = user_prompt or self.config.test_prompt
        test_model = model or self._get_test_model(provider)

        if not test_model:
            return []

        self._print(f"Testing chat with system for {provider.name}")

        try:
            client = self._create_client(provider)
            request = ChatRequest(
                model=test_model,
                messages=[
                    ChatMessage(role="system", content=system_prompt),
                    ChatMessage(role="user", content=test_prompt),
                ],
                temperature=0.7,
                max_tokens=100,
            )

            result = client.chat_completions(request)

            return [SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions_with_system",
                passed=result.success,
                message=result.message,
                duration_ms=result.duration_ms,
                details=result.data or {},
                error=result.error,
            )]

        except Exception as e:
            return [SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions_with_system",
                passed=False,
                message="Exception during test",
                duration_ms=0,
                error=str(e),
            )]

    def _get_test_model(self, provider: Provider) -> Optional[str]:
        """Get test model for provider."""
        if self.config.test_model:
            return self.config.test_model

        if provider.models:
            return provider.models[0]

        if provider.api_style == APIStyle.OPENAI:
            return "tingly-gpt"
        elif provider.api_style == APIStyle.ANTHROPIC:
            return "tingly-claude"
        elif provider.api_style == APIStyle.GOOGLE:
            return "tingly-claude"

        return None

    def run_all_tests(self, providers: Optional[list[Provider]] = None) -> SmokeTestSuiteResult:
        """Run all smoke tests for providers."""
        suite_result = SmokeTestSuiteResult(suite_name="Smoke Test Suite")
        test_providers = providers or self.config.providers

        if not test_providers:
            self._print("No providers to test")
            return suite_result

        start_time = time.time()

        for provider in test_providers:
            self._print(f"\n--- Testing {provider.name} ({provider.api_style.value}) ---")

            results = self.test_provider_model_fetch(provider)
            for r in results:
                suite_result.add_result(r)
                self._print(f"  list_models: {'PASS' if r.passed else 'FAIL'} - {r.message}")

            results = self.test_provider_chat(provider)
            for r in results:
                suite_result.add_result(r)
                self._print(f"  chat_completions: {'PASS' if r.passed else 'FAIL'} - {r.message}")

            results = self.test_provider_chat_with_system(provider)
            for r in results:
                suite_result.add_result(r)
                self._print(f"  chat_with_system: {'PASS' if r.passed else 'FAIL'} - {r.message}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result


class ProxySmokeTestSuite:
    """Smoke tests for tingly-box proxy endpoints."""

    def __init__(self, config: TestConfig, verbose: bool = False):
        self.config = config
        self.verbose = verbose
        self.proxy_client = ProxyClient(
            server_url=config.server_url,
            token=config.auth_token,
            timeout=config.timeout,
        )

    def _print(self, msg: str):
        if self.verbose:
            print(f"  [PROXY] {msg}")

    def test_proxy_list_models_openai(self) -> SmokeTestResult:
        """Test proxy OpenAI models endpoint."""
        result = self.proxy_client.list_models_openai()

        return SmokeTestResult(
            provider_name="proxy",
            api_style="openai",
            test_type="list_models",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            error=result.error,
        )

    def test_proxy_list_models_anthropic(self) -> SmokeTestResult:
        """Test proxy Anthropic models endpoint."""
        result = self.proxy_client.list_models_anthropic()

        return SmokeTestResult(
            provider_name="proxy",
            api_style="anthropic",
            test_type="list_models",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            error=result.error,
        )

    def test_proxy_chat_openai(
        self,
        model: str = "tingly-claude",
        prompt: Optional[str] = None,
    ) -> SmokeTestResult:
        """Test proxy OpenAI chat endpoint."""
        test_prompt = prompt or self.config.test_prompt

        result = self.proxy_client.chat_completions_openai(
            model=model,
            prompt=test_prompt,
            temperature=0.7,
            max_tokens=100,
        )

        return SmokeTestResult(
            provider_name="proxy",
            api_style="openai",
            test_type="chat_completions",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            error=result.error,
        )

    def test_proxy_chat_anthropic(
        self,
        model: str = "tingly-claude",
        prompt: Optional[str] = None,
    ) -> SmokeTestResult:
        """Test proxy Anthropic messages endpoint."""
        test_prompt = prompt or self.config.test_prompt

        result = self.proxy_client.messages_anthropic(
            model=model,
            prompt=test_prompt,
            temperature=0.7,
            max_tokens=100,
        )

        return SmokeTestResult(
            provider_name="proxy",
            api_style="anthropic",
            test_type="messages",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            error=result.error,
        )

    def run_all_tests(self) -> SmokeTestSuiteResult:
        """Run all proxy smoke tests."""
        suite_result = SmokeTestSuiteResult(suite_name="Proxy Smoke Test Suite")
        start_time = time.time()

        self._print("Testing proxy OpenAI models endpoint")
        result = self.test_proxy_list_models_openai()
        suite_result.add_result(result)
        self._print(f"  list_models: {'PASS' if result.passed else 'FAIL'}")

        self._print("Testing proxy Anthropic models endpoint")
        result = self.test_proxy_list_models_anthropic()
        suite_result.add_result(result)
        self._print(f"  anthropic_list_models: {'PASS' if result.passed else 'FAIL'}")

        self._print("Testing proxy OpenAI chat endpoint")
        result = self.test_proxy_chat_openai(model="tingly-claude")
        suite_result.add_result(result)
        self._print(f"  chat_completions: {'PASS' if result.passed else 'FAIL'}")

        self._print("Testing proxy Anthropic messages endpoint")
        result = self.test_proxy_chat_anthropic(model="tingly-claude")
        suite_result.add_result(result)
        self._print(f"  messages: {'PASS' if result.passed else 'FAIL'}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result
