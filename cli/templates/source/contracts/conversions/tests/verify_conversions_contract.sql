{{ config(tags=['segmentstream_source_verify']) }}

{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

with source_rows as (
  select
    cast(date as date) as date,
    cast(conversion_time as timestamp) as conversion_time,
    cast(conversion_name as string) as conversion_name,
    cast(conversion_id as string) as conversion_id,
    cast(conversion_value as float64) as conversion_value
  from {{ ref('conversions') }}
)

select *
from source_rows
where date is null
   or conversion_time is null
   or conversion_name is null
   or conversion_id is null
   or date < date('{{ segmentstream_start_date }}')
   or date >= date('{{ segmentstream_end_date }}')
   or date != date(conversion_time)
