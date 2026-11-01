# Configuration and discovery

The action and private collector read `observatory.instrumentation/1` JSON. Start
from [the example](../examples/config.json), keeping the protocol ID, issue
aliases and selected repository/workflow scope consistent across consumers.
Store public configuration in version control; keep people, device identities,
activity exports, credentials and sampled responses outside it.

| Field | Meaning |
| --- | --- |
| `protocol_id` | Caller-declared study/protocol identity |
| `repositories[].repository` | Explicit `owner/name` acquisition allowlist |
| `repositories[].workflow_paths` | Exact workflow files to discover |
| `tasks[]` | Optional stable aliases with `task_id`, `repository`, `issue_number` |
| `discovery.lookback_days` | Default bounded search window |
| `discovery.max_runs_per_repository` | Acquisition bound; reaching a bound is reported, not treated as complete coverage |

With no alias, an eligible issue receives
`github-issue-<numeric-repository-id>-<issue-number>`. Repository ID is GitHub's
numeric database identity, not the repository name or posting account. A
registered alias can preserve IDs from an existing protocol. Alias registration
is declared configuration; the closing-issue relationship is separately read
from GitHub. Neither proves human attention or requirement acceptance.

Automatic action resolution requires one eligible closing issue for the current
PR. A PR that closes multiple issues, an unavailable API response, a non-PR
event or an unsupported cross-repository relationship leaves task attribution
unknown. Explicit `task-id` declarations must be registered aliases and remain
caller declarations. Do not infer the actual controller from a human-looking
GitHub account.

## Discover ongoing work

Build the CLI and run it under an identity that can read the selected repositories:

```sh
observatory-instrumentation discover \
  --config /srv/observatory-private/config.json \
  --output-dir /srv/observatory-private/runs/2026-11-02/discovery
```

Use a new output directory each time. A bounded historical replay can supply
`--since 2026-11-01T00:00:00Z --until 2026-11-02T00:00:00Z`.
Discovery queries run metadata and PR closing-issue relationships; it never
checks out or executes a subject PR's code. Run status is not a selection filter:
failed, cancelled, queued and incomplete runs can be retained alongside success.

The output contains:

- `discovery.json`: selected runs, repository/workflow scope, attribution status,
  acquisition problems and the explicit completeness result.
- `manifest.json`: Observatory's task/run manifest, present when at least one
  task can be represented.
- `effort-links.json`: task/issue/PR/branch links for a future private effort batch.

Every selected run belongs in collection, including those with no resolved task.
The private adapter reads `discovery.json` rather than relying only on the task
manifest. Unresolved runs therefore still get independent run/attempt/job
metadata and whatever receipts were delivered. A task report covers its selected
manifest, not all engineering activity.

Partial API retrieval or a pagination bound causes an incomplete result and a
nonzero exit after retaining acquired evidence. Narrow the interval or raise a
reviewed bound and rerun into a new directory. Do not remove the incomplete
record or silently treat omitted runs as zero work. A rolling window can miss
work older than that window during an outage; use an explicit backfill interval.
Collect before artifact expiry. Repeated acquisition deduplicates receipts in
the durable store but can add newer platform observations.

Issue links can change after execution. Discovery records the relationships it
observes; it does not rewrite receipt declarations. If the declared task and
later relationship no longer match, keep that distinction and investigate it.
Use a new reviewed configuration revision for alias changes. Existing frozen
effort frames and selected samples keep their original bindings.

## Link a new effort batch

```sh
observatory-instrumentation link-effort \
  --template /srv/observatory-private/effort-template.json \
  --discovery /srv/observatory-private/runs/2026-11-02/discovery \
  --output /srv/observatory-private/batches/2026-11-02/linked-config.json
```

The command preserves participant windows and sources, requires the protocol to
match, and merges discovered task links. Existing project aliases remain
declarations. Observatory validates the complete effort configuration when
preparing the batch. Supply actual participants/windows privately; discovery
cannot infer them from commit authors or workflow actors.
