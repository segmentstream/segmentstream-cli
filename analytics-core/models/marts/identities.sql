-- Publish the resolved identity map as a flat contract for downstream models.
select distinct
  anonymous_id,
  identity_id,
  first_seen_date
from {{ ref('int_identities__resolved') }}
