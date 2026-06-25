with identity_link_key_observations as (
  select * from {{ ref('int_identity_link_key_observations') }}
)

-- Collapse repeated key observations into date ranges so link eligibility is
-- about whether two users' key histories are close, not how often they emitted.
select
  scope,
  scope_value,
  key_name,
  key_value,
  anonymous_id,
  tier,
  window_days,
  max_distinct_anonymous_ids,
  min(date) as first_key_seen_date,
  max(date) as last_key_seen_date
from identity_link_key_observations
group by 1, 2, 3, 4, 5, 6, 7, 8
