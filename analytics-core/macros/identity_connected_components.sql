{#
  Connected-components helper inspired by Unytics BigFunctions
  `connected_components`, authored by Furcy Pin and released under the MIT
  license:
  https://github.com/unytics/bigfunctions/blob/main/bigfunctions/transform/graph/connected_components.yaml

  BigFunctions uses BigQuery scripting and temp-table iteration. This dbt helper
  instead emits a bounded, warehouse-portable label-propagation query so identity
  graph resolution can run without BigFunctions, recursive CTEs, or procedures.
#}

{% macro segmentstream_identity_connected_components(input_cte, output_cte, max_iterations=32) %}
{{ output_cte }}__nodes as (
  select distinct node_id
  from (
    select l_node as node_id from {{ input_cte }}
    union all
    select r_node as node_id from {{ input_cte }}
  ) as component_nodes
  where node_id is not null
),

{{ output_cte }}__undirected_edges as (
  select distinct
    l_node,
    r_node
  from (
    select l_node, r_node from {{ input_cte }}
    union all
    select r_node as l_node, l_node as r_node from {{ input_cte }}
    union all
    select node_id as l_node, node_id as r_node from {{ output_cte }}__nodes
  ) as undirected_edge_union
  where l_node is not null
    and r_node is not null
),

{{ output_cte }}__labels_00 as (
  select
    node_id,
    node_id as component_id
  from {{ output_cte }}__nodes
)
{% set graph = namespace(previous_labels = output_cte ~ '__labels_00') %}
{% for iteration in range(max_iterations | int) %}
{% set suffix = "%02d" | format(iteration + 1) %}
{% set next_labels = output_cte ~ '__labels_' ~ suffix %}
,

{{ next_labels }} as (
  select
    {{ output_cte }}__undirected_edges.l_node as node_id,
    min({{ graph.previous_labels }}.component_id) as component_id
  from {{ output_cte }}__undirected_edges
  inner join {{ graph.previous_labels }}
    on {{ output_cte }}__undirected_edges.r_node = {{ graph.previous_labels }}.node_id
  group by 1
)
{% set graph.previous_labels = next_labels %}
{% endfor %}
,

{{ output_cte }} as (
  select
    node_id,
    component_id as connected_component_id
  from {{ graph.previous_labels }}
)
{% endmacro %}
