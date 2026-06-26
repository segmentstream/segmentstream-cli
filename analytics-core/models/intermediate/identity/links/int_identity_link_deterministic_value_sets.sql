with identity_link_key_observations as (
  select * from {{ ref('int_identity_link_key_observations') }}
)

-- Only deterministic keys become hard constraints. Probabilistic keys such as
-- IP address or click ID can create candidate links, but they are not compared
-- here as veto conditions.
select
  key_name,
  anonymous_id,
  to_json_string(array_agg(distinct key_value order by key_value)) as key_value_set
from identity_link_key_observations
where tier = 'deterministic'
group by 1, 2
