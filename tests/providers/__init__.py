#!/usr/bin/env python3
"""
Tingly-Box Test System

A comprehensive smoke test and differential test suite for AI provider proxies.
Tests:
1. Model fetching for each provider
2. Chat/completions (OpenAI/Anthropic/Google style)
3. Adaptor transformation (cross-provider)
4. Differential testing (O→A→O roundtrip verification)
"""

__version__ = "1.0.0"

from .config import ConfigLoader, TestConfig, load_config
from .client import BaseProviderClient, OpenAIClient, AnthropicClient, GoogleClient
from .smoke import SmokeTestSuite, ProxySmokeTestSuite
from .adaptor import AdaptorTestSuite
from .differential import DifferentialTestSuite, DifferentialResult
from .runner import TestRunner, main

__all__ = [
    "ConfigLoader",
    "TestConfig",
    "load_config",
    "BaseProviderClient",
    "OpenAIClient",
    "AnthropicClient",
    "GoogleClient",
    "SmokeTestSuite",
    "ProxySmokeTestSuite",
    "AdaptorTestSuite",
    "DifferentialTestSuite",
    "DifferentialResult",
    "TestRunner",
    "main",
]
