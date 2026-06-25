from pathlib import Path
import json
import sys
from typing import Any, Mapping

from dagster import AssetExecutionContext, AssetKey, AssetSelection, DailyPartitionsDefinition, Definitions, SourceAsset, define_asset_job
from dagster_dbt import DagsterDbtTranslator, DbtCliResource, dbt_assets


SEGMENTSTREAM_DIR = Path(__file__).resolve().parents[1]
DAGSTER_DIR = SEGMENTSTREAM_DIR / "dagster"
ANALYTICS_CORE_PACKAGE_NAME = "segmentstream_analytics_core"
sys.path.insert(0, str(DAGSTER_DIR))

from segmentstream import build_ingestion_assets, dbt_partition_vars, prepare_segmentstream_dbt_project


segmentstream_config = prepare_segmentstream_dbt_project()
ingestion_assets = build_ingestion_assets(segmentstream_config)
manifest_path = SEGMENTSTREAM_DIR / "dbt" / "target" / "manifest.json"
segmentstream_daily_partitions = DailyPartitionsDefinition(start_date="1970-01-01", end_offset=1)


def build_dbt_source_assets(path: Path) -> list[SourceAsset]:
    manifest = json.loads(path.read_text(encoding="utf-8"))
    return [
        SourceAsset(key=source_asset_key(source))
        for source in sorted(
            manifest.get("sources", {}).values(),
            key=lambda source: (str(source["source_name"]), str(source["name"])),
        )
    ]


def source_asset_key(dbt_source_props: Mapping[str, Any]) -> AssetKey:
    source_name = str(dbt_source_props["source_name"])
    source_slug = source_name.removesuffix("_raw")
    table_name = str(dbt_source_props["name"])
    return AssetKey([f"{table_name}_{source_slug}"])


class SegmentStreamDbtTranslator(DagsterDbtTranslator):
    def get_asset_key(self, dbt_resource_props: Mapping[str, Any]) -> AssetKey:
        if dbt_resource_props.get("resource_type") == "source":
            return source_asset_key(dbt_resource_props)
        package_name = dbt_resource_props.get("package_name")
        if (
            dbt_resource_props.get("resource_type") == "model"
            and package_name not in {"segmentstream", ANALYTICS_CORE_PACKAGE_NAME}
        ):
            return AssetKey([str(package_name), str(dbt_resource_props["name"])])
        return super().get_asset_key(dbt_resource_props)


@dbt_assets(
    manifest=manifest_path,
    select=f"package:{ANALYTICS_CORE_PACKAGE_NAME}",
    partitions_def=segmentstream_daily_partitions,
    dagster_dbt_translator=SegmentStreamDbtTranslator(),
)
def segmentstream_dbt_assets(context: AssetExecutionContext, dbt: DbtCliResource):
    yield from dbt.cli(["build", "--vars", dbt_partition_vars(context, segmentstream_config)], context=context).stream()


segmentstream_materialize_all = define_asset_job(
    name="segmentstream_materialize_all",
    selection=AssetSelection.all(),
)


defs = Definitions(
    assets=[*ingestion_assets, *build_dbt_source_assets(manifest_path), segmentstream_dbt_assets],
    resources={
        "dbt": DbtCliResource(
            project_dir=str(SEGMENTSTREAM_DIR),
            profiles_dir=str(SEGMENTSTREAM_DIR),
        ),
    },
    jobs=[segmentstream_materialize_all],
)
