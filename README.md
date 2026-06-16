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
segmentstream prepare
segmentstream update
segmentstream update --check
```

`segmentstream init` creates a v1 `segmentstream.yml`, `README.md`, and
`AGENTS.md` in the current directory if they do not already exist, ensures
`.segmentstream/` is listed in `.gitignore`, and prepares the generated local
runtime.

`segmentstream prepare` reads `segmentstream.yml` and recreates the disposable
project-local `.segmentstream/` runtime directory with Docker Compose, dbt, and
Dagster files. Do not edit files inside `.segmentstream/`; update
`segmentstream.yml` instead and run `segmentstream prepare` again.

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
