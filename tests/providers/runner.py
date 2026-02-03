#!/usr/bin/env python3
"""
Test runner for tingly-box provider tests.
"""

import argparse
import json
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Optional

from .config import load_config, TestConfig
from .smoke import SmokeTestSuite, ProxySmokeTestSuite
from .adaptor import AdaptorTestSuite
from .differential import DifferentialTestSuite


@dataclass
class TestRunResult:
    """Result of a complete test run."""
    run_id: str
    timestamp: str
    config_source: str
    suite_name: str
    total_tests: int
    passed: int
    failed: int
    skipped: int
    duration_ms: float
    success_rate: float
    results: list
    errors: list = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "run_id": self.run_id,
            "timestamp": self.timestamp,
            "config_source": self.config_source,
            "suite_name": self.suite_name,
            "total_tests": self.total_tests,
            "passed": self.passed,
            "failed": self.failed,
            "skipped": self.skipped,
            "duration_ms": self.duration_ms,
            "success_rate": self.success_rate,
            "results": self.results,
            "errors": self.errors,
        }


class TestRunner:
    """Main test runner for all test suites."""

    def __init__(
        self,
        config_path: Optional[str] = None,
        verbose: bool = False,
        output_dir: str = "./test_results",
    ):
        self.config_path = config_path
        self.verbose = verbose
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.config = load_config(config_path)

    def _print(self, msg: str):
        if self.verbose:
            print(msg)

    def _get_run_id(self) -> str:
        return datetime.now().strftime("%Y%m%d_%H%M%S")

    def run_smoke_tests(self) -> TestRunResult:
        """Run smoke tests for all providers."""
        self._print("\n=== Running Smoke Tests ===\n")

        suite = SmokeTestSuite(self.config, self.verbose)
        results = suite.run_all_tests()

        return TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="Smoke Tests",
            total_tests=results.total_tests,
            passed=results.passed,
            failed=results.failed,
            skipped=results.skipped,
            duration_ms=results.duration_ms,
            success_rate=results.success_rate,
            results=[r.__dict__ for r in results.results],
        )

    def run_proxy_smoke_tests(self) -> TestRunResult:
        """Run smoke tests for proxy endpoints."""
        self._print("\n=== Running Proxy Smoke Tests ===\n")

        suite = ProxySmokeTestSuite(self.config, self.verbose)
        results = suite.run_all_tests()

        return TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="Proxy Smoke Tests",
            total_tests=results.total_tests,
            passed=results.passed,
            failed=results.failed,
            skipped=results.skipped,
            duration_ms=results.duration_ms,
            success_rate=results.success_rate,
            results=[r.__dict__ for r in results.results],
        )

    def run_adaptor_tests(self) -> TestRunResult:
        """Run adaptor transformation tests."""
        self._print("\n=== Running Adaptor Tests ===\n")

        suite = AdaptorTestSuite(self.config, self.verbose)
        results = suite.run_all_tests()

        return TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="Adaptor Tests",
            total_tests=results.total_tests,
            passed=results.passed,
            failed=results.failed,
            skipped=0,
            duration_ms=results.duration_ms,
            success_rate=results.success_rate,
            results=[r.__dict__ for r in results.results],
        )

    def run_differential_tests(self) -> TestRunResult:
        """Run differential transformation tests."""
        self._print("\n=== Running Differential Tests ===\n")

        suite = DifferentialTestSuite(self.config, self.verbose)
        results = suite.run_all_tests()

        return TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="Differential Tests",
            total_tests=results.total_tests,
            passed=results.passed,
            failed=results.failed,
            skipped=results.inconclusive,
            duration_ms=results.duration_ms,
            success_rate=results.success_rate,
            results=[r.__dict__ for r in results.results],
        )

    def run_all_tests(self) -> TestRunResult:
        """Run all test suites."""
        self._print("=" * 60)
        self._print("TINGLY-BOX TEST SYSTEM")
        self._print("=" * 60)

        all_results = []
        total_tests = 0
        total_passed = 0
        total_failed = 0
        total_skipped = 0
        total_duration = 0.0

        start_time = time.time()

        test_suites = [
            ("Smoke Tests", self.run_smoke_tests),
            ("Proxy Smoke Tests", self.run_proxy_smoke_tests),
            ("Adaptor Tests", self.run_adaptor_tests),
            ("Differential Tests", self.run_differential_tests),
        ]

        for name, run_func in test_suites:
            try:
                result = run_func()
                all_results.append((name, result))
                total_tests += result.total_tests
                total_passed += result.passed
                total_failed += result.failed
                total_skipped += result.skipped
                total_duration += result.duration_ms
            except Exception as e:
                self._print(f"{name} failed: {e}")
                all_results.append((name, TestRunResult(
                    run_id=self._get_run_id(),
                    timestamp=datetime.now().isoformat(),
                    config_source=self.config_path or "default",
                    suite_name=name,
                    total_tests=0,
                    passed=0,
                    failed=0,
                    skipped=0,
                    duration_ms=0,
                    success_rate=0,
                    results=[],
                    errors=[str(e)],
                )))

        total_duration = (time.time() - start_time) * 1000
        success_rate = (total_passed / total_tests * 100) if total_tests > 0 else 0

        aggregate_result = TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="All Tests",
            total_tests=total_tests,
            passed=total_passed,
            failed=total_failed,
            skipped=total_skipped,
            duration_ms=total_duration,
            success_rate=success_rate,
            results=all_results,
        )

        return aggregate_result

    def save_results(self, result: TestRunResult, filename: Optional[str] = None) -> str:
        """Save test results to JSON file."""
        if filename is None:
            filename = f"test_results_{result.run_id}.json"

        filepath = self.output_dir / filename
        with open(filepath, "w", encoding="utf-8") as f:
            json.dump(result.to_dict(), f, indent=2, default=str)

        return str(filepath)

    def print_summary(self, result: TestRunResult):
        """Print test summary."""
        print("\n" + "=" * 60)
        print("TEST SUMMARY")
        print("=" * 60)
        print(f"Suite: {result.suite_name}")
        print(f"Run ID: {result.run_id}")
        print(f"Timestamp: {result.timestamp}")
        print(f"Duration: {result.duration_ms:.2f}ms")
        print("-" * 40)
        print(f"Total Tests: {result.total_tests}")
        print(f"  Passed: {result.passed} ✅")
        print(f"  Failed: {result.failed} ❌")
        if result.skipped > 0:
            print(f"  Skipped: {result.skipped} ⏭️")
        print(f"Success Rate: {result.success_rate:.1f}%")
        print("=" * 60)

        if result.errors:
            print("\nErrors:")
            for error in result.errors:
                print(f"  - {error}")


