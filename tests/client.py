"""
Provider client implementations for testing AI API providers.
"""

import json
import time
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from typing import Any, Optional
from enum import Enum
import httpx


class ProviderType(str, Enum):
    """Provider type enumeration."""
    OPENAI = "openai"
    ANTHROPIC = "anthropic"
    GOOGLE = "google"
    PROXY = "proxy"


@dataclass
class ChatMessage:
    """Chat message."""
    role: str
    content: str
    name: Optional[str] = None


@dataclass
class ChatRequest:
    """Chat completion request."""
    model: str
    messages: list[ChatMessage]
    temperature: Optional[float] = None
    max_tokens: Optional[int] = None
    stream: bool = False
    extra_params: dict = field(default_factory=dict)


@dataclass
class TestResult:
    """Test result."""
    success: bool
    provider: str
    test_type: str
    message: str
    duration_ms: float
    data: Optional[dict] = None
    error: Optional[str] = None
    raw_response: Optional[Any] = None


class BaseProviderClient(ABC):
    """Base class for provider clients."""

    def __init__(
        self,
        name: str,
        api_base: str,
        token: str,
        provider_type: ProviderType,
        proxy_url: Optional[str] = None,
        timeout: int = 60,
    ):
        self.name = name
        self.api_base = api_base.rstrip("/")
        self.token = token
        self.provider_type = provider_type
        self.proxy_url = proxy_url
        self.timeout = timeout

        if proxy_url:
            self.transport = httpx.HTTPProxyTransport(proxy_url=proxy_url)
        else:
            self.transport = None

    def _create_client(self) -> httpx.Client:
        """Create HTTP client."""
        timeout = httpx.Timeout(self.timeout)
        return httpx.Client(timeout=timeout, transport=self.transport)

    def _create_headers(self) -> dict:
        """Create request headers."""
        return {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
            "User-Agent": "Tingly-Box-Test/1.0",
        }

    @abstractmethod
    def list_models(self) -> TestResult:
        """List available models."""
        pass

    @abstractmethod
    def chat_completions(self, request: ChatRequest) -> TestResult:
        """Send chat completion request."""
        pass

    @abstractmethod
    def get_api_endpoint(self, endpoint: str) -> str:
        """Get full API endpoint URL."""
        pass


class OpenAIClient(BaseProviderClient):
    """OpenAI-compatible API client."""

    def __init__(
        self,
        name: str,
        api_base: str,
        token: str,
        proxy_url: Optional[str] = None,
        timeout: int = 60,
    ):
        super().__init__(name, api_base, token, ProviderType.OPENAI, proxy_url, timeout)

    def get_api_endpoint(self, endpoint: str) -> str:
        """Get OpenAI API endpoint."""
        # Remove trailing /v1 from api_base if present to avoid double /v1/v1
        base = self.api_base.rstrip("/")
        if base.endswith("/v1"):
            return f"{base}/{endpoint}"
        return f"{base}/v1/{endpoint}"

    def list_models(self) -> TestResult:
        """List models using OpenAI API."""
        start_time = time.time()

        try:
            with self._create_client() as client:
                response = client.get(
                    self.get_api_endpoint("models"),
                    headers=self._create_headers(),
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                models = [m["id"] for m in data.get("data", [])]
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="list_models",
                    message=f"Successfully listed {len(models)} models",
                    duration_ms=duration_ms,
                    data={"models": models},
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="list_models",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="list_models",
                message="Failed to list models",
                duration_ms=duration_ms,
                error=str(e),
            )

    def chat_completions(self, request: ChatRequest) -> TestResult:
        """Send chat completion request."""
        start_time = time.time()

        try:
            payload = {
                "model": request.model,
                "messages": [{"role": m.role, "content": m.content} for m in request.messages],
                "stream": request.stream,
            }

            if request.temperature is not None:
                payload["temperature"] = request.temperature
            if request.max_tokens is not None:
                payload["max_tokens"] = request.max_tokens
            payload.update(request.extra_params)

            with self._create_client() as client:
                response = client.post(
                    self.get_api_endpoint("chat/completions"),
                    headers=self._create_headers(),
                    json=payload,
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="chat_completions",
                    message="Chat completion successful",
                    duration_ms=duration_ms,
                    data={
                        "id": data.get("id"),
                        "model": data.get("model"),
                        "choices_count": len(data.get("choices", [])),
                        "usage": data.get("usage"),
                    },
                    raw_response=data,
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="chat_completions",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="chat_completions",
                message="Chat completion failed",
                duration_ms=duration_ms,
                error=str(e),
            )


