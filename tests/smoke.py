"""
Smoke test suite for provider API testing.
"""

import time
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime

from .config import TestConfig, Provider, APIStyle, Rule
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
    http_method: Optional[str] = None
    http_url: Optional[str] = None
    http_status: Optional[int] = None
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
        self.proxy_client = ProxyClient(
            server_url=config.server_url,
            token=config.auth_token,
            timeout=config.timeout,
        )
        self.proxy_only = True

    def _print(self, msg: str):
        if self.verbose:
            print(f"  [SMOKE] {msg}")

    def _create_client(self, provider: Provider) -> BaseProviderClient:
        """Create appropriate client for provider."""
        if self.proxy_only:
            raise RuntimeError("Direct provider client creation disabled in proxy-only mode")
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

    def _get_rule_for_provider(self, provider: Provider) -> Optional[Rule]:
        for rule in self.config.rules:
            if not rule.active:
                continue
            for service in rule.services or []:
                if service.get("provider") == provider.uuid:
                    return rule
        return None

    def test_provider_model_fetch(self, provider: Provider) -> list[SmokeTestResult]:
        """Test model fetching for a provider."""
        results = []
        self._print(f"Testing model fetch for {provider.name}")

        try:
            if self.proxy_only:
                rule = self._get_rule_for_provider(provider)
                if not rule:
                    raise RuntimeError("No routing rule found for provider")
                if provider.api_style == APIStyle.ANTHROPIC:
                    result = self.proxy_client.list_models_anthropic(scenario=rule.scenario)
                else:
                    result = self.proxy_client.list_models_openai(scenario=rule.scenario)
            else:
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
                http_method=(result.data or {}).get("http_method"),
                http_url=(result.data or {}).get("http_url"),
                http_status=(result.data or {}).get("http_status"),
                error=result.error,
            )
            results.append(smoke_result)

            # Print detailed result
            if self.verbose:
                if result.success:
                    self._print(f"  [SMOKE]   list_models: PASS - {result.message}")
                    if result.data and 'models' in result.data:
                        self._print(f"  [SMOKE]     Found {len(result.data['models'])} models")
                else:
                    self._print(f"  [SMOKE]   list_models: FAIL - {result.message}")
                    if result.error:
                        error_short = result.error[:100] + "..." if len(result.error) > 100 else result.error
                        self._print(f"  [SMOKE]     Error: {error_short}")

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
            if self.proxy_only:
                rule = self._get_rule_for_provider(provider)
                if not rule:
                    raise RuntimeError("No routing rule found for provider")
                if provider.api_style == APIStyle.ANTHROPIC:
                    result = self.proxy_client.messages_anthropic(
                        model=rule.request_model,
                        prompt=test_prompt,
                        scenario=rule.scenario,
                        temperature=0.7,
                        max_tokens=100,
                    )
                else:
                    result = self.proxy_client.chat_completions_openai(
                        model=rule.request_model,
                        prompt=test_prompt,
                        scenario=rule.scenario,
                        temperature=0.7,
                        max_tokens=100,
                    )
            else:
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
                http_method=(result.data or {}).get("http_method"),
                http_url=(result.data or {}).get("http_url"),
                http_status=(result.data or {}).get("http_status"),
                error=result.error,
            )
            results.append(smoke_result)

            # Print detailed result
            if self.verbose:
                if result.success:
                    self._print(f"  [SMOKE]   chat_completions: PASS - {result.message}")
                else:
                    self._print(f"  [SMOKE]   chat_completions: FAIL - {result.message}")
                    if result.error:
                        error_short = result.error[:100] + "..." if len(result.error) > 100 else result.error
                        self._print(f"  [SMOKE]     Error: {error_short}")

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
            if self.proxy_only:
                rule = self._get_rule_for_provider(provider)
                if not rule:
                    raise RuntimeError("No routing rule found for provider")
                if provider.api_style == APIStyle.ANTHROPIC:
                    result = self.proxy_client.messages_anthropic(
                        model=rule.request_model,
                        prompt=test_prompt,
                        scenario=rule.scenario,
                        system=system_prompt,
                        messages=[{"role": "user", "content": test_prompt}],
                        temperature=0.7,
                        max_tokens=100,
                    )
                else:
                    result = self.proxy_client.chat_completions_openai(
                        model=rule.request_model,
                        prompt=test_prompt,
                        scenario=rule.scenario,
                        messages=[
                            {"role": "system", "content": system_prompt},
                            {"role": "user", "content": test_prompt},
                        ],
                        temperature=0.7,
                        max_tokens=100,
                    )
            else:
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

            smoke_result = SmokeTestResult(
                provider_name=provider.name,
                api_style=provider.api_style.value,
                test_type="chat_completions_with_system",
                passed=result.success,
                message=result.message,
                duration_ms=result.duration_ms,
                details=result.data or {},
                http_method=(result.data or {}).get("http_method"),
                http_url=(result.data or {}).get("http_url"),
                http_status=(result.data or {}).get("http_status"),
                error=result.error,
            )

            # Print detailed result
            if self.verbose:
                if result.success:
                    self._print(f"  [SMOKE]   chat_completions_with_system: PASS - {result.message}")
                else:
                    self._print(f"  [SMOKE]   chat_completions_with_system: FAIL - {result.message}")
                    if result.error:
                        error_short = result.error[:100] + "..." if len(result.error) > 100 else result.error
                        self._print(f"  [SMOKE]     Error: {error_short}")

            return [smoke_result]

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

        # Use specific model names based on provider and API style
        if provider.api_style == APIStyle.OPENAI:
            if provider.name.lower() == "qwen":
                return "qwen-plus"
            elif provider.name.lower() == "deepseek":
                return "deepseek-chat"
            else:
                return "tingly-gpt"
        elif provider.api_style == APIStyle.ANTHROPIC:
            if provider.name.lower() == "glm":
                return "glm-4.7"
            else:
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
                if self.verbose:
                    self._print(f"  list_models: {'PASS' if r.passed else 'FAIL'} - {r.message}")

            results = self.test_provider_chat(provider)
            for r in results:
                suite_result.add_result(r)
                if self.verbose:
                    self._print(f"  chat_completions: {'PASS' if r.passed else 'FAIL'} - {r.message}")

            results = self.test_provider_chat_with_system(provider)
            for r in results:
                suite_result.add_result(r)
                if self.verbose:
                    self._print(f"  chat_with_system: {'PASS' if r.passed else 'FAIL'} - {r.message}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        # Print summary if verbose
        if self.verbose:
            self._print(f"\n--- Smoke Test Summary ---")
            self._print(f"Total: {suite_result.total_tests} | Passed: {suite_result.passed} | Failed: {suite_result.failed}")
            self._print(f"Success Rate: {suite_result.success_rate:.1f}%")

            # Group by provider
            from collections import defaultdict
            provider_stats = defaultdict(lambda: {'passed': 0, 'failed': 0, 'total': 0})
            for result in suite_result.results:
                provider = result.provider_name
                provider_stats[provider]['total'] += 1
                if result.passed:
                    provider_stats[provider]['passed'] += 1
                else:
                    provider_stats[provider]['failed'] += 1

            self._print("\nBy Provider:")
            for provider, stats in provider_stats.items():
                success_rate = (stats['passed'] / stats['total'] * 100) if stats['total'] > 0 else 0
                self._print(f"  {provider}: {stats['passed']}/{stats['total']} passed ({success_rate:.1f}%)")

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

    def _get_rule_for_scenario(self, scenario: str) -> Optional[Rule]:
        rule = self.config.get_rule_by_scenario(scenario)
        if rule:
            return rule
        return self.config.get_any_rule()

    def _get_rule_by_request_model(self, request_model: str) -> Optional[Rule]:
        for rule in self.config.rules:
            if rule.active and rule.request_model == request_model:
                return rule
        return None

    def _run_proxy_chat(self, request_model: str, test_type: str, api_style: str) -> SmokeTestResult:
        rule = self._get_rule_by_request_model(request_model)
        if not rule:
            return SmokeTestResult(
                provider_name="proxy",
                api_style=api_style,
                test_type=test_type,
                passed=False,
                message=f"Rule not found for request_model {request_model}",
                duration_ms=0,
                error="missing_rule",
            )

        test_prompt = self.config.test_prompt
        if api_style == "openai":
            result = self.proxy_client.chat_completions_openai(
                model=rule.request_model,
                prompt=test_prompt,
                scenario=rule.scenario,
                temperature=0.7,
                max_tokens=100,
            )
            test_type_label = "chat_completions"
        else:
            result = self.proxy_client.messages_anthropic(
                model=rule.request_model,
                prompt=test_prompt,
                scenario=rule.scenario,
                temperature=0.7,
                max_tokens=100,
            )
            test_type_label = "messages"

        return SmokeTestResult(
            provider_name="proxy",
            api_style=api_style,
            test_type=test_type_label,
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            http_method=(result.data or {}).get("http_method"),
            http_url=(result.data or {}).get("http_url"),
            http_status=(result.data or {}).get("http_status"),
            error=result.error,
        )

    def test_proxy_list_models_openai(self) -> SmokeTestResult:
        """Test proxy OpenAI models endpoint."""
        rule = self._get_rule_for_scenario("openai")
        scenario = rule.scenario if rule else "openai"
        result = self.proxy_client.list_models_openai(scenario=scenario)

        return SmokeTestResult(
            provider_name="proxy",
            api_style="openai",
            test_type="list_models",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            http_method=(result.data or {}).get("http_method"),
            http_url=(result.data or {}).get("http_url"),
            http_status=(result.data or {}).get("http_status"),
            error=result.error,
        )

    def test_proxy_list_models_anthropic(self) -> SmokeTestResult:
        """Test proxy Anthropic models endpoint."""
        rule = self._get_rule_for_scenario("anthropic")
        scenario = rule.scenario if rule else "anthropic"
        result = self.proxy_client.list_models_anthropic(scenario=scenario)

        return SmokeTestResult(
            provider_name="proxy",
            api_style="anthropic",
            test_type="list_models",
            passed=result.success,
            message=result.message,
            duration_ms=result.duration_ms,
            details=result.data or {},
            http_method=(result.data or {}).get("http_method"),
            http_url=(result.data or {}).get("http_url"),
            http_status=(result.data or {}).get("http_status"),
            error=result.error,
        )

    def test_proxy_chat_openai(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> SmokeTestResult:
        """Test proxy OpenAI chat endpoint."""
        test_prompt = prompt or self.config.test_prompt

        rule = self._get_rule_for_scenario("openai")
        scenario = rule.scenario if rule else "openai"
        request_model = rule.request_model if rule and rule.request_model else model

        result = self.proxy_client.chat_completions_openai(
            model=request_model or "",
            prompt=test_prompt,
            scenario=scenario,
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
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> SmokeTestResult:
        """Test proxy Anthropic messages endpoint."""
        test_prompt = prompt or self.config.test_prompt

        rule = self._get_rule_for_scenario("anthropic")
        scenario = rule.scenario if rule else "anthropic"
        request_model = rule.request_model if rule and rule.request_model else model

        result = self.proxy_client.messages_anthropic(
            model=request_model or "",
            prompt=test_prompt,
            scenario=scenario,
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

        targets = [
            ("qwen-test", "openai"),
            ("minimax-test", "anthropic"),
            ("glm-test", "anthropic"),
        ]
        for request_model, api_style in targets:
            self._print(f"Testing proxy {api_style} chat endpoint for {request_model}")
            result = self._run_proxy_chat(request_model, "chat", api_style)
            suite_result.add_result(result)
            self._print(f"  {request_model}: {'PASS' if result.passed else 'FAIL'}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result
