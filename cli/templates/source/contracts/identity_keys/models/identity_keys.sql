{{ config(materialized='ephemeral', alias='__SOURCE_NAME___identity_keys') }}

{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

{% if execute %}
  {{ exceptions.raise_compiler_error("Implement sources/__SOURCE_NAME__/models/identity_keys.sql by mapping raw inputs from models/schema.yml to the identity_keys contract.") }}
{% endif %}

-- SEGMENTSTREAM_TODO(model_mapping): Replace this query with source-specific SQL
-- that maps raw inputs from models/schema.yml to the identity_keys contract.
-- Template query. Replace this example with source-specific SQL.
-- 1. Declare raw warehouse inputs in models/schema.yml.
-- 2. Inspect the target contract with: segmentstream source contracts --type identity_keys --json
-- 3. Return one row per observed identity key, filtered to the SegmentStream date window.
select
  cast(null as date) as date,
  cast(null as timestamp) as observed_at,
  cast(null as string) as anonymous_id,
  cast(null as string) as key_name,
  cast(null as string) as key_value
where false
