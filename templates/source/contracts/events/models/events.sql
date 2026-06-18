{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

select
  cast(event_id as string) as event_id,
  cast(anonymous_id as string) as anonymous_id,
  cast(event_name as string) as event_name,
  cast(page_url as string) as page_url,
  cast(page_referrer as string) as page_referrer,
  cast(event_timestamp as timestamp) as event_timestamp,
  date(cast(event_timestamp as timestamp)) as event_date
from {{ source('__SOURCE_NAME___raw', 'events') }}
where date(cast(event_timestamp as timestamp)) >= date('{{ segmentstream_start_date }}')
  and date(cast(event_timestamp as timestamp)) < date('{{ segmentstream_end_date }}')
