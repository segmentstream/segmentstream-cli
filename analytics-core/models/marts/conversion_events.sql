select
  date,
  conversion_time,
  conversion_name,
  conversion_id,
  conversion_value
from {{ ref('int_conversion_events__unioned') }}
