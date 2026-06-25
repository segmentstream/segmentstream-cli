FROM python:3.11-slim

ENV PIP_NO_CACHE_DIR=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends git \
    && rm -rf /var/lib/apt/lists/*

RUN pip install \
    "dbt-bigquery>=1.8,<1.9" \
    "dagster>=1.13,<2" \
    "dagster-dbt>=0.29,<1" \
    "dagster-postgres>=0.29,<1" \
    "dagster-webserver>=1.13,<2" \
    "PyYAML>=6,<7"

WORKDIR /workspace/.segmentstream
