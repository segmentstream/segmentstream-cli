# __SOURCE_NAME__ Source Implementation Guide

This source package is the adapter between your raw warehouse data and the
SegmentStream events contract. The CLI created the package structure, but the
source is not implemented yet.

Your job is to declare the raw inputs and write the SQL model that emits
canonical SegmentStream events.

## Files

- `models/schema.yml` declares the raw warehouse tables this source reads from
  and documents the output `events` model.
- `models/events.sql` transforms those raw inputs into the events contract.
- `contract.yml` is the pinned contract snapshot this source must satisfy.
- `source.yml` mirrors the raw input example for humans and agents. The dbt
  declaration used by the package lives in `models/schema.yml`.

## Raw Inputs

Edit `models/schema.yml` and replace every `REPLACE_WITH_*` placeholder.

For BigQuery source declarations:

- `database` is the raw BigQuery project.
- `schema` is the raw BigQuery dataset.
- `identifier` is the physical table name.
- `name` under `tables` is the logical name used from SQL.

Example:

```yaml
sources:
  - name: __SOURCE_NAME___raw
    description: Raw tables for the __SOURCE_NAME__ source.
    database: my-raw-project
    schema: raw_events_dataset
    tables:
      - name: events
        identifier: raw_events_table
        description: Raw event records.
```

Then SQL can reference that table with:

```sql
from {{ source('__SOURCE_NAME___raw', 'events') }}
```

Add more `tables` entries or additional source declarations if this source
needs multiple raw tables.

## Output Contract

`models/events.sql` must return these columns:

- `event_id` as `STRING`
- `anonymous_id` as `STRING`
- `event_name` as `STRING`
- `page_url` as `STRING`
- `page_referrer` as `STRING`
- `event_timestamp` as `TIMESTAMP`
- `event_date` as `DATE`

`event_id` and `event_date` are required. Optional fields may be `null` when the
raw source does not provide them.

You can inspect the contract from the CLI:

```sh
segmentstream source contracts --type events --json
```

## Implementation Steps

1. Browse raw warehouse tables and schemas:

   ```sh
   segmentstream warehouse browse --path <project>/<dataset> --json
   segmentstream warehouse browse --path <project>/<dataset>/<table> --json
   ```

2. Replace placeholders in `models/schema.yml` with the raw input tables.

3. Replace the template query in `models/events.sql` with SQL that maps the raw
   inputs to the events contract.

4. Keep the SegmentStream date window in the model when the raw data has a date
   or timestamp field:

   ```sql
   where <event_date_expression> >= date('{{ segmentstream_start_date }}')
     and <event_date_expression> < date('{{ segmentstream_end_date }}')
   ```

5. Add this source package to the project `segmentstream.yml`:

   ```yaml
   sources:
     - name: __SOURCE_NAME__
       path: ./sources/__SOURCE_NAME__
   ```

6. Run the project once the source is implemented:

   ```sh
   segmentstream run
   ```
