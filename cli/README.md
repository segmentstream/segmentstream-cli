# SegmentStream CLI

SegmentStream CLI runs marketing analytics pipelines locally and writes the
results to your own data warehouse.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/segmentstream/segmentstream/main/cli/install.sh | sh
```

The installer places the binary in `$HOME/.segmentstream/bin` by default and
prints PATH guidance if needed. If `$HOME/.local/bin` already exists, is
writable, and is on PATH, the installer creates a safe symlink there.

For local development from this repository:

```sh
cd cli
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

Pass `--json` to any command when an agent or script needs structured output.
JSON mode emits one response object on stdout with `schema_version`, `command`,
`status`, and command-specific `data`; diagnostics and actions are structured
when present. Progress and interactive guidance use stderr so stdout remains
parseable.

Use `segmentstream init --json` when an agent needs setup state and the next
action. The init state machine is returned under `data.envelope`. A successful
state inspection exits `0` even when `data.envelope.ready` is `false`; use
`ready`, `stages`, `diagnostics`, and `next_action` to decide what to do next.
`next_action.type` is either `run_command` with an executable command or
`human_input` with structured `accepts` inputs and a `verify` command. The
envelope also reports supported auth methods under `capabilities.auth_methods`.
JSON mode is read-only unless you pass a mutation flag such as `--warehouse`.

## Scaffold A Source

Sources are project-owned dbt packages that contain source-specific
transformations. Agents should start by discovering supported source contracts:

```sh
segmentstream source contracts --json
segmentstream source contracts --type events --json
```

Scaffold a local source template from a contract:

```sh
segmentstream source scaffold ga4 --type events
```

This scaffolds `sources/ga4/` as a source template with a pinned `contract.yml`,
a `README.md`, dbt verification tests, and one author-editable model:
`sources/ga4/models/events.sql`. The scaffold is not implemented yet; read the
README to understand the source package and output contract.

Declare the source in `segmentstream.yml`:

```yaml
sources:
  - name: ga4
    path: ./sources/ga4
```

Verify the implemented source before running the pipeline:

```sh
segmentstream source verify ga4
```

On run, SegmentStream reads `segmentstream.yml`, installs declared sources as
dbt packages, and generates a core `events` model that unions each source
package's `events` model.

## Configure Your Warehouse

Authenticate with Google OAuth:

```sh
segmentstream warehouse auth login
```

The CLI prints a Google OAuth URL and waits for the browser redirect on a local
loopback callback. Open the URL in a browser on the same computer where the CLI
is running. The CLI stores an authorized-user credential outside the project and
writes only the credential name to `segmentstream.yml`.

For sandbox or forwarded-loopback testing, choose the callback port explicitly:

```sh
segmentstream warehouse auth login --port 40473
```

For headless servers, CI, or environments where a browser cannot reach the
CLI's local callback, authenticate with a BigQuery service-account key:

```sh
segmentstream warehouse auth --service-account-key=/path/to/service-account.json
```

Credentials are stored under `$HOME/.segmentstream/bigquery/`.

Browse available projects, datasets, tables, and schemas:

```sh
segmentstream warehouse browse --json
segmentstream warehouse browse --path my-gcp-project --json
segmentstream warehouse browse --path my-gcp-project/my_dataset --json
segmentstream warehouse browse --path my-gcp-project/my_dataset/my_table --json
```

Inspect raw rows with read-only SELECT queries:

```sh
segmentstream warehouse query \
  --sql "SELECT payload FROM \`my-gcp-project.my_dataset.my_table\` WHERE payload IS NOT NULL LIMIT 5" \
  --json

segmentstream warehouse query \
  --sql "SELECT JSON_VALUE(payload, '$.event') AS event_name, COUNT(*) AS events FROM \`my-gcp-project.my_dataset.my_table\` GROUP BY 1 ORDER BY events DESC LIMIT 50" \
  --json

segmentstream warehouse query \
  --sql "SELECT COUNTIF(payload IS NULL) AS null_payloads, COUNT(*) AS rows FROM \`my-gcp-project.my_dataset.my_table\`" \
  --json

segmentstream warehouse query \
  --sql "SELECT MIN(event_date) AS min_event_date, MAX(event_date) AS max_event_date FROM \`my-gcp-project.my_dataset.my_table\`" \
  --json
```

