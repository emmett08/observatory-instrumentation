# What the evidence means

The recording and estimation algorithms live in pinned Observatory core. This
integration resolves task relationships, invokes explicit wrappers, discovers
selected runs and prepares adoption files. Its JSON discovery/config versions
are separate from core receipt and effort schemas.

| Evidence | Interpretation |
| --- | --- |
| GitHub repository/run/attempt/job metadata | Platform-observed execution identity |
| Original actor, triggering actor and event | Platform account/trigger provenance, not proof of the actual controller |
| Controller, session, protocol and task declarations | Caller declarations with separate supporting provenance |
| GitHub closing-issue relationship | Observed issue/PR linkage at acquisition time |
| Configured task alias | Declared stable protocol mapping |
| Tool/version/model/usage/result receipt | What the explicit adapter observed or was told, with missing fields retained |
| Activity proxy | Auxiliary prediction, not an observed human duration |
| Sampled interval response | Human declaration bound to the frozen frame and selected sample |
| Optional public effort issue | Unbound supporting self-report, not an estimator sample |

An assistant can use a person's credentials; a person can launch an automated or
AI workflow. Preserve the distinction. Failed, cancelled and skipped platform
jobs remain observations. Missing receipts indicate an observation/delivery gap,
not zero tool use. A hash establishes integrity of retained bytes; it does not
certify a truthful controller declaration or a complete instrumentation boundary.

Only wrapped commands produce receipts. Multiple jobs publish separate bundles,
and durable collection validates them before import. A post-job or `always()`
publication step cannot guarantee delivery after runner loss. Independent run,
attempt and job acquisition is needed to see those gaps.

The effort model's sampling standard error covers its specified sampling
design. It does not quantify recall error, omitted people, missing devices,
misclassification or work outside the declared windows. Calibration and review
are still required before operational use. Neither receipts nor estimates
establish requirement acceptance, retained quality, individual performance or a
causal AI productivity effect.

See core's [execution contract](https://github.com/emmett08/engineering-work-observatory/blob/32420a941a97221709447f8b71d6715216c566e9/docs/execution-evidence.md)
and [effort guide](https://github.com/emmett08/engineering-work-observatory/blob/32420a941a97221709447f8b71d6715216c566e9/docs/effort-estimation.md)
for the canonical schemas and measurement assumptions.

## Occasional effort observations

For a worked aggregate observation, see
[EngArg #51](https://github.com/emmett08/engarg/issues/51), linked to task #49
and PR #50. It records the user's declared three human person-minutes separately
from GitHub's issue/PR lifecycle timestamps. Exact work start/end and the
timer-versus-recall method remain unknown. Such a declaration supplements the
execution evidence; it is not automatically treated as a sampled response.

