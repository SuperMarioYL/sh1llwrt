#!/usr/bin/env sh
# sh1llwrt installer: drops the binary into ~/.local/bin (or $GOPATH/bin via go install fallback).
# v0.1 is OSS-only, free, no account, no telemetry. This script is the curl|sh entry point.
set -eu

OWNER="SuperMarioYL"
REPO="sh1llwrt"
INSTALL_DIR="${HOME}/.local/bin"
GOPATH_BIN="${GOPATH:-${HOME}/go}/bin"

err() { printf "sh1llwrt: %s\n" "$*" >&2; }
note() { printf "sh1llwrt: %s\n" "$*"; }

# --- detect OS / arch (m1 ships darwin/arm64 + linux/amd64) ----------------
OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Darwin) os="darwin" ;;
  Linux)  os="linux"  ;;
  *) err "unsupported OS: $OS (v0.1 ships darwin/arm64 + linux/amd64)"; exit 1 ;;
esac
case "$ARCH" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) err "unsupported arch: $ARCH"; exit 1 ;;
esac

mkdir -p "$INSTALL_DIR"

# --- try the latest GitHub release binary --------------------------------
# Release asset naming from .goreleaser.yaml: sh1llwrt_<version>_<os>_<arch>.tar.gz
api_url="https://api.github.com/repos/${OWNER}/${REPO}/releases/latest"
if command -v curl >/dev/null 2>&1; then
  dl_url="$(curl -fsSL "$api_url" 2>/dev/null | grep -oE "browser_download_url.*${os}_${arch}\.tar\.gz" | head -1 | sed 's/.*browser_download_url": *"//; s/"$//')" || dl_url=""
elif command -v wget >/dev/null 2>&1; then
  dl_url="$(wget -qO- "$api_url" 2>/dev/null | grep -oE "browser_download_url.*${os}_${arch}\.tar\.gz" | head -1 | sed 's/.*browser_download_url": *"//; s/"$//')" || dl_url=""
else
  dl_url=""
fi

if [ -n "$dl_url" ] && [ "$dl_url" != "null" ]; then
  note "downloading $dl_url"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT
  ( cd "$tmpdir" \
    && curl -fsSL "$dl_url" -o sh1llwrt.tar.gz \
    && tar -xzf sh1llwrt.tar.gz \
    && mv sh1llwrt "${INSTALL_DIR}/sh1llwrt" )
  chmod +x "${INSTALL_DIR}/sh1llwrt"
  note "installed ${INSTALL_DIR}/sh1llwrt"
else
  # No release binary yet (pre-release). Fall back to go install from source.
  if ! command -v go >/dev/null 2>&1; then
    err "no release binary for ${os}/${arch} and no Go toolchain to build from source."
    err "install Go 1.24+ (https://go.dev/dl/) or open an issue: https://github.com/${OWNER}/${REPO}/issues"
    exit 1
  fi
  note "no release binary yet; building from source via go install"
  GOBIN="$INSTALL_DIR" go install "github.com/${OWNER}/${REPO}/cmd/sh1llwrt@latest"
fi

# --- next steps -----------------------------------------------------------
case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *) note "add ${INSTALL_DIR} to your PATH, then: sh1llwrt init && exec \$SHELL" ;;
esac
note "done. next: sh1llwrt init && exec \$SHELL"
note "first 'sh1llwrt ask' will download the 941MB model once (offline forever after)"
