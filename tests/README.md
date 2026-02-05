# Tests Directory

This directory contains the Python-based test infrastructure for tingly-box.

## Quick Start

### Automated Test (Recommended)

```bash
./run_automated_test.sh
```

This will:
- Create a temporary directory
- Generate test configuration
- Start the tingly-box server on port 12581
- Run all test suites
- Display results
- Clean up automatically

### Manual Test

```bash
# Terminal 1: Start server
tingly-box server --port 12581 --config tests/test_config_port12581.json

# Terminal 2: Run tests
python3 -m runner --verbose
```

## Test Configuration

### Production Server (Port 12580)

- **Config**: `~/.tingly-box/config.json`
- **Usage**: `python3 -m runner --verbose`

### Test Server (Port 12581)

- **Config**: `tests/test_config_port12581.json`
- **Usage**: Automated via `run_automated_test.sh`
- **Scenarios**: deepseek-test, glm-test, qwen-test

## Test Suites

1. **Smoke Tests** - Direct provider API tests
   ```bash
   python3 -m runner --smoke --verbose
   ```

2. **Proxy Tests** - Tingly-box proxy endpoint tests
   ```bash
   python3 -m runner --proxy-smoke --verbose
   ```

3. **Backend Validation** - Field existence and format compliance
   ```bash
   python3 -m runner --backend --verbose
   ```

4. **Adaptor Tests** - API format adaptation tests
   ```bash
   python3 -m runner --adaptor --verbose
   ```

5. **Differential Tests** - Cross-provider consistency
   ```bash
   python3 -m runner --differential --verbose
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
- **Directory**: `../test_results/`
- **Formats**: JSON and HTML

See [Complete Test Documentation](../docs/test.md) for detailed information.

## Current Status

✅ **GLM Backend**: 100% success (3/3 tests)
✅ **Deepseek Backend**: 100% success (3/3 tests)
⚠️ **Qwen Backend**: 0% (API key issue - external)
⚠️ **Proxy Tests**: Failing (server bug - needs backend fix)

See [Complete Test Documentation](../docs/test.md) for detailed information.
