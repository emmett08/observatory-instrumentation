"""Validate the actual wrapper → issue discovery → canonical receipt import path."""
import json
from pathlib import Path
import sys

discovery, collected, run_id = Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3]
def receipt_records(name):
    snapshot = json.loads((collected / name).read_text())
    return {item['record']['id']: item['record'] for item in snapshot['records']
            if item['record']['data'].get('source') == 'observatory.tool_receipt'
            and item['record']['data']['payload']['platform']['run_id'] == run_id}
first, second = receipt_records('first.json'), receipt_records('second.json')
assert len(first) == 4 and first == second, 'two jobs must retain the same four phase receipts on repeat collection'
manifest = json.loads((discovery / 'manifest.json').read_text()) if (discovery / 'manifest.json').exists() else None
tasks = {t['task_id'] for t in manifest['tasks'] if int(run_id) in t['run_ids']} if manifest else set()
observed = set()
for record in second.values():
    value = record['data']['payload']
    assert value['declared']['protocol_id'] == 'observatory-instrumentation-demo-v1'
    assert value['declared']['controller'] == 'unknown'
    task = value['declared']['task_id']
    if task is not None:
        assert task in tasks, 'receipt task must agree with independently discovered issue registration'
        observed.add(task)
    if value['phase'] == 'completion':
        assert value['result']['status'] == 'success'
result = {'schema': 'observatory.instrumentation-live-verification/1', 'run_id': run_id,
          'receipt_count': len(second), 'deduplicated': True, 'linked_task_ids': sorted(observed),
          'scope': 'Two instrumented matrix jobs; other tools and human effort are not inferred.'}
(collected / 'verification.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result))
