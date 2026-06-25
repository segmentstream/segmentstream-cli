with identity_graph_edges as (
  select * from {{ ref('int_identity_graph_edges') }}
),

deterministic_components as (
  select * from {{ ref('int_identity_graph_deterministic_components') }}
),

all_components as (
  select * from {{ ref('int_identity_graph_all_components') }}
),

all_graph_unresolved_edges as (
  select
    'all' as graph_phase,
    identity_graph_edges.source_anonymous_id,
    identity_graph_edges.target_anonymous_id,
    source_components.connected_component_id as source_connected_component_id,
    target_components.connected_component_id as target_connected_component_id
  from identity_graph_edges
  inner join all_components as source_components
    on identity_graph_edges.source_anonymous_id = source_components.anonymous_id
  inner join all_components as target_components
    on identity_graph_edges.target_anonymous_id = target_components.anonymous_id
  where source_components.connected_component_id != target_components.connected_component_id
),

deterministic_graph_unresolved_edges as (
  select
    'deterministic' as graph_phase,
    identity_graph_edges.source_anonymous_id,
    identity_graph_edges.target_anonymous_id,
    source_components.connected_component_id as source_connected_component_id,
    target_components.connected_component_id as target_connected_component_id
  from identity_graph_edges
  inner join deterministic_components as source_components
    on identity_graph_edges.source_anonymous_id = source_components.anonymous_id
  inner join deterministic_components as target_components
    on identity_graph_edges.target_anonymous_id = target_components.anonymous_id
  where identity_graph_edges.tier = 'deterministic'
    and source_components.connected_component_id != target_components.connected_component_id
)

-- If this returns rows, increase segmentstream_identity_graph_max_iterations or
-- investigate unexpected graph shape before trusting identity resolution.
select * from all_graph_unresolved_edges
union all
select * from deterministic_graph_unresolved_edges
