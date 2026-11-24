"""The SDK's translation wire contract, including actionable batch errors."""
import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Client, TinglyError  # noqa: E402


class TranslationSDKTest(unittest.TestCase):
    def setUp(self):
        self.calls = []
        calls = self.calls

        class Gateway(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                calls.append((self.path, self.headers["Authorization"], body))
                if body["target_lang"] == "unsupported":
                    status, response = 400, {"error": {
                        "type": "translation_error", "code": "invalid_request",
                        "message": "unsupported language", "input_index": 0,
                    }}
                else:
                    status, response = 200, {"model": body["model"], "translations": [
                        {"text": "你好，世界！"}, {"text": "保存更改"},
                    ]}
                data = json.dumps(response, ensure_ascii=False).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

        self.gateway = HTTPServer(("127.0.0.1", 0), Gateway)
        self.thread = threading.Thread(target=self.gateway.serve_forever, daemon=True)
        self.thread.start()
        self.client = Client(f"http://127.0.0.1:{self.gateway.server_port}/", "gateway-token", admin_token="admin-token")

    def tearDown(self):
        self.gateway.shutdown()
        self.gateway.server_close()
        self.thread.join()

    def test_ordered_batch_uses_gateway_auth_and_translation_contract(self):
        result = self.client.translate(["Hello, world!", "Save changes"], "zh-Hans")
        self.assertEqual(result["translations"], [{"text": "你好，世界！"}, {"text": "保存更改"}])
        self.assertEqual(self.calls, [("/tingly/custom/v1/translate", "Bearer gateway-token", {
            "model": "translate", "text": ["Hello, world!", "Save changes"],
            "source_lang": "auto", "target_lang": "zh-Hans",
        })])

    def test_scenario_and_model_overrides_do_not_change_native_wire_shape(self):
        self.client.translate(["你好"], "en", source_lang="zh-Hans", model="my-translator", scenario="openai")
        path, _, body = self.calls[-1]
        self.assertEqual(path, "/tingly/openai/v1/translate")
        self.assertEqual(body["model"], "my-translator")
        self.assertEqual(body["source_lang"], "zh-Hans")
        self.assertNotIn("messages", body)

    def test_failed_batch_preserves_http_status_and_input_index(self):
        with self.assertRaises(TinglyError) as caught:
            self.client.translate(["hello"], "unsupported")
        self.assertEqual(caught.exception.status, 400)
        self.assertEqual(json.loads(caught.exception.body)["error"]["input_index"], 0)
