{% set max_component_size = var('segmentstream_identity_graph_max_component_size', 64) %}
{% set max_deterministic_component_size = var('segmentstream_identity_graph_max_deterministic_component_size', 200) %}

with identity_graph_nodes as (
  select * from {{ ref('int_identity_graph_nodes') }}
),

deterministic_components as (
  select * from {{ ref('int_identity_graph_deterministic_components') }}
),

all_components as (
  select * from {{ ref('int_identity_graph_all_components') }}
),

valid_deterministic_components as (
  -- Deterministic clusters are allowed to be larger because they are anchored
  -- by high-confidence keys such as user IDs or email hashes.
  select *
  from deterministic_components
  where component_size <= {{ max_deterministic_component_size }}
),

valid_all_components as (
  -- Probabilistic bridges are useful, but the legacy guardrail keeps noisy
  -- identifiers from swallowing unrelated deterministic clusters.
  select *
  from all_components
  where component_size <= {{ max_component_size }}
)

select
  identity_graph_nodes.anonymous_id,
  coalesce(
    valid_all_components.identity_id,
    valid_deterministic_components.identity_id,
    identity_graph_nodes.anonymous_id
  ) as identity_id,
  identity_graph_nodes.first_seen_date
from identity_graph_nodes
left join valid_all_components
  on identity_graph_nodes.anonymous_id = valid_all_components.anonymous_id
left join valid_deterministic_components
  on identity_graph_nodes.anonymous_id = valid_deterministic_components.anonymous_id
