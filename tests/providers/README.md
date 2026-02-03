# Tingly-Box Test System

A comprehensive Python-based test system for the Tingly-Box AI model proxy toolbox.

## Features

- **Smoke Tests**: Basic connectivity and functionality tests for each provider
  - Model fetching
  - Chat completions (OpenAI/Anthropic/Google styles)
  - Multi-turn conversations
  - System message handling

- **Adaptor Tests**: Cross-provider transformation verification
  - OpenAI -> Anthropic transformation
  - Anthropic -> OpenAI transformation
  - OpenAI/Anthropic -> Google transformation
  - Multi-turn conversation preservation
  - System message handling

- **Differential Tests**: Advanced transformation correctness testing
  - Roundtrip transformation (O->A->O)
  - Multi-provider consistency checking
  - Response structure equivalence
  - "Minority is guilty" principle for outlier detection

## Installation

```bash
# Install dependencies
pip install -r tests/providers/requirements.txt
```

## Usage

### Command Line

```bash
# Run all tests
python -m tests.providers.runner --all

# Run only smoke tests
python -m tests.providers.runner --smoke

# Run adaptor tests with verbose output
python -m tests.providers.runner --adaptor -v

# Run differential tests
python -m tests.providers.runner --differential

# Use custom config file
python -m tests.providers.runner --all --config /path/to/config.json

# Save results to JSON
python -m tests.providers.runner --all --save
```

### Python API

```python
from tests.providers import (
    load_config,
    TestRunner,
    SmokeTestSuite,
    AdaptorTestSuite,
    DifferentialTestSuite,
)

# Load configuration
config = load_config("/path/to/config.json")

# Run all tests
runner = TestRunner(config_path="/path/to/config.json", verbose=True)
result = runner.run_all_tests()
print(f"Passed: {result.passed}, Failed: {result.failed}")

# Run specific test suite
smoke_suite = SmokeTestSuite(config, verbose=True)
results = smoke_suite.run_all_tests()

# Run adaptor tests
adaptor_suite = AdaptorTestSuite(config, verbose=True)
adaptor_results = adaptor_suite.run_all_tests()

# Run differential tests
diff_suite = DifferentialTestSuite(config, verbose=True)
diff_results = diff_suite.run_all_tests()
```

## Configuration

The test system reads configuration from:

1. Path specified via `--config` or `config` parameter
2. `~/.tingly-box/config.json` (default)
3. `config.json` in current directory
4. `/root/projects/tingly-box/build/config.yml` (fallback)

### Example Configuration

```json
{
    "providers": [
        {
            "uuid": "...",
            "name": "openai-main",
            "api_base": "https://api.openai.com/v1",
            "api_style": "openai",
            "token": "sk-...",
            "timeout": 60,
            "models": ["gpt-4", "gpt-3.5-turbo"]
        },
        {
            "uuid": "...",
            "name": "anthropic-main",
            "api_base": "https://api.anthropic.com",
            "api_style": "anthropic",
            "token": "sk-ant-...",
            "timeout": 60,
            "models": ["claude-sonnet-4-20250514", "claude-opus-4-20250514"]
        }
    ],
    "server_url": "http://localhost:12580",
    "test_model": "gpt-4",
    "test_prompt": "Hello, this is a test. Please respond briefly.",
    "timeout": 60,
    "verbose": true,
    "output_dir": "./test_results"
}
```

## Test Suites

### Smoke Test Suite

Tests basic provider functionality:

| Test | Description |
|------|-------------|
| `list_models` | Fetch available models from provider |
| `chat_completions` | Send chat completion request |
| `chat_completions_with_system` | Test with system message |
| `responses_api` | Test OpenAI Responses API |

### Adaptor Test Suite

Tests cross-provider transformations:

| Test | Description |
|------|-------------|
| `openai_to_anthropic` | OpenAI format to Anthropic backend |
| `anthropic_to_openai` | Anthropic format to OpenAI backend |
| `openai_to_google` | OpenAI format to Google backend |
| `anthropic_to_google` | Anthropic format to Google backend |
| `multi_turn_conversation` | Multi-turn conversation preservation |
| `system_message_handling` | System message transformation |

### Differential Test Suite

Advanced transformation verification using "minority is guilty" principle:

| Test | Description |
|------|-------------|
| `roundtrip_o_a_o` | OpenAI -> Anthropic -> OpenAI roundtrip |
| `anthropic_roundtrip` | Anthropic roundtrip transformation |
| `multi_provider_consistency` | Cross-provider response consistency |
| `response_structure_equivalence` | Response structure validation |

## Differential Testing Principle

The test system implements the **"minority is guilty"** principle:

> When comparing multiple responses (e.g., from different transformation paths),
> if one response differs from the others, that outlier is investigated rather
> than assuming the majority is correct.

This helps identify:
- Transformation bugs that affect specific paths
- Non-deterministic model behavior
- Provider-specific response variations

## Output

Test results are saved as JSON with the following structure:

```json
{
    "run_id": "20251220_120000",
    "timestamp": "2025-12-20T12:00:00.000000",
    "suite_name": "All Tests",
    "total_tests": 20,
    "passed": 18,
    "failed": 2,
    "skipped": 0,
    "duration_ms": 15000.5,
    "success_rate": 90.0,
    "results": [...]
}
```

## Project Structure

```
tests/providers/
├── __init__.py          # Package exports
├── config.py            # Configuration loader
├── client.py            # Provider client implementations
├── smoke.py             # Smoke test suite
├── adaptor.py           # Adaptor test suite
├── differential.py      # Differential test suite
├── runner.py            # Main test runner
└── requirements.txt     # Dependencies
```
