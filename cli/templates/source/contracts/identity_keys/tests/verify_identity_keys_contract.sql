{{ config(tags=['segmentstream_source_verify']) }}

{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

with source_rows as (
  select
    cast(date as date) as date,
    cast(observed_at as timestamp) as observed_at,
    cast(anonymous_id as string) as anonymous_id,
    cast(key_name as string) as key_name,
    cast(key_value as string) as key_value
  from {{ ref('identity_keys') }}
)

select *
from source_rows
where date is null
   or observed_at is null
   or anonymous_id is null
   or key_name is null
   or key_value is null
   or date < date('{{ segmentstream_start_date }}')
   or date >= date('{{ segmentstream_end_date }}')
   or date != date(observed_at)
