with identity_link_candidates as (
  select * from {{ ref('int_identity_link_candidates') }}
),

deterministic_value_sets as (
  select * from {{ ref('int_identity_link_deterministic_value_sets') }}
),

anonymous_id_first_seen as (
  select * from {{ ref('int_identity_anonymous_id_first_seen') }}
),

deterministic_conflicts as (
  -- Deterministic disagreements are vetoes even when the candidate edge came
  -- from a different, weaker key such as IP address.
  select distinct
    identity_link_candidates.scope,
    identity_link_candidates.scope_value,
    identity_link_candidates.key_name,
    identity_link_candidates.key_value,
    identity_link_candidates.anonymous_id_a,
    identity_link_candidates.anonymous_id_b
  from identity_link_candidates
  inner join deterministic_value_sets as source_value_sets
    on identity_link_candidates.anonymous_id_a = source_value_sets.anonymous_id
  inner join deterministic_value_sets as target_value_sets
    on identity_link_candidates.anonymous_id_b = target_value_sets.anonymous_id
    and source_value_sets.scope = target_value_sets.scope
    and source_value_sets.scope_value = target_value_sets.scope_value
    and source_value_sets.key_name = target_value_sets.key_name
  where source_value_sets.key_value_set != target_value_sets.key_value_set
),

allowed_candidates as (
  select identity_link_candidates.*
  from identity_link_candidates
  left join deterministic_conflicts
    on identity_link_candidates.scope = deterministic_conflicts.scope
    and identity_link_candidates.scope_value = deterministic_conflicts.scope_value
    and identity_link_candidates.key_name = deterministic_conflicts.key_name
    and identity_link_candidates.key_value = deterministic_conflicts.key_value
    and identity_link_candidates.anonymous_id_a = deterministic_conflicts.anonymous_id_a
    and identity_link_candidates.anonymous_id_b = deterministic_conflicts.anonymous_id_b
  where deterministic_conflicts.anonymous_id_a is null
),

allowed_candidates_with_first_seen as (
  select
    allowed_candidates.*,
    source_first_seen.first_seen_at as first_seen_at_a,
    source_first_seen.first_seen_date as first_seen_date_a,
    target_first_seen.first_seen_at as first_seen_at_b,
    target_first_seen.first_seen_date as first_seen_date_b
  from allowed_candidates
  inner join anonymous_id_first_seen as source_first_seen
    on allowed_candidates.anonymous_id_a = source_first_seen.anonymous_id
  inner join anonymous_id_first_seen as target_first_seen
    on allowed_candidates.anonymous_id_b = target_first_seen.anonymous_id
),

oriented_links as (
  -- Keep edge direction stable for downstream graph consumers: newer anonymous
  -- IDs point at older ones at timestamp precision, with lexical ordering as a
  -- deterministic tie-break.
  select
    case
      when first_seen_at_a > first_seen_at_b then anonymous_id_a
      when first_seen_at_a < first_seen_at_b then anonymous_id_b
      when anonymous_id_a > anonymous_id_b then anonymous_id_a
      else anonymous_id_b
    end as source_anonymous_id,
    case
      when first_seen_at_a > first_seen_at_b then anonymous_id_b
      when first_seen_at_a < first_seen_at_b then anonymous_id_a
      when anonymous_id_a > anonymous_id_b then anonymous_id_b
      else anonymous_id_a
    end as target_anonymous_id,
    key_name,
    key_value,
    tier,
    case
      when first_seen_at_a > first_seen_at_b then first_seen_at_a
      when first_seen_at_a < first_seen_at_b then first_seen_at_b
      when anonymous_id_a > anonymous_id_b then first_seen_at_a
      else first_seen_at_b
    end as source_first_seen_at,
    case
      when first_seen_at_a > first_seen_at_b then first_seen_at_b
      when first_seen_at_a < first_seen_at_b then first_seen_at_a
      when anonymous_id_a > anonymous_id_b then first_seen_at_b
      else first_seen_at_a
    end as target_first_seen_at,
    case
      when first_seen_at_a > first_seen_at_b then first_seen_date_a
      when first_seen_at_a < first_seen_at_b then first_seen_date_b
      when anonymous_id_a > anonymous_id_b then first_seen_date_a
      else first_seen_date_b
    end as source_first_seen_date,
    case
      when first_seen_at_a > first_seen_at_b then first_seen_date_b
      when first_seen_at_a < first_seen_at_b then first_seen_date_a
      when anonymous_id_a > anonymous_id_b then first_seen_date_b
      else first_seen_date_a
    end as target_first_seen_date
  from allowed_candidates_with_first_seen
)

select
  source_anonymous_id,
  target_anonymous_id,
  key_name,
  key_value,
  tier,
  source_first_seen_at,
  target_first_seen_at,
  source_first_seen_date,
  target_first_seen_date
from oriented_links
