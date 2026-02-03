"""
Configuration loader for tingly-box test system.
"""

import json
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional
from enum import Enum


class APIStyle(str, Enum):
    """API style enumeration."""
    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    GOOGLE = "google"


class AuthType(str, Enum):
    """Authentication type enumeration."""
    API_KEY = "api_key"
    OAUTH = "oauth"


@dataclass
class OAuthDetail:
    """OAuth configuration details."""
    client_id: str = ""
    client_secret: str = ""
    refresh_token: str = ""


@dataclass
class Provider:
    """Provider configuration."""
    uuid: str = ""
    name: str = ""
    api_base: str = ""
    api_style: APIStyle = APIStyle.OPENAI
    token: str = ""
    auth_type: AuthType = AuthType.API_KEY
    oauth_detail: Optional[OAuthDetail] = None
    proxy_url: str = ""
    timeout: int = 60
    models: list[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, data: dict) -> "Provider":
        """Create Provider from dictionary."""
        api_style_str = data.get("api_style", "openai")
        if not api_style_str:
            api_style_str = "openai"
        api_style = APIStyle(api_style_str)

        auth_type_str = data.get("auth_type", "api_key")
        if not auth_type_str:
            auth_type_str = "api_key"
        auth_type = AuthType(auth_type_str)

        oauth_detail = None
        if data.get("oauth_detail"):
            oauth_detail = OAuthDetail(**data["oauth_detail"])

        return cls(
            uuid=data.get("uuid", ""),
            name=data.get("name", ""),
            api_base=data.get("api_base", ""),
            api_style=api_style,
            token=data.get("token", ""),
            auth_type=auth_type,
            oauth_detail=oauth_detail,
            proxy_url=data.get("proxy_url", ""),
            timeout=data.get("timeout", 60),
            models=data.get("models", []),
        )


@dataclass
class Rule:
    """Routing rule configuration."""
    uuid: str = ""
    scenario: str = ""
    request_model: str = ""
    response_model: str = ""
    services: list = field(default_factory=list)
    lb_tactic: str = "round_robin"
    active: bool = True
    smart_enabled: bool = False


@dataclass
class TestConfig:
    """Test configuration for the test system."""
    providers: list[Provider] = field(default_factory=list)
    rules: list[Rule] = field(default_factory=list)
    server_url: str = "http://localhost:12580"
    auth_token: str = ""  # Model token for proxy authentication
    test_model: str = ""
    test_prompt: str = "Hello, this is a test message. Please respond briefly."
    timeout: int = 60
    verbose: bool = False
    output_dir: str = "./test_results"

    @property
    def provider_names(self) -> list[str]:
        """Get list of provider names."""
        return [p.name for p in self.providers]

    def get_provider(self, name: str) -> Optional[Provider]:
        """Get provider by name."""
        for p in self.providers:
            if p.name == name:
                return p
        return None


