"""Compatibility checks for the public entry point and its canonical core pin."""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


class Compatibility(unittest.TestCase):
    def test_core_pin_agrees_across_action_cli_and_private_workflow(self):
        config = (ROOT / 'internal/config/config.go').read_text()
        expected = re.search(r'CoreRevision\s*=\s*"([0-9a-f]{40})"', config)[1]
        action = (ROOT / 'action.yml').read_text()
        pins = re.findall(r'uses: emmett08/engineering-work-observatory@([0-9a-f]{40})', action)
        self.assertEqual([expected, expected], pins)
        private = (ROOT / 'templates/private-runner-workflow.yml').read_text()
        self.assertIn('ref: ' + expected, private)

    def test_example_configs_have_one_protocol_and_no_human_observations(self):
        demo = json.loads((ROOT / '.observatory/config.json').read_text())
        self.assertEqual('observatory.instrumentation/1', demo['schema'])
        self.assertEqual([], demo['tasks'])
        self.assertNotIn('frames', demo)
        template = json.loads((ROOT / 'examples/effort-template.json').read_text())
        self.assertFalse(template['frame_complete'])


if __name__ == '__main__':
    unittest.main()
