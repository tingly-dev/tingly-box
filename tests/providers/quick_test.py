#!/usr/bin/env python3
"""
Quick test script to verify the test system is working.
"""

import sys
import os

# Add the project root to path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from tests.providers import (
    TestConfig,
    ConfigLoader,
    SmokeTestSuite,
    AdaptorTestSuite,
    DifferentialTestSuite,
)


def test_imports():
    """Test that all modules import correctly."""
    print("Testing imports...")
    try:
        from tests.providers import (
            ConfigLoader,
            TestConfig,
            BaseProviderClient,
            OpenAIClient,
            AnthropicClient,
            GoogleClient,
            SmokeTestSuite,
            ProxySmokeTestSuite,
            AdaptorTestSuite,
            DifferentialTestSuite,
            DifferentialResult,
            TestRunner,
            main,
        )
        print("  All imports successful ✓")
        return True
    except ImportError as e:
        print(f"  Import failed: {e} ✗")
        return False


def test_config_loading():
    """Test configuration loading."""
    print("\nTesting config loading...")
    try:
        # Create a test config directly
        config = TestConfig(
            providers=[],
            server_url="http://localhost:12580",
            test_model="gpt-4",
            test_prompt="Hello, this is a test. Please respond briefly.",
            timeout=60,
            verbose=True,
        )
        print(f"  Config created: server={config.server_url}")
        print(f"  Test model: {config.test_model}")
        return True
    except Exception as e:
        print(f"  Config creation failed: {e} ✗")
        return False


def test_config_loader():
    """Test ConfigLoader class."""
    print("\nTesting ConfigLoader...")
    try:
        loader = ConfigLoader(config_path=None)
        print("  ConfigLoader instantiated ✓")

        # Try to find config
        config_path = loader.find_config()
        if config_path:
            print(f"  Found config at: {config_path}")
        else:
            print("  No config file found (expected if no config exists)")
        return True
    except Exception as e:
        print(f"  ConfigLoader failed: {e} ✗")
        return False


def test_client_creation():
    """Test client creation."""
    print("\nTesting client creation...")
    try:
        from tests.providers.client import OpenAIClient, AnthropicClient, GoogleClient

        clients = [
            OpenAIClient("test-openai", "https://api.openai.com/v1", "test-token"),
            AnthropicClient("test-anthropic", "https://api.anthropic.com", "test-token"),
            GoogleClient("test-google", "https://generativelanguage.googleapis.com", "test-token"),
        ]

        for client in clients:
            print(f"  Created {client.name} ({client.provider_type.value}) ✓")

        return True
    except Exception as e:
        print(f"  Client creation failed: {e} ✗")
        return False


def test_test_suites():
    """Test that test suites can be instantiated."""
    print("\nTesting test suite instantiation...")
    try:
        config = TestConfig(
            providers=[],
            server_url="http://localhost:12580",
            test_model="gpt-4",
            test_prompt="Hello, this is a test. Please respond briefly.",
            timeout=60,
            verbose=True,
        )

        smoke = SmokeTestSuite(config, verbose=True)
        print("  SmokeTestSuite created ✓")

        adaptor = AdaptorTestSuite(config, verbose=True)
        print("  AdaptorTestSuite created ✓")

        differential = DifferentialTestSuite(config, verbose=True)
        print("  DifferentialTestSuite created ✓")

        return True
    except Exception as e:
        print(f"  Test suite creation failed: {e} ✗")
        return False


def main():
    """Run all quick tests."""
    print("=" * 50)
    print("TINGLY-BOX TEST SYSTEM - QUICK VERIFICATION")
    print("=" * 50)

    results = []

    results.append(("Imports", test_imports()))
    results.append(("Config Creation", test_config_loading()))
    results.append(("ConfigLoader", test_config_loader()))
    results.append(("Client Creation", test_client_creation()))
    results.append(("Test Suites", test_test_suites()))

    print("\n" + "=" * 50)
    print("SUMMARY")
    print("=" * 50)

    all_passed = True
    for name, passed in results:
        status = "PASS" if passed else "FAIL"
        symbol = "✓" if passed else "✗"
        print(f"  {name}: {status} {symbol}")
        if not passed:
            all_passed = False

    print("=" * 50)

    if all_passed:
        print("\nAll quick tests passed! ✓")
        print("\nTo run the full test suite:")
        print("  PYTHONPATH=/root/projects/tingly-box python3 -m tests.providers.runner --all --save")
        return 0
    else:
        print("\nSome tests failed. Please check the errors above. ✗")
        return 1


if __name__ == "__main__":
    sys.exit(main())
