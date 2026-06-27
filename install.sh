#!/bin/sh
# rigwatch installer — downloads the latest release binary for your platform.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/allisonhere/rigwatch/master/install.sh | sh
#
# Environment overrides:
#   RIGWATCH_VERSION   release tag to install (default: latest)
#   RIGWATCH_INSTALL   install directory (default: /usr/local/bin, or ~/.local/bin if not writable)
set -eu

REPO="allisonhere/rigwatch"
BINARY="rigwatch"
VERSION="${RIGWATCH_VERSION:-latest}"

err() { printf 'error: %s\n' "$1" >&2; exit 1; }

# --- detect downloader ---
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO "$2" "$1"; }
else
  err "need curl or wget installed"
fi

# --- detect platform ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS: $os (this script handles Linux and macOS; on Windows download the .exe from the Releases page)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

asset="${BINARY}-${os}-${arch}"

# --- build download URL ---
if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

# --- choose install dir ---
if [ -n "${RIGWATCH_INSTALL:-}" ]; then
  dir="$RIGWATCH_INSTALL"
elif [ -w /usr/local/bin ] 2>/dev/null; then
  dir="/usr/local/bin"
elif command -v sudo >/dev/null 2>&1 && [ -d /usr/local/bin ]; then
  dir="/usr/local/bin"
  SUDO="sudo"
else
  dir="${HOME}/.local/bin"
fi
SUDO="${SUDO:-}"

# --- download to a temp file ---
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
printf 'Downloading %s (%s)...\n' "$asset" "$VERSION"
dl "$url" "$tmp" || err "download failed: $url (does a release exist for this platform?)"

# A real binary is not a tiny HTML/text error page — guard against a saved 404.
if [ "$(wc -c < "$tmp")" -lt 100000 ]; then
  err "downloaded file is too small to be the binary — the release asset may be missing"
fi

# --- install ---
chmod +x "$tmp"
$SUDO mkdir -p "$dir"
$SUDO mv "$tmp" "${dir}/${BINARY}"
trap - EXIT

printf '\nInstalled %s to %s/%s\n' "$BINARY" "$dir" "$BINARY"

# --- PATH hint ---
case ":${PATH}:" in
  *":${dir}:"*) ;;
  *) printf '\nNote: %s is not in your PATH. Add it with:\n  export PATH="%s:$PATH"\n' "$dir" "$dir" ;;
esac

"${dir}/${BINARY}" --version 2>/dev/null || true
