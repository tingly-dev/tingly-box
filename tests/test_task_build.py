"""Regression test for the root Taskfile's build orchestration.

Runs `task build` / `task web:build:dev` in a sandbox with fake pnpm/go on
PATH (no installs, no real compilation) and asserts the frontend commands,
asset embedding and CLI output name stay wired up. Note: go-task's built-in
shell cannot run pnpm's shebang-less fallback launcher (it passes a bare
argv[0], so the launcher cannot resolve its own file) — that happens when the
global pnpm install was set up with its install.js script blocked (e.g. newer
npm's allowScripts). A healthy pnpm is a native binary and bare `pnpm` works.
"""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(shutil.which("task"), "Go Task is required")
class TaskBuildTest(unittest.TestCase):
    def test_build_with_path_resolving_pnpm_launcher(self):
        with tempfile.TemporaryDirectory(prefix="tingly-task-build-") as directory:
            root = Path(directory)
            taskfiles = subprocess.check_output(
                ["git", "ls-files", "*Taskfile*.yml"], cwd=ROOT, text=True
            ).splitlines()
            for name in taskfiles:
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / name, target)
            (root / "frontend").mkdir()
            (root / "tools").mkdir()
            (root / "launchers").mkdir()
            log = root / "commands.log"

            # A normal shebang'd launcher: bare `pnpm` resolves to it fine.
            launcher = root / "launchers" / "pnpm"
            launcher.write_text(
                '#!/bin/sh\n'
                'printf "pnpm %s\\n" "$*" >> "$BUILD_TEST_LOG"\n'
                'case "$1" in\n'
                '  build|build:dev) mkdir -p dist; printf ui > dist/index.html ;;\n'
                'esac\n'
            )
            launcher.chmod(0o755)
            (root / "tools" / "pnpm").symlink_to("../launchers/pnpm")
            go = root / "tools" / "go"
            go.write_text(
                '#!/bin/sh\n'
                'printf "go %s\\n" "$*" >> "$BUILD_TEST_LOG"\n'
                'if [ "$1" = build ]; then\n'
                '  while [ "$#" -gt 0 ]; do\n'
                '    if [ "$1" = -o ]; then shift; touch "$1"; break; fi\n'
                '    shift\n'
                '  done\n'
                'fi\n'
            )
            go.chmod(0o755)
            env = dict(os.environ, PATH=f"{root / 'tools'}:{os.environ['PATH']}",
                       BUILD_TEST_LOG=str(log), CI="false")

            cases = [("build", "build", "tingly-box") for _ in range(20)]
            cases += [("web:build:dev", "build:dev", None),
                      ("build", "build", "custom-cli")]
            for command, frontend_build, binary_name in cases:
                with self.subTest(command=command, binary_name=binary_name):
                    log.write_text("")
                    args = ["task", command]
                    if binary_name == "custom-cli":
                        args.append("CLI_BIN_NAME=custom-cli")
                    result = subprocess.run(
                        args, cwd=root, env=env, text=True,
                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                    )
                    self.assertEqual(result.returncode, 0, result.stdout)
                    commands = log.read_text()
                    self.assertIn("pnpm install\n", commands)
                    self.assertIn("pnpm gen:api\n", commands)
                    self.assertIn(f"pnpm {frontend_build}\n", commands)
                    self.assertEqual((root / "internal/web/dist/index.html").read_text(), "ui")
                    if binary_name:
                        self.assertIn(f"-o ./build/{binary_name}", commands)
                        self.assertTrue((root / "build" / binary_name).is_file(), result.stdout)
                        self.assertFalse((root / "build/tingly-box-gui").exists(), result.stdout)


if __name__ == "__main__":
    unittest.main()
