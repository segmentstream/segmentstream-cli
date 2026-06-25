-- Use the earliest key observation as the stable direction signal for link
-- edges until a richer user/profile first-seen model exists.
select
  anonymous_id,
  min(daily_first_observed_at) as first_seen_at,
  {{ segmentstream_timestamp_to_date('min(daily_first_observed_at)') }} as first_seen_date
from {{ ref('identity_keys') }}
where date is not null
  and daily_first_observed_at is not null
  and anonymous_id is not null
group by 1
