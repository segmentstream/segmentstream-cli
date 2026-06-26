{% set identity_link_keys = var('segmentstream_identity_link_keys', []) %}

{% if identity_link_keys | length == 0 %}

-- Keep downstream identity-link models compilable when a project has not opted
-- into identity linking yet.
select
  cast(null as string) as key_name,
  cast(null as string) as tier,
  cast(null as int64) as window_days,
  cast(null as int64) as max_distinct_anonymous_ids
from (select 1) as empty_identity_link_config
where false

{% else %}

-- Treat the project YAML as data so link-generation logic can stay declarative
-- and independent of hard-coded key names.
{% for key in identity_link_keys %}
select
  {{ key["name"] | tojson }} as key_name,
  {{ key["tier"] | tojson }} as tier,
  cast({{ key["window_days"] }} as int64) as window_days,
  cast({{ key["max_distinct_anonymous_ids"] }} as int64) as max_distinct_anonymous_ids
{% if not loop.last %}

union all

{% endif %}
{% endfor %}

{% endif %}
