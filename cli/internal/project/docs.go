package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	AgentGuideFileName    = "AGENTS.md"
	ProjectReadmeFileName = "README.md"
)

func DefaultProjectReadmeMarkdown() string {
	return `# SegmentStream Project

This is a SegmentStream project. Use it to run marketing analytics pipelines
locally and produce tables in your configured warehouse.

## Getting Started

Before starting, make sure these are installed and available from your terminal:

- Docker Desktop or Docker Engine with Docker Compose V2.
- Git.

Start by selecting and configuring the warehouse:

` + "```sh" + `
segmentstream init --warehouse bigquery
segmentstream warehouse auth login
segmentstream warehouse configure --project example-project --dataset segmentstream --location US
segmentstream warehouse test
` + "```" + `

If the dataset does not exist yet, create it explicitly:

` + "```sh" + `
segmentstream warehouse configure --project example-project --dataset segmentstream --location US --create-dataset
` + "```" + `

The resulting ` + "`segmentstream.yml`" + ` should look like:

` + "```yaml" + `
version: 1

warehouse:
  type: bigquery
  auth: default-bigquery
  project: example-project
  dataset: segmentstream
  location: US
` + "```" + `

Choose the BigQuery dataset where SegmentStream should produce tables. The
configure command only creates the dataset when ` + "`--create-dataset`" + ` is set.

` + "`warehouse.auth`" + ` is a named credential reference. It is not a secret value.
Do not put tokens, private keys, passwords, or SQL in ` + "`segmentstream.yml`" + `.
Credentials are stored outside the project at ` + "`~/.segmentstream/bigquery/<auth>.json`" + `.

## Run The Pipeline

` + "```sh" + `
segmentstream run
` + "```" + `

` + "`segmentstream run`" + ` runs the SegmentStream pipeline and produces tables in
the configured warehouse. Each run refreshes the local project environment and
processes the last 30 UTC daily partitions by default.

To run from a specific date through today UTC:

` + "```sh" + `
segmentstream run --start-date 2026-05-01
` + "```" + `

Pipeline state persists across ` + "`segmentstream run`" + ` even though
` + "`.segmentstream/`" + ` is regenerated.

The first run can take a few minutes while SegmentStream sets up the local
environment. Later runs should be faster.

## Scaffold A Source

Sources are project-owned dbt packages for source-specific transformations.

` + "```sh" + `
segmentstream source contracts --json
segmentstream source contracts --type events --json
segmentstream source contracts --type conversion_events --json
segmentstream source contracts --type identity_keys --json
segmentstream source scaffold ga4 --type events
segmentstream source scaffold crm_conversion_events --type conversion_events
segmentstream source scaffold sdk_identity --type identity_keys
segmentstream source verify ga4
segmentstream source verify crm_conversion_events
segmentstream source verify sdk_identity
` + "```" + `

This scaffolds project-owned source templates with pinned ` + "`contract.yml`" + `
snapshots, generated ` + "`README.md`" + ` guides, dbt verification tests, and
author-editable contract models:

- ` + "`sources/ga4/models/events.sql`" + `
- ` + "`sources/crm_conversion_events/models/conversion_events.sql`" + `
- ` + "`sources/sdk_identity/models/identity_keys.sql`" + `

The scaffolds are not implemented yet. Use ` + "`--json`" + ` to inspect unresolved
implementation items, read the generated ` + "`README.md`" + ` for source-specific
context, then edit the generated files at their ` + "`SEGMENTSTREAM_TODO(...)`" + `
markers to map raw warehouse data to the contract.

Declare the sources in ` + "`segmentstream.yml`" + `:

` + "```yaml" + `
sources:
  - name: ga4
    path: ./sources/ga4
  - name: crm_conversion_events
    path: ./sources/crm_conversion_events
  - name: sdk_identity
    path: ./sources/sdk_identity
` + "```" + `

Verify implemented sources before running the pipeline:

` + "```sh" + `
segmentstream source verify ga4
segmentstream source verify crm_conversion_events
segmentstream source verify sdk_identity
` + "```" + `

On run, SegmentStream reads ` + "`segmentstream.yml`" + `, installs analytics-core and
declared sources as dbt packages, and materializes the core ` + "`events`" + `,
` + "`conversion_events`" + `, and ` + "`identity_keys`" + ` models from analytics-core.

Source packages that use the ` + "`conversion_events`" + ` contract emit raw conversion events
with ` + "`date`" + `, ` + "`conversion_time`" + `, ` + "`conversion_name`" + `, ` + "`conversion_id`" + `, and
nullable ` + "`conversion_value`" + `.

## Configure Identity Links

Source packages that use the ` + "`identity_keys`" + ` contract emit timestamped key
observation rows with ` + "`date`" + `, ` + "`observed_at`" + `, ` + "`anonymous_id`" + `, ` + "`key_name`" + `, and
` + "`key_value`" + `. Analytics-core compresses those observations into daily key spans
before building links.
Declare which keys may create identity links in ` + "`segmentstream.yml`" + `:

` + "```yaml" + `
identity:
  keys:
    - name: user_id
      tier: deterministic
      window_days: 180
      max_distinct_anonymous_ids: 1000
    - name: ip_address
      tier: probabilistic
      window_days: 3
      max_distinct_anonymous_ids: 100
` + "```" + `

Every linkable key must be declared explicitly. ` + "`deterministic`" + ` keys also
prevent links between anonymous IDs that have conflicting deterministic values.
Source SQL owns key extraction, normalization, and filtering.

## Commands

Pass ` + "`--json`" + ` to any command for a structured response object on stdout.
The response includes ` + "`schema_version`" + `, ` + "`command`" + `, ` + "`status`" + `, and command-specific
` + "`data`" + `; diagnostics and actions are structured when present.

` + "`segmentstream init`" + ` reports setup state and the next action. Use
` + "`segmentstream init --json`" + ` for stable agent-readable output with the
setup state machine under ` + "`data.envelope`" + `.

` + "`segmentstream init --warehouse bigquery`" + ` initializes warehouse selection in
the current directory. It is safe to run again: existing ` + "`segmentstream.yml`" + `,
` + "`README.md`" + `, and ` + "`AGENTS.md`" + ` are not overwritten.

` + "`segmentstream warehouse auth login [--port <port>]`" + ` prints a Google
OAuth URL, waits for a loopback browser redirect on the same computer, and
stores a named BigQuery OAuth credential outside the project. Use ` + "`--port`" + `
when a sandbox or container needs the callback port to be forwarded before the
command starts.

` + "`segmentstream warehouse auth --service-account-key=<path>`" + ` stores a
named BigQuery service-account credential outside the project for headless
servers, CI, or other non-interactive environments.

` + "`segmentstream warehouse browse --json`" + ` lists accessible BigQuery projects.
Use ` + "`segmentstream warehouse browse --path <project> --json`" + ` to list datasets.
Use ` + "`segmentstream warehouse browse --path <project>/<dataset> --json`" + ` to list tables.
Use ` + "`segmentstream warehouse browse --path <project>/<dataset>/<table> --json`" + ` to fetch a table schema.

` + "`segmentstream warehouse query --sql \"<select statement>\" [--job-location <location>] --json`" + ` runs
a dry-run-verified read-only SELECT query and returns row objects under ` + "`data`" + `.
Use it to inspect payload samples, null rates, distinct values, date ranges,
and JSON fields after browsing table schemas.

` + "`segmentstream warehouse configure --project --dataset --location`" + ` validates
and writes warehouse settings.

` + "`segmentstream warehouse test`" + ` checks BigQuery connect, read, create table,
and query permissions.

` + "`segmentstream run`" + ` runs the configured analytics pipeline and writes results
to the configured warehouse. It runs the last 30 UTC daily partitions by default;
use ` + "`segmentstream run --start-date YYYY-MM-DD`" + ` to start earlier or later.

` + "`segmentstream source contracts [--type events|conversion_events|identity_keys] [--json]`" + ` lists
supported source contracts and returns their schemas.

` + "`segmentstream source scaffold <name> --type events|conversion_events|identity_keys [--json]`" + ` scaffolds
a local source template under ` + "`sources/<name>/`" + `. The template must be
implemented next.

` + "`segmentstream source verify <name> [--start-date YYYY-MM-DD] [--json]`" + ` runs the
source package's dbt verification tests inside Docker. It defaults to the last 7
UTC days.

## Files

` + "```text" + `
segmentstream.yml       project configuration and warehouse wiring
README.md               project documentation
AGENTS.md               instructions for LLM/code agents
.gitignore              should include .segmentstream/
sources/                project-owned source packages
.segmentstream/         generated SegmentStream environment files, disposable
` + "```" + `

Future project-owned folders such as ` + "`sources/`" + `, ` + "`destinations/`" + `, and
` + "`attribution_models/`" + ` should live outside ` + "`.segmentstream/`" + `.

Do not edit files inside ` + "`.segmentstream/`" + ` directly. The CLI may delete and
recreate that directory at any time.
`
}