class AnthropicClient(BaseProviderClient):
    """Anthropic Messages API client."""

    def __init__(
        self,
        name: str,
        api_base: str,
        token: str,
        proxy_url: Optional[str] = None,
        timeout: int = 60,
    ):
        super().__init__(name, api_base, token, ProviderType.ANTHROPIC, proxy_url, timeout)

    def get_api_endpoint(self, endpoint: str) -> str:
        """Get Anthropic API endpoint."""
        # Remove trailing /v1 from api_base if present to avoid double /v1/v1
        base = self.api_base.rstrip("/")
        if base.endswith("/v1"):
            return f"{base}/{endpoint}"
        return f"{base}/v1/{endpoint}"

    def _create_headers(self) -> dict:
        """Create Anthropic-specific headers."""
        return {
            "x-api-key": self.token,
            "Content-Type": "application/json",
            "User-Agent": "Tingly-Box-Test/1.0",
            "Anthropic-Version": "2023-06-01",
        }

    def list_models(self) -> TestResult:
        """List models using Anthropic API."""
        start_time = time.time()

        try:
            with self._create_client() as client:
                response = client.get(
                    f"{self.api_base}/v1/models",
                    headers=self._create_headers(),
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                models = [m["id"] for m in data.get("data", [])]
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="list_models",
                    message=f"Successfully listed {len(models)} models",
                    duration_ms=duration_ms,
                    data={"models": models},
                )
            elif response.status_code == 404:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="list_models",
                    message="Models endpoint not available",
                    duration_ms=duration_ms,
                    error="Anthropic models endpoint not available (404)",
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="list_models",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="list_models",
                message="Failed to list models",
                duration_ms=duration_ms,
                error=str(e),
            )

    def chat_completions(self, request: ChatRequest) -> TestResult:
        """Send Anthropic messages API request."""
        start_time = time.time()

        try:
            system_message = None
            anthropic_messages = []

            for msg in request.messages:
                if msg.role == "system":
                    system_message = msg.content
                else:
                    anthropic_messages.append({"role": msg.role, "content": msg.content})

            payload = {
                "model": request.model,
                "messages": anthropic_messages,
                "stream": request.stream,
            }

            if system_message:
                payload["system"] = system_message
            if request.temperature is not None:
                payload["temperature"] = request.temperature
            if request.max_tokens is not None:
                payload["max_tokens"] = request.max_tokens
            payload.update(request.extra_params)

            with self._create_client() as client:
                response = client.post(
                    self.get_api_endpoint("messages"),
                    headers=self._create_headers(),
                    json=payload,
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="messages",
                    message="Anthropic messages API successful",
                    duration_ms=duration_ms,
                    data={
                        "id": data.get("id"),
                        "model": data.get("model"),
                        "stop_reason": data.get("stop_reason"),
                    },
                    raw_response=data,
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="messages",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="messages",
                message="Anthropic messages API failed",
                duration_ms=duration_ms,
                error=str(e),
            )


