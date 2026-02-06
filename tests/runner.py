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
from .backend_validation import BackendValidationTestSuite


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
        server_url: Optional[str] = None,
    ):
        self.config_path = config_path
        self.verbose = verbose
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.config = load_config(config_path)

        # Override server URL if provided
        if server_url:
            self.config.server_url = server_url

    def _print(self, msg: str):
        if self.verbose:
            print(msg)

    def _get_run_id(self) -> str:
        return datetime.now().strftime("%Y%m%d_%H%M%S")

    def run_smoke_tests(self) -> TestRunResult:
        """Run smoke tests for specified providers."""
        self._print("\n=== Running Smoke Tests ===\n")

        suite = SmokeTestSuite(self.config, self.verbose)

        # Filter to only test the true backends: qwen, glm, minimax
        target_backends = {"glm", "qwen", "minimax"}
        filtered_providers = [
            p for p in self.config.providers
            if p.name.lower() in target_backends
        ]

        if filtered_providers:
            self._print(f"Testing {len(filtered_providers)} backends: {', '.join(p.name for p in filtered_providers)}")
            results = suite.run_all_tests(providers=filtered_providers)
        else:
            self._print("Warning: No target backends (glm, deepseek, qwen) found in configuration")
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

    def run_backend_validation_tests(self) -> TestRunResult:
        """Run backend validation tests."""
        self._print("\n=== Running Backend Validation Tests ===\n")

        suite = BackendValidationTestSuite(self.config, self.verbose)
        results = suite.run_all_tests()

        return TestRunResult(
            run_id=self._get_run_id(),
            timestamp=datetime.now().isoformat(),
            config_source=self.config_path or "default",
            suite_name="Backend Validation Tests",
            total_tests=results.total_tests,
            passed=results.passed,
            failed=results.failed,
            skipped=0,
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
            ("Backend Validation Tests", self.run_backend_validation_tests),
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

    def save_html_report(self, result: TestRunResult, filename: Optional[str] = None) -> str:
        """Save test results as HTML report."""
        if filename is None:
            filename = f"test_results_{result.run_id}.html"

        filepath = self.output_dir / filename

        # Calculate percentages
        total = result.total_tests or 1
        passed_percent = (result.passed / total) * 100
        failed_percent = (result.failed / total) * 100
        skipped_percent = (result.skipped / total) * 100

        # Generate test results HTML
        def _render_test_items(items: list) -> str:
            html = ""
            for r in items:
                if not isinstance(r, dict):
                    continue
                status = "passed" if r.get("passed", False) else ("skipped" if r.get("verdict") == "inconclusive" else "failed")

                test_name_parts = []
                if r.get("backend_provider"):
                    backend = r.get("backend_provider", "")
                    style = r.get("client_style", "")
                    model = r.get("model", "")
                    test_name_parts.append(f"Testing {backend} backend")
                    test_name_parts.append(f"via {style.upper()} format")
                    if model:
                        test_name_parts.append(f"with model {model}")
                else:
                    test_name_parts.append(r.get('test_name', r.get('test_type', r.get('provider_name', 'Unknown'))))

                test_name = " ".join(test_name_parts)
                message = r.get('message', '')
                duration = r.get('duration_ms', 0)
                error = r.get('error', '')
                timestamp = r.get('timestamp', '')

                html += f"""
                <div class="test-item {status}">
                    <div class="test-item-header">
                        <div class="test-item-name">{test_name}</div>
                        <div class="test-item-status status-{status}">{status.upper()}</div>
                    </div>
                    <div class="test-item-message">{message}</div>
                    <div class="test-item-details">Duration: {duration:.2f}ms"""

                if timestamp:
                    try:
                        from datetime import datetime
                        dt = datetime.fromisoformat(timestamp)
                        formatted_time = dt.strftime("%Y-%m-%d %H:%M:%S")
                        html += f"""
                    <div class="test-item-timestamp">Timestamp: {formatted_time}</div>"""
                    except:
                        pass

                if r.get("http_method") or r.get("http_url") or r.get("http_status"):
                    html += """
                    <div class="test-item-http">"""
                    if r.get("http_method"):
                        html += f"<span class=\"http-method\">{r['http_method']}</span> "
                    if r.get("http_url"):
                        html += f"<span class=\"http-url\">{r['http_url']}</span>"
                    if r.get("http_status"):
                        status_code = r['http_status']
                        if 200 <= status_code < 300:
                            status_color = "#10b981"
                        elif 400 <= status_code < 500:
                            status_color = "#f59e0b"
                        elif 500 <= status_code < 600:
                            status_color = "#ef4444"
                        else:
                            status_color = "#6b7280"
                        html += f" <span class=\"http-status\" style=\"color: {status_color}\">\"{status_code}\"</span>"
                    html += """
                    </div>"""

                html += "</div>"

                if error:
                    html += f"""
                    <div class="error-box">{error}</div>"""

                if r.get("field_issues"):
                    def _issue_value(issue_obj, key, default=""):
                        if isinstance(issue_obj, dict):
                            return issue_obj.get(key, default)
                        return getattr(issue_obj, key, default)
                    html += '<div class="field-issues">'
                    for issue in r.get("field_issues", []):
                        severity_class = 'error' if _issue_value(issue, 'severity') == 'error' else ''
                        field_path = _issue_value(issue, 'field_path')
                        issue_type = _issue_value(issue, 'issue_type')
                        expected = _issue_value(issue, 'expected')
                        actual = _issue_value(issue, 'actual')

                        html += f"""
                        <div class="field-issue {severity_class}">
                            <span class="field-issue-path">{field_path}</span>
                            <span class="field-issue-detail"> - {issue_type}: expected {expected}"""
                        if actual:
                            html += f", got {actual}"
                        html += "</div>"
                    html += '</div>'

                if r.get("backend_provider"):
                    html += '<div class="test-context">'
                    html += f"<strong>Provider:</strong> {r.get('backend_provider')} | "
                    html += f"<strong>Style:</strong> {r.get('client_style', '').upper()}"
                    if r.get("missing_fields"):
                        html += f"<br><strong>Missing Fields:</strong> {', '.join(r.get('missing_fields', []))}"
                    if r.get("invalid_fields"):
                        invalid = r.get("invalid_fields", {})
                        if invalid:
                            html += f"<br><strong>Invalid Fields:</strong> {', '.join(invalid.keys())}"
                    html += '</div>'

                html += "</div>"
            return html

        test_results_html = ""

        if result.suite_name == "All Tests" and result.results:
            for suite_name, suite_result in result.results:
                suite_status = "passed"
                if suite_result.failed > 0:
                    suite_status = "failed"
                elif suite_result.skipped > 0:
                    suite_status = "skipped"

                suite_items_html = _render_test_items(suite_result.results or [])
                test_results_html += f"""
                <details class="suite">
                    <summary class="suite-summary {suite_status}">
                        <span class="suite-name">{suite_name}</span>
                        <span class="suite-meta">Passed: {suite_result.passed} | Failed: {suite_result.failed} | Skipped: {suite_result.skipped} | {suite_result.success_rate:.1f}%</span>
                    </summary>
                    <div class="suite-body">
                        <div class="test-item-details">Duration: {suite_result.duration_ms:.2f}ms</div>
                        {suite_items_html}
                    </div>
                </details>"""
        else:
            test_results_html = _render_test_items(result.results)

        # Build HTML document
        html_content = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Tingly-Box Test Report</title>
    <style>
        * {{
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Oxygen, Ubuntu, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            line-height: 1.6;
        }}
        .container {{
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            overflow: hidden;
        }}
        .header {{
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            text-align: center;
        }}
        .header h1 {{
            font-size: 2.5em;
            margin-bottom: 10px;
        }}
        .header .subtitle {{
            font-size: 1.1em;
            opacity: 0.9;
        }}
        .summary {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            padding: 30px;
            background: #f8f9fa;
        }}
        .summary-card {{
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            text-align: center;
        }}
        .summary-card .label {{
            font-size: 0.9em;
            color: #666;
            margin-bottom: 10px;
            text-transform: uppercase;
            letter-spacing: 1px;
        }}
        .summary-card .value {{
            font-size: 2.5em;
            font-weight: bold;
        }}
        .summary-card .value.passed {{ color: #10b981; }}
        .summary-card .value.failed {{ color: #ef4444; }}
        .summary-card .value.skipped {{ color: #f59e0b; }}
        .summary-card .value.rate {{ color: #667eea; }}
        .progress-bar {{
            padding: 30px;
        }}
        .progress-track {{
            background: #e5e7eb;
            height: 30px;
            border-radius: 15px;
            overflow: hidden;
            display: flex;
        }}
        .progress-segment {{
            height: 100%;
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: bold;
            font-size: 0.9em;
            transition: width 0.3s ease;
        }}
        .progress-passed {{ background: #10b981; }}
        .progress-failed {{ background: #ef4444; }}
        .progress-skipped {{ background: #f59e0b; }}
        .test-sections {{
            padding: 30px;
        }}
        .section-title {{
            font-size: 1.5em;
            margin-bottom: 20px;
            color: #1f2937;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }}
        .test-item {{
            background: #f9fafb;
            border-left: 4px solid #d1d5db;
            padding: 15px;
            margin-bottom: 15px;
            border-radius: 0 8px 8px 0;
        }}
        .test-item.passed {{
            border-left-color: #10b981;
            background: #f0fdf4;
        }}
        .test-item.failed {{
            border-left-color: #ef4444;
            background: #fef2f2;
        }}
        .test-item.skipped {{
            border-left-color: #f59e0b;
            background: #fffbeb;
        }}
        .test-item-header {{
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }}
        .test-item-name {{
            font-weight: bold;
            color: #1f2937;
        }}
        .test-item-status {{
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: bold;
            text-transform: uppercase;
        }}
        .status-passed {{ background: #10b981; color: white; }}
        .status-failed {{ background: #ef4444; color: white; }}
        .status-skipped {{ background: #f59e0b; color: white; }}
        .test-item-message {{
            color: #6b7280;
            font-size: 0.95em;
            margin-bottom: 8px;
        }}
        .test-item-details {{
            font-size: 0.85em;
            color: #9ca3af;
        }}
        .error-box {{
            background: #fef2f2;
            border: 1px solid #fecaca;
            border-radius: 6px;
            padding: 12px;
            margin-top: 10px;
            font-family: monospace;
            font-size: 0.85em;
            color: #991b1b;
            overflow-x: auto;
        }}
        .footer {{
            background: #1f2937;
            color: white;
            padding: 20px;
            text-align: center;
            font-size: 0.9em;
        }}
        .footer a {{
            color: #667eea;
            text-decoration: none;
        }}
        .field-issues {{
            margin-top: 10px;
        }}
        .field-issue {{
            background: #fffbeb;
            border-left: 3px solid #f59e0b;
            padding: 8px 12px;
            margin-bottom: 6px;
            border-radius: 0 4px 4px 0;
            font-size: 0.85em;
        }}
        .field-issue.error {{
            background: #fef2f2;
            border-left-color: #ef4444;
        }}
        .field-issue-path {{
            font-weight: bold;
            color: #1f2937;
        }}
        .field-issue-detail {{
            color: #6b7280;
        }}
        .test-item-timestamp {{
            font-size: 0.8em;
            color: #9ca3af;
            margin-top: 4px;
        }}
        .test-item-http {{
            font-size: 0.8em;
            color: #6b7280;
            margin-top: 4px;
            padding: 8px;
            background: #f3f4f6;
            border-radius: 4px;
            font-family: monospace;
        }}
        .http-method {{
            color: #667eea;
            font-weight: bold;
        }}
        .http-url {{
            color: #059669;
            word-break: break-all;
        }}
        .http-status {{
            font-weight: bold;
        }}
        .test-context {{
            margin-top: 10px;
            padding: 10px;
            background: #f9fafb;
            border-left: 3px solid #d1d5db;
            border-radius: 4px;
            font-size: 0.85em;
        }}
        .test-context strong {{
            color: #374151;
        }}
        details.suite {{
            border: 1px solid #e5e7eb;
            border-radius: 8px;
            margin-bottom: 16px;
            background: #fff;
        }}
        .suite-summary {{
            cursor: pointer;
            padding: 12px 16px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-weight: bold;
            color: #1f2937;
            list-style: none;
        }}
        .suite-summary::-webkit-details-marker {{
            display: none;
        }}
        .suite-summary::before {{
            content: "▸";
            margin-right: 8px;
            color: #6b7280;
        }}
        details[open] > .suite-summary::before {{
            content: "▾";
        }}
        .suite-summary.passed {{
            background: #f0fdf4;
            border-bottom: 1px solid #e5e7eb;
        }}
        .suite-summary.failed {{
            background: #fef2f2;
            border-bottom: 1px solid #e5e7eb;
        }}
        .suite-summary.skipped {{
            background: #fffbeb;
            border-bottom: 1px solid #e5e7eb;
        }}
        .suite-name {{
            font-size: 1.05em;
        }}
        .suite-meta {{
            font-weight: normal;
            color: #6b7280;
            font-size: 0.9em;
        }}
        .suite-body {{
            padding: 16px;
        }}
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🧪 Tingly-Box Test Report</h1>
            <div class="subtitle">{result.suite_name}</div>
        </div>

        <div class="summary">
            <div class="summary-card">
                <div class="label">Total Tests</div>
                <div class="value">{result.total_tests}</div>
            </div>
            <div class="summary-card">
                <div class="label">Passed</div>
                <div class="value passed">{result.passed}</div>
            </div>
            <div class="summary-card">
                <div class="label">Failed</div>
                <div class="value failed">{result.failed}</div>
            </div>
            <div class="summary-card">
                <div class="label">Skipped</div>
                <div class="value skipped">{result.skipped}</div>
            </div>
            <div class="summary-card">
                <div class="label">Success Rate</div>
                <div class="value rate">{result.success_rate:.1f}%</div>
            </div>
        </div>

        <div class="progress-bar">
            <div class="progress-track">
                <div class="progress-segment progress-passed" style="width: {passed_percent:.1f}%">
                    {result.passed}
                </div>
                <div class="progress-segment progress-failed" style="width: {failed_percent:.1f}%">
                    {result.failed}
                </div>
                <div class="progress-segment progress-skipped" style="width: {skipped_percent:.1f}%">
                    {result.skipped}
                </div>
            </div>
        </div>

        <div class="test-sections">
            <h2 class="section-title">Test Results</h2>
            {test_results_html}
        </div>

        <div class="footer">
            <p>Generated by Tingly-Box Test System | {result.timestamp} | Duration: {result.duration_ms:.2f}ms</p>
        </div>
    </div>
</body>
</html>"""

        with open(filepath, "w", encoding="utf-8") as f:
            f.write(html_content)

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
  python -m tests.runner --all --save
  python -m tests.runner --smoke -v
  python -m tests.runner --adaptor --config ~/.tingly-box/config.json
  python -m tests.runner --differential --config ~/.tingly-box/config.json
  python -m tests.runner --backend --config ~/.tingly-box/config.json
  python -m tests.runner --all --html --save
        """,
    )

    parser.add_argument("--all", "-a", action="store_true", help="Run all test suites")
    parser.add_argument("--smoke", "-s", action="store_true", help="Run smoke tests")
    parser.add_argument("--proxy-smoke", "-p", action="store_true", help="Run proxy smoke tests")
    parser.add_argument("--adaptor", "-d", action="store_true", help="Run adaptor transformation tests")
    parser.add_argument("--differential", "-f", action="store_true", help="Run differential transformation tests")
    parser.add_argument("--backend", "-b", action="store_true", help="Run backend validation tests")
    parser.add_argument("--config", "-c", type=str, default=None, help="Path to config file")
    parser.add_argument("--server-url", type=str, default=None, help="Server URL to test against (overrides config)")
    parser.add_argument("--output", "-o", type=str, default="./test_results", help="Output directory for test results")
    parser.add_argument("--html", "-H", action="store_true", help="Generate HTML report")
    parser.add_argument("--verbose", "-v", action="store_true", help="Enable verbose output")
    parser.add_argument("--save", "-S", action="store_true", help="Save results to JSON file")

    args = parser.parse_args()

    if not any([args.all, args.smoke, args.proxy_smoke, args.adaptor, args.differential, args.backend]):
        args.all = True

    runner = TestRunner(
        config_path=args.config,
        verbose=args.verbose,
        output_dir=args.output,
        server_url=args.server_url,
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
    elif args.backend:
        result = runner.run_backend_validation_tests()
    else:
        result = runner.run_all_tests()

    runner.print_summary(result)

    if args.save:
        filepath = runner.save_results(result)
        print(f"\nResults saved to: {filepath}")

    if args.html:
        filepath = runner.save_html_report(result)
        print(f"HTML report saved to: {filepath}")

    sys.exit(1 if result.failed > 0 else 0)


if __name__ == "__main__":
    main()
