from pathlib import Path
import sys

from dagster import AssetExecutionContext, AssetSelection, DailyPartitionsDefinition, Definitions, define_asset_job
from dagster_dbt import DbtCliResource, dbt_assets


SEGMENTSTREAM_DIR = Path(__file__).resolve().parents[1]
DAGSTER_DIR = SEGMENTSTREAM_DIR / "dagster"
sys.path.insert(0, str(DAGSTER_DIR))

from segmentstream import build_ingestion_assets, dbt_partition_vars, prepare_segmentstream_dbt_project


segmentstream_config = prepare_segmentstream_dbt_project()
ingestion_assets = build_ingestion_assets(segmentstream_config)
manifest_path = SEGMENTSTREAM_DIR / "dbt" / "target" / "manifest.json"
segmentstream_daily_partitions = DailyPartitionsDefinition(start_date="1970-01-01", end_offset=1)


@dbt_assets(manifest=manifest_path, partitions_def=segmentstream_daily_partitions)
def segmentstream_dbt_assets(context: AssetExecutionContext, dbt: DbtCliResource):
    yield from dbt.cli(["build", "--vars", dbt_partition_vars(context)], context=context).stream()


segmentstream_materialize_all = define_asset_job(
    name="segmentstream_materialize_all",
    selection=AssetSelection.all(),
)


defs = Definitions(
    assets=[*ingestion_assets, segmentstream_dbt_assets],
    resources={
        "dbt": DbtCliResource(
            project_dir=str(SEGMENTSTREAM_DIR),
            profiles_dir=str(SEGMENTSTREAM_DIR),
        ),
    },
    jobs=[segmentstream_materialize_all],
)
