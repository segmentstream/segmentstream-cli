with identity_link_key_config as (
  select * from {{ ref('int_identity_link_key_config') }}
),

identity_keys as (
  select * from {{ ref('identity_keys') }}
)

select
  identity_keys.segmentstream_source,
  identity_keys.anonymous_id,
  identity_keys.date,
  identity_link_key_config.key_name,
  identity_keys.key_value,
  identity_keys.daily_first_observed_at,
  identity_keys.daily_last_observed_at,
  identity_link_key_config.tier,
  identity_link_key_config.window_days,
  identity_link_key_config.max_distinct_anonymous_ids
from identity_keys
inner join identity_link_key_config
  on identity_keys.key_name = identity_link_key_config.key_name
-- Source packages own extraction quality; core still guards against malformed
-- contract rows before they can create graph edges.
where identity_keys.segmentstream_source is not null
  and identity_keys.date is not null
  and identity_keys.daily_first_observed_at is not null
  and identity_keys.daily_last_observed_at is not null
  and identity_keys.anonymous_id is not null
  and identity_keys.key_name is not null
  and identity_keys.key_value is not null
