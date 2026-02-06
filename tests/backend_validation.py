"""
Backend validation test suite for verifying field existence and format compliance.

Tests qwen/glm backends through OpenAI/Anthropic client interfaces to ensure:
- Required field presence (id, choices, usage, model, etc.)
- Response format compatibility
- Field type correctness
"""

import json
import time
from dataclasses import dataclass, field
from typing import Any, Optional
from datetime import datetime

from .config import TestConfig, APIStyle
from .client import ProxyClient


# Expected field definitions for different response formats
OPENAI_REQUIRED_FIELDS = [
    "id",
    "object",
    "created",
    "model",
    "choices",
]

OPENAI_CHOICE_FIELDS = [
    "index",
    "message",
    "finish_reason",
]

OPENAI_MESSAGE_FIELDS = [
    "role",
    "content",
]

OPENAI_USAGE_FIELDS = [
    "prompt_tokens",
    "completion_tokens",
    "total_tokens",
]

ANTHROPIC_REQUIRED_FIELDS = [
    "id",
    "type",
    "role",
    "content",
    "model",
    "stop_reason",
]

ANTHROPIC_CONTENT_BLOCK_FIELDS = [
    "type",
    "text",
]

ANTHROPIC_USAGE_FIELDS = [
    "input_tokens",
    "output_tokens",
]


@dataclass
class FieldValidationIssue:
    """Represents a field validation issue."""
    field_path: str
    issue_type: str  # "missing", "wrong_type", "invalid_value", "empty"
    expected: str
    actual: Optional[str] = None
    severity: str = "error"  # "error", "warning"


@dataclass
class BackendValidationResult:
    """Result of a backend validation test."""
    backend_provider: str
    client_style: str
    test_type: str
    model: Optional[str] = None
    passed: bool = False
    message: str = ""
    duration_ms: float = 0.0
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    missing_fields: list[str] = field(default_factory=list)
    invalid_fields: dict = field(default_factory=dict)
    field_issues: list[FieldValidationIssue] = field(default_factory=list)
    raw_response: dict = field(default_factory=dict)

    detail: str = ""
    error: Optional[str] = None


@dataclass
class BackendValidationSuiteResult:
    """Aggregate result of all backend validation tests."""
    suite_name: str
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    results: list[BackendValidationResult] = field(default_factory=list)
    duration_ms: float = 0.0
    timestamp: str = field(default_factory=lambda: datetime.now().isoformat())

    @property
    def success_rate(self) -> float:
        if self.total_tests == 0:
            return 0.0
        return (self.passed / self.total_tests) * 100


