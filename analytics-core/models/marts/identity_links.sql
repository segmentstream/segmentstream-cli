-- Publish one canonical edge per shared key so downstream graph construction can
-- reason about edge evidence without repeated source observations.
select distinct
  source_anonymous_id,
  target_anonymous_id,
  key_name,
  key_value,
  tier,
  source_first_seen_date,
  target_first_seen_date
from {{ ref('int_identity_links__filtered') }}
