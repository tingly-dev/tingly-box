"""tingly.vmodel without a tb: building scripts, writing them, and the attach
flow against a stand-in tb that only lists models. The real thing is
test_vmodel_e2e.py."""

import json
import os
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tingly import TinglyError, vmodel  # noqa: E402


class ScriptTest(unittest.TestCase):
    def test_steps_build_the_documented_schema(self):
        script = (vmodel.Script("flow", on_exhaust="clamp")
                  .tool("Read", {"file_path": "/a"}, say="look", usage={"input": 5})
                  .tool("Edit", {"x": 1}, tool_id="t2")
                  .error(529, message="busy")
                  .cut("event", after=3, say="partial")
                  .say("done", stop_reason="end_turn", repeat=2))
        self.assertEqual(script.to_dict(), {
            "id": "flow", "on_exhaust": "clamp",
            "steps": [
                {"say": "look", "tool": {"name": "Read", "arguments": {"file_path": "/a"}}, "usage": {"input": 5}},
                {"tool": {"name": "Edit", "arguments": {"x": 1}, "id": "t2"}},
                {"status": 529, "error_message": "busy"},
                {"midstream": {"mode": "event", "after_events": 3}, "say": "partial"},
                {"say": "done", "stop_reason": "end_turn", "repeat": 2},
            ]})

    def test_dumps_is_json_and_so_valid_yaml(self):
        script = vmodel.Script("flow").say("héllo — “quoted”\nline two")
        self.assertEqual(json.loads(script.dumps())["steps"][0]["say"], "héllo — “quoted”\nline two")

    def test_cut_rejects_an_unknown_mode(self):
        with self.assertRaises(ValueError):
            vmodel.Script("x").cut("explode")

    def test_write_is_atomic_and_remove_cleans_up(self):
        with tempfile.TemporaryDirectory() as conf:
            script = vmodel.Script("flow").say("hi")
            path = script.write(conf)
            self.assertEqual(path, os.path.join(conf, "vmodels", "flow.yaml"))
            self.assertEqual(os.listdir(os.path.join(conf, "vmodels")), ["flow.yaml"], "no temp file left behind")
            script.remove(conf)
            script.remove(conf)  # idempotent
            self.assertEqual(os.listdir(os.path.join(conf, "vmodels")), [])


class SessionTest(unittest.TestCase):
    def test_model_names_the_session(self):
        tb = vmodel.Testbed()
        s = tb.session("t-1")
        self.assertEqual(s.model(vmodel.Script("flow")), "flow@t-1")
        self.assertEqual(s.model("flow"), "flow@t-1")

    def test_default_session_ids_are_unique_and_valid(self):
        tb = vmodel.Testbed()
        ids = {tb.session().id for _ in range(50)}
        self.assertEqual(len(ids), 50)

    def test_invalid_session_names_are_rejected(self):
        for bad in ("has space", "a@b", "x" * 65):
            with self.assertRaises(ValueError):
                vmodel.Testbed().session(bad)


class _FakeTB(BaseHTTPRequestHandler):
    """Lists whatever is in the config dir's vmodels/, like tb would; 404s
    with tb's error text for anything else."""

    conf_dir = ""
    hidden: set = set()  # ids on disk that this tb "failed to load"
    builtin: set = {"echo-model"}  # models that exist without any file

    def log_message(self, *args):
        pass

    def do_GET(self):
        directory = os.path.join(self.conf_dir, "vmodels")
        ids = sorted(self.builtin)
        for n in sorted(os.listdir(directory)) if os.path.isdir(directory) else []:
            if n.endswith(".yaml") and n[:-5] not in self.hidden:
                with open(os.path.join(directory, n)) as f:
                    ids.append(vmodel._declared_id(f.read()) or n[:-5])
        self._send(200, {"data": [{"id": i} for i in ids]})

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", 0)))
        self._send(404, {"error": {"message": "Model not found: x (script load errors: x.yaml: boom)"}})

    def _send(self, code, body):
        raw = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


