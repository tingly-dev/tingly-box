"""The sugar layer end to end: plain functions registered with
@tingly.openai_chat / openai_responses / anthropic_message / image /
image_edit, served by tingly.serve()."""

import base64
import importlib.util
import os
import struct
import sys
import unittest
import urllib.error

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

import tingly  # noqa: E402
from tingly import sugar  # noqa: E402
from helpers import get_json, post_json, post_multipart, post_sse, serve_sugar_in_background, stop_sugar  # noqa: E402


class SugarTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        sugar._reset()
        cls.seen = {}

        @tingly.openai_chat("echo")
        def reply(messages):
            cls.seen["text"] = messages
            return f"you said: {messages[-1]['content'][0]['text']}"

        @tingly.openai_responses("resp")
        def respond(input, instructions=None):
            cls.seen["responses"] = (input, instructions)
            return "responded"

        @tingly.anthropic_message("claude-ish")
        def answer(messages, system=None):
            cls.seen["anthropic"] = (messages, system)
            return "answered"

        @tingly.image("bare")
        def bare(prompt):  # accepts only prompt: size/n must not be passed
            cls.seen["bare"] = prompt
            return b"bare-png"

        @tingly.image("sized")
        def sized(prompt, size=None):
            cls.seen["sized"] = (prompt, size)
            return b"sized-png"

        @tingly.image("everything")
        def everything(prompt, **rest):
            cls.seen["everything"] = rest
            return [b"a", b"b"]

        @tingly.image_edit("editor")
        def edit(prompt, images, mask=None):
            cls.seen["edit"] = (prompt, images, mask)
            return images[-1]

        cls.base = serve_sugar_in_background()

    @classmethod
    def tearDownClass(cls):
        stop_sugar()

    def test_text_gets_the_messages_list_unchanged_and_a_str_reply_is_wrapped(self):
        messages = [{"role": "user", "content": [{"type": "text", "text": "hi"}]}]
        body = post_json(f"{self.base}/v1/chat/completions", {"model": "echo", "messages": messages})
        self.assertEqual(self.seen["text"], messages)  # content parts stay parts: unpacked, not converted
        self.assertEqual(body["choices"][0]["message"]["content"], "you said: hi")

    def test_openai_responses_gets_input_as_sent_and_declared_keywords(self):
        items = [{"role": "user", "content": [{"type": "input_text", "text": "hi"}]}]
        body = post_json(f"{self.base}/v1/responses", {"model": "resp", "input": items, "instructions": "be brief"})
        self.assertEqual(self.seen["responses"], (items, "be brief"))
        self.assertEqual(body["output"][0]["content"][0]["text"], "responded")

    def test_anthropic_message_gets_messages_and_system_unchanged(self):
        messages = [{"role": "user", "content": [{"type": "text", "text": "hi"}]}]
        system = [{"type": "text", "text": "be brief"}]
        body = post_json(f"{self.base}/v1/messages",
                         {"model": "claude-ish", "max_tokens": 10, "messages": messages, "system": system})
        self.assertEqual(self.seen["anthropic"], (messages, system))
        self.assertEqual(body["content"], [{"type": "text", "text": "answered"}])

    def test_sugar_replies_stream_like_raw_ones(self):
        _, events = post_sse(f"{self.base}/v1/messages", {"model": "claude-ish", "max_tokens": 10, "stream": True,
                                                           "messages": [{"role": "user", "content": "hi"}]})
        deltas = [data["delta"] for event, data in events if event == "content_block_delta"]
        self.assertEqual(deltas, [{"type": "text_delta", "text": "answered"}])

    def test_image_function_only_gets_the_arguments_it_declares(self):
        body = post_json(f"{self.base}/v1/images/generations",
                         {"model": "bare", "prompt": "a cat", "size": "512x512", "n": 1})
        self.assertEqual(self.seen["bare"], "a cat")
        self.assertEqual(base64.b64decode(body["data"][0]["b64_json"]), b"bare-png")

    def test_a_declared_keyword_is_filled_from_the_body(self):
        post_json(f"{self.base}/v1/images/generations", {"model": "sized", "prompt": "p", "size": "64x64"})
        self.assertEqual(self.seen["sized"], ("p", "64x64"))

    def test_var_keyword_gets_the_whole_rest_of_the_body(self):
        body = post_json(f"{self.base}/v1/images/generations",
                         {"model": "everything", "prompt": "p", "n": 2, "quality": "high"})
        self.assertEqual(self.seen["everything"], {"n": 2, "quality": "high"})
        self.assertEqual(len(body["data"]), 2)

    def test_image_edit_gets_prompt_images_and_mask(self):
        body = post_multipart(f"{self.base}/v1/images/edits", [
            ("model", "editor"), ("prompt", "fix it"),
            ("image[]", b"one"), ("image[]", b"two"), ("mask", b"m"),
        ])
        self.assertEqual(self.seen["edit"], ("fix it", [b"one", b"two"], b"m"))
        self.assertEqual(base64.b64decode(body["data"][0]["b64_json"]), b"two")

    def test_models_lists_every_registered_model(self):
        ids = [m["id"] for m in get_json(f"{self.base}/v1/models")["data"]]
        self.assertEqual(ids, ["echo", "resp", "claude-ish", "bare", "sized", "everything", "editor"])

    def test_an_unknown_model_is_an_error_when_the_endpoint_has_several(self):
        with self.assertRaises(urllib.error.HTTPError) as ctx:
            post_json(f"{self.base}/v1/images/generations", {"model": "nope", "prompt": "p"})
        self.assertEqual(ctx.exception.code, 500)
        self.assertIn("nope", ctx.exception.read().decode())

    def test_a_single_function_on_an_endpoint_answers_any_model_name(self):
        body = post_json(f"{self.base}/v1/chat/completions",
                         {"model": "typo", "messages": [{"role": "user", "content": [{"type": "text", "text": "yo"}]}]})
        self.assertEqual(body["choices"][0]["message"]["content"], "you said: yo")


class ServeWithoutRegistrationTest(unittest.TestCase):
    def test_serve_refuses_to_start_empty(self):
        sugar._reset()
        with self.assertRaises(RuntimeError):
            tingly.serve()


class ImageExampleTest(unittest.TestCase):
    """examples/image.py actually runs and returns a real PNG."""

    @classmethod
    def setUpClass(cls):
        sugar._reset()
        path = os.path.join(os.path.dirname(__file__), "..", "examples", "image.py")
        spec = importlib.util.spec_from_file_location("image_example", path)
        spec.loader.exec_module(importlib.util.module_from_spec(spec))
        cls.base = serve_sugar_in_background()

    @classmethod
    def tearDownClass(cls):
        stop_sugar()

    def test_the_fake_model_returns_a_png_of_the_requested_size(self):
        body = post_json(f"{self.base}/v1/images/generations",
                         {"model": "fake-image", "prompt": "a red fox", "size": "32x16"})
        png = base64.b64decode(body["data"][0]["b64_json"])
        self.assertEqual(png[:8], b"\x89PNG\r\n\x1a\n")
        self.assertEqual(struct.unpack(">II", png[16:24]), (32, 16))


if __name__ == "__main__":
    unittest.main()
