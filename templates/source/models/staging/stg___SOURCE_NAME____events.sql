-- Map the raw source table to SegmentStream's canonical event shape.
-- If your raw table uses different column names, change this model and keep
-- the exported model contract stable.

select
  cast(event_id as string) as source_event_id,
  cast(anonymous_id as string) as anonymous_id,
  cast(user_id as string) as user_id,
  cast(event_name as string) as event_name,
  cast(event_timestamp as timestamp) as event_timestamp,
  date(cast(event_timestamp as timestamp)) as event_date
from {{ source('__SOURCE_NAME___raw', 'events') }}
