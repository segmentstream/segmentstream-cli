with identity_link_key_spans as (
  select * from {{ ref('int_identity_link_key_spans') }}
),

link_key_group_sizes as (
  -- Highly shared values are usually noisy identifiers, so they should not
  -- explode into dense edge cliques.
  select
    key_name,
    key_value,
    count(distinct anonymous_id) as distinct_anonymous_ids
  from identity_link_key_spans
  group by 1, 2
),

not_skewed_key_spans as (
  select identity_link_key_spans.*
  from identity_link_key_spans
  inner join link_key_group_sizes
    on identity_link_key_spans.key_name = link_key_group_sizes.key_name
    and identity_link_key_spans.key_value = link_key_group_sizes.key_value
  where link_key_group_sizes.distinct_anonymous_ids
    <= identity_link_key_spans.max_distinct_anonymous_ids
)

select
  source_key_span.key_name,
  source_key_span.key_value,
  source_key_span.tier,
  source_key_span.anonymous_id as anonymous_id_a,
  target_key_span.anonymous_id as anonymous_id_b,
  source_key_span.first_key_seen_at as first_key_seen_at_a,
  source_key_span.last_key_seen_at as last_key_seen_at_a,
  target_key_span.first_key_seen_at as first_key_seen_at_b,
  target_key_span.last_key_seen_at as last_key_seen_at_b
from not_skewed_key_spans as source_key_span
inner join not_skewed_key_spans as target_key_span
  on source_key_span.key_name = target_key_span.key_name
  and source_key_span.key_value = target_key_span.key_value
  and source_key_span.anonymous_id < target_key_span.anonymous_id
-- A key can link users when their observation ranges overlap or are close
-- enough to plausibly represent the same underlying identity signal.
where {{ segmentstream_timestamp_diff_seconds(
  'greatest(source_key_span.first_key_seen_at, target_key_span.first_key_seen_at)',
  'least(source_key_span.last_key_seen_at, target_key_span.last_key_seen_at)'
) }} <= source_key_span.window_days * 86400