class AttachTest(unittest.TestCase):
    def setUp(self):
        self.conf = tempfile.mkdtemp(prefix="tingly-attach-")
        with open(os.path.join(self.conf, "config.json"), "w") as f:
            json.dump({"model_token": "tok-123"}, f)
        self.handler = type("H", (_FakeTB,), {"conf_dir": self.conf, "hidden": set()})
        self.httpd = HTTPServer(("127.0.0.1", 0), self.handler)
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def tearDown(self):
        self.httpd.shutdown()
        self.httpd.server_close()

    def test_attach_writes_into_the_running_tb_and_cleans_up_on_exit(self):
        flow = vmodel.Script("flow").say("hi")
        with vmodel.Testbed.attach(flow, config_dir=self.conf, base_url=self.url) as tb:
            self.assertEqual(tb.token, "tok-123")
            self.assertEqual(tb.anthropic_base, self.url + "/virtual/anthropic")
            self.assertEqual(tb.openai_base, self.url + "/virtual/openai/v1")
            self.assertTrue(os.path.exists(os.path.join(self.conf, "vmodels", "flow.yaml")))
        self.assertFalse(os.path.exists(os.path.join(self.conf, "vmodels", "flow.yaml")),
                         "attach must remove only the scripts it added")
        self.assertTrue(os.path.exists(os.path.join(self.conf, "config.json")))

    def test_a_script_tb_refuses_raises_with_tbs_own_message(self):
        self.handler.hidden = {"x"}  # on disk, but tb does not list it
        with vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url) as tb:
            with self.assertRaises(vmodel.ScriptError) as ctx:
                tb.add(vmodel.Script("x").say("hi"))
        self.assertIn("script load errors: x.yaml: boom", str(ctx.exception))

    def test_add_before_start_is_allowed_and_stop_before_start_is_safe(self):
        tb = vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url)
        tb.token = "tok-123"
        tb.add(vmodel.Script("early").say("hi"))
        tb.stop()  # attached: removes what add() wrote
        self.assertFalse(os.path.exists(os.path.join(self.conf, "vmodels", "early.yaml")))
        vmodel.Testbed().stop()  # never started

    def test_attach_never_overwrites_or_deletes_a_file_it_did_not_write(self):
        os.makedirs(os.path.join(self.conf, "vmodels"))
        mine = os.path.join(self.conf, "vmodels", "flow.yaml")
        with open(mine, "w") as f:
            f.write("steps: [200]\n")
        with vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url) as tb:
            with self.assertRaises(vmodel.ScriptError) as ctx:
                tb.add(vmodel.Script("flow").say("hi"))
            self.assertIn("not written by this Testbed", str(ctx.exception))
        self.assertTrue(os.path.exists(mine), "the user's own file survives")
        with open(mine) as f:
            self.assertEqual(f.read(), "steps: [200]\n")

    def test_a_script_cannot_silently_shadow_a_builtin_or_another_scripts_model(self):
        with vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url) as tb:
            with self.assertRaises(vmodel.ScriptError) as ctx:
                tb.add(vmodel.Script("echo-model").say("hi"))
            self.assertIn("already served", str(ctx.exception))
            self.assertFalse(os.path.exists(os.path.join(self.conf, "vmodels", "echo-model.yaml")), "nothing written")
            tb.add(vmodel.Script("mine").say("hi"))
            tb.add(vmodel.Script("mine").say("again"))  # re-adding one's own is fine

    def test_add_path_uses_the_ids_the_file_declares(self):
        src = os.path.join(tempfile.mkdtemp(prefix="tingly-src-"), "by-file-name.yaml")
        with open(src, "w") as f:
            f.write("id: declared-id\nsteps:\n  - say: hi\n")
        with vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url) as tb:
            self.assertEqual(tb.add(src), "declared-id")

    def test_declared_id_reads_json_and_yaml(self):
        self.assertEqual(vmodel._declared_id('{"id": "j-1", "steps": []}'), "j-1")
        self.assertEqual(vmodel._declared_id("steps: []\nid: y.2\n"), "y.2")
        self.assertEqual(vmodel._declared_id('id: "quoted"\nsteps: []'), "quoted")
        self.assertIsNone(vmodel._declared_id("steps: [200]"))
        self.assertIsNone(vmodel._declared_id("steps:\n  - say: id: not top level"))

    def test_attach_without_a_model_token_explains_itself(self):
        os.remove(os.path.join(self.conf, "config.json"))
        with self.assertRaises(RuntimeError) as ctx:
            vmodel.Testbed.attach(config_dir=self.conf, base_url=self.url).start()
        self.assertIn("model_token", str(ctx.exception))

    def test_missing_binary_is_explained(self):
        old = {k: os.environ.pop(k, None) for k in ("TINGLY_TB_BIN",)}
        try:
            tb = vmodel.Testbed(tb_bin=None)
            import shutil
            real_which = shutil.which
            shutil.which = lambda name: None
            try:
                with self.assertRaises(RuntimeError) as ctx:
                    tb.start()
            finally:
                shutil.which = real_which
            self.assertIn("TINGLY_TB_BIN", str(ctx.exception))
        finally:
            for k, v in old.items():
                if v is not None:
                    os.environ[k] = v


if __name__ == "__main__":
    unittest.main()
