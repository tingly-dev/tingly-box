"""End to end through a real tb: every hop between a client and a plugin is
tb's own code — routing, its upstream clients, its multipart encoding, its
stream re-emission, its image persistence.

Skipped unless TINGLY_TB_BIN points at a tb binary (`task test:py:e2e`
builds one and runs this). The plugins run in-process; tb runs with a
throwaway --config-dir on a free port and is set up through its admin API
exactly as a user would: one Custom endpoint provider (OpenAI, no key),
then rules pointing tb model names at the plugin's models.
"""

import base64
import importlib.util
import json
import os
import shutil
import socket
import struct
import subprocess
import sys
import tempfile
import time
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import tingly  # noqa: E402
from tingly import sugar  # noqa: E402
from helpers import post_json, post_multipart, post_sse, serve_sugar_in_background, stop_sugar  # noqa: E402

TB_BIN = os.environ.get("TINGLY_TB_BIN")
EXAMPLE = os.path.join(os.path.dirname(__file__), "..", "examples", "image.py")
PNG_SIGNATURE = b"\x89PNG\r\n\x1a\n"


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def text_in(content) -> str:
    """A Chat message's text, whether tb sent it as a string or as parts."""
    if isinstance(content, str):
        return content
    return "".join(part.get("text", "") for part in content)


