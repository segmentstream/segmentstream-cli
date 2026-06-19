# SegmentStream CLI

SegmentStream CLI runs marketing analytics pipelines locally and writes the
results to your own data warehouse.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/segmentstream/segmentstream-cli/main/install.sh | sh
```

The installer places the binary in `$HOME/.segmentstream/bin` by default and
prints PATH guidance if needed. If `$HOME/.local/bin` already exists, is
writable, and is on PATH, the installer creates a safe symlink there.

For local development from this repository:

```sh
make install
```

## Getting Started

Before starting, make sure these are installed and available from your terminal:

- Docker Desktop or Docker Engine with Docker Compose V2.
- Git.

Create a new SegmentStream project in an empty directory:

```sh
mkdir my-segmentstream-project
cd my-segmentstream-project
segmentstream init --warehouse bigquery
```

`segmentstream init --warehouse bigquery` creates the project files you edit:

```text
segmentstream.yml   warehouse configuration
README.md           project guide
AGENTS.md           instructions for coding agents
```

Use `segmentstream init --json` when an agent or script needs a stable state
envelope with the next action. JSON mode is read-only unless you pass a mutation
flag such as `--warehouse`.

The `init --json` envelope uses `schema_version: "2"`. A successful state
inspection exits `0` even when `ready` is `false`; use `ready`, `stages`,
`diagnostics`, and `next_action` to decide what to do next. `next_action.type`
is either `run_command` with an executable command or `human_input` with
structured `accepts` inputs and a `verify` command. The envelope also reports
supported auth methods under `capabilities.auth_methods`.

## Create A Source

Sources are project-owned dbt packages that contain source-specific
transformations. Agents should start by discovering supported source contracts:

```sh
segmentstream source contracts --json
segmentstream source contracts --type events --json
```

Create a local source package from a contract:

```sh
segmentstream source create ga4 --type events
```

`segmentstream source init ga4` is kept as a compatibility alias for creating a
source from the default contract.

This creates `sources/ga4/` as a minimal dbt package with a pinned
`contract.yml` snapshot and one author-editable model:
`sources/ga4/models/events.sql`.

Declare the source in `segmentstream.yml`:

```yaml
sources:
  - name: ga4
    path: ./sources/ga4
```

On run, SegmentStream reads `segmentstream.yml`, installs declared sources as
dbt packages, and generates a core `events` model that unions each source
package's `events` model.

## Configure Your Warehouse

Authenticate with a BigQuery service-account key:

```sh
segmentstream warehouse auth --service-account-key /path/to/service-account.json
```

The CLI copies the key to
`$HOME/.segmentstream/bigquery/default-bigquery.json` and writes only the
credential name to `segmentstream.yml`.

Browse available projects, datasets, tables, and schemas:

```sh
segmentstream warehouse browse --json
segmentstream warehouse browse --path my-gcp-project --json
segmentstream warehouse browse --path my-gcp-project/my_dataset --json
segmentstream warehouse browse --path my-gcp-project/my_dataset/my_table --json
```

Configure the BigQuery project, dataset, and location:

```sh
segmentstream warehouse configure --project my-gcp-project --dataset segmentstream --location US
segmentstream warehouse test
```

The resulting `segmentstream.yml` should look like:

```yaml
version: 1

warehouse:
  type: bigquery
  auth: default-bigquery
  project: my-gcp-project
  dataset: segmentstream
  location: US
```

Fields:

- `warehouse.type`: currently only `bigquery` is supported.
- `warehouse.auth`: named credential reference. It is not a secret value.
- `warehouse.project`: your Google Cloud project ID.
- `warehouse.dataset`: the BigQuery dataset where SegmentStream writes tables.
- `warehouse.location`: BigQuery dataset location.

Credentials are handled separately from `segmentstream.yml`. Do not put tokens,
private keys, or passwords in this file.

## Run The Pipeline

```sh
segmentstream run
```

`segmentstream run` runs the SegmentStream pipeline and produces tables in the
configured warehouse. Each run refreshes the local project environment and
processes the last 30 UTC daily partitions by default.

To run from a specific date through today UTC:

```sh
segmentstream run --start-date 2026-05-01
```

Pipeline state persists across `segmentstream run` even though `.segmentstream/`
is regenerated.

The first run can take a few minutes while SegmentStream sets up the local
environment. Later runs should be faster.

## Commands

`segmentstream init` reports current setup state and the next action.
`segmentstream init --json` emits a stable state-machine envelope for agents.
`segmentstream init --warehouse bigquery` selects BigQuery in `segmentstream.yml`.

`segmentstream run` runs the configured analytics pipeline and writes results to
the configured warehouse. It runs the last 30 UTC daily partitions by default;
use `segmentstream run --start-date YYYY-MM-DD` to start earlier or later.

`segmentstream source contracts [--type events] [--json]` lists supported source
contracts and returns their schemas.

`segmentstream source create <name> --type events [--json]` creates a local
source package under `sources/<name>/`.

`segmentstream source init <name>` is a compatibility alias that uses the
default source contract.

`segmentstream warehouse auth --service-account-key <path>` stores a BigQuery
service-account credential outside the project.

`segmentstream warehouse browse [--path <project>[/<dataset>[/<table>]]] [--json]`
lists BigQuery projects, datasets, tables, or a table schema.

`segmentstream warehouse configure --project --dataset --location` validates and
writes warehouse settings.

`segmentstream warehouse test` checks BigQuery connect, read, create table, and
query permissions.

`segmentstream update` updates an installed CLI release.

`segmentstream update --check` checks whether an update is available without
installing it.

`segmentstream version` prints the installed CLI version.

## Release

Publish a GitHub Release with a semver tag to build and attach release assets:

1. Open https://github.com/segmentstream/segmentstream-cli/releases/new
2. Create or choose a tag like `v0.1.0`.
3. Publish the release.

The release workflow runs when the release is published and uses GoReleaser to
attach platform archives and `checksums.txt`. The installer waits for those
assets, so it is safe to run shortly after publishing a release.
