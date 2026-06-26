{{ config(materialized='ephemeral', alias='__SOURCE_NAME___conversions') }}

{% set segmentstream_start_date = var('segmentstream_start_date', none) %}
{% set segmentstream_end_date = var('segmentstream_end_date', none) %}

{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}
  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}
{% endif %}

{% if execute %}
  {{ exceptions.raise_compiler_error("Implement sources/__SOURCE_NAME__/models/conversions.sql by mapping raw inputs from models/schema.yml to the conversions contract.") }}
{% endif %}

-- Template query. Replace this example with source-specific SQL.
-- 1. Declare raw warehouse inputs in models/schema.yml.
-- 2. Inspect the target contract with: segmentstream source contracts --type conversions --json
-- 3. Return the contract columns below, filtered to the SegmentStream date window.
select
  cast(null as date) as date,
  cast(null as timestamp) as conversion_time,
  cast(null as string) as conversion_name,
  cast(null as string) as conversion_id,
  cast(null as float64) as conversion_value
where false
