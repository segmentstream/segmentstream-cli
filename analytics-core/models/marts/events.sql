select
  segmentstream_source,
  event_id,
  anonymous_id,
  event_name,
  page_url,
  page_referrer,
  event_timestamp,
  event_date
from {{ ref('int_events__unioned') }}
