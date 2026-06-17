# SegmentStream CLI

CLI for SegmentStream marketing analytics.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/segmentstream/segmentstream-cli/main/install.sh | sh
```

The installer places the binary in `$HOME/.segmentstream/bin` by default and prints PATH guidance if needed.
If `$HOME/.local/bin` already exists, is writable, and is on PATH, the installer creates a safe symlink there.

## Commands

```sh
segmentstream version
segmentstream init
segmentstream run
segmentstream update
segmentstream update --check
```

`segmentstream init` creates a v1 `segmentstream.yml`, `README.md`, and
`AGENTS.md` in the current directory if they do not already exist, ensures
`.segmentstream/` is listed in `.gitignore`, and prepares the generated local
runtime.

`segmentstream run` reads `segmentstream.yml`, recreates the disposable
project-local `.segmentstream/` runtime directory, rebuilds/restarts the local
Dagster/dbt environment at http://localhost:3000 using Docker Compose, and runs
the SegmentStream materialization. Do not edit files inside `.segmentstream/`;
update `segmentstream.yml` instead and run `segmentstream run` again. The first
run can take a few minutes while Docker downloads and builds the local
environment; later runs should be faster.

The generated project `README.md` explains the project structure, generated
runtime boundary, commands, and v1 BigQuery warehouse configuration.

The generated `AGENTS.md` tells LLM/code agents to read `README.md` before
editing SegmentStream project files.

Example `segmentstream.yml`:

```yaml
version: 1

warehouse:
  type: bigquery
  auth: default-bigquery
  project: your-gcp-project
  dataset: segmentstream
  location: US
```

## Release

Publish a GitHub Release with a semver tag to build and attach release assets:

1. Open https://github.com/segmentstream/segmentstream-cli/releases/new
2. Create or choose a tag like `v0.1.0`.
3. Publish the release.

The release workflow runs when the release is published and uses GoReleaser to attach platform archives and `checksums.txt`.
The installer waits for those assets, so it is safe to run shortly after publishing a release.
