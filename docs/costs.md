# Operating costs

The kit adds no required hosted service or model/API subscription. ActivityWatch
is free software. The selected wrappers still incur whatever costs their actual
tools normally have; collecting a receipt does not make AI API use free.

GitHub's current billing documentation states that standard public hosted
runners and self-hosted runners have no GitHub compute charge. Larger hosted
runners and storage overages have their own rates. Check the
[current billing terms](https://docs.github.com/en/billing/concepts/product-billing/github-actions)
and account usage before budgeting a broader rollout.

The private workflow needs an existing trusted machine or a separately funded
host, persistent private storage and backups. Its default two weekday runs are
approximately 44 runs per 22-working-day month. Cold builds, API pagination,
historical backfills and the number of retained runs affect duration. The action
also builds its small Go resolver; do not use the earlier thin-wrapper timing as
a measured timing for this integration version.

Human review is a separate operating cost. For illustration, four sampled
checks per working day at one minute each would take about 1.5 hours per person
per month. These are planning assumptions, not measured response times or a
recommended sample size. Include initial source configuration, calibration,
credential rotation, failure review and maintenance in a deployment budget.
