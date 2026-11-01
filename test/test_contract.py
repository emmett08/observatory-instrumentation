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

    def test_public_observation_workflow_pins_one_canonical_core_revision(self):
        workflow = (ROOT / '.github/workflows/observation-learning-demo.yml').read_text()
        workflow_ref = re.search(r'observation-learning-demo.yml@([0-9a-f]{40})', workflow)[1]
        checkout_ref = re.search(r"core-ref: '([0-9a-f]{40})'", workflow)[1]
        self.assertEqual(workflow_ref, checkout_ref)
        self.assertIn('actions: read', workflow)
        self.assertIn('issues: read', workflow)
        self.assertNotIn('secrets: inherit', workflow)

    def test_checked_in_effort_form_matches_installer_template(self):
        template = (ROOT / 'internal/installer/templates/effort.yml.tmpl').read_text()
        rendered = template.replace('[[yaml .Protocol]]', '"observatory-instrumentation-demo-v1"')
        rendered = rendered.replace('[[yaml .IssueExample]]', '"https://github.com/emmett08/observatory-instrumentation/issues/<number>"')
        self.assertEqual(rendered, (ROOT / '.github/ISSUE_TEMPLATE/effort-observation.yml').read_text())


if __name__ == '__main__':
    unittest.main()
