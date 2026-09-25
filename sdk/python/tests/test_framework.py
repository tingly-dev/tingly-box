"""End-to-end test of the v1 framework loop:

    test client --(A)--> tingly.Server --(B, via .tb)--> stub tb --(back)-->

(A) exercises the Server half (receive, dispatch to the registered
handler(s)). (B) exercises the Client half (call out, parse an OpenAI-shaped
response). A passing run proves the loop the design doc describes actually
closes, without needing a real tingly-box instance.
"""

import base64
import json
import os
import sys
import threading
import time
import unittest
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import Client, Server, TinglyError, text_of  # noqa: E402
from helpers import get_json, post_json, post_multipart, post_sse, wait_until_serving  # noqa: E402


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
        def handle_chat(body):
            return cls.srv.tb.chat(model="downstream-model", messages=body["messages"])

        @cls.srv.messages
        def handle_messages(body):
            # Raw passthrough, no normalization: body["messages"] is exactly
            # what the caller sent on the wire.
            return text_of(cls.srv.tb.chat(model="downstream-model", messages=body["messages"]))

        @cls.srv.responses
        def handle_responses(body):
            # A plain string reply — exercises _wrap_text_openai_responses,
            # no round trip through .tb needed for this one.
            return f"responded to: {body['input']}"

        threading.Thread(target=cls.srv.run, kwargs={"host": "127.0.0.1", "port": 0}, daemon=True).start()
        _wait_until_serving(cls.srv)
        cls.srv_port = cls.srv._httpd.server_address[1]

    @classmethod
    def tearDownClass(cls):
        cls.srv._httpd.shutdown()
        cls.stub.shutdown()

    def _post(self, path: str, body: dict) -> dict:
        request = urllib.request.Request(
            f"http://127.0.0.1:{self.srv_port}{path}",
            data=json.dumps(body).encode(),
            method="POST",
            headers={"Content-Type": "application/json"},
        )
        with urllib.request.urlopen(request) as resp:
            return json.loads(resp.read())

    def test_models_endpoint_advertises_the_server_name(self):
        with urllib.request.urlopen(f"http://127.0.0.1:{self.srv_port}/v1/models") as resp:
            body = json.loads(resp.read())
        self.assertEqual(body["data"][0]["id"], "relay-test")

    def test_models_endpoint_also_answers_without_the_v1_prefix(self):
        with urllib.request.urlopen(f"http://127.0.0.1:{self.srv_port}/models") as resp:
            body = json.loads(resp.read())
        self.assertEqual(body["data"][0]["id"], "relay-test")

    def test_chat_completions_closes_the_loop_through_the_stub_gateway(self):
        body = self._post("/v1/chat/completions", {
            "model": "relay-test",
            "messages": [{"role": "user", "content": "hello"}],
        })
        self.assertEqual(text_of(body), "echo: hello")
        self.assertEqual(StubTB.calls[-1]["model"], "downstream-model")

    def test_chat_completions_also_answers_without_the_v1_prefix(self):
        body = self._post("/chat/completions", {
            "model": "relay-test",
            "messages": [{"role": "user", "content": "hi again"}],
        })
        self.assertEqual(text_of(body), "echo: hi again")

    def test_messages_endpoint_gets_the_raw_body_with_no_normalization(self):
        """@srv.messages sees exactly the wire body — including a content
        block, which nothing here flattens — and the reply comes back as a
        plain Anthropic Messages envelope, unconditionally (no v1/beta
        branching)."""
        body = self._post("/v1/messages", {
            "model": "relay-test",
            "max_tokens": 1024,
            "system": "be terse",
            "messages": [{"role": "user", "content": [{"type": "text", "text": "hello"}]}],
        })

        self.assertEqual(body["type"], "message")
        self.assertEqual(body["role"], "assistant")
        # The handler received body["messages"] untouched (content blocks
        # and all) and forwarded that straight to the stub gateway, which
        # only reads the raw content of the last message.
        forwarded = StubTB.calls[-1]["messages"][-1]["content"]
        self.assertEqual(forwarded, [{"type": "text", "text": "hello"}])

    def test_messages_endpoint_also_answers_without_the_v1_prefix(self):
        body = self._post("/messages", {
            "model": "relay-test",
            "max_tokens": 1024,
            "messages": [{"role": "user", "content": "hi"}],
        })
        self.assertEqual(body["content"], [{"type": "text", "text": "echo: hi"}])

    def test_responses_endpoint_gets_the_raw_body_and_wraps_a_string_reply(self):
        """@srv.responses sees the raw OpenAI Responses request (the
        `input` field, not `messages`) and a string reply comes back as a
        minimal, wire-accurate Responses envelope."""
        body = self._post("/v1/responses", {
            "model": "relay-test",
            "input": "hello",
        })

        self.assertEqual(body["object"], "response")
        self.assertEqual(body["status"], "completed")
        self.assertEqual(body["model"], "relay-test")
        self.assertEqual(body["output"][0]["type"], "message")
        self.assertEqual(body["output"][0]["role"], "assistant")
        self.assertEqual(body["output"][0]["content"][0]["type"], "output_text")
        self.assertEqual(body["output"][0]["content"][0]["text"], "responded to: hello")

    def test_responses_endpoint_also_answers_without_the_v1_prefix(self):
        body = self._post("/responses", {"model": "relay-test", "input": "hi again"})
        self.assertEqual(body["output"][0]["content"][0]["text"], "responded to: hi again")

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