`warehouse query --json` returns only row objects under `data`. For BigQuery,
the CLI validates the SQL with a dry run and executes it only when BigQuery
reports the statement type as `SELECT`.

Configure the BigQuery project, dataset, and location:

```sh
segmentstream warehouse configure --project my-gcp-project --dataset segmentstream --location US
segmentstream warehouse test
```

If the dataset does not exist yet, create it explicitly:

```sh
segmentstream warehouse configure --project my-gcp-project --dataset segmentstream --location US --create-dataset
```

If source tables are in a different BigQuery location than the newly created
SegmentStream dataset, recreate the configured dataset in the source location.
This is intended for initial scaffolding when the SegmentStream dataset is still
empty:

```sh
segmentstream warehouse destroy --json
segmentstream warehouse configure --project my-gcp-project --dataset segmentstream --location EU --create-dataset --json
segmentstream warehouse test --json
```

`warehouse destroy` only targets the project and dataset configured in the
current `segmentstream.yml`. It refuses to delete datasets that contain tables
or views unless you pass `--force`.

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
`segmentstream init --json` emits the common JSON response with the setup
state-machine envelope under `data.envelope`.
`segmentstream init --warehouse bigquery` selects BigQuery in `segmentstream.yml`.

`segmentstream run` runs the configured analytics pipeline and writes results to
the configured warehouse. It runs the last 30 UTC daily partitions by default;
use `segmentstream run --start-date YYYY-MM-DD` to start earlier or later.

`segmentstream source contracts [--type events] [--json]` lists supported source
contracts and returns their schemas.

`segmentstream source scaffold <name> --type events [--json]` scaffolds a local
source template under `sources/<name>/`. The template must be implemented next.

`segmentstream source verify <name> [--start-date YYYY-MM-DD] [--json]` runs the
source package's dbt verification tests inside Docker. It defaults to the last 7
UTC days.

`segmentstream warehouse auth login [--port <port>]` prints a Google OAuth URL,
waits for a loopback browser redirect on the same computer, and stores a
BigQuery OAuth credential outside the project. Use `--port` when a sandbox or
container needs the callback port to be forwarded before the command starts.

`segmentstream warehouse auth --service-account-key=<path>` stores a BigQuery
service-account credential outside the project for headless servers, CI, or
other non-interactive environments.

`segmentstream warehouse browse [--path <project>[/<dataset>[/<table>]]] [--json]`
lists BigQuery projects, datasets, tables, or a table schema.

`segmentstream warehouse query --sql "<select statement>" [--max-rows 100] [--timeout 30s] [--maximum-bytes-billed <bytes>] [--json]`
runs a dry-run-verified read-only SELECT query and returns rows.

`segmentstream warehouse configure --project --dataset --location [--create-dataset]`
validates and writes warehouse settings. Use `--create-dataset` to create a
missing BigQuery dataset explicitly.

`segmentstream warehouse destroy [--force] [--json]` deletes the configured
warehouse dataset and clears `warehouse.project`, `warehouse.dataset`, and
`warehouse.location` from `segmentstream.yml`. Without `--force`, BigQuery
datasets must have no tables or views.

`segmentstream warehouse test [--json]` checks BigQuery connect, read, create
table, and query permissions.

`segmentstream update [--json]` updates an installed CLI release.

`segmentstream update --check [--json]` checks whether an update is available
without installing it.

`segmentstream version [--json]` prints the installed CLI version.

## Release

Publish a GitHub Release with a semver tag to build and attach release assets:

1. Open https://github.com/segmentstream/segmentstream/releases/new
2. Create or choose a tag like `v0.1.0`.
3. Publish the release.

The release workflow runs when the release is published and uses GoReleaser to
attach platform archives and `checksums.txt`. The installer waits for those
assets, so it is safe to run shortly after publishing a release.

Release builds bundle the SegmentStream desktop OAuth client using these
GitHub Actions secrets:

- `SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_ID`
- `SEGMENTSTREAM_GOOGLE_OAUTH_CLIENT_SECRET`

For local builds, put the same variable names in `.env` or export them before
running `make install`.
