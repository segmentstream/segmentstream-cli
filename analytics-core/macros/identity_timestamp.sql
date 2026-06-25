{% macro segmentstream_timestamp_to_date(timestamp_expression) -%}
  {{ return(adapter.dispatch('segmentstream_timestamp_to_date', 'segmentstream_analytics_core')(timestamp_expression)) }}
{%- endmacro %}

{% macro default__segmentstream_timestamp_to_date(timestamp_expression) -%}
  cast({{ timestamp_expression }} as date)
{%- endmacro %}

{% macro bigquery__segmentstream_timestamp_to_date(timestamp_expression) -%}
  date({{ timestamp_expression }})
{%- endmacro %}

{% macro snowflake__segmentstream_timestamp_to_date(timestamp_expression) -%}
  to_date({{ timestamp_expression }})
{%- endmacro %}

{% macro databricks__segmentstream_timestamp_to_date(timestamp_expression) -%}
  to_date({{ timestamp_expression }})
{%- endmacro %}

{% macro segmentstream_timestamp_diff_seconds(later_timestamp_expression, earlier_timestamp_expression) -%}
  {{ return(adapter.dispatch('segmentstream_timestamp_diff_seconds', 'segmentstream_analytics_core')(later_timestamp_expression, earlier_timestamp_expression)) }}
{%- endmacro %}

{% macro default__segmentstream_timestamp_diff_seconds(later_timestamp_expression, earlier_timestamp_expression) -%}
  datediff(second, {{ earlier_timestamp_expression }}, {{ later_timestamp_expression }})
{%- endmacro %}

{% macro bigquery__segmentstream_timestamp_diff_seconds(later_timestamp_expression, earlier_timestamp_expression) -%}
  timestamp_diff({{ later_timestamp_expression }}, {{ earlier_timestamp_expression }}, second)
{%- endmacro %}

{% macro snowflake__segmentstream_timestamp_diff_seconds(later_timestamp_expression, earlier_timestamp_expression) -%}
  datediff(second, {{ earlier_timestamp_expression }}, {{ later_timestamp_expression }})
{%- endmacro %}

{% macro databricks__segmentstream_timestamp_diff_seconds(later_timestamp_expression, earlier_timestamp_expression) -%}
  timestampdiff(second, {{ earlier_timestamp_expression }}, {{ later_timestamp_expression }})
{%- endmacro %}
