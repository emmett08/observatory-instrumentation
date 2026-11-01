# Configuration examples

`config.json` is a public study configuration. Replace `owner/application` with
the adopting repository and use one protocol ID consistently in the action and
private collector. Add each deliberately selected workflow path. `tasks: []`
enables deterministic issue-derived IDs without manual task alias registration.

`effort-template.json` is a private preparation template. Its participant, device,
buckets and dates are placeholders, not observations. Replace them with the
declared study frame and actual selected ActivityWatch buckets. `frame_complete`
is deliberately false and must not be changed merely because telemetry arrived.
`link-effort` fills issue/task links from retained discovery before a new batch is
frozen. Neither this example nor the public issue form manufactures sampled
responses.
