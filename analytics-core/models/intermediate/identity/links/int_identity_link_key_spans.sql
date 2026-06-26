with identity_link_key_observations as (
  select * from {{ ref('int_identity_link_key_observations') }}
)

-- Collapse repeated daily key spans so link eligibility is about whether two
-- users' key histories are close, not how often they emitted.
select
  key_name,
  key_value,
  anonymous_id,
  tier,
  window_days,
  max_distinct_anonymous_ids,
  min(daily_first_observed_at) as first_key_seen_at,
  max(daily_last_observed_at) as last_key_seen_at
from identity_link_key_observations
group by 1, 2, 3, 4, 5, 6
