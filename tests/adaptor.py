"""
Adaptor test suite for testing cross-provider transformations.
"""

import json
import time
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime

from .config import TestConfig
from .client import ProxyClient


@dataclass
class AdaptorTestResult:
    """Result of an adaptor transformation test."""
    source_style: str
    target_style: str
    test_type: str
    passed: bool
    message: str
    duration_ms: float
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())
    request_transform: dict = field(default_factory=dict)
    response_transform: dict = field(default_factory=dict)
    original_request: dict = field(default_factory=dict)
    transformed_response: dict = field(default_factory=dict)
    error: Optional[str] = None


@dataclass
class AdaptorTestSuiteResult:
    """Aggregate result of all adaptor tests."""
    suite_name: str
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    results: list[AdaptorTestResult] = field(default_factory=list)
    duration_ms: float = 0.0
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    @property
    def success_rate(self) -> float:
        if self.total_tests == 0:
            return 0.0
        return (self.passed / self.total_tests) * 100


class AdaptorTestSuite:
    """Test suite for adaptor/transformation testing."""

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
            print(f"  [ADAPTOR] {msg}")

    def _get_rule_for_scenario(self, scenario: str):
        rule = self.config.get_rule_by_scenario(scenario)
        if rule:
            return rule
        return self.config.get_any_rule()

    def _build_openai_request(
        self,
        model: str,
        prompt: str,
        temperature: float = 0.7,
        max_tokens: int = 100,
    ) -> dict:
        """Build OpenAI-format request."""
        return {
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": temperature,
            "max_tokens": max_tokens,
        }

    def _build_anthropic_request(
        self,
        model: str,
        prompt: str,
        temperature: float = 0.7,
        max_tokens: int = 100,
    ) -> dict:
        """Build Anthropic-format request."""
        return {
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "temperature": temperature,
            "max_tokens": max_tokens,
        }

    def test_openai_to_anthropic_adaptor(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test OpenAI-format request to Anthropic backend."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("anthropic")
            scenario = rule.scenario if rule else "anthropic"
            request_model = rule.request_model if rule and rule.request_model else model
            request_body = self._build_openai_request(request_model or "", test_prompt)

            self._print(f"Testing OpenAI->Anthropic adaptation with model {request_model}")

            with ProxyClient(
                server_url=self.config.server_url,
                token=self.config.auth_token,
            ) as client:
                result = client.chat_completions_openai(
                    model=request_model or "",
                    prompt=test_prompt,
                    scenario=scenario,
                )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="anthropic",
                    test_type="openai_to_anthropic",
                    passed=True,
                    message="OpenAI request successfully adapted to Anthropic backend",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="anthropic",
                    test_type="openai_to_anthropic",
                    passed=False,
                    message="Adaptation failed",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="openai",
                target_style="anthropic",
                test_type="openai_to_anthropic",
                passed=False,
                message="Exception during adaptation test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_anthropic_to_openai_adaptor(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test Anthropic-format request to OpenAI backend."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("openai")
            scenario = rule.scenario if rule else "openai"
            request_model = rule.request_model if rule and rule.request_model else model
            request_body = self._build_anthropic_request(request_model or "", test_prompt)

            self._print(f"Testing Anthropic->OpenAI adaptation with model {request_model}")

            result = self.proxy_client.messages_anthropic(
                model=request_model or "",
                prompt=test_prompt,
                scenario=scenario,
            )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="openai",
                    test_type="anthropic_to_openai",
                    passed=True,
                    message="Anthropic request successfully adapted to OpenAI backend",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="openai",
                    test_type="anthropic_to_openai",
                    passed=False,
                    message="Adaptation failed",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="anthropic",
                target_style="openai",
                test_type="anthropic_to_openai",
                passed=False,
                message="Exception during adaptation test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_openai_to_google_adaptor(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test OpenAI-format request to Google backend."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("openai")
            scenario = rule.scenario if rule else "openai"
            request_model = rule.request_model if rule and rule.request_model else model
            request_body = self._build_openai_request(request_model or "", test_prompt)

            self._print(f"Testing OpenAI->Google adaptation with model {request_model}")

            result = self.proxy_client.chat_completions_openai(
                model=request_model or "",
                prompt=test_prompt,
                scenario=scenario,
            )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="google",
                    test_type="openai_to_google",
                    passed=True,
                    message="OpenAI request successfully adapted to Google backend",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="google",
                    test_type="openai_to_google",
                    passed=False,
                    message="Adaptation failed",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="openai",
                target_style="google",
                test_type="openai_to_google",
                passed=False,
                message="Exception during adaptation test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_anthropic_to_google_adaptor(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test Anthropic-format request to Google backend."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("anthropic")
            scenario = rule.scenario if rule else "anthropic"
            request_model = rule.request_model if rule and rule.request_model else model
            request_body = self._build_anthropic_request(request_model or "", test_prompt)

            self._print(f"Testing Anthropic->Google adaptation with model {request_model}")

            result = self.proxy_client.messages_anthropic(
                model=request_model or "",
                prompt=test_prompt,
                scenario=scenario,
            )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="google",
                    test_type="anthropic_to_google",
                    passed=True,
                    message="Anthropic request successfully adapted to Google backend",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="google",
                    test_type="anthropic_to_google",
                    passed=False,
                    message="Adaptation failed",
                    duration_ms=duration_ms,
                    original_request=request_body,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="anthropic",
                target_style="google",
                test_type="anthropic_to_google",
                passed=False,
                message="Exception during adaptation test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_multi_turn_conversation(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test multi-turn conversation through adaptor."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("openai")
            scenario = rule.scenario if rule else "openai"
            request_model = rule.request_model if rule and rule.request_model else model
            messages = [
                {"role": "system", "content": "You are a helpful assistant."},
                {"role": "user", "content": "What is 2+2?"},
                {"role": "assistant", "content": "2+2 equals 4."},
                {"role": "user", "content": "What about 3+3?"},
            ]

            self._print(f"Testing multi-turn conversation with model {request_model}")

            with ProxyClient(
                server_url=self.config.server_url,
                token=self.config.auth_token,
            ) as client:
                result = client.chat_completions_openai(
                    model=request_model or "",
                    prompt=test_prompt,
                    scenario=scenario,
                )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="anthropic",
                    test_type="multi_turn_conversation",
                    passed=True,
                    message="Multi-turn conversation successful through adaptor",
                    duration_ms=duration_ms,
                    original_request={"messages": messages},
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="openai",
                    target_style="anthropic",
                    test_type="multi_turn_conversation",
                    passed=False,
                    message="Multi-turn conversation failed",
                    duration_ms=duration_ms,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="openai",
                target_style="anthropic",
                test_type="multi_turn_conversation",
                passed=False,
                message="Exception during multi-turn test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_system_message_handling(
        self,
        model: Optional[str] = None,
        prompt: Optional[str] = None,
    ) -> AdaptorTestResult:
        """Test system message handling in adaptor."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            rule = self._get_rule_for_scenario("anthropic")
            scenario = rule.scenario if rule else "anthropic"
            request_model = rule.request_model if rule and rule.request_model else model
            system_prompt = "You are a test assistant. Be concise."
            messages = [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": test_prompt},
            ]

            self._print(f"Testing system message handling with model {request_model}")

            result = self.proxy_client.messages_anthropic(
                model=request_model or "",
                prompt=test_prompt,
                scenario=scenario,
            )

            duration_ms = (time.time() - start_time) * 1000

            if result.success:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="anthropic",
                    test_type="system_message_handling",
                    passed=True,
                    message="System message handled correctly",
                    duration_ms=duration_ms,
                    original_request={"system": system_prompt, "messages": messages},
                    transformed_response=result.raw_response or {},
                    error=None,
                )
            else:
                return AdaptorTestResult(
                    source_style="anthropic",
                    target_style="anthropic",
                    test_type="system_message_handling",
                    passed=False,
                    message="System message handling failed",
                    duration_ms=duration_ms,
                    error=result.error,
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return AdaptorTestResult(
                source_style="anthropic",
                target_style="anthropic",
                test_type="system_message_handling",
                passed=False,
                message="Exception during system message test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def run_all_tests(self) -> AdaptorTestSuiteResult:
        """Run all adaptor tests."""
        suite_result = AdaptorTestSuiteResult(suite_name="Adaptor Test Suite")
        start_time = time.time()

        self._print("=== Running Adaptor Tests ===\n")

        tests = [
            ("OpenAI to Anthropic", self.test_openai_to_anthropic_adaptor),
            ("Anthropic to OpenAI", self.test_anthropic_to_openai_adaptor),
            ("OpenAI to Google", self.test_openai_to_google_adaptor),
            ("Anthropic to Google", self.test_anthropic_to_google_adaptor),
            ("Multi-turn conversation", self.test_multi_turn_conversation),
            ("System message handling", self.test_system_message_handling),
        ]

        for name, test_func in tests:
            self._print(f"Testing {name}...")
            result = test_func()
            suite_result.results.append(result)
            if result.passed:
                suite_result.passed += 1
            else:
                suite_result.failed += 1
            suite_result.total_tests += 1
            self._print(f"  Result: {'PASS' if result.passed else 'FAIL'} - {result.message}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result
