"""
Differential test suite for verifying transformation correctness.

The principle: "The minority is guilty" - when comparing responses,
if one result differs from others, investigate the different one.
"""

import hashlib
import time
from dataclasses import dataclass, field
from typing import Optional
from datetime import datetime
from pathlib import Path
from difflib import SequenceMatcher
import statistics

from .config import TestConfig
from .client import ProxyClient


@dataclass
class DifferentialResult:
    """Result of a differential comparison test."""
    test_name: str
    comparison_type: str
    passed: bool
    verdict: str
    message: str
    duration_ms: float
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    baseline_response: dict = field(default_factory=dict)
    comparison_response: dict = field(default_factory=dict)
    differences: list = field(default_factory=list)
    similarity_score: float = 0.0

    baseline_tokens: int = 0
    comparison_tokens: int = 0
    token_difference: int = 0

    error: Optional[str] = None


@dataclass
class DifferentialTestSuiteResult:
    """Aggregate result of all differential tests."""
    suite_name: str
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    inconclusive: int = 0
    results: list[DifferentialResult] = field(default_factory=list)
    duration_ms: float = 0.0
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    @property
    def success_rate(self) -> float:
        if self.total_tests == 0:
            return 0.0
        return (self.passed / self.total_tests) * 100


class DifferentialTestSuite:
    """Differential testing suite for transformation verification."""

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
            print(f"  [DIFF] {msg}")

    def _hash_response(self, response: dict) -> str:
        """Create a hash of the response content for comparison."""
        content_parts = []

        if "choices" in response:
            for choice in response.get("choices", []):
                if "message" in choice:
                    content_parts.append(choice["message"].get("content", ""))
                elif "delta" in choice:
                    content_parts.append(choice["delta"].get("content", ""))

        if "content" in response:
            for block in response.get("content", []):
                if block.get("type") == "text":
                    content_parts.append(block.get("text", ""))

        if "candidates" in response:
            for candidate in response.get("candidates", []):
                if "content" in candidate:
                    content = candidate["content"].get("parts", [])
                    for part in content:
                        if "text" in part:
                            content_parts.append(part["text"])

        content_text = "".join(content_parts)
        return hashlib.md5(content_text.encode()).hexdigest()

    def _extract_content(self, response: dict) -> str:
        """Extract text content from response."""
        content_parts = []

        if "choices" in response:
            for choice in response.get("choices", []):
                if "message" in choice:
                    content_parts.append(choice["message"].get("content", ""))
                elif "delta" in choice:
                    content_parts.append(choice["delta"].get("content", ""))

        if "content" in response:
            for block in response.get("content", []):
                if block.get("type") == "text":
                    content_parts.append(block.get("text", ""))

        if "candidates" in response:
            for candidate in response.get("candidates", []):
                if "content" in candidate:
                    content = candidate["content"].get("parts", [])
                    for part in content:
                        if "text" in part:
                            content_parts.append(part["text"])

        return "".join(content_parts)

    def _calculate_similarity(self, text1: str, text2: str) -> float:
        """Calculate similarity between two texts."""
        if not text1 and not text2:
            return 1.0
        if not text1 or not text2:
            return 0.0
        return SequenceMatcher(None, text1, text2).ratio()

    def _count_tokens_estimate(self, text: str) -> int:
        """Estimate token count."""
        return max(1, len(text) // 4)

    def _normalize_response(self, response: dict) -> dict:
        """Normalize response for comparison."""
        normalized = {}

        content = self._extract_content(response)
        normalized["content"] = content
        normalized["content_hash"] = self._hash_response(response)
        normalized["model"] = response.get("model", "")

        if "usage" in response:
            normalized["usage"] = response.get("usage", {})
        elif "usage" in response.get("message", {}):
            normalized["usage"] = response["message"].get("usage", {})

        return normalized

    def test_roundtrip_openai_anthropic_openai(
        self,
        model: str = "tingly-claude",
        prompt: Optional[str] = None,
    ) -> DifferentialResult:
        """Test roundtrip: OpenAI -> Anthropic -> OpenAI."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            responses = []

            self._print("Getting baseline from direct OpenAI API...")
            direct_result = self.proxy_client.chat_completions_openai(
                model=model,
                prompt=test_prompt,
            )

            if direct_result.success:
                responses.append({
                    "method": "direct_openai",
                    "response": direct_result.raw_response or {},
                    "success": True,
                })

            self._print("Getting comparison from transformed OpenAI->Anthropic->OpenAI...")
            transformed_result = self.proxy_client.chat_completions_openai(
                model=model,
                prompt=test_prompt,
            )

            if transformed_result.success:
                responses.append({
                    "method": "transformed_roundtrip",
                    "response": transformed_result.raw_response or {},
                    "success": True,
                })

            duration_ms = (time.time() - start_time) * 1000

            if len(responses) < 2:
                return DifferentialResult(
                    test_name="roundtrip_o_a_o",
                    comparison_type="roundtrip",
                    passed=False,
                    verdict="inconclusive",
                    message="Insufficient responses for comparison",
                    duration_ms=duration_ms,
                    error="Need at least 2 responses for comparison",
                )

            baseline = responses[0]
            comparison = responses[1]

            baseline_normalized = self._normalize_response(baseline["response"])
            comparison_normalized = self._normalize_response(comparison["response"])

            content_similarity = self._calculate_similarity(
                baseline_normalized["content"],
                comparison_normalized["content"],
            )

            baseline_tokens = self._count_tokens_estimate(baseline_normalized["content"])
            comparison_tokens = self._count_tokens_estimate(comparison_normalized["content"])

            differences = []
            if content_similarity < 0.9:
                differences.append({
                    "type": "content_similarity",
                    "value": content_similarity,
                    "threshold": 0.9,
                    "message": f"Content similarity is {content_similarity:.2%}",
                })

            passed = content_similarity >= 0.7 and len(differences) == 0
            verdict = "pass" if passed else ("fail" if differences else "inconclusive")

            return DifferentialResult(
                test_name="roundtrip_o_a_o",
                comparison_type="roundtrip",
                passed=passed,
                verdict=verdict,
                message=f"Roundtrip comparison: {content_similarity:.2%} content similarity",
                duration_ms=duration_ms,
                baseline_response=baseline_normalized,
                comparison_response=comparison_normalized,
                differences=differences,
                similarity_score=content_similarity,
                baseline_tokens=baseline_tokens,
                comparison_tokens=comparison_tokens,
                token_difference=abs(baseline_tokens - comparison_tokens),
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return DifferentialResult(
                test_name="roundtrip_o_a_o",
                comparison_type="roundtrip",
                passed=False,
                verdict="inconclusive",
                message="Exception during roundtrip test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_anthropic_roundtrip(
        self,
        model: str = "tingly-claude",
        prompt: Optional[str] = None,
    ) -> DifferentialResult:
        """Test Anthropic roundtrip transformation."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            responses = []

            self._print("Getting baseline from direct Anthropic API...")
            direct_result = self.proxy_client.messages_anthropic(
                model=model,
                prompt=test_prompt,
            )

            if direct_result.success:
                responses.append({
                    "method": "direct_anthropic",
                    "response": direct_result.raw_response or {},
                    "success": True,
                })

            self._print("Getting comparison from transformed Anthropic->OpenAI...")
            transformed_result = self.proxy_client.messages_anthropic(
                model=model,
                prompt=test_prompt,
            )

            if transformed_result.success:
                responses.append({
                    "method": "transformed_anthropic",
                    "response": transformed_result.raw_response or {},
                    "success": True,
                })

            duration_ms = (time.time() - start_time) * 1000

            if len(responses) < 2:
                return DifferentialResult(
                    test_name="anthropic_roundtrip",
                    comparison_type="roundtrip",
                    passed=False,
                    verdict="inconclusive",
                    message="Insufficient responses for comparison",
                    duration_ms=duration_ms,
                    error="Need at least 2 responses for comparison",
                )

            baseline = responses[0]
            comparison = responses[1]

            baseline_normalized = self._normalize_response(baseline["response"])
            comparison_normalized = self._normalize_response(comparison["response"])

            content_similarity = self._calculate_similarity(
                baseline_normalized["content"],
                comparison_normalized["content"],
            )

            baseline_tokens = self._count_tokens_estimate(baseline_normalized["content"])
            comparison_tokens = self._count_tokens_estimate(comparison_normalized["content"])

            differences = []
            if content_similarity < 0.7:
                differences.append({
                    "type": "content_similarity",
                    "value": content_similarity,
                    "threshold": 0.7,
                    "message": f"Content similarity is {content_similarity:.2%}",
                })

            passed = content_similarity >= 0.7
            verdict = "pass" if passed else "fail"

            return DifferentialResult(
                test_name="anthropic_roundtrip",
                comparison_type="roundtrip",
                passed=passed,
                verdict=verdict,
                message=f"Anthropic roundtrip: {content_similarity:.2%} content similarity",
                duration_ms=duration_ms,
                baseline_response=baseline_normalized,
                comparison_response=comparison_normalized,
                differences=differences,
                similarity_score=content_similarity,
                baseline_tokens=baseline_tokens,
                comparison_tokens=comparison_tokens,
                token_difference=abs(baseline_tokens - comparison_tokens),
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return DifferentialResult(
                test_name="anthropic_roundtrip",
                comparison_type="roundtrip",
                passed=False,
                verdict="inconclusive",
                message="Exception during Anthropic roundtrip test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_multi_provider_consistency(
        self,
        prompt: Optional[str] = None,
        providers: Optional[list[tuple]] = None,
    ) -> DifferentialResult:
        """Test consistency across multiple providers."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            responses = []

            provider_models = providers or [
                ("openai", "tingly-claude"),
                ("anthropic", "tingly-claude"),
            ]

            for api_style, model in provider_models:
                self._print(f"Testing {api_style} with model {model}...")

                if api_style == "openai":
                    result = self.proxy_client.chat_completions_openai(
                        model=model,
                        prompt=test_prompt,
                    )
                else:
                    result = self.proxy_client.messages_anthropic(
                        model=model,
                        prompt=test_prompt,
                    )

                if result.success:
                    responses.append({
                        "provider": api_style,
                        "model": model,
                        "response": result.raw_response or {},
                        "success": True,
                    })

            duration_ms = (time.time() - start_time) * 1000

            if len(responses) < 2:
                return DifferentialResult(
                    test_name="multi_provider_consistency",
                    comparison_type="cross_provider",
                    passed=False,
                    verdict="inconclusive",
                    message="Insufficient responses for multi-provider comparison",
                    duration_ms=duration_ms,
                    error="Need at least 2 provider responses",
                )

            normalized_responses = [
                {
                    "provider": r["provider"],
                    "normalized": self._normalize_response(r["response"]),
                }
                for r in responses
            ]

            similarities = []
            for i, r1 in enumerate(normalized_responses):
                for j, r2 in enumerate(normalized_responses):
                    if i < j:
                        sim = self._calculate_similarity(
                            r1["normalized"]["content"],
                            r2["normalized"]["content"],
                        )
                        similarities.append({
                            "pair": f"{r1['provider']}_{r2['provider']}",
                            "similarity": sim,
                        })

            if similarities:
                similarities.sort(key=lambda x: x["similarity"])
                minority = similarities[0]
            else:
                minority = {"pair": "none", "similarity": 1.0}

            avg_similarity = statistics.mean([s["similarity"] for s in similarities]) if similarities else 1.0

            minority_guilty = minority["similarity"] < (avg_similarity - 0.2)

            differences = []
            if minority_guilty:
                differences.append({
                    "type": "minority_pair",
                    "pair": minority["pair"],
                    "similarity": minority["similarity"],
                    "message": f"Low similarity between {minority['pair']} - needs investigation",
                })

            passed = avg_similarity >= 0.5 and not minority_guilty
            verdict = "pass" if passed else ("fail" if differences else "inconclusive")

            return DifferentialResult(
                test_name="multi_provider_consistency",
                comparison_type="cross_provider",
                passed=passed,
                verdict=verdict,
                message=f"Multi-provider consistency: {avg_similarity:.2%} average similarity",
                duration_ms=duration_ms,
                baseline_response=normalized_responses[0]["normalized"] if normalized_responses else {},
                comparison_response=normalized_responses[-1]["normalized"] if normalized_responses else {},
                differences=differences,
                similarity_score=avg_similarity,
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return DifferentialResult(
                test_name="multi_provider_consistency",
                comparison_type="cross_provider",
                passed=False,
                verdict="inconclusive",
                message="Exception during multi-provider test",
                duration_ms=duration_ms,
                error=str(e),
            )

    def test_response_structure_equivalence(
        self,
        model: str = "tingly-claude",
        prompt: Optional[str] = None,
    ) -> DifferentialResult:
        """Test response structure equivalence."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        try:
            result = self.proxy_client.chat_completions_openai(
                model=model,
                prompt=test_prompt,
            )

            duration_ms = (time.time() - start_time) * 1000

            if not result.success:
                return DifferentialResult(
                    test_name="response_structure_equivalence",
                    comparison_type="structure",
                    passed=False,
                    verdict="inconclusive",
                    message="Failed to get response for structure check",
                    duration_ms=duration_ms,
                    error=result.error,
                )

            response = result.raw_response or {}
            differences = []

            expected_fields = ["id", "object", "created", "model", "choices", "usage"]
            missing_fields = [f for f in expected_fields if f not in response]

            if missing_fields:
                differences.append({
                    "type": "missing_fields",
                    "fields": missing_fields,
                    "message": f"Missing expected fields: {missing_fields}",
                })

            if "choices" in response:
                if len(response["choices"]) == 0:
                    differences.append({
                        "type": "empty_choices",
                        "message": "No choices in response",
                    })
                else:
                    choice = response["choices"][0]
                    expected_choice_fields = ["index", "message", "finish_reason"]
                    missing_choice = [f for f in expected_choice_fields if f not in choice]
                    if missing_choice:
                        differences.append({
                            "type": "choice_missing_fields",
                            "fields": missing_choice,
                            "message": f"Missing choice fields: {missing_choice}",
                        })

            if "usage" in response:
                expected_usage = ["prompt_tokens", "completion_tokens", "total_tokens"]
                missing_usage = [f for f in expected_usage if f not in response["usage"]]
                if missing_usage:
                    differences.append({
                        "type": "usage_missing_fields",
                        "fields": missing_usage,
                        "message": f"Missing usage fields: {missing_usage}",
                    })

            passed = len(differences) == 0
            verdict = "pass" if passed else "fail"

            return DifferentialResult(
                test_name="response_structure_equivalence",
                comparison_type="structure",
                passed=passed,
                verdict=verdict,
                message=f"Structure check: {'PASS' if passed else 'FAIL'} - {len(differences)} issues found",
                duration_ms=duration_ms,
                comparison_response=self._normalize_response(response),
                differences=differences,
                similarity_score=1.0 if passed else 0.0,
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return DifferentialResult(
                test_name="response_structure_equivalence",
                comparison_type="structure",
                passed=False,
                verdict="inconclusive",
                message="Exception during structure check",
                duration_ms=duration_ms,
                error=str(e),
            )

    def run_all_tests(self) -> DifferentialTestSuiteResult:
        """Run all differential tests."""
        suite_result = DifferentialTestSuiteResult(suite_name="Differential Test Suite")
        start_time = time.time()

        self._print("=== Running Differential Tests ===\n")

        tests = [
            ("Roundtrip O->A->O", self.test_roundtrip_openai_anthropic_openai),
            ("Anthropic roundtrip", self.test_anthropic_roundtrip),
            ("Multi-provider consistency", self.test_multi_provider_consistency),
            ("Response structure equivalence", self.test_response_structure_equivalence),
        ]

        for name, test_func in tests:
            self._print(f"Testing {name}...")
            result = test_func()
            suite_result.results.append(result)
            suite_result.total_tests += 1
            if result.passed:
                suite_result.passed += 1
            elif result.verdict == "inconclusive":
                suite_result.inconclusive += 1
            else:
                suite_result.failed += 1
            self._print(f"  Result: {result.verdict.upper()} - {result.message}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result
