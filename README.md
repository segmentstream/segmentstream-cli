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

Push a semver tag to publish GitHub Release assets:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow uses GoReleaser to publish platform archives and `checksums.txt`.
