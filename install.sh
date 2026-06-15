#!/bin/sh
set -eu

REPO="segmentstream/segmentstream-cli"
INSTALL_DIR="${HOME}/.segmentstream/bin"
METADATA_DIR="${HOME}/.segmentstream"
GITHUB_API_BASE_URL="${SEGMENTSTREAM_GITHUB_API_BASE_URL:-https://api.github.com}"
GITHUB_DOWNLOAD_BASE_URL="${SEGMENTSTREAM_GITHUB_DOWNLOAD_BASE_URL:-https://github.com}"

usage() {
  cat <<EOF
Install SegmentStream CLI.

Usage:
  install.sh [--install-dir DIR]

Options:
  --install-dir DIR  Install segmentstream into DIR. Defaults to $HOME/.segmentstream/bin.
  -h, --help         Show this help.
EOF
}

fail() {
  printf 'segmentstream install: %s\n' "$1" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir)
      [ "$#" -ge 2 ] || fail "--install-dir requires a value"
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown option: $1"
      ;;
  esac
done

case "$INSTALL_DIR" in
  *\"*) fail "install directory must not contain double quotes" ;;
esac

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need_cmd curl
need_cmd tar
need_cmd uname
need_cmd mktemp

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin|linux) ;;
  *) fail "unsupported OS: $OS" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) fail "unsupported architecture: $ARCH" ;;
esac

if command -v shasum >/dev/null 2>&1; then
  sha256_file() {
    shasum -a 256 "$1" | awk '{print $1}'
  }
elif command -v sha256sum >/dev/null 2>&1; then
  sha256_file() {
    sha256sum "$1" | awk '{print $1}'
  }
else
  fail "shasum or sha256sum is required"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

LATEST_JSON="$TMP_DIR/latest.json"
curl -fsSL "$GITHUB_API_BASE_URL/repos/$REPO/releases/latest" -o "$LATEST_JSON"
VERSION="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$LATEST_JSON" | head -n 1)"
[ -n "$VERSION" ] || fail "could not determine latest release version"

ASSET="segmentstream_${OS}_${ARCH}.tar.gz"
ARCHIVE_URL="$GITHUB_DOWNLOAD_BASE_URL/$REPO/releases/download/$VERSION/$ASSET"
CHECKSUMS_URL="$GITHUB_DOWNLOAD_BASE_URL/$REPO/releases/download/$VERSION/checksums.txt"
ARCHIVE_PATH="$TMP_DIR/$ASSET"
CHECKSUMS_PATH="$TMP_DIR/checksums.txt"

printf 'Installing segmentstream %s for %s/%s\n' "$VERSION" "$OS" "$ARCH"
curl -fsSL "$CHECKSUMS_URL" -o "$CHECKSUMS_PATH"
curl -fsSL "$ARCHIVE_URL" -o "$ARCHIVE_PATH"

EXPECTED="$(awk -v asset="$ASSET" 'NF >= 2 && $NF == asset {print $1}' "$CHECKSUMS_PATH" | head -n 1)"
[ -n "$EXPECTED" ] || fail "checksum for $ASSET was not found"
ACTUAL="$(sha256_file "$ARCHIVE_PATH")"
[ "$EXPECTED" = "$ACTUAL" ] || fail "checksum mismatch for $ASSET"

tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
[ -f "$TMP_DIR/segmentstream" ] || fail "archive did not contain segmentstream binary"
chmod +x "$TMP_DIR/segmentstream"

mkdir -p "$INSTALL_DIR"
mv "$TMP_DIR/segmentstream" "$INSTALL_DIR/segmentstream"

mkdir -p "$METADATA_DIR"
cat > "$METADATA_DIR/install.json" <<EOF
{
  "method": "script",
  "install_dir": "$INSTALL_DIR",
  "repo": "$REPO",
  "version": "${VERSION#v}",
  "os": "$OS",
  "arch": "$ARCH"
}
EOF

printf 'segmentstream installed to %s/segmentstream\n' "$INSTALL_DIR"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '\n%s is not on your PATH.\n' "$INSTALL_DIR"
    printf 'Add it for your current shell with:\n'
    printf '  export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    printf '\nAdd that line to your shell profile to make it permanent.\n'
    ;;
esac
