{% set segmentstream_sources = var('segmentstream_sources', []) %}
{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

{% if segmentstream_sources | length == 0 %}

select
  cast(null as string) as segmentstream_source,
  cast(null as string) as event_id,
  cast(null as string) as anonymous_id,
  cast(null as string) as event_name,
  cast(null as string) as page_url,
  cast(null as string) as page_referrer,
  cast(null as timestamp) as event_timestamp,
  cast(null as date) as event_date
from (select 1) as empty_project
where false

{% else %}

{% for source in segmentstream_sources %}
select
  '{{ source["name"] }}' as segmentstream_source,
  event_id,
  anonymous_id,
  event_name,
  page_url,
  page_referrer,
  event_timestamp,
  event_date
from {{ ref(source["package_name"], source["events_model_name"]) }}
where event_date >= date('{{ segmentstream_start_date }}')
  and event_date < date('{{ segmentstream_end_date }}')
{% if not loop.last %}

union all

{% endif %}
{% endfor %}

{% endif %}
