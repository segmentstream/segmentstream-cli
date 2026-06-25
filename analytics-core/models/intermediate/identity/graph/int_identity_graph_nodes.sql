-- Resolve the graph over every anonymous ID that has emitted an identity key,
-- including users that never form an edge.
select
  anonymous_id,
  min(daily_first_observed_at) as first_seen_at,
  {{ segmentstream_timestamp_to_date('min(daily_first_observed_at)') }} as first_seen_date
from {{ ref('identity_keys') }}
where anonymous_id is not null
  and date is not null
  and daily_first_observed_at is not null
group by 1
