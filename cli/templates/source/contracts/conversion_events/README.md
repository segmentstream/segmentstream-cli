# __SOURCE_NAME__ Conversion Events Source Scaffold

This directory is a generated SegmentStream source scaffold. It is the local dbt
package where the `__SOURCE_NAME__` raw warehouse data is adapted into the
canonical SegmentStream `conversion_events` model.

The scaffold exists so source-specific SQL, raw input declarations, and contract
verification stay close together and separate from the generated SegmentStream
core project. It is intentionally unfinished: an implementation agent or
developer owns the source-specific mapping from raw data to SegmentStream
conversion events.

## Implementation

Use `segmentstream source scaffold __SOURCE_NAME__ --type conversion_events --json`
for the machine-readable list of unresolved implementation items. Then edit:

- `models/schema.yml` to bind raw warehouse inputs.
- `models/conversion_events.sql` to map raw rows into the conversion events
  contract.

The generated files include `SEGMENTSTREAM_TODO(...)` markers at the primary
edit points.

## Output Schema

The expected output schema is defined in `contract.yml`. The same
`conversion_events` columns are mirrored under `models/schema.yml` so dbt can
document and verify the implemented model.

`models/conversion_events.sql` is the author-editable model that emits rows
matching that schema. Emit `conversion_time` in UTC and `date` equal to the UTC
date of `conversion_time`; `conversion_value` may be null.
