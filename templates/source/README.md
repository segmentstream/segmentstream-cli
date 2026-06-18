# __PACKAGE_NAME__

This is a standard dbt project for the `__SOURCE_NAME__` SegmentStream source.

This is a scaffold, not a completed source implementation. An agent or developer
must inspect the raw source schema and implement the dbt models before treating
this source as ready.

Use this project for source-specific dbt transformations. It should produce
exported models that SegmentStream can compose with core SegmentStream tables.

## Project Shape

```text
dbt_project.yml
macros/
models/
  staging/
  exports/
seeds/
snapshots/
tests/
```

This directory is user-owned project code. It is safe to commit. Generated
SegmentStream environment files still live under the project root
`.segmentstream/` directory.

Models under `models/staging/` are ephemeral by default. dbt inlines them into
exported models instead of creating separate warehouse tables or views.

## Exported Models

`events___SOURCE_NAME__` is the first exported model in this POC. Replace the
raw source mapping in `models/staging/stg_events___SOURCE_NAME__.sql` with the
transformation that reads raw data for this source and returns SegmentStream
event rows.

Exported models under `models/exports/` are incremental and partitioned by
`event_date` by default. Keep `event_date` as a `date` column on every exported
model so SegmentStream can build efficient partitioned tables.

SegmentStream runs exported models with a daily partition window:

```yaml
segmentstream_start_date: "2026-06-17"
segmentstream_end_date: "2026-06-18"
```

Source exports should filter `event_date >= segmentstream_start_date` and
`event_date < segmentstream_end_date`. The end date is exclusive. This keeps
the source export and the core SegmentStream `events` table aligned on the same
daily partition.

The export contract is declared in `models/exports/schema.yml` using dbt-native
model metadata:

```yaml
config:
  meta:
    segmentstream:
      source_name: __SOURCE_NAME__
      export_name: events
      contract: events_v1
```

The SegmentStream CLI will use dbt metadata in a later slice to compose local
and remote sources into the generated environment.

## Raw Source

`models/staging/sources.yml` declares a raw `events` table. By default, it reads
from the configured SegmentStream BigQuery project and dataset using these
variables:

```yaml
__SOURCE_NAME___raw_project
__SOURCE_NAME___raw_dataset
__SOURCE_NAME___raw_events_table
```

Override those variables when this source reads from a different project,
dataset, or table.
