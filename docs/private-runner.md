# Set up private collection and reporting

Use a private control repository and a dedicated trusted Linux runner for human
activity and sampled responses. This public repository distributes templates;
it does not register a runner or receive participant data.

## One-time setup

1. Register a trusted Linux self-hosted runner in the private control repository
   with label `effort-collector`. Install Git, `gh`, Python 3.10+, Go 1.26.10 and
   Rustup with Rust 1.99.1. Restrict who can edit or dispatch the default-branch
   workflow. Do not attach this runner to untrusted public PR workflows.
2. Create a durable directory outside the checkout workspace, owned by the
   runner account with permissions `0700`, for example `/srv/observatory-private`.
   Use `umask 077`, arrange private backups and choose a retention policy.
3. Copy the reviewed public study configuration to `config.json` in that
   directory. Use the same protocol and task aliases as the adopting actions;
   allowlist the repositories and workflow paths deliberately.
4. Copy [the workflow template](../templates/private-runner-workflow.yml) to
   `.github/workflows/observatory-private.yml` in the private control repository.
   Replace its integration placeholder, if present, with a reviewed full commit
   from this repository. Keep the reviewed full core revision pin. Set
   repository variable `OBSERVATORY_PRIVATE_ROOT` to the durable directory.
5. Configure `OBSERVATORY_READ_TOKEN` with only the selected repositories'
   Actions, pull-request, issue and required metadata read access. Use an
   appropriate GitHub App or fine-grained token. The private control repository's
   own `GITHUB_TOKEN` does not provide cross-repository access.
6. If public aggregate comments are wanted, separately configure
   `OBSERVATORY_PUBLISH_TOKEN` as a user token with issue/PR comment permissions
   on those repositories. The pinned publisher calls `GET /user`, so a GitHub
   App installation token is not supported for publication. The write token is
   exposed only on an explicit publication dispatch.

The template refuses a public control repository or a non-default-branch run.
It checks out reviewed integration/core commits, builds their tools, and reads
downloaded artifacts as data. It never checks out subject PR code on this
runner. Logs, SQLite state, forms and responses stay in private storage; the
workflow has no artifact upload step.

## Collect real execution evidence first

Dispatch `collect-and-report`. All operations refresh discovery and collect the
selected runs. `collect` stops there. If there are known tasks, reports join
their validated receipts to independently collected GitHub run, attempt and job
metadata. If none can be resolved, discovery/collection coverage is retained
without inventing a task manifest. Without a participant batch, human estimates
are unavailable and the execution report is still useful.

The default schedule runs at 19:15 and 21:15 UTC on weekdays, approximately 44
runs in a 22-working-day month. It collects and reports without posting comments.
Change the cadence and search window for the repository's workload and artifact
retention. Each run uses a new immutable acquisition directory and the same
durable receipt store.

```text
/srv/observatory-private/
  config.json                    # Public study scope, retained privately too
  effort-template.json           # Actual people/windows/sources, when configured
  executions/                    # Verified durable receipt and platform store
  runs/<run-id>-<attempt>/
    discovery/                   # Selected runs, links, coverage and manifest
    collection.json              # All selected run acquisition outcomes
    reports/                     # Safe aggregate outputs
    *.log                        # Private diagnostics
  batches/YYYY-MM-DD/
    inputs/                      # Original config/link/manifest inputs + ready marker
    config.json
    activity.json
    frame.json
    sample.json
    review-index.html
    review-<hash>.html
    responses/
```

The adapter [collect_discovered.py](../templates/collect_discovered.py) serializes
selected run collection, including unknown tasks, invokes Observatory's schema/
hash importer and verifies the store. It does not implement another receipt
validator. Collection continues after an individual failure and retains the
failure. Incomplete discovery or collection leaves the workflow unsuccessful;
retained evidence can still produce a private report.

## Configure a real effort batch

Install ActivityWatch and an appropriate context watcher on each participating
workstation. The ordinary active-window watcher alone cannot reliably provide
task/project/branch attribution. Confirm the chosen watcher emits the allowlisted
fields the core collector accepts. See the canonical
[activity-source instructions](https://github.com/emmett08/engineering-work-observatory/blob/32420a941a97221709447f8b71d6715216c566e9/docs/effort-estimation.md).

Privately copy and edit [effort-template.json](../examples/effort-template.json):

- Replace its placeholder person, device, bucket and UTC window with the declared
  participants and intended observation frame.
- Keep its protocol ID equal to the public study configuration. Discovery fills
  issue/PR links before the new batch is frozen; exact project aliases, if used,
  remain declared and should be unambiguous.
- Set `frame_complete` true only when the declared roster/windows define the
  intended target. Telemetry arriving does not establish this.
- Choose `OBSERVATORY_SAMPLE_SIZE` before drawing. The suitable size depends on
  variability and the precision needed; the software does not validate accuracy
  from a fixed sample count.

The runner must reach the actual source. A runner on the participant workstation
can read ActivityWatch on `http://127.0.0.1:5600`. A remote runner cannot reach a
desktop's loopback server. For a separate host, transfer a bounded export
privately and set `OBSERVATORY_ACTIVITY_EXPORT` to its local path, or follow the
core's multi-device collection/merge procedure. Never upload raw activity to the
public repository.

After the declared observation window ends, dispatch `prepare` and select its
UTC date. The workflow retains the first linked configuration, task manifest and
discovery evidence under `batches/<day>/inputs/`, then calls Observatory's
preparation pipeline. Retries reuse these inputs and the saved probability
sample; they do not draw a more convenient sample. A partially written input
directory without its `ready` marker stops preparation for inspection.

Send each participant only their own offline review form. They allocate the
selected intervals to task/role and non-work, then save the downloaded JSON into
that batch's `responses/` directory. Uncertain recall stays nonresponse. The
form hides model predictions. The human must supply these observations; the
code cannot estimate their truth from CI elapsed time.

## Report and optionally publish

Dispatch `report` for the batch date after responses arrive. The workflow uses
the batch's retained manifest, so new discovery cannot silently change the
frozen sample's task scope. Execution-only reports use the current discovery
manifest. Each report identifies its selected run scope.

Only a deliberate dispatch with `publish_comment: true` can expose the write
token and request comments on registered issues/PRs. Scheduled runs never post.
Publication also requires complete current discovery and collection; an
incomplete acquisition can still generate a private report for investigation.
Missing activity or sampled responses remains unavailable, rather than zero.
Synthetic effort is refused for live publication. The core publisher upserts
its own marked aggregate comments and omits participant identities, raw
activity and response contents.

Public effort-observation issues are optional supporting declarations. They
cannot substitute for responses bound to the saved frame and sample. The
separate [public hosted demo](automatic-effort-improvement.md) automatically
imports configured declarations and evaluates task-total predictions without a
private runner; it does not insert them into this sampled-interval estimator.

## Recover and extend

Review private discovery/collection diagnostics when a workflow fails. If a
bound truncated the search, use a narrower explicit UTC interval or a reviewed
higher bound in a new acquisition directory. If a run completed later or a
receipt arrived late, reacquire it; repeated receipts deduplicate. Runner loss
or expired artifacts can leave permanent gaps, which must remain visible.

Back up the database and its associated retained evidence together. Stop writes
or use SQLite's supported backup procedure; copying an actively changing SQLite
file alone is not a consistent backup. Keep per-job receipt bundles separate
until serial collection. Upgrade pinned code only after reviewing compatibility
and preserving replayable historical inputs.
