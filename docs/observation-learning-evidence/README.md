# Reusable hosted demo proof — 1 November 2026

[Run 41346760938](https://github.com/emmett08/observatory-instrumentation/actions/runs/41346760938)
passed the pinned reusable workflow at core revision
`159f3de6345d1f787922a52e642e21b7373f4659` on a GitHub-hosted runner.

The run imported closed EngArg #51 as **3 declared human minutes**, linked it to
task issue #49 / PR #50 and collected both selected tool invocations. It reported
their 42.951 seconds separately from human labour. Live model assessment excluded
the declaration because its method and coverage are unknown. PR previews do not
fit or score live models.

The separate synthetic example fitted on 83 tasks, evaluated on 45 held-out
tasks, passed its promotion gates and produced new predictions. The workflow
sealed, uploaded, downloaded and verified its checkpoint. The downloaded report
also passed an independent digest check; see [verification.json](verification.json).

This demonstrates reuse and delivery without a private runner or paid model API.
It supplies no evidence of improved real predictions. The core and caller runs
use the same observation and synthetic fixture, so their counts must not be
combined as independent evidence.

The full public report and source snapshot are retained in
[Observatory core](https://github.com/emmett08/engineering-work-observatory/tree/b40ce9ffecfa86321210329f9d752e43d459a6c8/docs/observation-learning-evidence).
For ongoing default-branch collection after merge, perform the first manual
`bootstrap=true` run, then use the schedule or ordinary manual dispatches. See
the [operation and recovery guide](../automatic-effort-improvement.md).
