# AXprobe

This directory defines SegmentStream CLI e2e scenarios for
[`segmentstream/axprobe`](https://github.com/segmentstream/axprobe).

The axprobe box image is built from this checkout and contains the
`segmentstream` binary plus runtime CA certificates. It does not install
`gcloud`, `bq`, `gsutil`, or any other BigQuery helper CLI; `.axprobe/config.yaml`
fails the run if any of those commands are present.

## Run

Install axprobe once if it is not already on `PATH`:

```sh
go install github.com/segmentstream/axprobe@latest
```

Run the full analytics scenario:

```sh
OPENROUTER_API_KEY=... ./scripts/axprobe-e2e.sh
```

The BigQuery service-account key is copied from
`.config/segmentstream-axprobe-e2e.json` into the sandbox at
`/run/secrets/segmentstream-bigquery.json`. The sandbox also exposes its path as
`SEGMENTSTREAM_BIGQUERY_SERVICE_ACCOUNT_KEY`, so the run should not prompt for
OAuth or any credential input.

Reports are written under `tests/axprobe-reports/`, which stays ignored with the
rest of `tests/`.

The scenario's expected result is `goal_reached`: axprobe should configure
BigQuery, create or reuse the test dataset, set up a minimal synthetic events
source, and complete `segmentstream run`.
