# __SOURCE_NAME__ Identity Keys Source Scaffold

This directory is a generated SegmentStream source scaffold. It is the local dbt
package where the `__SOURCE_NAME__` raw warehouse data is adapted into the
canonical SegmentStream `identity_keys` model.

The scaffold exists so source-specific SQL, raw input declarations, and contract
verification stay close together and separate from the generated SegmentStream
core project. It is intentionally unfinished: an implementation agent or
developer owns the source-specific mapping from raw data to normalized identity
keys.

## Output Schema

The expected output schema is defined in `contract.yml`. The same
`identity_keys` columns are mirrored under `models/schema.yml` so dbt can
document and verify the implemented model.

`models/identity_keys.sql` is the author-editable model that emits rows matching
that schema.
