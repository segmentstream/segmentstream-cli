from pathlib import Path

from dagster import Definitions
from dagster_dbt import DbtCliResource


SEGMENTSTREAM_DIR = Path(__file__).resolve().parents[1]

defs = Definitions(
    resources={
        "dbt": DbtCliResource(
            project_dir=str(SEGMENTSTREAM_DIR),
            profiles_dir=str(SEGMENTSTREAM_DIR),
        )
    }
)