class GoogleClient(BaseProviderClient):
    """Google Gemini API client."""

    def __init__(
        self,
        name: str,
        api_base: str,
        token: str,
        proxy_url: Optional[str] = None,
        timeout: int = 60,
    ):
        super().__init__(name, api_base, token, ProviderType.GOOGLE, proxy_url, timeout)

    def get_api_endpoint(self, endpoint: str) -> str:
        """Get Google API endpoint."""
        return f"{self.api_base}/v1beta/{endpoint}"

    def _create_headers(self) -> dict:
        """Create Google-specific headers."""
        return {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }

    def list_models(self) -> TestResult:
        """List models using Google API."""
        start_time = time.time()

        try:
            with self._create_client() as client:
                response = client.get(
                    f"{self.api_base}/v1beta/models",
                    headers=self._create_headers(),
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                models = [m["name"].split("/")[-1] for m in data.get("models", [])]
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="list_models",
                    message=f"Successfully listed {len(models)} models",
                    duration_ms=duration_ms,
                    data={"models": models},
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="list_models",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="list_models",
                message="Failed to list models",
                duration_ms=duration_ms,
                error=str(e),
            )

    def chat_completions(self, request: ChatRequest) -> TestResult:
        """Send Google generate content request."""
        start_time = time.time()

        try:
            contents = []
            for msg in request.messages:
                parts = [{"text": msg.content}]
                contents.append({"role": msg.role, "parts": parts})

            payload = {"contents": contents}

            if request.temperature is not None:
                payload["temperature"] = request.temperature
            if request.max_tokens is not None:
                payload["max_output_tokens"] = request.max_tokens
            payload.update(request.extra_params)

            model_path = f"models/{request.model}"
            with self._create_client() as client:
                response = client.post(
                    f"{self.api_base}/v1beta/{model_path}:generateContent",
                    headers=self._create_headers(),
                    json=payload,
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return TestResult(
                    success=True,
                    provider=self.name,
                    test_type="generateContent",
                    message="Google generate content successful",
                    duration_ms=duration_ms,
                    data={
                        "model": request.model,
                        "prompt_feedback": data.get("promptFeedback"),
                    },
                    raw_response=data,
                )
            else:
                return TestResult(
                    success=False,
                    provider=self.name,
                    test_type="generateContent",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider=self.name,
                test_type="generateContent",
                message="Google generate content failed",
                duration_ms=duration_ms,
                error=str(e),
            )


class ProxyClient:
    """Client for testing through tingly-box proxy."""

    def __init__(
        self,
        server_url: str,
        token: str = "",
        timeout: int = 60,
    ):
        self.server_url = server_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self._client: Optional[httpx.Client] = None

    def __enter__(self) -> "ProxyClient":
        self._client = self._create_client()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> None:
        if self._client:
            self._client.close()
            self._client = None

    def close(self) -> None:
        if self._client:
            self._client.close()
            self._client = None

    def _create_client(self) -> httpx.Client:
        timeout = httpx.Timeout(self.timeout)
        return httpx.Client(timeout=timeout)

    def _get_client(self) -> httpx.Client:
        if self._client is None:
            return self._create_client()
        return self._client

    def _create_headers(self) -> dict:
        headers = {
            "Content-Type": "application/json",
            "User-Agent": "Tingly-Box-Test/1.0",
        }
        if self.token:
            bearer_token = self.token
            raw_token = self.token
            if self.token.startswith("tingly-box-"):
                raw_token = self.token[len("tingly-box-"):]
            # Send both forms to maximize compatibility with server config.
            headers["Authorization"] = f"Bearer {bearer_token}"
            headers["X-Api-Key"] = raw_token
        return headers

    def list_models_openai(self, scenario: Optional[str] = None) -> TestResult:
        """List models via OpenAI endpoint."""
        start_time = time.time()

        try:
            with self._create_client() as client:
                if scenario:
                    url = f"{self.server_url}/tingly/{scenario}/models"
                else:
                    url = f"{self.server_url}/openai/v1/models"
                response = client.get(
                    url,
                    headers=self._create_headers(),
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                models = [m["id"] for m in data.get("data", [])]
                return TestResult(
                    success=True,
                    provider="proxy_openai",
                    test_type="list_models",
                    message=f"Successfully listed {len(models)} models",
                    duration_ms=duration_ms,
                    data={"models": models},
                )
            else:
                return TestResult(
                    success=False,
                    provider="proxy_openai",
                    test_type="list_models",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider="proxy_openai",
                test_type="list_models",
                message="Failed to list models",
                duration_ms=duration_ms,
                error=str(e),
            )

    def list_models_anthropic(self, scenario: Optional[str] = None) -> TestResult:
        """List models via Anthropic endpoint."""
        start_time = time.time()

        try:
            with self._create_client() as client:
                if scenario:
                    url = f"{self.server_url}/tingly/{scenario}/models"
                else:
                    url = f"{self.server_url}/anthropic/v1/models"
                response = client.get(
                    url,
                    headers=self._create_headers(),
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                models = [m["id"] for m in data.get("data", [])]
                return TestResult(
                    success=True,
                    provider="proxy_anthropic",
                    test_type="list_models",
                    message=f"Successfully listed {len(models)} models",
                    duration_ms=duration_ms,
                    data={"models": models},
                )
            else:
                return TestResult(
                    success=False,
                    provider="proxy_anthropic",
                    test_type="list_models",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider="proxy_anthropic",
                test_type="list_models",
                message="Failed to list models",
                duration_ms=duration_ms,
                error=str(e),
            )

    def chat_completions_openai(self, model: str, prompt: str, scenario: Optional[str] = None, **kwargs) -> TestResult:
        """Send chat completion via OpenAI endpoint."""
        start_time = time.time()

        try:
            payload = {
                "model": model,
                "messages": [{"role": "user", "content": prompt}],
                **kwargs,
            }

            # Use scenario-based route if scenario provided
            if scenario:
                url = f"{self.server_url}/tingly/{scenario}/chat/completions"
            else:
                url = f"{self.server_url}/openai/v1/chat/completions"

            with self._create_client() as client:
                response = client.post(
                    url,
                    headers=self._create_headers(),
                    json=payload,
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return TestResult(
                    success=True,
                    provider="proxy_openai",
                    test_type="chat_completions",
                    message="Chat completion successful",
                    duration_ms=duration_ms,
                    data={
                        "id": data.get("id"),
                        "model": data.get("model"),
                        "choices_count": len(data.get("choices", [])),
                    },
                    raw_response=data,
                )
            else:
                return TestResult(
                    success=False,
                    provider="proxy_openai",
                    test_type="chat_completions",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider="proxy_openai",
                test_type="chat_completions",
                message="Chat completion failed",
                duration_ms=duration_ms,
                error=str(e),
            )

    def messages_anthropic(self, model: str, prompt: str, scenario: Optional[str] = None, **kwargs) -> TestResult:
        """Send messages request via Anthropic endpoint."""
        start_time = time.time()

        try:
            payload = {
                "model": model,
                "messages": [{"role": "user", "content": prompt}],
                **kwargs,
            }

            # Use scenario-based route if scenario provided
            if scenario:
                url = f"{self.server_url}/tingly/{scenario}/messages"
            else:
                url = f"{self.server_url}/anthropic/v1/messages"

            with self._create_client() as client:
                response = client.post(
                    url,
                    headers=self._create_headers(),
                    json=payload,
                )

            duration_ms = (time.time() - start_time) * 1000

            if response.status_code == 200:
                data = response.json()
                return TestResult(
                    success=True,
                    provider="proxy_anthropic",
                    test_type="messages",
                    message="Anthropic messages API successful",
                    duration_ms=duration_ms,
                    data={
                        "id": data.get("id"),
                        "model": data.get("model"),
                    },
                    raw_response=data,
                )
            else:
                return TestResult(
                    success=False,
                    provider="proxy_anthropic",
                    test_type="messages",
                    message=f"API returned status {response.status_code}",
                    duration_ms=duration_ms,
                    error=response.text[:500],
                )

        except Exception as e:
            duration_ms = (time.time() - start_time) * 1000
            return TestResult(
                success=False,
                provider="proxy_anthropic",
                test_type="messages",
                message="Anthropic messages API failed",
                duration_ms=duration_ms,
                error=str(e),
            )
