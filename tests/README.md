# Tests Directory

This directory contains the Python-based test infrastructure for tingly-box.

## Quick Start

### Automated Test (Recommended)

```bash
task tests:interface
```

This will:
- Create a temporary directory
- Generate test configuration
- Start the tingly-box server on a free port
- Run all test suites
- Display results
- Clean up automatically

### Manual Test

```bash
# Terminal 1: Start server
tingly-box start --port 12581 --config-dir ~/.tingly-box --browser=false

# Terminal 2: Run tests
python3 -m tests.runner --verbose
```

## Test Configuration

### Production Server (Port 12580)

- **Config**: `~/.tingly-box/config.json`
- **Usage**: `python3 -m tests.runner --verbose`

### Test Server (Port 12581)

- **Config**: `tests/test_config_port12581.json`
- **Usage**: Automated via `task tests:interface`
- **Scenarios**: openai, anthropic (generated from real config)

## Test Suites

1. **Smoke Tests** - Tingly-box scenario endpoints (no direct provider calls)
   ```bash
   python3 -m tests.runner --smoke --verbose
   ```

2. **Proxy Tests** - Tingly-box proxy endpoint tests
   ```bash
   python3 -m tests.runner --proxy-smoke --verbose
   ```

3. **Backend Validation** - Field existence and format compliance
   ```bash
   python3 -m tests.runner --backend --verbose
   ```

4. **Adaptor Tests** - API format adaptation tests
   ```bash
   python3 -m tests.runner --adaptor --verbose
   ```

5. **Differential Tests** - Three-path differential via tingly-box
   - **Direct**: sends the request through the provider’s rule and scenario without extra transforms.
   - **Request-transform**: sends through the opposite scenario (OpenAI↔Anthropic) to exercise request conversion.
   - **Response-roundtrip**: sends through the provider’s scenario but roundtrips the response through the opposite format and back.
   ```bash
   python3 -m tests.runner --differential --verbose
   ```

## Dependencies

Install test dependencies:

```bash
# Minimal (for basic testing)
pip install -r requirements-minimal.txt

# Full (for all features)
pip install -r requirements.txt
```

## Test Results

Results are saved to:
- **Directory**: `tests/test_results/`
- **Formats**: JSON and HTML

See [Complete Test Documentation](../docs/test.md) for detailed information.

See [Complete Test Documentation](../docs/test.md) for detailed information.