func DefaultAgentGuideMarkdown() string {
	return `# Agent Instructions

This repository is a SegmentStream project.

Before editing SegmentStream project files, read ` + "`README.md`" + ` in this directory.

## Rules

- Treat ` + "`segmentstream.yml`" + ` as the source of truth for project configuration.
- Do not edit files inside ` + "`.segmentstream/`" + `; it is generated and disposable.
- Use ` + "`segmentstream init`" + ` to initialize the project.
- Use ` + "`segmentstream run`" + ` to run the pipeline and produce tables in the configured warehouse.
- Use ` + "`segmentstream source contracts`" + ` to inspect supported source contracts.
- Use ` + "`segmentstream source scaffold <name> --type events|conversion_events|identity_keys`" + ` to scaffold local source templates outside the generated environment.
- Use ` + "`segmentstream warehouse browse`" + ` and ` + "`segmentstream warehouse query`" + ` to inspect warehouse tables before implementing sources.
- Do not put secrets, credentials, private keys, tokens, or SQL in ` + "`segmentstream.yml`" + `.
- For BigQuery warehouse configuration, use the guidance in ` + "`README.md`" + `.
`
}

func EnsureProjectReadme(projectRoot string) (bool, error) {
	return ensureGuideFile(projectRoot, ProjectReadmeFileName, DefaultProjectReadmeMarkdown())
}

func EnsureAgentGuide(projectRoot string) (bool, error) {
	return ensureGuideFile(projectRoot, AgentGuideFileName, DefaultAgentGuideMarkdown())
}

func ensureGuideFile(projectRoot, name, content string) (bool, error) {
	path := filepath.Join(projectRoot, name)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return false, fmt.Errorf("write %s: %w", name, err)
			}
			return true, nil
		}
		return false, fmt.Errorf("check %s: %w", name, err)
	}
	return false, nil
}