class ConfigLoader:
    """Load and parse tingly-box configuration files."""

    def __init__(self, config_path: Optional[str] = None):
        """
        Initialize config loader.

        Args:
            config_path: Path to config file. If None, uses default locations.
        """
        self.config_path = config_path

    def find_config(self) -> Optional[Path]:
        """Find config file in default locations."""
        if self.config_path:
            path = Path(self.config_path)
            if path.exists():
                return path

        # Default locations
        default_locations = [
            Path(os.environ.get("TINGLY_BOX_CONFIG", "")),
            Path.home() / ".tingly-box" / "config.json",
            Path.cwd() / "config.json",
            Path("/root/projects/tingly-box/build/config.yml"),
        ]

        for loc in default_locations:
            if loc.exists() and loc.stat().st_size > 0:
                return loc

        return None

    def load(self) -> TestConfig:
        """
        Load configuration from file.

        Returns:
            TestConfig object with loaded settings.
        """
        config_path = self.find_config()
        if not config_path:
            # Return empty config with defaults
            return TestConfig()

        # Determine file type and load
        if config_path.suffix == ".json":
            return self._load_json(config_path)
        elif config_path.suffix in (".yml", ".yaml"):
            return self._load_yaml(config_path)
        else:
            # Try JSON first, then YAML
            try:
                return self._load_json(config_path)
            except Exception:
                return self._load_yaml(config_path)

    def _load_json(self, path: Path) -> TestConfig:
        """Load JSON configuration file."""
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)

        providers = []
        # Support both providers and providers_v2 fields
        provider_sources = []
        if "providers_v2" in data:
            provider_sources.append(data["providers_v2"])
        if "providers" in data:
            provider_sources.append(data["providers"])

        for source in provider_sources:
            for p in source:
                try:
                    # Handle provider fields mapping
                    provider_data = dict(p)

                    # Map enabled -> skip if disabled
                    if provider_data.get("enabled") is False:
                        continue

                    # Set default values if not present
                    if "api_style" not in provider_data or not provider_data["api_style"]:
                        provider_data["api_style"] = "openai"

                    providers.append(Provider.from_dict(provider_data))
                except Exception as e:
                    print(f"Warning: Failed to parse provider {p.get('name')}: {e}")

        rules = []
        if "rules" in data:
            for r in data["rules"]:
                try:
                    rules.append(Rule(
                        uuid=r.get("uuid", ""),
                        scenario=r.get("scenario", ""),
                        request_model=r.get("request_model", ""),
                        response_model=r.get("response_model", ""),
                        services=r.get("services", []),
                        lb_tactic=r.get("lb_tactic", "round_robin"),
                        active=r.get("active", True),
                        smart_enabled=r.get("smart_enabled", False),
                    ))
                except Exception as e:
                    print(f"Warning: Failed to parse rule: {e}")

        # Extract server settings
        server_url = "http://localhost:12580"
        if "server_url" in data:
            server_url = data["server_url"]
        elif "ServerPort" in data:
            server_url = f"http://localhost:{data['ServerPort']}"

        # Extract auth token (model_token with optional tingly-box- prefix)
        auth_token = ""
        if "model_token" in data:
            token = data["model_token"]
            if token and not token.startswith("tingly-box-"):
                auth_token = f"tingly-box-{token}"
            else:
                auth_token = token

        return TestConfig(
            providers=providers,
            rules=rules,
            server_url=server_url,
            auth_token=auth_token,
            test_model=data.get("test_model", ""),
            test_prompt=data.get("test_prompt", "Hello, this is a test. Please respond briefly."),
            timeout=data.get("timeout", 60),
            verbose=data.get("verbose", False),
            output_dir=data.get("output_dir", "./test_results"),
        )

    def _load_yaml(self, path: Path) -> TestConfig:
        """Load YAML configuration file."""
        try:
            import yaml
        except ImportError:
            raise ImportError("PyYAML is required for YAML config files. Install with: pip install pyyaml")

        with open(path, "r", encoding="utf-8") as f:
            data = yaml.safe_load(f)

        providers = []
        if "providers" in data:
            for p in data["providers"]:
                p["api_style"] = p.get("api_style", "openai")
                p["auth_type"] = p.get("auth_type", "api_key")
                try:
                    providers.append(Provider.from_dict(p))
                except Exception as e:
                    print(f"Warning: Failed to parse provider {p.get('name')}: {e}")

        rules = []
        if "rules" in data:
            for r in data["rules"]:
                try:
                    rules.append(Rule(
                        uuid=r.get("uuid", ""),
                        scenario=r.get("scenario", ""),
                        request_model=r.get("request_model", ""),
                        response_model=r.get("response_model", ""),
                        services=r.get("services", []),
                        lb_tactic=r.get("lb_tactic", "round_robin"),
                        active=r.get("active", True),
                        smart_enabled=r.get("smart_enabled", False),
                    ))
                except Exception as e:
                    print(f"Warning: Failed to parse rule: {e}")

        return TestConfig(
            providers=providers,
            rules=rules,
            server_url="http://localhost:12580",
            test_model=data.get("test_model", ""),
            test_prompt=data.get("test_prompt", "Hello, this is a test. Please respond briefly."),
            timeout=data.get("timeout", 60),
            verbose=data.get("verbose", False),
            output_dir=data.get("output_dir", "./test_results"),
        )


def load_config(config_path: Optional[str] = None) -> TestConfig:
    """
    Convenience function to load configuration.

    Args:
        config_path: Path to config file (optional).

    Returns:
        TestConfig object.
    """
    loader = ConfigLoader(config_path)
    return loader.load()