class FakePILImage:
    """Stands in for a PIL image: all the raw layer relies on is .save()."""

    def save(self, fp, format):
        fp.write(b"saved-as-" + format.encode())


class ImagesTest(unittest.TestCase):
    """@srv.images / @srv.image_edits: the two image endpoints tb forwards to
    a provider, and the image-to-b64_json wrapping."""

    @classmethod
    def setUpClass(cls):
        cls.srv = Server("img-test")
        cls.seen = {}

        @cls.srv.images
        def handle_images(body):
            cls.seen["images"] = body
            if body["prompt"] == "two":
                return [b"first", FakePILImage()]
            if body["prompt"] == "dict":
                return {"created": 1, "data": [{"url": "http://example/x.png"}]}
            return b"png-bytes"

        @cls.srv.image_edits
        def handle_image_edits(body):
            cls.seen["image_edits"] = body
            return body["image"][0]

        threading.Thread(target=cls.srv.run, kwargs={"host": "127.0.0.1", "port": 0}, daemon=True).start()
        wait_until_serving(cls.srv)
        cls.base = f"http://127.0.0.1:{cls.srv._httpd.server_address[1]}"

    @classmethod
    def tearDownClass(cls):
        cls.srv._httpd.shutdown()

    def test_generation_gets_the_raw_json_body_and_wraps_bytes_as_b64_json(self):
        body = post_json(f"{self.base}/v1/images/generations", {"model": "img-test", "prompt": "a cat", "size": "512x512"})
        self.assertEqual(self.seen["images"], {"model": "img-test", "prompt": "a cat", "size": "512x512"})
        self.assertEqual(base64.b64decode(body["data"][0]["b64_json"]), b"png-bytes")
        self.assertIn("created", body)

    def test_generation_wraps_a_list_and_saves_pil_like_images_as_png(self):
        body = post_json(f"{self.base}/images/generations", {"model": "img-test", "prompt": "two"})
        decoded = [base64.b64decode(item["b64_json"]) for item in body["data"]]
        self.assertEqual(decoded, [b"first", b"saved-as-PNG"])

    def test_generation_passes_a_dict_reply_through(self):
        body = post_json(f"{self.base}/v1/images/generations", {"model": "img-test", "prompt": "dict"})
        self.assertEqual(body, {"created": 1, "data": [{"url": "http://example/x.png"}]})

    def test_edit_decodes_the_multipart_form_and_collects_image_brackets(self):
        """openai-go sends several images as image[] — they, and a single
        image, all land in body["image"]; text fields stay strings."""
        body = post_multipart(f"{self.base}/v1/images/edits", [
            ("model", "img-test"), ("prompt", "make it blue"), ("n", "1"),
            ("image[]", b"img-one"), ("image[]", b"img-two"), ("mask", b"the-mask"),
        ])
        seen = self.seen["image_edits"]
        self.assertEqual(seen["image"], [b"img-one", b"img-two"])
        self.assertEqual(seen["mask"], b"the-mask")
        self.assertEqual((seen["prompt"], seen["n"]), ("make it blue", "1"))
        self.assertEqual(base64.b64decode(body["data"][0]["b64_json"]), b"img-one")

    def test_edit_with_a_single_image_field(self):
        post_multipart(f"{self.base}/images/edits", [("model", "img-test"), ("prompt", "p"), ("image", b"only")])
        self.assertEqual(self.seen["image_edits"]["image"], [b"only"])

    def test_edit_rejects_a_non_multipart_body(self):
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            post_json(f"{self.base}/v1/images/edits", {"prompt": "p"})
        self.assertEqual(ctx.exception.code, 400)

    def test_models_lists_the_advertised_models(self):
        self.srv.models.append("img-test-2")
        try:
            ids = [m["id"] for m in get_json(f"{self.base}/v1/models")["data"]]
        finally:
            self.srv.models.remove("img-test-2")
        self.assertEqual(ids, ["img-test", "img-test-2"])


