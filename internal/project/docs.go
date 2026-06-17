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
- Google Cloud CLI (` + "`gcloud`" + `) for BigQuery authentication.
- Git.

Start by configuring the warehouse in ` + "`segmentstream.yml`" + `.

` + "```yaml" + `
version: 1

warehouse:
  type: bigquery
  auth: default-bigquery
  project: your-gcp-project
  dataset: segmentstream
  location: US
` + "```" + `

Replace ` + "`your-gcp-project`" + ` with your Google Cloud project ID. Choose the
BigQuery dataset where SegmentStream should produce tables. ` + "`location`" + ` is
optional and defaults to ` + "`US`" + `.

` + "`warehouse.auth`" + ` is a named credential reference. It is not a secret value.
Do not put tokens, private keys, passwords, or SQL in ` + "`segmentstream.yml`" + `.

## Run The Pipeline

` + "```sh" + `
segmentstream run
` + "```" + `

` + "`segmentstream run`" + ` runs the SegmentStream pipeline and produces tables in
the configured warehouse. Each run regenerates the local runtime,
rebuilds/restarts the local Docker environment if needed, and asks Dagster to
run materialization. Dagster materializes all declared assets.

The first run can take a few minutes while Docker downloads and builds the
local environment. Later runs should be faster.

## Create A Source

Sources are project-owned dbt packages for source-specific transformations.

` + "```sh" + `
segmentstream source init ga4
` + "```" + `

This creates ` + "`sources/ga4/`" + ` as a standard dbt project with a staging
model and an exported events model. Exported models are incremental and
partitioned by ` + "`event_date`" + `. SegmentStream-specific export metadata lives
in dbt model properties.

Declare the source in ` + "`segmentstream.yml`" + `:

` + "```yaml" + `
sources:
  - name: ga4
    path: ./sources/ga4
` + "```" + `

On run, the Dagster runtime reads ` + "`segmentstream.yml`" + `, installs declared
sources as dbt packages, and generates a core ` + "`events`" + ` model that unions each declared
` + "`events_<source>`" + ` export. dbt models are declared as Dagster assets.

## Commands

` + "`segmentstream init`" + ` initializes the project in the current directory. It is
safe to run again: existing ` + "`segmentstream.yml`" + `, ` + "`README.md`" + `, and
` + "`AGENTS.md`" + ` are not overwritten.

` + "`segmentstream run`" + ` runs the configured analytics pipeline and writes results
to the configured warehouse.

` + "`segmentstream source init <name>`" + ` creates a local source package template
under ` + "`sources/<name>/`" + `.

## Files

` + "```text" + `
segmentstream.yml       project configuration and warehouse wiring
README.md               project documentation
AGENTS.md               instructions for LLM/code agents
.gitignore              should include .segmentstream/
sources/                project-owned source packages
.segmentstream/         generated dbt/Dagster/Docker runtime, disposable
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
- Use ` + "`segmentstream source init <name>`" + ` to create local source packages outside the generated runtime.
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
