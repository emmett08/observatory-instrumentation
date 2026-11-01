# Observatory instrumentation

Record explicit tool executions in GitHub Actions and connect them to issues,
pull requests and an optional private human-effort study. This repository supplies
the reusable action, Go installer, task/run discovery and adoption templates.
[Engineering Work Observatory](https://github.com/emmett08/engineering-work-observatory)
supplies the recorder, validated collector, evidence store and estimator.

The [live demonstration](docs/live-demo/README.md) verifies the reusable action,
automatic issue linkage, receipt collection and repeated-delivery deduplication
on Ubuntu and macOS.

The action sees only commands passed through its wrappers. It cannot observe
every tool's internals, another job or work outside Actions. GitHub actor,
triggering actor and event are retained separately from the declared controller
and assistant session. Missing provenance remains unknown. Tool duration is
never converted into human labour.

## Start in one repository

Use Go 1.26.10 or later, Git and an authenticated GitHub CLI. On a reviewed
checkout of this repository:

```sh
go build -o /tmp/observatory-instrumentation ./cmd/observatory-instrumentation
integration_revision=$(git rev-parse HEAD)
/tmp/observatory-instrumentation init \
  --directory /path/to/your/repository \
  --repository owner/project \
  --protocol engineering-study-v1 \
  --action-ref "$integration_revision" \
  --tool go --tool-version unknown \
  -- go test ./...
```

Review the generated `.observatory/config.json`, bounded test workflow, task and
effort issue forms, and adoption guide. Add any tool installation or dependency
setup your selected command needs. The installer preserves existing files and
refuses conflicting content. The example action revision must be a reviewed
commit that is available in `emmett08/observatory-instrumentation`; a local
unpublished commit cannot be fetched by GitHub Actions.

Use `adopt` with the same arguments and `--branch observatory/adoption` to create,
push and open an adoption PR from a clean checkout. `init` only writes local
files. `adopt` performs those GitHub writes explicitly and never merges the PR.
See [installation](docs/installation.md) for a complete walkthrough.

Create a task issue and put `Closes #123` in the implementing PR's description.
When GitHub returns exactly one eligible closing-issue relationship, the action
derives the task ID `github-issue-<repository-id>-<issue-number>`. The private
discovery process uses the same rule. Registered aliases support existing study
IDs. Multiple issues, unavailable relationships and unmapped events retain an
unknown task; inspect the resolution evidence rather than guessing.

## Add collection and human-effort observations

The [private runner template](templates/private-runner-workflow.yml) discovers
new runs in allowlisted repositories/workflows, collects every selected run,
and reports known tasks. Failed, cancelled, skipped and unresolved executions
remain evidence. The template belongs in a private control repository on a
trusted Linux runner. It is not activated by this public repository's CI.

Follow the [private runner guide](docs/private-runner.md) to configure durable
storage, credentials and optional ActivityWatch sources. For an effort study,
declare the participant/time frame, freeze predictions and answer the selected
private review forms. No continuous timesheet is required, but the code cannot
invent those observations. Missing responses leave human estimates unavailable.
The optional public effort issue form records a separate declared observation;
it does not automatically enter the estimator.

| Material | Purpose |
| --- | --- |
| [Installation](docs/installation.md) | Installer, generated workflow and issue linking |
| [Configuration and discovery](docs/configuration.md) | Shared IDs, scope, unknown attribution and bounded acquisition |
| [Private runner](docs/private-runner.md) | Collection, sampled forms, reporting and recovery |
| [Evidence contract](docs/evidence.md) | Observed versus declared evidence and interpretation limits |
| [Examples](examples/) | Public configuration and incomplete private effort template |
| [Compatibility and releases](docs/releases.md) | Core pin, supported hosts and upgrades |

The current integration action uses standard Ubuntu/macOS runners with Go,
Python 3 and `gh`; its setup installs the pinned Go version. The private
orchestration template targets Linux. No model/API service is required by the
collector or estimator. See [costs](docs/costs.md) for the operating assumptions.

This repository retains its [GPL-3.0 licence](LICENSE). Installing the kit does
not deploy a private runner, create participant observations or establish
measurement accuracy. The [existing EngArg/EAL pilot](https://github.com/emmett08/engineering-work-observatory/blob/32420a941a97221709447f8b71d6715216c566e9/docs/linked-pilot-evidence/README.md)
contains real execution evidence and explicitly unavailable human estimates.
