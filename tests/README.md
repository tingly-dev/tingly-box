# Tests Directory

This directory contains the Python-based test infrastructure for tingly-box.

## Codex Responses completion proof

`codex-responses-completion-proof.py` runs a real Codex CLI against the full
gateway and the vmodel scenario server. It compares the current handler with
the handler at an explicit pre-fix revision using a Go source overlay; all
other code and SDK versions stay the same. Codex uses a fresh configuration
directory and a synthetic local credential, without loading the user's config.

```bash
python3 tests/codex-responses-completion-proof.py \
  --codex /path/to/codex \
  --baseline 167d3867819e10da479460674bedf45120aa1e0a \
  --output /tmp/codex-completion-proof
```

The five cases cover valid usage, absent usage, null usage, null detail
counters, and a genuinely truncated stream. Assertions check exit status,
answer content, completion/failure events, reconnects, and actual gateway and
upstream Responses request counts. Chat fallback is forbidden. Add
`--case null-usage --retries 5` to reproduce all five retries from the report.
The output directory must be new. It retains both SSE boundaries, request
fingerprints, raw Codex stdout/stderr, test logs, source snapshots, version and
binary hashes, and SHA256SUMS; upstream fixtures must match byte for byte.

The Go experiment is opt-in (`TINGLY_CODEX_BIN`); ordinary test runs skip it.
Recorded results and the complete proof archive are under
[`evidence/`](evidence/). These prove the synthetic failure and fix, not the
unobserved cause of any particular user's deployment.

## Quick Start

### Automated Test (Recommended)

```bash
export TINGLY_BOX_TEST_CONFIG=/path/to/isolated-test-fixture.json
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
test_dir="$(mktemp -d)"
tingly-box start --port 12581 --config-dir "$test_dir" --browser=false

# Terminal 2: Run tests
TINGLY_BOX_API_KEY=test-token python3 -m tests.runner --verbose
```

## Test Configuration

### Test Server (Port 12581)

- **Config**: generated under a temporary directory from the explicitly supplied test fixture
- **Usage**: Automated via `task tests:interface`
- **Scenarios**: openai, anthropic (generated from fixture providers)

Tests never auto-discover, copy, or modify the developer configuration. Provide
an isolated fixture through `TINGLY_BOX_TEST_CONFIG`; web-search mutation tests
instead require an isolated directory through `TINGLY_BOX_TEST_CONFIG_DIR`.

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