class BackendValidationTestSuite:
    """Test suite for backend field validation."""

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
            print(f"  [BACKEND] {msg}")

    def _build_backend_cases(self) -> list[dict]:
        cases = []
        for rule in self.config.rules:
            if not rule.active or not rule.request_model:
                continue
            service = None
            for svc in rule.services or []:
                if svc.get("provider") and svc.get("model"):
                    service = svc
                    break
            if not service:
                continue
            provider = self.config.get_provider_by_uuid(service.get("provider"))
            if not provider:
                continue
            if provider.api_style not in (APIStyle.OPENAI, APIStyle.ANTHROPIC):
                continue

            test_format = "openai" if provider.api_style == APIStyle.OPENAI else "anthropic"
            cases.append({
                "backend": provider.name,
                "scenario": rule.scenario,
                "request_model": rule.request_model,
                "provider_model": service.get("model", rule.request_model),
                "test_format": test_format,
            })

        return cases

    def _validate_field_type(
        self,
        value: Any,
        expected_type: str,
        field_path: str,
    ) -> list[FieldValidationIssue]:
        """Validate field type and return list of issues."""
        issues = []

        if value is None:
            issues.append(FieldValidationIssue(
                field_path=field_path,
                issue_type="invalid_value",
                expected=f"non-null {expected_type}",
                actual="null",
                severity="error",
            ))
            return issues

        type_checks = {
            "str": lambda v: isinstance(v, str),
            "int": lambda v: isinstance(v, int),
            "float": lambda v: isinstance(v, (int, float)),
            "bool": lambda v: isinstance(v, bool),
            "list": lambda v: isinstance(v, list),
            "dict": lambda v: isinstance(v, dict),
            "str|list": lambda v: isinstance(v, (str, list)),
        }

        if expected_type not in type_checks:
            return issues

        if not type_checks[expected_type](value):
            actual_type = type(value).__name__
            issues.append(FieldValidationIssue(
                field_path=field_path,
                issue_type="wrong_type",
                expected=expected_type,
                actual=actual_type,
                severity="error",
            ))

        if expected_type in ("str", "list", "dict") and not value:
            issues.append(FieldValidationIssue(
                field_path=field_path,
                issue_type="empty",
                expected=f"non-empty {expected_type}",
                actual=f"empty {expected_type}",
                severity="warning",
            ))

        return issues

    def _validate_openai_response(
        self,
        response: dict,
        model: str,
    ) -> list[FieldValidationIssue]:
        """Validate OpenAI-format response fields."""
        issues = []

        # Check required top-level fields
        for field in OPENAI_REQUIRED_FIELDS:
            if field not in response:
                issues.append(FieldValidationIssue(
                    field_path=field,
                    issue_type="missing",
                    expected=f"field '{field}' to be present",
                    severity="error",
                ))
                continue

            # Type validation
            if field == "id":
                issues.extend(self._validate_field_type(response[field], "str", f"{field}"))
            elif field == "object":
                issues.extend(self._validate_field_type(response[field], "str", f"{field}"))
            elif field == "created":
                issues.extend(self._validate_field_type(response[field], "int", f"{field}"))
            elif field == "model":
                issues.extend(self._validate_field_type(response[field], "str", f"{field}"))
            elif field == "choices":
                issues.extend(self._validate_field_type(response[field], "list", f"{field}"))

        # Validate choices
        if "choices" in response and response["choices"]:
            choice = response["choices"][0]
            for field in OPENAI_CHOICE_FIELDS:
                if field not in choice:
                    issues.append(FieldValidationIssue(
                        field_path=f"choices[0].{field}",
                        issue_type="missing",
                        expected=f"field '{field}' to be present",
                        severity="error",
                    ))
                    continue

                if field == "index":
                    issues.extend(self._validate_field_type(choice[field], "int", f"choices[0].{field}"))
                elif field == "finish_reason":
                    issues.extend(self._validate_field_type(choice[field], "str", f"choices[0].{field}"))
                elif field == "message":
                    issues.extend(self._validate_field_type(choice[field], "dict", f"choices[0].{field}"))

                    # Validate message fields
                    msg = choice["message"]
                    for msg_field in OPENAI_MESSAGE_FIELDS:
                        if msg_field not in msg:
                            issues.append(FieldValidationIssue(
                                field_path=f"choices[0].message.{msg_field}",
                                issue_type="missing",
                                expected=f"field '{msg_field}' to be present",
                                severity="error",
                            ))
                            continue
                        issues.extend(self._validate_field_type(
                            msg[msg_field], "str", f"choices[0].message.{msg_field}"
                        ))

        # Validate usage if present
        if "usage" in response:
            usage = response["usage"]
            for field in OPENAI_USAGE_FIELDS:
                if field not in usage:
                    issues.append(FieldValidationIssue(
                        field_path=f"usage.{field}",
                        issue_type="missing",
                        expected=f"field '{field}' to be present",
                        severity="warning",
                    ))
                    continue
                issues.extend(self._validate_field_type(usage[field], "int", f"usage.{field}"))

        return issues

    def _validate_anthropic_response(
        self,
        response: dict,
        model: str,
    ) -> list[FieldValidationIssue]:
        """Validate Anthropic-format response fields."""
        issues = []

        # Check required top-level fields
        for field in ANTHROPIC_REQUIRED_FIELDS:
            if field not in response:
                issues.append(FieldValidationIssue(
                    field_path=field,
                    issue_type="missing",
                    expected=f"field '{field}' to be present",
                    severity="error",
                ))
                continue

            # Type validation
            if field in ("id", "type", "role", "model", "stop_reason"):
                issues.extend(self._validate_field_type(response[field], "str", f"{field}"))
            elif field == "content":
                issues.extend(self._validate_field_type(response[field], "list", f"{field}"))

                # Validate content blocks
                if response["content"]:
                    content_block = response["content"][0]
                    for cb_field in ANTHROPIC_CONTENT_BLOCK_FIELDS:
                        if cb_field not in content_block:
                            issues.append(FieldValidationIssue(
                                field_path=f"content[0].{cb_field}",
                                issue_type="missing",
                                expected=f"field '{cb_field}' to be present",
                                severity="error",
                            ))
                            continue
                        if cb_field == "type":
                            issues.extend(self._validate_field_type(
                                content_block[cb_field], "str", f"content[0].{cb_field}"
                            ))
                        elif cb_field == "text":
                            issues.extend(self._validate_field_type(
                                content_block[cb_field], "str", f"content[0].{cb_field}"
                            ))

        # Validate usage if present
        if "usage" in response:
            usage = response["usage"]
            for field in ANTHROPIC_USAGE_FIELDS:
                if field not in usage:
                    issues.append(FieldValidationIssue(
                        field_path=f"usage.{field}",
                        issue_type="missing",
                        expected=f"field '{field}' to be present",
                        severity="warning",
                    ))
                    continue
                issues.extend(self._validate_field_type(usage[field], "int", f"usage.{field}"))

        return issues

    def test_backend_openai_format(
        self,
        backend: str,
        scenario: str,
        request_model: str,
        prompt: Optional[str] = None,
    ) -> BackendValidationResult:
        """Test backend response through OpenAI client format."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        self._print(f"Testing {backend} backend via OpenAI format with scenario {scenario}, request_model {request_model}")

        try:
            endpoint = f"/tingly/{scenario}/chat/completions"
            result = self.proxy_client.chat_completions_openai(
                model=request_model,
                prompt=test_prompt,
                scenario=scenario,  # Scenario for URL routing
                temperature=0.7,
                max_tokens=100,
            )

            duration_ms = (time.time() - start_time) * 1000

            if not result.success:
                return BackendValidationResult(
                    backend_provider=backend,
                    client_style="openai",
                    test_type="openai_format_validation",
                    model=request_model,
                    passed=False,
                    message="Request failed",
                    duration_ms=duration_ms,
                    detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
                    error=result.error,
                )

            response = result.raw_response or {}
            issues = self._validate_openai_response(response, request_model)

            missing_fields = [i.field_path for i in issues if i.issue_type == "missing"]
            invalid_fields = {i.field_path: i.expected for i in issues if i.issue_type == "wrong_type"}

            passed = len([i for i in issues if i.severity == "error"]) == 0
            message = f"OpenAI format validation: {len(issues)} issues found"

            return BackendValidationResult(
                backend_provider=backend,
                client_style="openai",
                test_type="openai_format_validation",
                model=request_model,
                passed=passed,
                message=message,
                duration_ms=duration_ms,
                missing_fields=missing_fields,
                invalid_fields=invalid_fields,
                field_issues=issues,
                raw_response=response,
                detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return BackendValidationResult(
                backend_provider=backend,
                client_style="openai",
                test_type="openai_format_validation",
                model=request_model,
                passed=False,
                message="Exception during validation",
                duration_ms=duration_ms,
                detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
                error=str(e),
            )

    def test_backend_anthropic_format(
        self,
        backend: str,
        scenario: str,
        request_model: str,
        prompt: Optional[str] = None,
    ) -> BackendValidationResult:
        """Test backend response through Anthropic client format."""
        test_prompt = prompt or self.config.test_prompt
        start_time = time.time()

        self._print(f"Testing {backend} backend via Anthropic format with scenario {scenario}, request_model {request_model}")

        try:
            endpoint = f"/tingly/{scenario}/messages"
            result = self.proxy_client.messages_anthropic(
                model=request_model,
                prompt=test_prompt,
                scenario=scenario,  # Scenario for URL routing
                temperature=0.7,
                max_tokens=100,
            )

            duration_ms = (time.time() - start_time) * 1000

            if not result.success:
                return BackendValidationResult(
                    backend_provider=backend,
                    client_style="anthropic",
                    test_type="anthropic_format_validation",
                    model=request_model,
                    passed=False,
                    message="Request failed",
                    duration_ms=duration_ms,
                    detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
                    error=result.error,
                )

            response = result.raw_response or {}
            issues = self._validate_anthropic_response(response, request_model)

            missing_fields = [i.field_path for i in issues if i.issue_type == "missing"]
            invalid_fields = {i.field_path: i.expected for i in issues if i.issue_type == "wrong_type"}

            passed = len([i for i in issues if i.severity == "error"]) == 0
            message = f"Anthropic format validation: {len(issues)} issues found"

            return BackendValidationResult(
                backend_provider=backend,
                client_style="anthropic",
                test_type="anthropic_format_validation",
                model=request_model,
                passed=passed,
                message=message,
                duration_ms=duration_ms,
                missing_fields=missing_fields,
                invalid_fields=invalid_fields,
                field_issues=issues,
                raw_response=response,
                detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
            )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return BackendValidationResult(
                backend_provider=backend,
                client_style="anthropic",
                test_type="anthropic_format_validation",
                model=request_model,
                passed=False,
                message="Exception during validation",
                duration_ms=duration_ms,
                detail=f"scenario={scenario} endpoint={endpoint} model={request_model}",
                error=str(e),
            )

    def test_all_backends(
        self,
    ) -> BackendValidationSuiteResult:
        """Test all specified backends with their appropriate client formats."""
        suite_result = BackendValidationSuiteResult(suite_name="Backend Validation Test Suite")
        cases = self._build_backend_cases()

        self._print("=== Running Backend Validation Tests ===\n")

        start_time = time.time()

        for case in cases:
            backend = case["backend"]
            scenario = case["scenario"]
            request_model = case["request_model"]
            test_format = case["test_format"]

            self._print(f"\n--- Testing {backend} backend ---")
            self._print(f"  {test_format.upper()} format...")
            if test_format == "openai":
                result = self.test_backend_openai_format(backend, scenario, request_model)
            else:
                result = self.test_backend_anthropic_format(backend, scenario, request_model)

            suite_result.results.append(result)
            suite_result.total_tests += 1
            if result.passed:
                suite_result.passed += 1
            else:
                suite_result.failed += 1
            self._print(f"    Result: {'PASS' if result.passed else 'FAIL'} - {result.message}")

        suite_result.duration_ms = (time.time() - start_time) * 1000

        return suite_result

    def run_all_tests(self) -> BackendValidationSuiteResult:
        """Run all backend validation tests."""
        return self.test_all_backends()
