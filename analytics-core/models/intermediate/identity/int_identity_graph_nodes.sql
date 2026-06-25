-- Resolve the graph over every anonymous ID that has emitted an identity key,
-- including users that never form an edge.
select
  anonymous_id,
  min(date) as first_seen_date
from {{ ref('identity_keys') }}
where anonymous_id is not null
  and date is not null
group by 1
