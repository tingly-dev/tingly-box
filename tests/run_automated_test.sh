#!/bin/bash
# Simple wrapper for Python-based test automation
# Usage: ./tests/run_automated_test.sh

exec python3 -m tests.test_automation "$@"
