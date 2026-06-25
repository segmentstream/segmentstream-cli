with identity_links as (
  select * from {{ ref('identity_links') }}
)

-- Graph resolution is undirected; normalize endpoints so duplicate evidence
-- cannot create duplicate graph edges.
select distinct
  case
    when source_anonymous_id <= target_anonymous_id then source_anonymous_id
    else target_anonymous_id
  end as source_anonymous_id,
  case
    when source_anonymous_id <= target_anonymous_id then target_anonymous_id
    else source_anonymous_id
  end as target_anonymous_id,
  tier
from identity_links
where source_anonymous_id is not null
  and target_anonymous_id is not null
  and source_anonymous_id != target_anonymous_id
  and tier in ('deterministic', 'probabilistic')
