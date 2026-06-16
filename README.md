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
segmentstream auth bigquery
segmentstream update
segmentstream update --check
```

`segmentstream auth bigquery` uses the installed Google Cloud SDK to open Google
authentication in the browser. It runs gcloud with an isolated SegmentStream
config directory and stores BigQuery ADC credentials at
`$HOME/.segmentstream/gcloud/application_default_credentials.json`. The gcloud
ADC login requests the required `cloud-platform` scope plus BigQuery scope.

## Release

Publish a GitHub Release with a semver tag to build and attach release assets:

1. Open https://github.com/segmentstream/segmentstream-cli/releases/new
2. Create or choose a tag like `v0.1.0`.
3. Publish the release.

The release workflow runs when the release is published and uses GoReleaser to attach platform archives and `checksums.txt`.
The installer waits for those assets, so it is safe to run shortly after publishing a release.
