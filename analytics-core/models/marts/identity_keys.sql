select distinct
  segmentstream_source,
  date,
  anonymous_id,
  key_name,
  key_value,
  daily_first_observed_at,
  daily_last_observed_at
from {{ ref('int_identity_keys__daily_spans') }}
