select distinct
  segmentstream_source,
  date,
  anonymous_id,
  key_name,
  key_value
from {{ ref('int_identity_keys__unioned') }}
