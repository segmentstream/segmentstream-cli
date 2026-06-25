{% set segmentstream_identity_key_sources = var('segmentstream_identity_key_sources', []) %}
{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

{% if segmentstream_identity_key_sources | length == 0 %}

select
  cast(null as string) as segmentstream_source,
  cast(null as date) as date,
  cast(null as timestamp) as observed_at,
  cast(null as string) as anonymous_id,
  cast(null as string) as key_name,
  cast(null as string) as key_value
from (select 1) as empty_project
where false

{% else %}

{% for source in segmentstream_identity_key_sources %}
select
  '{{ source["name"] }}' as segmentstream_source,
  date,
  observed_at,
  anonymous_id,
  key_name,
  key_value
from {{ ref(source["package_name"], source["identity_keys_model_name"]) }}
where date >= date('{{ segmentstream_start_date }}')
  and date < date('{{ segmentstream_end_date }}')
{% if not loop.last %}

union all

{% endif %}
{% endfor %}

{% endif %}
