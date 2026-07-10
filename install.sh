#!/bin/sh
# api2convert installer — detects your OS/arch, downloads the latest release
# from GitHub, verifies its checksum, and installs the binary onto your PATH.
#
#   curl -fsSL https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.sh | sh
#
# Pin a version with:  API2CONVERT_VERSION=v1.2.3 sh install.sh
set -eu

REPO="QaamGo/api2convert-cli"
BIN="api2convert"

info() { printf '%s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  *) die "unsupported OS: $os (use the Windows PowerShell installer or download manually)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl is required"

tag="${API2CONVERT_VERSION:-}"
if [ -z "$tag" ]; then
  info "> Resolving the latest release…"
  tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep -oE '"tag_name":[[:space:]]*"[^"]+"' | head -1 | cut -d'"' -f4)"
fi
[ -n "$tag" ] || die "could not determine the latest version"
ver="${tag#v}"

asset="${BIN}_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"
info "> Downloading $asset"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

info "> Verifying checksum"
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$tmp" && grep " $asset\$" checksums.txt | sha256sum -c - >/dev/null ) || die "checksum verification failed"
elif command -v shasum >/dev/null 2>&1; then
  ( cd "$tmp" && grep " $asset\$" checksums.txt | shasum -a 256 -c - >/dev/null ) || die "checksum verification failed"
elif [ "${API2CONVERT_INSECURE_SKIP_VERIFY:-}" = "1" ]; then
  # Fail closed by default. Skipping verification requires an explicit opt-out
  # so a missing sha256 tool can never silently install an unverified binary.
  info "! no sha256 tool found; skipping verification (API2CONVERT_INSECURE_SKIP_VERIFY=1)"
else
  die "no sha256 tool found (install coreutils or perl/shasum); or set API2CONVERT_INSECURE_SKIP_VERIFY=1 to bypass at your own risk"
fi

tar -xzf "$tmp/$asset" -C "$tmp"

dest="${API2CONVERT_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dest" 2>/dev/null || true
if [ ! -w "$dest" ]; then
  dest="/usr/local/bin"
  info "> Installing to $dest (may require sudo)"
  sudo install -m 0755 "$tmp/$BIN" "$dest/$BIN"
else
  install -m 0755 "$tmp/$BIN" "$dest/$BIN"
fi

info ""
info "Installed: $("$dest/$BIN" version 2>/dev/null || echo "$dest/$BIN")"
case ":$PATH:" in
  *":$dest:"*) : ;;
  *) info "Add $dest to your PATH:  export PATH=\"$dest:\$PATH\"" ;;
esac
info "Next: api2convert login"
