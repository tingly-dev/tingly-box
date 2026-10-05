"""tingly.vmodel against a real tb: a script written in Python is served by
the tb's own vmodel endpoints, in both wire protocols, with the errors and
mid-stream cuts it declares.

Skipped unless TINGLY_TB_BIN points at a tb binary (`task test:py:e2e`).
"""

import os
import sys
import tempfile
import unittest
import http.client

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from helpers import post_sse  # noqa: E402
from tingly import TinglyError, vmodel  # noqa: E402

TB_BIN = os.environ.get("TINGLY_TB_BIN")

FLOW = (vmodel.Script("flow")
        .tool("Read", {"file_path": "/tmp/a.go"}, say="Let me look.")
        .tool("Edit", {"file_path": "/tmp/a.go", "old_string": "foo", "new_string": "bar"})
        .error(529)
        .say("Done.", usage={"input": 1200, "output": 40}))

FLAKY = (vmodel.Script("flaky")
         .cut("eof", after=2, say="never finishes")
         .cut("event", after=2, say="never finishes")
         .say("recovered"))


@unittest.skipUnless(TB_BIN, "set TINGLY_TB_BIN to a tb binary to run the end-to-end test")
class VModelThroughTB(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tb = vmodel.Testbed(FLOW, FLAKY, tb_bin=TB_BIN).start()

    @classmethod
    def tearDownClass(cls):
        cls.tb.stop()

    def fresh(self, script):
        """Re-add a script: same id replaces it and restarts its program."""
        self.tb.add(script)
        return script.model

    def test_an_anthropic_agent_loop_walks_the_script(self):
        model = self.fresh(FLOW)
        first = self.tb.messages(model, "fix foo")
        self.assertEqual(first["stop_reason"], "tool_use")
        text, tool = first["content"]
        self.assertEqual(text["text"], "Let me look.")
        self.assertEqual((tool["name"], tool["input"]), ("Read", {"file_path": "/tmp/a.go"}))

        second = self.tb.messages(model, "(tool result)")
        self.assertEqual(second["stop_reason"], "tool_use")
        edit = [b for b in second["content"] if b["type"] == "tool_use"][0]
        self.assertEqual(edit["name"], "Edit")
        self.assertNotEqual(edit["id"], tool["id"], "each tool call gets its own id")

        with self.assertRaises(TinglyError) as ctx:
            self.tb.messages(model, "(tool result)")
        self.assertEqual(ctx.exception.status, 529)
        self.assertIn("overloaded_error", ctx.exception.body)

        done = self.tb.messages(model, "retry")
        self.assertEqual(done["stop_reason"], "end_turn")
        self.assertEqual(done["content"][0]["text"], "Done.")

    def test_the_same_script_answers_openai_chat_with_its_own_cursor(self):
        model = self.fresh(FLOW)
        first = self.tb.chat(model, "go")["choices"][0]
        self.assertEqual(first["finish_reason"], "tool_calls")
        self.assertEqual(first["message"]["content"], "Let me look.")
        self.assertEqual(first["message"]["tool_calls"][0]["function"]["name"], "Read")

    def test_mid_stream_cuts_are_visible_on_the_wire(self):
        model = self.fresh(FLAKY)
        url = f"{self.tb.anthropic_base}/v1/messages"
        headers = {"x-api-key": self.tb.token}
        body = {"model": model, "max_tokens": 64, "stream": True,
                "messages": [{"role": "user", "content": "hi"}]}

        _, events = post_sse(url, body, headers)  # eof: ends cleanly, never completes
        self.assertNotIn("message_stop", [e for e, _ in events])
        _, events = post_sse(url, body, headers)  # event: an in-band error
        self.assertNotIn("message_stop", [e for e, _ in events])
        _, events = post_sse(url, body, headers)  # recovered
        self.assertEqual([e for e, _ in events][-1], "message_stop")

    def test_usage_is_advertised_on_a_streamed_step(self):
        self.tb.add(vmodel.Script("metered").say("hi", usage={"input": 1200, "output": 40, "cache_read": 1000}))
        _, events = post_sse(f"{self.tb.anthropic_base}/v1/messages", {
            "model": "metered", "max_tokens": 64, "stream": True,
            "messages": [{"role": "user", "content": "hi"}]}, {"x-api-key": self.tb.token})
        usage = [d["usage"] for e, d in events if e == "message_delta"][-1]
        self.assertEqual(usage["output_tokens"], 40)
        self.assertEqual(usage["cache_read_input_tokens"], 1000)

    def test_replacing_and_removing_a_script_takes_effect_on_the_next_request(self):
        script = vmodel.Script("swap").say("one")
        self.tb.add(script)
        self.assertEqual(self.tb.chat("swap", "x")["choices"][0]["message"]["content"], "one")

        self.tb.add(vmodel.Script("swap").say("two"))
        self.assertEqual(self.tb.chat("swap", "x")["choices"][0]["message"]["content"], "two")

        self.tb.remove(script)
        with self.assertRaises(TinglyError) as ctx:
            self.tb.chat("swap", "x")
        self.assertEqual(ctx.exception.status, 404)

    def test_a_script_tb_rejects_explains_why(self):
        with self.assertRaises(vmodel.ScriptError) as ctx:
            self.tb.add(vmodel.Script("broken").error(302))
        self.assertIn("status 302", str(ctx.exception))
        self.tb.remove(vmodel.Script("broken"))

    def test_re_adding_a_broken_script_is_an_error_not_a_stale_success(self):
        self.tb.add(vmodel.Script("edited").say("good"))
        with self.assertRaises(vmodel.ScriptError):
            self.tb.add(vmodel.Script("edited").error(302))
        with self.assertRaises(TinglyError) as ctx:  # tb serves what is on disk: nothing
            self.tb.chat("edited", "x")
        self.assertEqual(ctx.exception.status, 404)
        self.tb.remove(vmodel.Script("edited"))

    def test_a_hand_written_yaml_file_loads_like_a_python_script(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "by-hand.yaml")
            with open(path, "w") as f:
                f.write("steps:\n  - say: written by hand\n  - 429\n")
            self.assertEqual(self.tb.add(path), "by-hand")
        self.assertEqual(self.tb.chat("by-hand", "x")["choices"][0]["message"]["content"], "written by hand")
        with self.assertRaises(TinglyError) as ctx:
            self.tb.chat("by-hand", "x")
        self.assertEqual(ctx.exception.status, 429)

    def test_attach_serves_scripts_from_an_already_running_tb(self):
        script = vmodel.Script("attached").say("from attach")
        with vmodel.Testbed.attach(script, config_dir=self.tb.config_dir, base_url=self.tb.base_url) as other:
            self.assertEqual(other.chat("attached", "x")["choices"][0]["message"]["content"], "from attach")
        with self.assertRaises(TinglyError):
            self.tb.chat("attached", "x")  # removed again on exit

    def test_every_step_kind_python_can_write_is_accepted_by_tb(self):
        everything = (vmodel.Script("everything", on_exhaust="clamp", description="all kinds",
                                    default_content="dflt")
                      .say("a", usage={"input": 1, "output": 2, "cache_read": 3, "cache_write": 4, "reasoning": 5},
                           stop_reason="end_turn")
                      .tool("T", {"k": "v"}, tool_id="my-id")
                      .error(503, message="down", error_type="overloaded_error")
                      .cut("close", after=1, say="x")
                      .say("z", repeat=2))
        self.assertEqual(self.tb.add(everything), "everything")


class TestbedLifecycle(unittest.TestCase):
    @unittest.skipUnless(TB_BIN, "set TINGLY_TB_BIN to a tb binary")
    def test_stop_ends_the_process_and_removes_the_config_dir(self):
        tb = vmodel.Testbed(vmodel.Script("x").say("hi"), tb_bin=TB_BIN).start()
        conf, proc = tb.config_dir, tb._proc
        self.assertTrue(os.path.isdir(conf))
        tb.stop()
        self.assertIsNotNone(proc.poll())
        self.assertFalse(os.path.exists(conf))


if __name__ == "__main__":
    unittest.main()