def main():
    """Main entry point for test runner."""
    parser = argparse.ArgumentParser(
        description="Tingly-Box Provider Test System",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python -m tests.providers.runner --all --save
  python -m tests.providers.runner --smoke -v
  python -m tests.providers.runner --adaptor --config ~/.tingly-box/config.json
  python -m tests.providers.runner --differential --config ~/.tingly-box/config.json
        """,
    )

    parser.add_argument("--all", "-a", action="store_true", help="Run all test suites")
    parser.add_argument("--smoke", "-s", action="store_true", help="Run smoke tests")
    parser.add_argument("--proxy-smoke", "-p", action="store_true", help="Run proxy smoke tests")
    parser.add_argument("--adaptor", "-d", action="store_true", help="Run adaptor transformation tests")
    parser.add_argument("--differential", "-f", action="store_true", help="Run differential transformation tests")
    parser.add_argument("--config", "-c", type=str, default=None, help="Path to config file")
    parser.add_argument("--output", "-o", type=str, default="./test_results", help="Output directory for test results")
    parser.add_argument("--verbose", "-v", action="store_true", help="Enable verbose output")
    parser.add_argument("--save", "-S", action="store_true", help="Save results to JSON file")

    args = parser.parse_args()

    if not any([args.all, args.smoke, args.proxy_smoke, args.adaptor, args.differential]):
        args.all = True

    runner = TestRunner(
        config_path=args.config,
        verbose=args.verbose,
        output_dir=args.output,
    )

    if args.all:
        result = runner.run_all_tests()
    elif args.smoke:
        result = runner.run_smoke_tests()
    elif args.proxy_smoke:
        result = runner.run_proxy_smoke_tests()
    elif args.adaptor:
        result = runner.run_adaptor_tests()
    elif args.differential:
        result = runner.run_differential_tests()
    else:
        result = runner.run_all_tests()

    runner.print_summary(result)

    if args.save:
        filepath = runner.save_results(result)
        print(f"\nResults saved to: {filepath}")

    sys.exit(1 if result.failed > 0 else 0)


if __name__ == "__main__":
    main()
