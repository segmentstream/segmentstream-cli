-- Use the earliest observed identity-key date as the stable direction signal for
-- link edges until a richer user/profile first-seen model exists.
select
  anonymous_id,
  min(date) as first_seen_date
from {{ ref('identity_keys') }}
where date is not null
  and anonymous_id is not null
group by 1
