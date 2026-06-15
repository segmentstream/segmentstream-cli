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

download() {
  url="$1"
  output="$2"
  label="$3"
  attempts="${SEGMENTSTREAM_INSTALL_RETRIES:-30}"
  delay="${SEGMENTSTREAM_INSTALL_RETRY_DELAY:-5}"
  attempt=1

  while [ "$attempt" -le "$attempts" ]; do
    if curl -fsSL "$url" -o "$output"; then
      return 0
    fi

    if [ "$attempt" -eq "$attempts" ]; then
      fail "could not download $label after $attempts attempts"
    fi

    printf 'Waiting for %s to become available (%s/%s)...\n' "$label" "$attempt" "$attempts" >&2
    sleep "$delay"
    attempt=$((attempt + 1))
  done
}

path_has_dir() {
  case ":$PATH:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

LOCAL_BIN="${HOME}/.local/bin"
LOCAL_BIN_LINK="$LOCAL_BIN/segmentstream"
INSTALLED_BIN="$INSTALL_DIR/segmentstream"
LOCAL_BIN_LINKED=0

maybe_link_local_bin() {
  INSTALLED_BIN="$INSTALL_DIR/segmentstream"

  if [ ! -d "$LOCAL_BIN" ] || [ ! -w "$LOCAL_BIN" ]; then
    return 0
  fi

  if [ -L "$LOCAL_BIN_LINK" ]; then
    current_target="$(readlink "$LOCAL_BIN_LINK" 2>/dev/null || true)"
    if [ "$current_target" = "$INSTALLED_BIN" ]; then
      LOCAL_BIN_LINKED=1
      return 0
    fi

    case "$current_target" in
      "$HOME/.segmentstream/bin/segmentstream")
        rm "$LOCAL_BIN_LINK"
        ln -s "$INSTALLED_BIN" "$LOCAL_BIN_LINK"
        LOCAL_BIN_LINKED=1
        printf 'Linked segmentstream into %s\n' "$LOCAL_BIN_LINK"
        return 0
        ;;
      *)
        printf '\n%s already exists and points to %s; leaving it unchanged.\n' "$LOCAL_BIN_LINK" "$current_target" >&2
        return 0
        ;;
    esac
  fi

  if [ -e "$LOCAL_BIN_LINK" ]; then
    printf '\n%s already exists; leaving it unchanged.\n' "$LOCAL_BIN_LINK" >&2
    return 0
  fi

  ln -s "$INSTALLED_BIN" "$LOCAL_BIN_LINK"
  LOCAL_BIN_LINKED=1
  printf 'Linked segmentstream into %s\n' "$LOCAL_BIN_LINK"
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
download "$CHECKSUMS_URL" "$CHECKSUMS_PATH" "checksums.txt"
download "$ARCHIVE_URL" "$ARCHIVE_PATH" "$ASSET"

EXPECTED="$(awk -v asset="$ASSET" 'NF >= 2 && $NF == asset {print $1}' "$CHECKSUMS_PATH" | head -n 1)"
[ -n "$EXPECTED" ] || fail "checksum for $ASSET was not found"
ACTUAL="$(sha256_file "$ARCHIVE_PATH")"
[ "$EXPECTED" = "$ACTUAL" ] || fail "checksum mismatch for $ASSET"

tar -xzf "$ARCHIVE_PATH" -C "$TMP_DIR"
[ -f "$TMP_DIR/segmentstream" ] || fail "archive did not contain segmentstream binary"
chmod +x "$TMP_DIR/segmentstream"

mkdir -p "$INSTALL_DIR"
mv "$TMP_DIR/segmentstream" "$INSTALL_DIR/segmentstream"
maybe_link_local_bin

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

if ! path_has_dir "$INSTALL_DIR" && { [ "$LOCAL_BIN_LINKED" -ne 1 ] || ! path_has_dir "$LOCAL_BIN"; }; then
  if [ "$LOCAL_BIN_LINKED" -eq 1 ]; then
    path_dir="$LOCAL_BIN"
  else
    path_dir="$INSTALL_DIR"
  fi

  printf '\n%s is not on your PATH.\n' "$path_dir"
  printf 'Add it for your current shell with:\n'
  printf '  export PATH="%s:$PATH"\n' "$path_dir"
  printf '\nAdd that line to your shell profile to make it permanent.\n'
fi
