"""
Automated Tingly-Box Test Infrastructure

This module provides an automated test runner that:
- Creates a temporary directory
- Sets up test configuration
- Starts the tingly-box server
- Runs tests
- Cleans up resources

Example usage:
    from tests.test_automation import TestAutomation

    automation = TestAutomation()
    automation.run()
"""

import json
import os
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Optional
import logging

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


@dataclass
class ServerConfig:
    """Server configuration settings."""
    port: int
    config_path: Path
    binary_path: Path
    log_path: Path


@dataclass
class TestConfig:
    """Test configuration settings."""
    deepseek_scenario: str = "deepseek-test"
    glm_scenario: str = "glm-test"
    qwen_scenario: str = "qwen-test"
    server_port: int = 12581


@dataclass
class TestResult:
    """Test execution result."""
    exit_code: int
    duration_ms: float
    log_path: Path


class TestAutomation:
    """Automated test infrastructure for tingly-box."""
    TARGET_PROVIDER_NAMES = {"qwen-test", "glm-test", "minimax-test"}
    TARGET_PROVIDER_ALIASES = {
        "qwen-test": "qwen",
        "glm-test": "glm",
        "minimax-test": "minimax",
    }

    def __init__(
        self,
        test_port: int = 12581,
        project_dir: Optional[Path] = None,
        verbose: bool = True,
        config_path: Optional[str] = None
    ):
        """
        Initialize test automation.

        Args:
            test_port: Port to run test server on
            project_dir: Path to tingly-box project (auto-detected if None)
            verbose: Enable verbose logging
            config_path: Path to tingly-box config (default: ~/.tingly-box/config.json)
        """
        self.test_port = self._select_test_port(test_port)
        self.project_dir = project_dir or self._find_project_dir()
        self.verbose = verbose
        self.temp_dir: Optional[Path] = None
        self.server_process: Optional[subprocess.Popen] = None
        self.config_path = Path(config_path or Path.home() / ".tingly-box" / "config.json")

        # Paths
        self.server_binary = self.project_dir / "build" / "tingly-box"
        self.config_template_path = self.project_dir / "tests" / "test_config_port12581.json"

        logger.info(f"Test automation initialized")
        logger.info(f"Project dir: {self.project_dir}")
        logger.info(f"Server binary: {self.server_binary}")
        logger.info(f"Test port: {self.test_port}")
        logger.info(f"Config path: {self.config_path}")

    def _select_test_port(self, preferred: int) -> int:
        """Pick an available local port, preferring the provided one."""
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.bind(("127.0.0.1", preferred))
                return preferred
        except OSError:
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
                sock.bind(("127.0.0.1", 0))
                return sock.getsockname()[1]

    @staticmethod
    def _find_project_dir() -> Path:
        """Find the tingly-box project directory."""
        current = Path.cwd()
        for parent in [current] + list(current.parents):
            if (parent / "go.mod").exists() and (parent / "internal").exists():
                return parent
        raise RuntimeError("Could not find tingly-box project directory")

    def _load_config(self) -> Dict:
        """
        Load tingly-box configuration file.

        Returns:
            Configuration dictionary

        Raises:
            RuntimeError: If config file doesn't exist
        """
        if not self.config_path.exists():
            raise RuntimeError(
                f"Config file not found at {self.config_path}\n"
                f"Please run tingly-box first to create config, or specify --config-path"
            )

        logger.info(f"Loading config from: {self.config_path}")
        with open(self.config_path, 'r') as f:
            config = json.load(f)

        logger.info(f"Config loaded successfully")
        return config

    def _create_temp_directory(self) -> Path:
        """Create and return path to temporary directory."""
        temp_dir = Path(tempfile.mkdtemp(prefix="tingly-box-test-"))
        logger.info(f"Created temp directory: {temp_dir}")
        return temp_dir

    def _create_test_config(self, temp_dir: Path) -> Path:
        """
        Create test configuration file in temp directory.

        Args:
            temp_dir: Directory to create config in

        Returns:
            Path to created config file
        """
        config_path = temp_dir / "config.json"
        logger.info("Creating config from real config")
        config_data = self._generate_config_data()
        with open(config_path, 'w') as f:
            json.dump(config_data, f, indent=2)

        logger.info(f"Created test config: {config_path}")
        return config_path

    def _generate_config_data(self) -> Dict:
        """
        Generate test configuration data.

        Returns:
            Configuration dictionary
        """
        config = self._load_config()
        providers = self._extract_providers(config)
        rules, provider_uuids = self._build_rules_from_config(config, providers)
        if not rules:
            raise RuntimeError(
                "No test rules could be generated from the real config. "
                "Ensure providers have models or existing rules with services."
            )
        scenarios = self._build_scenarios_from_rules(rules)

        filtered_providers = [p for p in providers if p.get("uuid") in provider_uuids]

        model_token = config.get("model_token", "")
        jwt_secret = config.get("jwt_secret", "1765265404669863000")

        test_config = {
            "rules": rules,
            "default_request_id": config.get("default_request_id", 14),
            "user_token": config.get("user_token", "tingly-box-user-token"),
            "model_token": model_token,
            "encrypt_providers": config.get("encrypt_providers", False),
            "scenarios": scenarios,
            "gui": config.get("gui", {"debug": False, "port": 0, "verbose": False}),
            "tool_interceptor": config.get("tool_interceptor", {"enabled": False}),
            "providers_v2": filtered_providers,
            "server_port": self.test_port,
            "jwt_secret": jwt_secret,
            "default_max_tokens": config.get("default_max_tokens", 8192),
            "verbose": config.get("verbose", True),
        }

        if "providers" in config and not config.get("providers_v2"):
            test_config["providers"] = providers

        return test_config

    def _extract_providers(self, config: Dict) -> list:
        """Extract enabled providers from real config."""
        providers = []
        if config.get("providers_v2"):
            providers = list(config.get("providers_v2", []))
        elif config.get("providers"):
            providers = list(config.get("providers", []))

        enabled = []
        for provider in providers:
            if provider.get("enabled") is False:
                continue
            enabled.append(provider)
        return enabled

    def _build_rules_from_config(self, config: Dict, providers: list) -> tuple[list, set]:
        """Build rules for test config based on real providers."""
        rules = []
        provider_uuids = set()
        config_rules = config.get("rules", []) or []
        provider_lookup = {str(p.get("name", "")).lower(): p for p in providers}

        def scenario_for_provider(provider: Dict) -> str:
            api_style = (provider.get("api_style") or "openai").lower()
            if api_style in ("anthropic", "claude_code", "opencode"):
                return api_style
            return "openai"

        def find_service_for_provider(provider_uuid: str) -> Optional[Dict]:
            for rule in config_rules:
                for service in rule.get("services", []) or []:
                    if service.get("provider") == provider_uuid and service.get("model"):
                        return {
                            "model": service.get("model"),
                        }
            return None

        for target_name in self.TARGET_PROVIDER_NAMES:
            target_key = target_name.lower()
            alias_key = self.TARGET_PROVIDER_ALIASES.get(target_name, target_name).lower()

            candidates = []
            target_provider = provider_lookup.get(target_key)
            if target_provider:
                candidates.append(target_provider)
            alias_provider = provider_lookup.get(alias_key)
            if alias_provider and alias_provider not in candidates:
                candidates.append(alias_provider)

            if not candidates:
                logger.warning(f"Skipping provider '{target_name}' - not found in config")
                continue

            provider = None
            model = None
            for candidate in candidates:
                candidate_uuid = candidate.get("uuid")
                service_info = find_service_for_provider(candidate_uuid)
                if service_info:
                    provider = candidate
                    model = service_info["model"]
                    break
                if candidate.get("models"):
                    provider = candidate
                    model = candidate["models"][0]
                    break

            if not provider or not model:
                logger.warning(f"Skipping provider '{target_name}' - no model found in config")
                continue

            provider_uuid = provider.get("uuid")
            provider_name = provider.get("name") or target_name
            scenario = scenario_for_provider(provider)

            provider_uuids.add(provider_uuid)
            rules.append({
                "uuid": f"test-{target_name}-rule",
                "scenario": scenario,
                "request_model": target_name,
                "response_model": "",
                "description": f"Test rule for {provider_name} using {scenario} scenario",
                "services": [
                    {
                        "provider": provider_uuid,
                        "model": model,
                        "weight": 1,
                        "active": True,
                        "time_window": 300,
                    }
                ],
                "lb_tactic": {
                    "type": "round_robin",
                    "params": {
                        "request_threshold": 10,
                    },
                },
                "active": True,
                "smart_enabled": False,
            })

        return rules, provider_uuids

    def _build_scenarios_from_rules(self, rules: list) -> list:
        """Build scenario flags for the test config."""
        scenarios = []
        seen = set()
        for rule in rules:
            scenario = rule.get("scenario")
            if not scenario or scenario in seen:
                continue
            seen.add(scenario)
            scenarios.append({
                "scenario": scenario,
                "flags": {
                    "unified": False,
                    "separate": True,
                    "smart": False,
                },
            })
        return scenarios

    def _check_server_binary(self) -> None:
        """Check if server binary exists."""
        if not self.server_binary.exists():
            raise RuntimeError(
                f"Server binary not found at {self.server_binary}\n"
                f"Please build the server first:\n"
                f"  cd {self.project_dir}\n"
                f"  task go:build\n"
                f"Or use existing binary at {self.server_binary}"
            )
        logger.info(f"Server binary found: {self.server_binary}")

    def _start_server(self, config_path: Path, log_path: Path) -> subprocess.Popen:
        """
        Start the tingly-box server.

        Args:
            config_path: Path to server configuration
            log_path: Path to write server log

        Returns:
            Started process object
        """
        logger.info(f"Starting server on port {self.test_port}")
        logger.info(f"Config: {config_path}")
        logger.info(f"Log: {log_path}")

        with open(log_path, 'w') as log_file:
            process = subprocess.Popen(
                [
                    str(self.server_binary),
                    "start",
                    "--config-dir", str(self.temp_dir),
                    "--port", str(self.test_port),
                "--browser", "false"
                ],
                stdout=log_file,
                stderr=subprocess.STDOUT,
                cwd=str(self.temp_dir)
            )

        logger.info("Server started")
        return process

    def _wait_for_server(self, max_attempts: int = 30) -> bool:
        """
        Wait for server to be ready.

        Args:
            max_attempts: Maximum number of attempts

        Returns:
            True if server is ready, False otherwise
        """
        import urllib.request

        url = f"http://localhost:{self.test_port}/tingly/openai/v1/models"
        logger.info(f"Waiting for server to be ready at {url}")
        model_token = ""
        try:
            config = self._load_config()
            model_token = config.get("model_token", "")
        except Exception:
            model_token = ""

        for attempt in range(1, max_attempts + 1):
            try:
                req = urllib.request.Request(url)
                if model_token:
                    req.add_header("Authorization", f"Bearer {model_token}")
                with urllib.request.urlopen(req, timeout=2) as response:
                    if response.status in (200, 401):
                        logger.info("✓ Server is ready!")
                        return True
            except urllib.error.HTTPError as e:
                # Any HTTP response means server is up (even if auth/route fails)
                logger.info(f"✓ Server is ready (HTTP {e.code})!")
                return True
                if attempt == max_attempts:
                    logger.error(f"Server failed to start after {max_attempts} attempts")
                    return False
                logger.debug(f"Attempt {attempt}/{max_attempts} failed: {e}")
                time.sleep(1)
            except Exception as e:
                if attempt == max_attempts:
                    logger.error(f"Server failed to start after {max_attempts} attempts")
                    return False
                logger.debug(f"Attempt {attempt}/{max_attempts} failed: {e}")
                time.sleep(1)

        return False

    def _run_tests(self, config_path: Path) -> TestResult:
        """
        Run the test suite.

        Args:
            config_path: Path to test configuration

        Returns:
            TestResult with exit code and duration
        """
        logger.info("Running test suite")

        # Create permanent output directory in tests/ with timestamp
        from datetime import datetime
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        permanent_output_dir = self.project_dir / "tests" / "test_results" / timestamp
        permanent_output_dir.mkdir(parents=True, exist_ok=True)
        logger.info(f"Results will be saved to: {permanent_output_dir}")

        start_time = time.time()

        # Run tests using Python module against the test server
        # Output directly to permanent directory
        server_url = f"http://localhost:{self.test_port}"
        sys.argv = [
            "tests.runner",
            "--all",
            "--html",
            "--save",
            "--verbose",
            "--config", str(config_path),
            "--server-url", server_url,
            "--output", str(permanent_output_dir)
        ]

        try:
            from tests.runner import main as run_tests
            exit_code = run_tests()
            duration_ms = (time.time() - start_time) * 1000
            logger.info(f"Tests completed with exit code: {exit_code}")

            return TestResult(
                exit_code=exit_code,
                duration_ms=duration_ms,
                log_path=permanent_output_dir
            )
        except Exception as e:
            logger.exception(f"Test execution failed: {e}")
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                exit_code=1,
                duration_ms=duration_ms,
                log_path=permanent_output_dir
            )

    def _cleanup(self) -> None:
        """Clean up resources."""
        logger.info("Cleaning up resources")

        # Stop server if running
        if self.server_process and self.server_process.poll() is None:
            try:
                self.server_process.terminate()
                self.server_process.wait(timeout=10)
                logger.info("Server stopped")
            except Exception as e:
                logger.warning(f"Failed to stop server: {e}")

        # Remove temp directory
        if self.temp_dir and self.temp_dir.exists():
            logger.info(f"Removing temp directory: {self.temp_dir}")
            shutil.rmtree(self.temp_dir)

    def run(self) -> int:
        """
        Execute the complete test automation workflow.

        Returns:
            Exit code (0 for success, non-zero for failure)
        """
        logger.info("=" * 60)
        logger.info("Tingly-Box Automated Test Runner")
        logger.info("=" * 60)
        logger.info("")

        try:
            # Step 1: Create temp directory
            logger.info("Step 1: Creating temporary directory")
            self.temp_dir = self._create_temp_directory()
            logger.info(f"✓ Temp directory created: {self.temp_dir}")
            logger.info("")

            # Step 2: Create test config
            logger.info("Step 2: Creating test configuration")
            config_path = self._create_test_config(self.temp_dir)
            logger.info(f"✓ Test config created: {config_path}")
            logger.info("")

            # Step 3: Check server binary
            logger.info("Step 3: Checking server binary")
            self._check_server_binary()
            logger.info("✓ Server binary check passed")
            logger.info("")

            # Step 4: Start server
            logger.info("Step 4: Starting server")
            log_path = self.temp_dir / "server.log"
            self.server_process = self._start_server(config_path, log_path)
            logger.info("")

            # Step 5: Wait for server
            logger.info("Step 5: Waiting for server to be ready")
            if not self._wait_for_server():
                logger.error("Server failed to start")
                logger.info(f"Server log: {log_path}")
                with open(log_path) as f:
                    logger.info("Server output:")
                    logger.info(f.read())
                return 1
            logger.info("")

            # Step 6: Run tests
            logger.info("Step 6: Running tests")
            result = self._run_tests(config_path)
            logger.info(f"✓ Tests completed in {result.duration_ms:.2f}ms")
            logger.info("")

            # Step 7: Display results
            self._display_results(result)

            return result.exit_code

        except Exception as e:
            logger.exception(f"Test automation failed: {e}")
            return 1

        finally:
            # Always cleanup
            self._cleanup()

    def _display_results(self, result: TestResult) -> None:
        """
        Display test results.

        Args:
            result: TestResult object
        """
        logger.info("=" * 60)
        logger.info("Test Results")
        logger.info("=" * 60)
        logger.info("")
        logger.info(f"Exit Code: {result.exit_code}")
        logger.info(f"Duration: {result.duration_ms:.2f}ms")
        logger.info(f"Results Directory: {result.log_path}")
        logger.info("")

        # List saved files
        if result.log_path.exists():
            result_files = list(result.log_path.glob("*"))
            if result_files:
                logger.info("Saved files:")
                for file in sorted(result_files):
                    logger.info(f"  - {file.name}")
        logger.info("")

        # Display summary
        if result.exit_code == 0:
            logger.info("✓ Tests completed successfully")
        else:
            logger.warning("⚠ Tests completed with errors")

        logger.info("")
        logger.info("Note: Test results are saved permanently in tests/test_results/")
        logger.info("      Temp directory will be removed after script exits")


def main() -> int:
    """Main entry point for the test automation."""
    import argparse

    parser = argparse.ArgumentParser(
        description="Automated test infrastructure for tingly-box"
    )
    parser.add_argument(
        "--port",
        type=int,
        default=12581,
        help="Port to run test server on (default: 12581)"
    )
    parser.add_argument(
        "--config-path",
        type=str,
        default=None,
        help="Path to tingly-box config (default: ~/.tingly-box/config.json)"
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        default=True,
        help="Enable verbose output (default: True)"
    )
    parser.add_argument(
        "--quiet", "-q",
        action="store_true",
        help="Disable verbose output"
    )

    args = parser.parse_args()

    try:
        automation = TestAutomation(
            test_port=args.port,
            verbose=args.verbose and not args.quiet,
            config_path=args.config_path
        )
        return automation.run()
    except KeyboardInterrupt:
        logger.info("\nTest execution interrupted by user")
        return 130
    except Exception as e:
        logger.exception(f"Unexpected error: {e}")
        return 1


if __name__ == "__main__":
    sys.exit(main())
