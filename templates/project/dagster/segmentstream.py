from __future__ import annotations

import re
import subprocess
import os
import json
from dataclasses import dataclass
from pathlib import Path

import yaml


SEGMENTSTREAM_DIR = Path(__file__).resolve().parents[1]
PROJECT_ROOT = SEGMENTSTREAM_DIR.parent
CONFIG_PATH = PROJECT_ROOT / "segmentstream.yml"
IDENTIFIER_RE = re.compile(r"^[a-z][a-z0-9_]*$")


@dataclass(frozen=True)
class SegmentStreamSource:
    name: str
    path: Path
    package_name: str


def prepare_segmentstream_dbt_project(log=None) -> dict:
    config = load_segmentstream_config()
    sources = parse_sources(config)
    write_packages_yml(sources)
    write_core_events_model(sources)
    run_dbt_command(["deps", "--project-dir", str(SEGMENTSTREAM_DIR)], log)
    run_dbt_command(
        [
            "parse",
            "--project-dir",
            str(SEGMENTSTREAM_DIR),
            "--profiles-dir",
            str(SEGMENTSTREAM_DIR),
        ],
        log,
    )
    if log is not None:
        log.info("Prepared SegmentStream dbt project from segmentstream.yml")
    return config


def build_ingestion_assets(config: dict) -> list:
    return []


def dbt_partition_vars(context) -> str:
    window = context.partition_time_window
    return json.dumps(
        {
            "segmentstream_start_date": window.start.date().isoformat(),
            "segmentstream_end_date": window.end.date().isoformat(),
        }
    )


def load_segmentstream_config() -> dict:
    if not CONFIG_PATH.exists():
        raise RuntimeError(f"{CONFIG_PATH} was not found")
    with CONFIG_PATH.open("r", encoding="utf-8") as file:
        config = yaml.safe_load(file) or {}
    if not isinstance(config, dict):
        raise RuntimeError("segmentstream.yml must contain a YAML mapping")
    return config


def parse_sources(config: dict) -> list[SegmentStreamSource]:
    raw_sources = config.get("sources") or []
    if not isinstance(raw_sources, list):
        raise RuntimeError("segmentstream.yml field sources must be a list")

    seen: set[str] = set()
    sources: list[SegmentStreamSource] = []
    for raw_source in raw_sources:
        if not isinstance(raw_source, dict):
            raise RuntimeError("each source in segmentstream.yml must be a mapping")

        name = normalize_required_string(raw_source.get("name"), "source.name")
        validate_identifier(name, "source.name")
        if name in seen:
            raise RuntimeError(f'duplicate source "{name}"')
        seen.add(name)

        path_value = normalize_required_string(raw_source.get("path"), f"sources.{name}.path")
        package_name = normalize_optional_string(raw_source.get("package_name"))
        if package_name == "":
            package_name = f"segmentstream_source_{name}"
        validate_identifier(package_name, f"sources.{name}.package_name")

        path = resolve_source_path(name, path_value)
        sources.append(
            SegmentStreamSource(
                name=name,
                path=path,
                package_name=package_name,
            )
        )
    return sources


def normalize_required_string(value, field: str) -> str:
    value = normalize_optional_string(value)
    if value == "":
        raise RuntimeError(f"missing required field {field}")
    return value


def normalize_optional_string(value) -> str:
    if value is None:
        return ""
    if not isinstance(value, str):
        raise RuntimeError("source fields must be strings")
    value = value.strip()
    if "\n" in value or "\r" in value:
        raise RuntimeError("source fields must not contain newlines")
    return value


def validate_identifier(value: str, field: str) -> None:
    if not IDENTIFIER_RE.match(value):
        raise RuntimeError(
            f"invalid {field} {value!r}; use lowercase letters, numbers, and underscores, starting with a letter"
        )


def resolve_source_path(name: str, value: str) -> Path:
    path = Path(value)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    path = path.resolve()
    project_root = PROJECT_ROOT.resolve()

    try:
        relative = path.relative_to(project_root)
    except ValueError as exc:
        raise RuntimeError(f'source "{name}" path is outside the project root: {path}') from exc

    if relative.parts and relative.parts[0] == ".segmentstream":
        raise RuntimeError(f'source "{name}" path must not be inside .segmentstream')
    if not path.is_dir():
        raise RuntimeError(
            f'source "{name}" path {value} does not exist; run segmentstream source init {name} or update segmentstream.yml'
        )
    return path


def write_packages_yml(sources: list[SegmentStreamSource]) -> None:
    packages = []
    for source in sources:
        packages.append({"local": relative_to_runtime(source.path)})
    data = {"packages": packages}
    (SEGMENTSTREAM_DIR / "packages.yml").write_text(
        yaml.safe_dump(data, sort_keys=False),
        encoding="utf-8",
    )


def relative_to_runtime(path: Path) -> str:
    return Path(os.path.relpath(path.resolve(), SEGMENTSTREAM_DIR.resolve())).as_posix()


def write_core_events_model(sources: list[SegmentStreamSource]) -> None:
    path = SEGMENTSTREAM_DIR / "dbt" / "models" / "exports" / "events.sql"
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(render_core_events_model(sources), encoding="utf-8")


def render_core_events_model(sources: list[SegmentStreamSource]) -> str:
    header = [
        "-- Generated by SegmentStream. Do not edit directly.",
        "-- Update segmentstream.yml or source packages, then run segmentstream run.",
        "",
        "{% set segmentstream_start_date = var('segmentstream_start_date', none) %}",
        "{% set segmentstream_end_date = var('segmentstream_end_date', none) %}",
        "",
        "{% if execute and (segmentstream_start_date is none or segmentstream_end_date is none) %}",
        '  {{ exceptions.raise_compiler_error("SegmentStream vars segmentstream_start_date and segmentstream_end_date are required") }}',
        "{% endif %}",
        "",
    ]
    if not sources:
        return "\n".join(
            header
            + [
                "select",
                "  cast(null as string) as segmentstream_source,",
                "  cast(null as string) as event_id,",
                "  cast(null as string) as anonymous_id,",
                "  cast(null as string) as event_name,",
                "  cast(null as string) as page_url,",
                "  cast(null as string) as page_referrer,",
                "  cast(null as timestamp) as event_timestamp,",
                "  cast(null as date) as event_date",
                "from (select 1) as empty_project",
                "where false",
                "",
            ]
        )

    blocks = []
    for source in sources:
        blocks.append(
            "\n".join(
                [
                    "select",
                    f"  '{source.name}' as segmentstream_source,",
                    "  event_id,",
                    "  anonymous_id,",
                    "  event_name,",
                    "  page_url,",
                    "  page_referrer,",
                    "  event_timestamp,",
                    "  event_date",
                    f'from {{{{ ref("{source.package_name}", "events_{source.name}") }}}}',
                    "where event_date >= date('{{ segmentstream_start_date }}')",
                    "  and event_date < date('{{ segmentstream_end_date }}')",
                ]
            )
        )
    return "\n".join(header) + "\nunion all\n\n".join(blocks) + "\n"


def run_dbt_command(args: list[str], log=None) -> None:
    command = ["dbt"] + args
    result = subprocess.run(
        command,
        cwd=str(SEGMENTSTREAM_DIR),
        text=True,
        capture_output=True,
        check=False,
    )
    output = "\n".join(part for part in [result.stdout.strip(), result.stderr.strip()] if part)
    if log is not None and output:
        log.info(output)
    if result.returncode != 0:
        raise RuntimeError(f"dbt command failed: {' '.join(command)}\n{output}")
