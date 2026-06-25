with identity_key_observations as (
  select * from {{ ref('int_identity_keys__unioned') }}
)

-- Source contracts stay observation-shaped; core owns daily compression so the
-- public key mart is stable and downstream graph work avoids repeated events.
select
  segmentstream_source,
  date,
  anonymous_id,
  key_name,
  key_value,
  min(observed_at) as daily_first_observed_at,
  max(observed_at) as daily_last_observed_at
from identity_key_observations
where segmentstream_source is not null
  and date is not null
  and observed_at is not null
  and anonymous_id is not null
  and key_name is not null
  and key_value is not null
  and date = {{ segmentstream_timestamp_to_date('observed_at') }}
group by 1, 2, 3, 4, 5
