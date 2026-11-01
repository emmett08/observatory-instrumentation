# Install the instrumentation

Start with one selected check and one task issue. Keep the workflow small enough
to see exactly what it observes. Existing test setup, caches and application
commands remain the adopting repository's responsibility.

## Generate an adoption change

Clone this public repository at a reviewed commit and build its Go CLI:

```sh
git clone https://github.com/emmett08/observatory-instrumentation.git
cd observatory-instrumentation
git checkout REVIEWED_FULL_COMMIT_SHA
go build -o /tmp/observatory-instrumentation ./cmd/observatory-instrumentation
integration_revision=$(git rev-parse HEAD)
```

The supplied revision must be a full commit from this integration repository.
Until a release is published, build the reviewed source; do not assume a release
asset exists. Run `init` against your application checkout:

```sh
/tmp/observatory-instrumentation init \
  --directory /path/to/application \
  --repository owner/application \
  --protocol engineering-study-v1 \
  --action-ref "$integration_revision" \
  --tool python3 --tool-version unknown \
  -- python3 -m unittest discover -s tests -p 'test_*.py'
```

Everything after `--` is the selected command's argument vector. The installer
quotes those arguments for the generated wrapper; it does not execute the test.
Use a checked-in script if the check requires a shell pipeline. Review the
workflow before committing, especially dependency/tool installation and trigger
scope. Its `unknown` version is an explicit missing field; replace it with a
known version or a deliberately observed version command where appropriate.

The generated files are:

- `.observatory/config.json`: protocol, repositories, selected workflows and optional task aliases.
- `.github/workflows/observatory.yml`: setup, explicit command wrapper and publication.
- `.github/ISSUE_TEMPLATE/observatory-task.yml`: task registration and acceptance context.
- `.github/ISSUE_TEMPLATE/effort-observation.yml`: optional aggregate self-report.
- `docs/observatory.md`: repository-specific directions.

Existing conflicting files are not overwritten. Choose a clean worktree for
adoption and review any earlier integration instead of deleting it blindly.

To have the CLI create the adoption PR, use `adopt` instead of `init`, with the
same flags plus `--branch observatory/adoption`. This explicitly creates a
branch, commits the generated change, pushes it and opens a PR using configured
`git`/`gh` authentication. It requires a clean checkout and does not merge.

## Link a task and verify a run

1. Open a task issue, using the generated form once it is available on the
   default branch. A normal issue works too.
2. Add `Closes #123` to the implementing PR's body. The task resolver uses
   GitHub's closing-issue relationship, not arbitrary text matching.
3. Run the selected workflow. Grant its read-only token `contents: read` and
   `pull-requests: read` and `issues: read`. The action keeps the controller unknown unless the
   caller supplies a separate supported declaration.
4. Inspect the action's resolution evidence and uploaded receipt bundle.
   Setup outputs include `protocol-id`, `task-id`, `resolution-path`, `recorder`
   and `directory`. An unknown task still permits execution evidence.
5. Use discovery and the private collector to verify the bundle and compare
   its run, attempt and job identity against GitHub's own metadata.

One invocation currently identifies one task. If a PR closes multiple issues,
task resolution remains unknown. Split the check/task scope or use a reviewed
registered alias declaration; do not silently choose the first issue. Automatic
attribution currently supports issues in the same repository as the PR/run.
Cross-repository closing issues remain unresolved for core-manifest compatibility.

Parallel jobs each configure and publish their own directory/bundle. Supply
the exact expanded `job-name` and an explicit flat matrix allowlist. Never point
parallel jobs at a shared evolving evidence database.

## Instrument other tools

The exported `OBSERVATORY_RECORDER` is Observatory's pinned recorder. Its `run`
adapter supports ordinary commands and preserves their exit result. Use its
`codex` adapter for supported assistant output; see the canonical
[execution guide](https://github.com/emmett08/engineering-work-observatory/blob/32420a941a97221709447f8b71d6715216c566e9/docs/execution-evidence.md).
Configured model identity and reported model/token usage remain distinct, and
missing usage remains missing. Merely installing the action does not intercept
unwrapped tool activity or make assistant API calls.

Publication is an `always()` step conditioned on successful setup. A failed
command can therefore leave a completion receipt, but runner loss can still
prevent delivery. Collection and reconciliation must preserve that gap.
