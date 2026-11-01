import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("collect_discovered", Path(__file__).with_name("collect_discovered.py"))
adapter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(adapter)


class CollectionAdapterTests(unittest.TestCase):
    def fixture(self, root, complete=True):
        discovery = root / "discovery.json"
        discovery.write_text(json.dumps({"schema": "observatory.discovery/1", "complete": complete,
            "selected_runs": [{"repository": "example/project", "run_id": 2, "task_id": None},
                              {"repository": "example/project", "run_id": 1, "task_id": "known"},
                              {"repository": "example/project", "run_id": 2, "task_id": None}]}))
        return discovery

    def test_unknown_task_runs_collected_once_and_store_verified(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(adapter.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)) as run:
                self.assertTrue(adapter.collect(self.fixture(root), root / "state", root / "observatory", root / "outcome.json"))
            commands = [call.args[0] for call in run.call_args_list]
            self.assertEqual(len(commands), 3)
            self.assertEqual([command[-1] for command in commands], ["1", "2", "verify"])

    def test_failure_and_incomplete_discovery_remain_incomplete_but_continue(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            with patch.object(adapter.subprocess, "run", side_effect=[subprocess.CompletedProcess([], 1), subprocess.CompletedProcess([], 0), subprocess.CompletedProcess([], 0)]):
                self.assertFalse(adapter.collect(self.fixture(root, False), root / "state", root / "observatory", root / "outcome.json"))
            report = json.loads((root / "outcome.json").read_text())
            self.assertFalse(report["discovery_complete"])
            self.assertTrue(report["store_verified"])
            self.assertEqual([item["returncode"] for item in report["outcomes"]], [1, 0])

    def test_invalid_repository_and_boolean_run_rejected_before_execution(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / "bad.json"
            for run in [{"repository": "../escape", "run_id": 1}, {"repository": "owner/..", "run_id": 1},
                        {"repository": "owner/repo", "run_id": True}, "malformed"]:
                path.write_text(json.dumps({"schema": "observatory.discovery/1", "complete": True, "selected_runs": [run]}))
                with patch.object(adapter.subprocess, "run") as command, self.assertRaises(ValueError):
                    adapter.collect(path, root / "state", root / "observatory", root / "outcome.json")
                command.assert_not_called()

    def test_existing_outcome_is_preserved(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            output = root / "outcome.json"
            output.write_text("retained")
            with self.assertRaises(ValueError):
                adapter.collect(self.fixture(root), root / "state", root / "observatory", output)
            self.assertEqual(output.read_text(), "retained")


if __name__ == "__main__":
    unittest.main()
