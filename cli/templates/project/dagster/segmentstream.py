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
ANALYTICS_CORE_GIT_URL = "https://github.com/segmentstream/segmentstream.git"
ANALYTICS_CORE_GIT_SUBDIRECTORY = "analytics-core"
ANALYTICS_CORE_CONTAINER_PATH = "/opt/segmentstream/analytics-core"
ANALYTICS_CORE_LOCAL_PATH_ENV = "SEGMENTSTREAM_ANALYTICS_CORE_LOCAL_PATH"
ANALYTICS_CORE_REVISION_ENV = "SEGMENTSTREAM_ANALYTICS_CORE_REVISION"


@dataclass(frozen=True)
class SegmentStreamSource:
    name: str
    path: Path
    package_name: str
    events_model_name: str


def prepare_segmentstream_dbt_project(log=None) -> dict:
    config = load_segmentstream_config()
    sources = parse_sources(config)
    write_packages_yml(sources)
    run_dbt_command(["deps", "--project-dir", str(SEGMENTSTREAM_DIR)], log)
    run_dbt_command(
        [
            "parse",
            "--project-dir",
            str(SEGMENTSTREAM_DIR),
            "--profiles-dir",
            str(SEGMENTSTREAM_DIR),
            "--vars",
            dbt_vars(sources),
        ],
        log,
    )
    if log is not None:
        log.info("Prepared SegmentStream dbt project from segmentstream.yml")
    return config


def build_ingestion_assets(config: dict) -> list:
    return []


def dbt_partition_vars(context, config: dict) -> str:
    window = context.partition_time_window
    sources = parse_sources(config)
    return dbt_vars(
        sources,
        segmentstream_start_date=window.start.date().isoformat(),
        segmentstream_end_date=window.end.date().isoformat(),
    )


def dbt_vars(
    sources: list[SegmentStreamSource],
    segmentstream_start_date: str | None = None,
    segmentstream_end_date: str | None = None,
) -> str:
    data = {"segmentstream_sources": source_vars(sources)}
    if segmentstream_start_date is not None:
        data["segmentstream_start_date"] = segmentstream_start_date
    if segmentstream_end_date is not None:
        data["segmentstream_end_date"] = segmentstream_end_date
    return json.dumps(data)


def source_vars(sources: list[SegmentStreamSource]) -> list[dict[str, str]]:
    return [
        {
            "name": source.name,
            "package_name": source.package_name,
            "events_model_name": source.events_model_name,
        }
        for source in sources
    ]


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
        events_model_name = discover_events_model_name(name, path)
        sources.append(
            SegmentStreamSource(
                name=name,
                path=path,
                package_name=package_name,
                events_model_name=events_model_name,
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
            f'source "{name}" path {value} does not exist; run segmentstream source scaffold {name} --type events or update segmentstream.yml'
        )
    return path


def discover_events_model_name(name: str, path: Path) -> str:
    if (path / "models" / "events.sql").is_file():
        return "events"

    legacy_model = f"events_{name}"
    if (path / "models" / "exports" / f"{legacy_model}.sql").is_file():
        return legacy_model

    return "events"


def write_packages_yml(sources: list[SegmentStreamSource]) -> None:
    packages = [analytics_core_package()]
    for source in sources:
        packages.append({"local": relative_to_runtime(source.path)})
    data = {"packages": packages}
    (SEGMENTSTREAM_DIR / "packages.yml").write_text(
        yaml.safe_dump(data, sort_keys=False),
        encoding="utf-8",
    )


def analytics_core_package() -> dict[str, str]:
    local_path = normalize_optional_string(os.environ.get(ANALYTICS_CORE_LOCAL_PATH_ENV))
    if local_path != "":
        return {"local": ANALYTICS_CORE_CONTAINER_PATH}

    revision = normalize_optional_string(os.environ.get(ANALYTICS_CORE_REVISION_ENV))
    if revision == "":
        raise RuntimeError(
            f"{ANALYTICS_CORE_REVISION_ENV} is required unless {ANALYTICS_CORE_LOCAL_PATH_ENV} is set"
        )
    return {
        "git": ANALYTICS_CORE_GIT_URL,
        "revision": revision,
        "subdirectory": ANALYTICS_CORE_GIT_SUBDIRECTORY,
    }


def relative_to_runtime(path: Path) -> str:
    return Path(os.path.relpath(path.resolve(), SEGMENTSTREAM_DIR.resolve())).as_posix()


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
