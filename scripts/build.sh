#!/usr/bin/env bash
set -euo pipefail

# Build script for Crush (macOS/Linux)
# - Builds an OS/arch-appropriate binary by default (current host)
# - Supports cross-compiling all supported targets with --all
# - Optional install to ~/.local/bin as "crush" (no suffix) with --install

usage() {
  cat <<'USAGE'
Usage: scripts/build.sh [options]

Options:
  --all            Build for linux and macOS, amd64 and arm64.
  --os <os>        Target OS: linux|macos (default: host OS).
  --arch <arch>    Target arch: amd64|arm64 (default: host arch).
  --install        Install the host OS/arch binary to ~/.local/bin/crush.
  -h, --help       Show this help.

Notes:
  - Output binaries go to ./build named as crush_<oslabel>_<arch>[.ext]
  - On install, the suffix is removed and the binary is named "crush".
USAGE
}

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BUILD_DIR="$ROOT_DIR/build"
mkdir -p "$BUILD_DIR"

want_all=false
want_install=false
target_os=""
target_arch=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --all)
      want_all=true; shift ;;
    --install)
      want_install=true; shift ;;
    --os)
      target_os="${2:-}"; shift 2 ;;
    --arch)
      target_arch="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "Unknown argument: $1" >&2
      usage; exit 1 ;;
  esac
done

detect_host_os() {
  case "$(uname -s)" in
    Darwin) echo "macos" ;;
    Linux)  echo "linux" ;;
    *) echo "unsupported" ;;
  esac
}

detect_host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "unsupported" ;;
  esac
}

to_go_os() {
  case "$1" in
    macos) echo "darwin" ;;
    linux) echo "linux" ;;
    *) echo "" ;;
  esac
}

build_one() {
  local os_label="$1"   # linux|macos
  local arch="$2"       # amd64|arm64
  local goos
  goos="$(to_go_os "$os_label")"
  if [[ -z "$goos" ]]; then
    echo "Skipping unsupported OS label: $os_label" >&2
    return 1
  fi

  local ext=""
  local out="$BUILD_DIR/crush_${os_label}_${arch}${ext}"
  echo "Building $out (GOOS=$goos GOARCH=$arch)"
  CGO_ENABLED=${CGO_ENABLED:-0} GOOS="$goos" GOARCH="$arch" go build -o "$out" .
}

install_host() {
  local os_label="$1"
  local arch="$2"
  local src="$BUILD_DIR/crush_${os_label}_${arch}"
  local bin_dir="$HOME/.local/bin"
  mkdir -p "$bin_dir"
  local dest="$bin_dir/crush"
  if [[ -f "$dest" ]]; then
    echo "Found existing binary at $dest, deleting before installing new binary"
    rm -f "$dest"
  fi
  echo "Installing $src -> $dest"
  install -m 0755 "$src" "$dest"
}

main() {
  if [[ "$want_all" == true ]]; then
    # Build for both OS (linux, macos) and both arch (amd64, arm64)
    for os_label in linux macos; do
      for arch in amd64 arm64; do
        build_one "$os_label" "$arch"
      done
    done
    # Optional host install after building all
    if [[ "$want_install" == true ]]; then
      local host_os host_arch
      host_os="${target_os:-$(detect_host_os)}"
      host_arch="${target_arch:-$(detect_host_arch)}"
      if [[ "$host_os" == unsupported || "$host_arch" == unsupported ]]; then
        echo "Cannot install: unsupported host ($host_os/$host_arch)" >&2
        exit 1
      fi
      install_host "$host_os" "$host_arch"
    fi
    exit 0
  fi

  # Single target: default to host if not specified
  local os_label="${target_os:-$(detect_host_os)}"
  local arch="${target_arch:-$(detect_host_arch)}"

  if [[ "$os_label" == unsupported || "$arch" == unsupported ]]; then
    echo "Unsupported host or target (os=$os_label arch=$arch)." >&2
    exit 1
  fi

  build_one "$os_label" "$arch"

  if [[ "$want_install" == true ]]; then
    install_host "$os_label" "$arch"
  fi
}

main "$@"

