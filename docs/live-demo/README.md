# Live adoption demonstration

[Workflow run 41342977554](https://github.com/emmett08/observatory-instrumentation/actions/runs/41342977554)
passed on Ubuntu and macOS. The action under test resolved
[PR #2](https://github.com/emmett08/observatory-instrumentation/pull/2)'s actual
closing relationship to [issue #1](https://github.com/emmett08/observatory-instrumentation/issues/1).
Both job receipts and the independently discovered collection manifest use
protocol `observatory-instrumentation-demo-v1` and task
`github-issue-1442160414-1`.

The pinned core collector downloaded both bundles, verified 29 retained records,
and retained the same four start/completion receipts on repeated acquisition.
Both test commands succeeded, and both invocations matched their GitHub jobs.
The snapshot was collected while the collector job was still running; that job
and the workflow subsequently completed successfully.

The Go suite contains 31 tests, including race-detector checks. Six Python
checks cover private collection orchestration and compatibility. Workflow lint,
generated issue-form structure and installer smoke checks also passed. A separate
read-only discovery exercise found eight EngArg/EAL runs, including five cancelled
runs, and preserved their two issue/PR bindings.

Human effort remains unavailable for this task. GitHub actor/triggering actor
are platform identities; the declared controller remains unknown. No activity
or sampled human responses were fabricated.

- [Deduplication and task-link verification](verification.json)
- [Automatic discovery evidence](discovery.json)
- [Canonical collector manifest](manifest.json)
- [Matching effort links](effort-links.json)
- [Retained collector snapshot](snapshot.json)
- [Aggregate report index](report-index.json)
- [File hashes and archive provenance](evidence-index.json)

The `*-bundle.json` files preserve the original receipt bytes. The collector
snapshot and discovery files came from the CI artifact. Aggregate reports were
generated from that verified store with the pinned core reporter and no human
batch. The full database remains in the workflow artifact, subject to its
30-day retention. These committed public records contain no private participant
data or credentials.
