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
        self.test_port = test_port
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

    def _get_provider_token(self, config: Dict, provider_name: str) -> Optional[str]:
        """
        Get provider token from config.

        Args:
            config: Configuration dictionary
            provider_name: Name of provider (qwen, glm, deepseek)

        Returns:
            Provider token or None if not found
        """
        providers_v2 = config.get("providers_v2", [])
        for provider in providers_v2:
            if provider.get("name") == provider_name:
                return provider.get("token")
        logger.warning(f"Provider '{provider_name}' not found in config")
        return None

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

        # Read template if it exists
        if self.config_template_path.exists():
            logger.info(f"Using template config: {self.config_template_path}")
            shutil.copy(self.config_template_path, config_path)
        else:
            logger.info("Creating config from scratch")
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
        # Load existing config to get model_token and provider tokens
        config = self._load_config()
        model_token = config.get("model_token", "")

        # Get provider tokens from config
        qwen_token = self._get_provider_token(config, "qwen") or ""
        glm_token = self._get_provider_token(config, "glm") or ""
        deepseek_token = self._get_provider_token(config, "deepseek") or ""

        # Get jwt_secret from config
        jwt_secret = config.get("jwt_secret", "1765265404669863000")

        return {
            "rules": [
                {
                    "uuid": "test-deepseek-rule",
                    "scenario": "deepseek-test",
                    "request_model": "deepseek-test",
                    "response_model": "",
                    "description": "Test scenario for Deepseek backend",
                    "services": [
                        {
                            "provider": "25419e90-f5af-11f0-8c69-6ec60317391b",
                            "model": "deepseek-chat",
                            "weight": 1,
                            "active": True,
                            "time_window": 300
                        }
                    ],
                    "lb_tactic": {
                        "type": "round_robin",
                        "params": {
                            "request_threshold": 10
                        }
                    },
                    "active": True,
                    "smart_enabled": False
                },
                {
                    "uuid": "test-glm-rule",
                    "scenario": "glm-test",
                    "request_model": "glm-test",
                    "response_model": "",
                    "description": "Test scenario for GLM backend",
                    "services": [
                        {
                            "provider": "fc958d9c-df0d-11f0-a608-6ec70917391c",
                            "model": "glm-4.7",
                            "weight": 1,
                            "active": True,
                            "time_window": 300
                        }
                    ],
                    "lb_tactic": {
                        "type": "round_robin",
                        "params": {
                            "request_threshold": 10
                        }
                    },
                    "active": True,
                    "smart_enabled": False
                },
                {
                    "uuid": "test-qwen-rule",
                    "scenario": "qwen-test",
                    "request_model": "qwen-test",
                    "response_model": "",
                    "description": "Test scenario for Qwen backend",
                    "services": [
                        {
                            "provider": "fc958d9c-df0d-11f0-a608-6ec60317391b",
                            "model": "qwen-plus",
                            "weight": 1,
                            "active": True,
                            "time_window": 300
                        }
                    ],
                    "lb_tactic": {
                        "type": "round_robin",
                        "params": {
                            "request_threshold": 10
                        }
                    },
                    "active": True,
                    "smart_enabled": False
                }
            ],
            "default_request_id": 14,
            "user_token": "tingly-box-user-token",
            "model_token": model_token,
            "encrypt_providers": False,
            "scenarios": [
                {
                    "scenario": "deepseek-test",
                    "flags": {
                        "unified": False,
                        "separate": True,
                        "smart": False
                    }
                },
                {
                    "scenario": "glm-test",
                    "flags": {
                        "unified": False,
                        "separate": True,
                        "smart": False
                    }
                },
                {
                    "scenario": "qwen-test",
                    "flags": {
                        "unified": False,
                        "separate": True,
                        "smart": False
                    }
                }
            ],
            "gui": {
                "debug": False,
                "port": 0,
                "verbose": False
            },
            "tool_interceptor": {
                "enabled": False
            },
            "providers_v2": [
                {
                    "uuid": "fc958d9c-df0d-11f0-a608-6ec60317391b",
                    "name": "qwen",
                    "api_base": "https://dashscope.aliyuncs.com/compatible-mode/v1",
                    "api_style": "openai",
                    "token": qwen_token,
                    "no_key_required": False,
                    "enabled": True,
                    "proxy_url": "",
                    "timeout": 1800,
                    "last_updated": "2025-12-22T16:12:41+08:00",
                    "auth_type": ""
                },
                {
                    "uuid": "fc958d9c-df0d-11f0-a608-6ec70917391c",
                    "name": "glm",
                    "api_base": "https://open.bigmodel.cn/api/anthropic",
                    "api_style": "anthropic",
                    "token": glm_token,
                    "no_key_required": False,
                    "enabled": True,
                    "proxy_url": "",
                    "timeout": 1800,
                    "auth_type": ""
                },
                {
                    "uuid": "25419e90-f5af-11f0-8c69-6ec60317391b",
                    "name": "deepseek",
                    "api_base": "https://api.deepseek.com/v1",
                    "api_style": "openai",
                    "token": deepseek_token,
                    "no_key_required": False,
                    "enabled": True,
                    "proxy_url": "",
                    "timeout": 1800,
                    "auth_type": ""
                }
            ],
            "server_port": self.test_port,
            "jwt_secret": jwt_secret,
            "default_max_tokens": 8192,
            "verbose": True
        }

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
            env = os.environ.copy()
            env['TINGLY_CONFIG_DIR'] = str(self.temp_dir)

            process = subprocess.Popen(
                [
                    str(self.server_binary),
                    "start",
                    "--port", str(self.test_port),
                    "--browser", "false",
                    "--daemon"
                ],
                stdout=log_file,
                stderr=subprocess.STDOUT,
                cwd=str(self.temp_dir),
                env=env
            )

        # For daemon mode, the process exits immediately after starting the server
        # So we can't use the return process to check if it's running
        logger.info(f"Server started in daemon mode")
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

        for attempt in range(1, max_attempts + 1):
            try:
                with urllib.request.urlopen(url, timeout=2) as response:
                    if response.status == 200:
                        logger.info("✓ Server is ready!")
                        return True
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

        output_dir = self.temp_dir / "test_results"
        output_dir.mkdir(exist_ok=True)

        start_time = time.time()

        # Run tests using Python module
        sys.argv = [
            "tests.runner",
            "--all",
            "--html",
            "--save",
            "--verbose",
            "-c", str(config_path),
            "--output", str(output_dir)
        ]

        try:
            from tests.runner import main as run_tests
            exit_code = run_tests()
            duration_ms = (time.time() - start_time) * 1000
            logger.info(f"Tests completed with exit code: {exit_code}")
            return TestResult(
                exit_code=exit_code,
                duration_ms=duration_ms,
                log_path=output_dir
            )
        except Exception as e:
            logger.exception(f"Test execution failed: {e}")
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                exit_code=1,
                duration_ms=duration_ms,
                log_path=output_dir
            )

    def _cleanup(self) -> None:
        """Clean up resources."""
        logger.info("Cleaning up resources")

        # Stop server if running (for daemon mode)
        if self.temp_dir:
            try:
                # Use tingly-box stop command to properly stop the daemon
                env = os.environ.copy()
                env['TINGLY_CONFIG_DIR'] = str(self.temp_dir)
                subprocess.run(
                    [
                        str(self.server_binary),
                        "stop"
                    ],
                    capture_output=True,
                    timeout=10,
                    env=env
                )
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

        # Display summary
        if result.exit_code == 0:
            logger.info("✓ Tests completed successfully")
        else:
            logger.warning("⚠ Tests completed with errors")

        logger.info("")
        logger.info("Note: Temp directory will be removed after script exits")


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
