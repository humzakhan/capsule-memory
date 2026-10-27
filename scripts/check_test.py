#!/usr/bin/env python3
"""Exercise the check gate in disposable repositories; never edit product source."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]


class GateTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="capsule-check-test-", dir="/tmp")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / "scripts").mkdir()
        shutil.copy2(SOURCE / "scripts/check.sh", self.root / "scripts/check.sh")
        self.cmd("git", "init", "-q")
        self.cmd("git", "config", "user.name", "Synthetic Check")
        self.cmd("git", "config", "user.email", "synthetic@example.invalid")
        self.commit("chore: initialize fixture")
        self.base = self.cmd("git", "rev-parse", "HEAD").stdout.strip()

    def cmd(self, *args, **kwargs):
        return subprocess.run(args, cwd=self.root, text=True, capture_output=True,
                              check=True, **kwargs)

    def commit(self, subject):
        self.cmd("git", "add", ".")
        self.cmd("git", "-c", "core.hooksPath=/dev/null", "commit", "-q",
                 "--allow-empty", "-m", subject)

    def gate(self, *args, env=None, expected=0):
        result = subprocess.run(["/bin/bash", "scripts/check.sh", *args],
                                cwd=self.root, text=True, capture_output=True,
                                env=env, timeout=90)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        return result.stdout + result.stderr

    def test_conventional_subjects_and_ranges(self):
        for subject in ("feat: add fixture", "fix(storage): preserve writes",
                        "refactor!: change contract", "feat(wire)!: new framing"):
            self.commit(subject)
        output = self.gate("--phase", "commits", "--range", f"{self.base}..HEAD")
        self.assertEqual(output.count("PASS commit"), 4)
        self.commit("invalid subject")
        output = self.gate("--phase", "commits", "--range", f"{self.base}..HEAD", expected=1)
        self.assertIn("FAIL commit", output)
        self.assertIn("invalid", output)
        for args in [[], ["--phase", "invalid"], ["--range"],
                     ["--phase", "commits", "--range", "HEAD..HEAD"],
                     ["--range", "HEAD...HEAD"], ["--range", "..HEAD"],
                     ["--range", "missing..HEAD"],
                     ["--range", f"HEAD..{self.base}"],
                     ["--phase", "quality", "--range", f"{self.base}..HEAD"],
                     ["--phase", "commits", "--phase", "go"]]:
            self.gate(*args, expected=2)

    def go_fixture(self):
        (self.root / "go.mod").write_text("module example.invalid/checkfixture\n\ngo 1.24.0\n")
        (self.root / "main.go").write_text("package main\n\nfunc main() {}\n")
        (self.root / "main_test.go").write_text(
            'package main\n\nimport "testing"\n\nfunc TestFixture(t *testing.T) { main() }\n')

    def test_go_success_and_no_checkout_outputs(self):
        self.go_fixture()
        before = {str(p.relative_to(self.root)): p.read_bytes()
                  for p in self.root.rglob("*") if p.is_file() and ".git" not in p.parts}
        output = self.gate("--phase", "go")
        self.assertIn("RESULT Go build: exit 0", output)
        after = {str(p.relative_to(self.root)): p.read_bytes()
                 for p in self.root.rglob("*") if p.is_file() and ".git" not in p.parts}
        self.assertEqual(before, after, "Gate rewrote/created checkout files")

    def test_go_failure_is_collected_and_arbitrary_paths_preserved(self):
        self.go_fixture()
        filename = "-odd name\n[fixture].go"
        contents = "package main\nfunc broken( {\n"
        (self.root / filename).write_text(contents)
        output = self.gate("--phase", "go", expected=1)
        self.assertIn("RESULT Go vet: exit", output)
        self.assertIn("RESULT Go test: exit", output)
        self.assertIn("RESULT Go build: exit", output)
        self.assertEqual((self.root / filename).read_text(), contents)

    def test_no_go_source_fails(self):
        output = self.gate("--phase", "go", expected=1)
        self.assertIn("FAIL: no Go source found", output)

    def test_missing_go_fails(self):
        tools = self.root / "limited-tools"
        tools.mkdir()
        for tool in ("git", "dirname", "mktemp", "rm"):
            target = shutil.which(tool)
            self.assertIsNotNone(target)
            (tools / tool).symlink_to(target)
        env = dict(os.environ, PATH=str(tools))
        output = self.gate("--phase", "go", env=env, expected=1)
        self.assertIn("MISSING prerequisite: go", output)
        self.assertNotIn("RESULT Go build: exit 0", output)

    def relay_fixture(self):
        relay = self.root / "cloudflare"
        (relay / "src").mkdir(parents=True)
        for name in ("tsconfig.json", ".prettierrc.json"):
            shutil.copy2(SOURCE / "cloudflare" / name, relay / name)
        (relay / "package.json").write_text(json.dumps({"private": True}, indent=2) + "\n")
        (relay / "node_modules").symlink_to(SOURCE / "cloudflare/node_modules", target_is_directory=True)
        # This must resolve Worker globals from the configured declaration package.
        (relay / "src/index.ts").write_text(
            "export const archive: R2Bucket | undefined = undefined;\n")
        return relay

    def test_full_default_gate(self):
        self.go_fixture()
        self.relay_fixture()
        self.commit("build(checks)!: exercise full fixture")
        output = self.gate("--range", f"{self.base}..HEAD")
        self.assertIn("Gate exit: 0", output)
        self.assertIn("PASS commit", output)
        self.gate("--phase", "commits", "--range", "HEAD~1..HEAD")

    def test_relay_pass_and_real_type_error(self):
        relay = self.relay_fixture()
        output = self.gate("--phase", "relay")
        self.assertIn("RESULT Relay no-emit typecheck: exit 0", output)
        self.assertIn("RESULT Relay formatting: exit 0", output)
        (relay / "src/index.ts").write_text("export const archive: R2Bucket = 42;\n")
        output = self.gate("--phase", "relay", expected=1)
        self.assertIn("RESULT Relay no-emit typecheck: exit 2", output)
        self.assertFalse((relay / "src/index.js").exists())

    def test_relay_formatting_failure_and_missing_dependency(self):
        relay = self.relay_fixture()
        (relay / "src/index.ts").write_text("export const value=1\n")
        output = self.gate("--phase", "relay", expected=1)
        self.assertIn("RESULT Relay formatting: exit 1", output)
        self.assertEqual((relay / "src/index.ts").read_text(), "export const value=1\n")
        (relay / "node_modules").unlink()
        output = self.gate("--phase", "relay", expected=1)
        self.assertIn("MISSING prerequisite: local TypeScript", output)
        self.assertIn("MISSING prerequisite: local Prettier", output)


if __name__ == "__main__":
    unittest.main(verbosity=2)
