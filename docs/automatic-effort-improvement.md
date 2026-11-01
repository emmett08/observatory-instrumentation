# Automatic public observation import and prediction demo

The [public demo workflow](../.github/workflows/observation-learning-demo.yml)
runs on a standard GitHub-hosted Ubuntu runner. It imports declared effort from
public issues, links it to execution evidence and tests whether a task-total
prediction model has improved. It needs no private runner, workstation activity,
AI API or personal access token. It uses the same canonical implementation as
Observatory core; this repository does not maintain another model or importer.

The [verified hosted run](observation-learning-evidence/README.md) retains the
demo result and artifact hashes.

For the real pilot, [EngArg #51](https://github.com/emmett08/engarg/issues/51)
declares **three human person-minutes** for task
[EngArg #49](https://github.com/emmett08/engarg/issues/49) and
[PR #50](https://github.com/emmett08/engarg/pull/50). Its measurement method and
coverage remain unknown. The report retains those three minutes as a declaration
beside execution evidence. That observation alone cannot train or validate a
useful predictor. A separate, explicitly synthetic dataset demonstrates the
model update mechanics; it provides no evidence of real predictive accuracy.

## Run and inspect the demo

1. Open **Actions → Public observation learning demo → Run workflow** and choose
   the default branch after this workflow is merged. For the first trusted run,
   select `bootstrap`; leave it off for subsequent runs. No secrets or runner
   setup are needed. Relevant pull requests also run an isolated preview that
   imports real declarations without fitting or scoring a live model; the
   synthetic learning example still runs. Live model evaluation is enabled only
   for trusted default-branch runs. The weekly schedule becomes active on the
   default branch after merge.
2. Read the run summary, then download its report and checkpoint artifacts.
   Their names begin `observatory-observation-report-` and
   `observatory-observation-state-`. The report distinguishes declarations,
   execution receipts, missing evidence, real-data model status and synthetic
   demonstrations. It does not post issue
   or PR comments or edit observations.
3. Run it again. The workflow restores the last successful trusted checkpoint,
   fetches current versions of open **and closed** observation issues and
   deduplicates repeated versions. An unchanged declaration does not become
   another training example because a workflow ran again.

The workflow calls the full-commit-pinned core reusable workflow. Its `core-ref`
must equal the commit in `uses`; update both together. The recorder action has
its own compatible core pin, documented in [releases](releases.md). Reusing the
public workflow requires only `contents: read`, `issues: read` and
`actions: read`. It does not inherit caller secrets.

Core and this integration repository each expose the same demonstration with
separate checkpoint histories. Running both repeats the demonstration on the
same source observations; it does not create two independent studies or more
validation evidence. Choose one canonical repository/workflow history for a
real study and retain its checkpoints. Do not combine promotions or validation
claims from independently restarted copies of the same data.

## Record a usable declaration

The installer supplies the versioned
[effort observation issue form](../.github/ISSUE_TEMPLATE/effort-observation.yml).
Open an issue with that form in a configured public source repository. It is
fine to close the issue after recording it: closure does not remove the
observation or make it more or less suitable for training.

Use the same protocol and task IDs as the execution receipts. The collector's
task mapping registers the task issue and related PRs; the form's optional task
ID and PR links must agree with that mapping. For the current EngArg example,
the protocol is `observatory-linked-pilot-v1`, the task ID is
`engarg-authoring-effort-pilot`, the task issue is `emmett08/engarg#49` and the
PR is `emmett08/engarg#50`. The observation issue is a separate record.

| Field | What to enter |
| --- | --- |
| Observation schema | Keep `observatory.effort-observation/1` |
| Declared human person-minutes | One non-negative number, or `unknown`; do not use issue age or CI duration |
| Measurement method | `timer`, `recall` or `unknown` |
| Coverage | `complete`, `partial` or `unknown`, covering all people in the declared scope |
| Scope | `task_total`, `partial_task` or `unknown` |
| Work completed at | Known RFC3339 UTC timestamp, or `unknown`; it is not an effort duration |

Unknown is the default. Do not select complete coverage or a known method merely
to make a row eligible. The importer validates exact versioned fields and does
not infer missing effort from narrative text. A narrowly supported legacy
adapter preserves the known, hash-checked EngArg declaration; arbitrary
unstructured issues are not parsed as effort. If its body is edited, it must
migrate to the versioned form or canonical payload before import can accept it.
Closing or reopening it does not require migration. No participant identity or private activity is
required in the public form. A GitHub posting account is source provenance,
not proof that the account holder did the work.

To correct a declaration, edit its source issue. The next successful import
retains the newly observed revision and validates the latest record for
reporting. A malformed latest revision is reported as rejected, rather
than silently reusing an earlier value. Conflicting records are excluded from
learning. The importer preserves revisions it actually fetched; GitHub's
current-issue API cannot reconstruct every intermediate edit between polls.

## How automatic improvement is decided

The model predicts a **whole task's declared human minutes**. It is separate
from the private estimator that uses sampled responses bound to a frozen time
frame. Public task declarations never substitute for those sampled responses.

Only complete task totals with a numeric value and a known timer/recall method
are eligible for learning. Partial and unknown observations still appear in the
report. Duplicate deliveries and duplicate task labels do not add statistical
weight. The real-data and synthetic model states are kept separate.

The initial predictor is the median effort in the training set. The candidate
adds per-repository medians, pulled toward the overall median when a repository
has few observations. Neither predictor uses workflow duration, commit spacing
or issue age as human minutes. This intentionally small model makes the demo
auditable; repository membership alone may not capture task difficulty.

Before fitting, a stable task-based split assigns about 70% of tasks to training
and 30% to validation. All revisions of one task stay in the same partition.
Training data fits the models; validation data measures their absolute error
and bias. The demo requires at least eight training tasks and four validation
tasks, then promotes a candidate only if validation mean absolute error improves
over the baseline by both 10% and one minute without worsening error relative
to an existing active model. These are demonstration safeguards, not a sample
size justification or a guarantee of accuracy.

Model versions, training revision IDs, policy, predictions, validation errors
and promotion decisions are retained. Repeating the same evaluation is
idempotent. Validation tasks consumed by an adaptive comparison are not reused
as fresh validation for the next candidate; new comparisons need new eligible
validation tasks. A failed or unsupported update retains a still-valid active
model. If a supporting training or approval-validation declaration is corrected,
withdrawn or becomes ineligible, its active model is retired and predictions
remain unavailable until a fresh supported model is approved. This prevents a
retracted effort label from silently continuing to support predictions.

The live pilot should currently report insufficient eligible data. Synthetic
fixtures exercise fitting, validation, conditional promotion and future
prediction in a separate section. Their successful checks demonstrate software
behaviour only. Even held-out results on real historical declarations need
prospective validation on later tasks before operational accuracy is claimed.

## State, scope and recovery

Core owns schema validation, issue acquisition, revision retention, receipt
ingestion, model updates and report generation. This repository owns the
installer form and public entry workflow. Core's reviewed public demo
configuration allowlists source repositories, observation issue discovery and
task/PR mappings; it checks that source repositories are public before
retaining data. Extending it requires a reviewed configuration change with
matching protocol/task identifiers, not a wider token or a guessed task link.

Issue acquisition includes `state=all`, explicit known observation issues and
bounded discovery. Incomplete API acquisition or missing execution uploads
remain visible gaps. A missing receipt is not zero tool use, and an issue's
elapsed lifetime is not observed human work. Allowlisted execution evidence is
retained through the canonical receipt collector.

Each run writes a separate checkpoint and report. Only successful default-branch
scheduled or manually dispatched runs in the same repository/workflow can
supply the next trusted checkpoint. Pull-request previews cannot replace that
history. Restore validates the checkpoint's hash manifest and archive paths;
it never executes downloaded files. Per-ref concurrency serializes updates.

Artifacts have 30-day retention. A first trusted run requires explicit manual
bootstrap; the schedule cannot initialize history silently. If a known prior
checkpoint expires, is missing or fails integrity checks, restoration fails
instead of silently discarding revisions and model
history. Keep the schedule running and download/back up checkpoints if a longer
audit history is needed. The bootstrap option never bypasses a broken existing
history. Retention is not a durable database service; recovery requires a
verified retained checkpoint and a reviewed recovery path, or an explicitly
reviewed new workflow namespace.

An intervening failed or cancelled trusted run may have evaluated validation
labels without saving their consumption. The next run therefore fails closed
when that cannot be ruled out; it does not fall back to an older checkpoint and
reuse those labels. A first bootstrap that fails before observation import can
be retried only when GitHub's job records establish that the import step was
skipped. Ambiguous interrupted histories need the reviewed recovery path above.

Automation removes repeated import, deduplication, reporting and model
comparison work. It does not create observations of human attention. Better
real estimates still need enough suitable declarations with known coverage,
fresh validation data and review of systematic reporting error. No private
runner is required for this public demonstration; the separate
[private collection setup](private-runner.md) remains relevant when collecting
real activity and sampled responses.