@unittest.skipUnless(TB_BIN, "set TINGLY_TB_BIN to a tb binary to run the end-to-end test")
class EndToEndThroughTB(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.seen = {}
        cls.plugin_base = cls.start_plugins()
        cls.confdir = tempfile.mkdtemp(prefix="tingly-e2e-")
        try:
            cls.start_tb()
            cls.register()
        except BaseException:
            cls.tearDownClass()
            raise

    @classmethod
    def tearDownClass(cls):
        tb = getattr(cls, "tb", None)
        if tb is not None:
            tb.terminate()
            try:
                tb.wait(timeout=10)
            except subprocess.TimeoutExpired:
                tb.kill()
            cls.tb_log.close()
        shutil.rmtree(cls.confdir, ignore_errors=True)
        stop_sugar()

    @classmethod
    def start_plugins(cls) -> str:
        sugar._reset()
        spec = importlib.util.spec_from_file_location("image_example", EXAMPLE)
        spec.loader.exec_module(importlib.util.module_from_spec(spec))  # registers "fake-image", unchanged

        @tingly.image_edit("fake-image")
        def edit(prompt, images, mask=None):
            cls.seen["edit"] = (prompt, images, mask)
            return images[-1]

        @tingly.text("fake-chat")
        def reply(messages):
            cls.seen["text"] = messages
            return "echo: " + text_in(messages[-1]["content"])

        return serve_sugar_in_background()

    @classmethod
    def start_tb(cls):
        port = free_port()
        cls.tb_base = f"http://127.0.0.1:{port}"
        cls.tb_log = open(os.path.join(cls.confdir, "tb-stdout.log"), "wb")
        cls.tb = subprocess.Popen(
            [TB_BIN, "--config-dir", cls.confdir, "start", "--no-daemon",
             "--port", str(port), "--host", "127.0.0.1", "--browser=false"],
            stdout=cls.tb_log, stderr=subprocess.STDOUT,
        )
        deadline = time.time() + 60
        while True:
            if cls.tb.poll() is not None:
                raise RuntimeError(f"tb exited with {cls.tb.returncode}; see {cls.tb_log.name}")
            if time.time() > deadline:
                raise TimeoutError("tb did not come up within 60s")
            try:
                with open(os.path.join(cls.confdir, "config.json")) as f:
                    config = json.load(f)
                with socket.create_connection(("127.0.0.1", port), timeout=1):
                    pass
                if config.get("user_token") and config.get("model_token"):
                    break
            except (OSError, ValueError):
                pass
            time.sleep(0.2)
        cls.admin = {"Authorization": f"Bearer {config['user_token']}"}
        cls.gateway = {"Authorization": f"Bearer {config['model_token']}"}

    @classmethod
    def register(cls):
        provider = post_json(f"{cls.tb_base}/api/v2/providers", {
            "name": "tingly-e2e-plugin", "api_base": f"{cls.plugin_base}/v1",
            "api_style": "openai", "no_key_required": True, "token": "",
        }, cls.admin)
        cls.provider_uuid = provider["data"]["uuid"]
        cls.discovered = post_json(f"{cls.tb_base}/api/v2/provider-models/{cls.provider_uuid}", {}, cls.admin)
        for scenario, tb_model, plugin_model in [
            ("imagegen", "my-image", "fake-image"),
            ("openai", "my-chat", "fake-chat"),
            ("anthropic", "my-chat", "fake-chat"),
        ]:
            post_json(f"{cls.tb_base}/api/v1/rule", {
                "scenario": scenario, "request_model": tb_model, "response_model": tb_model, "active": True,
                "services": [{"provider": cls.provider_uuid, "model": plugin_model, "weight": 1, "active": True}],
            }, cls.admin)

    def test_tb_discovers_every_plugin_model(self):
        self.assertIn("fake-image", json.dumps(self.discovered))
        self.assertIn("fake-chat", json.dumps(self.discovered))

    def test_image_generation_through_tb_and_tb_keeps_the_png(self):
        body = post_json(f"{self.tb_base}/tingly/imagegen/v1/images/generations",
                         {"model": "my-image", "prompt": "a red fox", "size": "48x32"}, self.gateway)
        png = base64.b64decode(body["data"][0]["b64_json"])
        self.assertEqual(png[:8], PNG_SIGNATURE)
        self.assertEqual(struct.unpack(">II", png[16:24]), (48, 32))
        saved = []
        for root, _, names in os.walk(os.path.join(self.confdir, "image")):
            for name in names:
                with open(os.path.join(root, name), "rb") as f:
                    saved.append(f.read())
        self.assertIn(png, saved)

    def test_image_edit_through_tb_arrives_as_multipart_with_every_image_and_the_mask(self):
        body = post_multipart(f"{self.tb_base}/tingly/imagegen/v1/images/edits", [
            ("model", "my-image"), ("prompt", "make it blue"),
            ("image[]", b"IMG-A"), ("image[]", b"IMG-B"), ("mask", b"MASK"),
        ], self.gateway)
        self.assertEqual(self.seen["edit"], ("make it blue", [b"IMG-A", b"IMG-B"], b"MASK"))
        self.assertEqual(base64.b64decode(body["data"][0]["b64_json"]), b"IMG-B")

    def chat_request(self, text, **extra):
        return {"model": "my-chat", "messages": [{"role": "user", "content": text}], **extra}

    def test_chat_through_tb(self):
        body = post_json(f"{self.tb_base}/tingly/openai/v1/chat/completions",
                         self.chat_request("hello"), self.gateway)
        self.assertEqual(body["choices"][0]["message"]["content"], "echo: hello")

    def test_chat_streams_through_tb(self):
        _, events = post_sse(f"{self.tb_base}/tingly/openai/v1/chat/completions",
                             self.chat_request("stream me", stream=True), self.gateway)
        text = "".join(choice["delta"].get("content") or ""
                       for _, data in events if isinstance(data, dict) for choice in data.get("choices", []))
        self.assertEqual(text, "echo: stream me")
        self.assertEqual(events[-1], (None, "[DONE]"))

    def anthropic_request(self, text, **extra):
        return {"model": "my-chat", "max_tokens": 64, "messages": [{"role": "user", "content": text}], **extra}

    def test_an_anthropic_client_reaches_a_chat_only_plugin(self):
        body = post_json(f"{self.tb_base}/tingly/anthropic/v1/messages",
                         self.anthropic_request("hi from claude"), self.gateway)
        self.assertEqual(body["content"][0]["text"], "echo: hi from claude")

    def test_an_anthropic_client_streams_from_a_chat_only_plugin(self):
        _, events = post_sse(f"{self.tb_base}/tingly/anthropic/v1/messages",
                             self.anthropic_request("stream from claude", stream=True), self.gateway)
        text = "".join(data["delta"].get("text", "") for event, data in events if event == "content_block_delta")
        self.assertEqual(text, "echo: stream from claude")
        self.assertEqual(events[-1][0], "message_stop")


if __name__ == "__main__":
    unittest.main()
