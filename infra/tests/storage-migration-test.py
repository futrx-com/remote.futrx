"""Exercise the migration's safety sequence with a fake LXD CLI only."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "migrate-project-storage.sh"

class MigrationTests(unittest.TestCase):
    def run_case(self, execute=False, running=False, export_fail=False):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            project = root / "project"
            (project / "workspace").mkdir(parents=True)
            (project / "workspace" / "keep").write_text("durable")
            binary = root / "bin"
            binary.mkdir()
            lxc = binary / "lxc"
            lxc.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args=sys.argv[1:];root=Path(os.environ["TEST_ROOT"])
with (root/"calls").open("a") as out: out.write(" ".join(args)+"\\n")
if args[0]=="query" and "instances" in args[1]:
 print(json.dumps({"status":"Running" if os.environ["TEST_RUNNING"]=="1" else "Stopped","expanded_devices":{"root":{"type":"disk","path":"/","pool":"target" if (root/"moved").exists() else "source"},"workspace":{"type":"disk","path":"/workspace","source":str(root/"project/workspace")}}}))
elif args[0]=="query": print(json.dumps({"driver":"zfs"}))
elif args[0]=="export":
 if os.environ["TEST_EXPORT_FAIL"]=="1": sys.exit(1)
 Path(args[2]).write_text("export")
elif args[0]=="move": (root/"moved").write_text("moved")
else: print("driver: dir")
''')
            lxc.chmod(0o755)
            env = dict(os.environ, PATH=str(binary)+os.pathsep+os.environ["PATH"], TEST_ROOT=str(root), TEST_RUNNING=str(int(running)), TEST_EXPORT_FAIL=str(int(export_fail)))
            args = [str(SCRIPT), "--instance", "project-1", "--pool", "target", "--backup-dir", str(root / "backup"), "--persistent-dir", str(project)]
            if execute: args.append("--execute")
            result = subprocess.run(args, env=env, capture_output=True, text=True)
            calls = (root / "calls").read_text()
            if running or export_fail:
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("move ", calls)
            else:
                self.assertEqual(result.returncode, 0, result.stderr)
                if execute:
                    self.assertLess(calls.index("export "), calls.index("move "))
                    self.assertTrue((root / "backup/persistent.tar.gz").exists())
                    self.assertTrue((root / "backup/instance-before.json").exists())
                else:
                    self.assertNotIn("move ", calls)
                    self.assertFalse((root / "backup").exists())
            self.assertEqual((project / "workspace/keep").read_text(), "durable")
            self.assertNotIn("delete", calls)
    def test_preflight_is_read_only(self): self.run_case()
    def test_backups_precede_move(self): self.run_case(execute=True)
    def test_running_instance_is_refused(self): self.run_case(execute=True, running=True)
    def test_export_failure_prevents_move(self): self.run_case(execute=True, export_fail=True)

if __name__ == "__main__": unittest.main()
