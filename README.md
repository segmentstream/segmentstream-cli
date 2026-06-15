# SegmentStream CLI

CLI for SegmentStream marketing analytics.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/segmentstream/segmentstream-cli/main/install.sh | sh
```

The installer places the binary in `$HOME/.segmentstream/bin` by default and prints PATH guidance if needed.

## Commands

```sh
segmentstream version
segmentstream update
segmentstream update --check
```

## Release

Publish a GitHub Release with a semver tag to build and attach release assets:

1. Open https://github.com/segmentstream/segmentstream-cli/releases/new
2. Create or choose a tag like `v0.1.0`.
3. Publish the release.

The release workflow runs when the release is published and uses GoReleaser to attach platform archives and `checksums.txt`.
