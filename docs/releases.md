# Compatibility and releases

This repository is the integration/distribution layer. The pinned Observatory
core for the action and private collector is `32420a941a97221709447f8b71d6715216c566e9`, which supplies the recorder,
receipt importer, platform reconciliation and sampled-effort pipeline. This kit
does not fork those algorithms or schemas.

The separate public observation-learning workflow requires core
`159f3de6345d1f787922a52e642e21b7373f4659`, which adds the versioned issue importer,
task-total prediction demo and verified checkpoints. Its reusable-workflow pin
and `core-ref` input must match. This newer consumer remains compatible with
the recorder's existing receipt schema; enabling the learning demo does not
require changing the recording action or private sampled-effort pipeline.

The Go CLI targets Go 1.26.10. The composite action installs that version and
expects Python 3 and `gh` on standard Ubuntu/macOS runners. The private template
requires Linux, Python 3.10+, Go 1.26.10, Git, authenticated `gh` and Rust 1.99.1.
Windows action/private-runner support is not claimed by this initial release.
The installer itself can generate files without running the selected tool.

Consumers should pin the action and private integration checkout to a reviewed
full commit. The same integration revision should supply installer templates,
task resolution and private discovery. Tagged versions are useful for finding
releases; a full reviewed commit preserves the executable dependency identity.
Build from reviewed source until release binaries actually exist. Do not assume
that publishing a release creates or configures any private runner.

Before changing a core pin, check receipt/effort schema compatibility, run the
Go/Python tests and workflow linting, and replay representative valid, incomplete,
ambiguous and failed evidence. Keep immutable historical snapshots and frozen
samples. A new code version cannot retroactively turn missing provenance into
an observed controller or establish human effort for an unobserved period.

The repository retains its existing GPL-3.0 licence. Core has its own retained
licence and attribution. Generated adoption files and the integration code
should be reviewed under those terms when redistributed.
