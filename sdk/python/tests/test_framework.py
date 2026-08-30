"""End-to-end test of the v1 framework loop:

    test client --(A)--> tingly.Server --(B, via .tb)--> stub tb --(back)-->

(A) exercises the Server half (receive, dispatch to the registered
handler). (B) exercises the Client half (call out, parse an OpenAI-shaped
response). A passing run proves the loop the design doc describes actually
closes, without needing a real tingly-box instance.
"""

import json
import os
import sys
import threading
import time
import unittest
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Client, Server, TinglyError, text_of  # noqa: E402


def _wait_until_serving(srv: Server, timeout: float = 2.0):
    deadline = time.time() + timeout
    while srv._httpd is None:
        if time.time() > deadline:
            raise TimeoutError("server did not start")
        time.sleep(0.01)


class StubTB(BaseHTTPRequestHandler):
    """Stands in for a real tingly-box gateway: echoes the last message back
    as an OpenAI-shaped chat completion, and records the scenario/path it
    was called on."""

    calls = []

    def log_message(self, fmt, *args):
        pass

    def do_POST(self):
        assert self.path == "/tingly/custom/v1/chat/completions"
        assert self.headers.get("Authorization") == "Bearer test-token"
        length = int(self.headers.get("Content-Length", 0))
        body = json.loads(self.rfile.read(length))
        StubTB.calls.append(body)
        last = body["messages"][-1]["content"]
        response = {
            "id": "chatcmpl-stub",
            "object": "chat.completion",
            "model": body["model"],
            "choices": [{"index": 0, "message": {"role": "assistant", "content": f"echo: {last}"}}],
        }
        payload = json.dumps(response).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


class FrameworkTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        StubTB.calls = []
        cls.stub = HTTPServer(("127.0.0.1", 0), StubTB)
        cls.stub_port = cls.stub.server_address[1]
        threading.Thread(target=cls.stub.serve_forever, daemon=True).start()

        cls.srv = Server(
            "relay-test",
            tb_base_url=f"http://127.0.0.1:{cls.stub_port}",
            tb_token="test-token",
        )

        @cls.srv.chat
        def handle(req):
            return cls.srv.tb.chat(model="downstream-model", messages=req.raw["messages"])

        threading.Thread(target=cls.srv.run, kwargs={"host": "127.0.0.1", "port": 0}, daemon=True).start()
        _wait_until_serving(cls.srv)
        cls.srv_port = cls.srv._httpd.server_address[1]

    @classmethod
    def tearDownClass(cls):
        cls.srv._httpd.shutdown()
        cls.stub.shutdown()

    def test_models_endpoint_advertises_the_server_name(self):
        with urllib.request.urlopen(f"http://127.0.0.1:{self.srv_port}/v1/models") as resp:
            body = json.loads(resp.read())
        self.assertEqual(body["data"][0]["id"], "relay-test")

    def test_relay_closes_the_loop_through_the_stub_gateway(self):
        request = urllib.request.Request(
            f"http://127.0.0.1:{self.srv_port}/v1/chat/completions",
            data=json.dumps({
                "model": "relay-test",
                "messages": [{"role": "user", "content": "hello"}],
            }).encode(),
            method="POST",
            headers={"Content-Type": "application/json"},
        )
        with urllib.request.urlopen(request) as resp:
            body = json.loads(resp.read())

        self.assertEqual(text_of(body), "echo: hello")
        self.assertEqual(StubTB.calls[-1]["model"], "downstream-model")

    def test_client_wraps_a_non_2xx_response_as_tingly_error(self):
        class AlwaysBadRequest(BaseHTTPRequestHandler):
            def log_message(self, fmt, *args):
                pass

            def do_POST(self):
                payload = b'{"error": "nope"}'
                self.send_response(400)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(payload)))
                self.end_headers()
                self.wfile.write(payload)

        server = HTTPServer(("127.0.0.1", 0), AlwaysBadRequest)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            client = Client(base_url=f"http://127.0.0.1:{server.server_address[1]}", token="unused")
            with self.assertRaises(TinglyError) as ctx:
                client.chat(model="m", messages=[{"role": "user", "content": "x"}])
            self.assertEqual(ctx.exception.status, 400)
        finally:
            server.shutdown()


if __name__ == "__main__":
    unittest.main()
