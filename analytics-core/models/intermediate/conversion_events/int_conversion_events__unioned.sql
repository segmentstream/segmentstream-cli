{% set segmentstream_conversion_event_sources = var('segmentstream_conversion_event_sources', []) %}
{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

{% if segmentstream_conversion_event_sources | length == 0 %}

select
  cast(null as date) as date,
  cast(null as timestamp) as conversion_time,
  cast(null as string) as conversion_name,
  cast(null as string) as conversion_id,
  cast(null as float64) as conversion_value
from (select 1) as empty_project
where false

{% else %}

{% for source in segmentstream_conversion_event_sources %}
select
  date,
  conversion_time,
  conversion_name,
  conversion_id,
  conversion_value
from {{ ref(source["package_name"], source["conversion_events_model_name"]) }}
where date >= date('{{ segmentstream_start_date }}')
  and date < date('{{ segmentstream_end_date }}')
{% if not loop.last %}

union all

{% endif %}
{% endfor %}

{% endif %}
