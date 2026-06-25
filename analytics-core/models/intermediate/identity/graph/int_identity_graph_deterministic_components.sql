{{ config(materialized='table') }}

{% set max_iterations = var('segmentstream_identity_graph_max_iterations', 32) %}

with identity_graph_nodes as (
  select * from {{ ref('int_identity_graph_nodes') }}
),

identity_graph_edges as (
  select * from {{ ref('int_identity_graph_edges') }}
),

graph_edges as (
  select distinct
    l_node,
    r_node
  from (
    select
      anonymous_id as l_node,
      anonymous_id as r_node
    from identity_graph_nodes

    union all

    select
      source_anonymous_id as l_node,
      target_anonymous_id as r_node
    from identity_graph_edges
    where tier = 'deterministic'
  ) as graph_edge_union
),

{{ segmentstream_identity_connected_components('graph_edges', 'connected_components', max_iterations) }},

component_sizes as (
  select
    connected_component_id,
    count(distinct node_id) as component_size
  from connected_components
  group by 1
),

component_identity_candidates as (
  select
    connected_components.connected_component_id,
    connected_components.node_id,
    row_number() over (
      partition by connected_components.connected_component_id
      order by identity_graph_nodes.first_seen_at, connected_components.node_id
    ) as identity_rank
  from connected_components
  inner join identity_graph_nodes
    on connected_components.node_id = identity_graph_nodes.anonymous_id
),

component_identity_ids as (
  select
    connected_component_id,
    node_id as identity_id
  from component_identity_candidates
  where identity_rank = 1
)

select
  connected_components.node_id as anonymous_id,
  connected_components.connected_component_id,
  component_sizes.component_size,
  component_identity_ids.identity_id
from connected_components
inner join component_sizes
  on connected_components.connected_component_id = component_sizes.connected_component_id
inner join component_identity_ids
  on connected_components.connected_component_id = component_identity_ids.connected_component_id
