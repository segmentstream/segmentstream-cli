{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

select
  event_id,
  anonymous_id,
  event_name,
  page_url,
  page_referrer,
  event_timestamp,
  event_date
from {{ ref('stg_events___SOURCE_NAME__') }}
where event_date >= date('{{ segmentstream_start_date }}')
  and event_date < date('{{ segmentstream_end_date }}')
