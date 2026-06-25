{{ config(tags=['segmentstream_source_verify']) }}

{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

with source_rows as (
  select 1 as present
  from {{ ref('identity_keys') }}
  limit 1
)

select failure
from unnest(['Source returned no identity key rows for the SegmentStream verification window.']) as failure
where not exists (select 1 from source_rows)