class StreamingTest(unittest.TestCase):
    """`"stream": true` on the three text endpoints: the handler's complete
    reply comes back as that protocol's SSE, all content in one chunk."""

    TOOL_CALL = {"id": "call_1", "type": "function", "function": {"name": "f", "arguments": '{"x": 1}'}}

    @classmethod
    def setUpClass(cls):
        cls.srv = Server("stream-test")

        @cls.srv.chat
        def handle_chat(body):
            if body["messages"][-1]["content"] == "tools":
                return {"id": "chatcmpl-x", "object": "chat.completion", "created": 1, "model": "m",
                        "choices": [{"index": 0, "finish_reason": "tool_calls",
                                     "message": {"role": "assistant", "content": None, "tool_calls": [cls.TOOL_CALL]}}],
                        "usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}}
            if body["messages"][-1]["content"] == "boom":
                raise RuntimeError("model fell over")
            return "hello there"

        @cls.srv.messages
        def handle_messages(body):
            if body["messages"][-1]["content"] == "tools":
                return {"id": "msg_x", "type": "message", "role": "assistant", "model": "m",
                        "content": [{"type": "text", "text": "calling"},
                                    {"type": "tool_use", "id": "tu_1", "name": "f", "input": {"x": 1}}],
                        "stop_reason": "tool_use", "stop_sequence": None,
                        "usage": {"input_tokens": 3, "output_tokens": 7}}
            return "hello there"

        @cls.srv.responses
        def handle_responses(body):
            return "hello there"

        threading.Thread(target=cls.srv.run, kwargs={"host": "127.0.0.1", "port": 0}, daemon=True).start()
        wait_until_serving(cls.srv)
        cls.base = f"http://127.0.0.1:{cls.srv._httpd.server_address[1]}"

    @classmethod
    def tearDownClass(cls):
        cls.srv._httpd.shutdown()

    def chat(self, content):
        return post_sse(f"{self.base}/v1/chat/completions",
                        {"model": "stream-test", "stream": True, "messages": [{"role": "user", "content": content}]})

    def test_chat_str_reply_streams_as_one_chunk_then_done(self):
        content_type, events = self.chat("hi")
        self.assertEqual(content_type, "text/event-stream")
        self.assertEqual(len(events), 2)
        chunk = events[0][1]
        self.assertEqual(chunk["object"], "chat.completion.chunk")
        self.assertEqual(chunk["choices"][0]["delta"], {"role": "assistant", "content": "hello there"})
        self.assertEqual(chunk["choices"][0]["finish_reason"], "stop")
        self.assertEqual(events[1], (None, "[DONE]"))

    def test_chat_dict_reply_streams_tool_calls_with_an_index_and_a_usage_chunk(self):
        _, events = self.chat("tools")
        delta = events[0][1]["choices"][0]["delta"]
        self.assertEqual(delta["tool_calls"], [{"index": 0, **self.TOOL_CALL}])
        self.assertEqual(events[0][1]["choices"][0]["finish_reason"], "tool_calls")
        self.assertEqual(events[1][1]["choices"], [])
        self.assertEqual(events[1][1]["usage"]["total_tokens"], 5)
        self.assertEqual(events[2], (None, "[DONE]"))

    def test_a_handler_error_is_still_a_plain_500_when_streaming(self):
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            self.chat("boom")
        self.assertEqual(ctx.exception.code, 500)
        self.assertIn("model fell over", ctx.exception.read().decode())

    def test_stream_false_is_still_plain_json(self):
        body = post_json(f"{self.base}/v1/chat/completions",
                         {"model": "stream-test", "stream": False, "messages": [{"role": "user", "content": "hi"}]})
        self.assertEqual(body["choices"][0]["message"]["content"], "hello there")

    def messages(self, content):
        return post_sse(f"{self.base}/v1/messages", {"model": "stream-test", "stream": True, "max_tokens": 10,
                                                     "messages": [{"role": "user", "content": content}]})

    def test_messages_str_reply_streams_the_anthropic_event_sequence(self):
        _, events = self.messages("hi")
        self.assertEqual([e for e, _ in events], [
            "message_start", "content_block_start", "content_block_delta", "content_block_stop",
            "message_delta", "message_stop",
        ])
        self.assertTrue(all(data["type"] == event for event, data in events))
        self.assertEqual(events[0][1]["message"]["content"], [])
        self.assertEqual(events[1][1]["content_block"], {"type": "text", "text": ""})
        self.assertEqual(events[2][1]["delta"], {"type": "text_delta", "text": "hello there"})
        self.assertEqual(events[4][1]["delta"]["stop_reason"], "end_turn")

    def test_messages_tool_use_input_streams_as_one_json_delta(self):
        _, events = self.messages("tools")
        deltas = [data["delta"] for event, data in events if event == "content_block_delta"]
        self.assertEqual(deltas[0], {"type": "text_delta", "text": "calling"})
        self.assertEqual(deltas[1]["type"], "input_json_delta")
        self.assertEqual(json.loads(deltas[1]["partial_json"]), {"x": 1})
        message_delta = next(data for event, data in events if event == "message_delta")
        self.assertEqual(message_delta["delta"]["stop_reason"], "tool_use")
        self.assertEqual(message_delta["usage"]["output_tokens"], 7)

    def test_responses_str_reply_streams_the_responses_event_sequence(self):
        _, events = post_sse(f"{self.base}/v1/responses", {"model": "stream-test", "stream": True, "input": "hi"})
        self.assertEqual([e for e, _ in events], [
            "response.created", "response.output_item.added", "response.content_part.added",
            "response.output_text.delta", "response.output_text.done", "response.content_part.done",
            "response.output_item.done", "response.completed",
        ])
        self.assertEqual([data["sequence_number"] for _, data in events], list(range(len(events))))
        self.assertEqual(events[0][1]["response"]["status"], "in_progress")
        self.assertEqual(events[3][1]["delta"], "hello there")
        completed = events[-1][1]["response"]
        self.assertEqual(completed["status"], "completed")
        self.assertEqual(completed["output"][0]["content"][0]["text"], "hello there")


class StubAdmin(BaseHTTPRequestHandler):
    """Stands in for tb's admin plane (`/api/v1/*`): serves fixed
    provider-quota responses and records the bearer token it was called
    with, so the test can confirm `admin_token` (not the gateway `token`) is
    what reaches these endpoints."""

    calls = []

    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        StubAdmin.calls.append((self.path, self.headers.get("Authorization")))
        if self.path == "/api/v1/provider-quota":
            body = {
                "meta": {"total": 1, "updated_at": "2026-08-30T00:00:00Z"},
                "data": [{
                    "provider_uuid": "p1", "provider_name": "Anthropic", "provider_type": "anthropic",
                    "fetched_at": "2026-08-30T00:00:00Z", "expires_at": "2026-08-30T01:00:00Z",
                }],
            }
        elif self.path == "/api/v1/provider-quota/p1":
            body = {
                "provider_uuid": "p1", "provider_name": "Anthropic", "provider_type": "anthropic",
                "fetched_at": "2026-08-30T00:00:00Z", "expires_at": "2026-08-30T01:00:00Z",
            }
        elif self.path == "/api/v1/provider-quota/summary":
            body = {
                "total_providers": 3, "ok_providers": 2, "warning_providers": 1, "error_providers": 0,
                "by_status": {"ok": 2, "warning": 1}, "by_type": {"anthropic": 2, "openai": 1},
            }
        else:
            self.send_response(404)
            self.end_headers()
            return
        payload = json.dumps(body).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


class QuotaTest(unittest.TestCase):
    """Client.list_quota()/.get_quota()/.quota_summary() against tb's admin
    plane — a different endpoint family and a different credential
    (admin_token) from Client.chat()'s gateway calls."""

    @classmethod
    def setUpClass(cls):
        StubAdmin.calls = []
        cls.admin = HTTPServer(("127.0.0.1", 0), StubAdmin)
        cls.admin_port = cls.admin.server_address[1]
        threading.Thread(target=cls.admin.serve_forever, daemon=True).start()
        cls.client = Client(
            base_url=f"http://127.0.0.1:{cls.admin_port}",
            token="gateway-token",
            admin_token="admin-token",
        )

    @classmethod
    def tearDownClass(cls):
        cls.admin.shutdown()

    def test_list_quota_returns_the_generated_model_and_uses_admin_token(self):
        result = self.client.list_quota()
        self.assertEqual(result.data[0].provider_uuid, "p1")
        self.assertEqual(result.meta.total, 1)
        path, auth = StubAdmin.calls[-1]
        self.assertEqual(path, "/api/v1/provider-quota")
        self.assertEqual(auth, "Bearer admin-token")

    def test_get_quota_returns_the_generated_model(self):
        result = self.client.get_quota("p1")
        self.assertEqual(result.provider_name, "Anthropic")

    def test_quota_summary_returns_the_generated_model(self):
        result = self.client.quota_summary()
        self.assertEqual(result.total_providers, 3)
        self.assertEqual(result.by_status["ok"], 2)

    def test_admin_token_defaults_to_the_gateway_token_when_not_given(self):
        client = Client(base_url=f"http://127.0.0.1:{self.admin_port}", token="shared-token")
        client.quota_summary()
        _, auth = StubAdmin.calls[-1]
        self.assertEqual(auth, "Bearer shared-token")


if __name__ == "__main__":
    unittest.main()
