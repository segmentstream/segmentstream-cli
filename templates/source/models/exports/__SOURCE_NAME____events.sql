-- Exported source models are incremental and partitioned by event_date through
-- dbt_project.yml. Keep this model at one row per source event.

select
  source_event_id,
  anonymous_id,
  user_id,
  event_name,
  event_timestamp,
  event_date
from {{ ref('stg___SOURCE_NAME____events') }}
where event_date is not null

{% if is_incremental() %}
  and event_date >= coalesce(date_sub(date(_dbt_max_partition), interval 1 day), date('1970-01-01'))
{% endif %}
