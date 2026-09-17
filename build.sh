#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X main.version=${VERSION}"

INSTALL=false
for arg in "$@"; do
  case "$arg" in
    --install) INSTALL=true ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

if $INSTALL; then
  platforms=("$(go env GOOS)/$(go env GOARCH)")
else
  platforms=(
    windows/amd64
    windows/arm64
    linux/amd64
    linux/arm64
    darwin/amd64
    darwin/arm64
  )
fi

mkdir -p dist

for p in "${platforms[@]}"; do
  os="${p%/*}"
  arch="${p#*/}"
  out="dist/barista-${os}-${arch}"
  [ "$os" = "windows" ] && out="${out}.exe"
  echo "building ${out} (${VERSION})"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/barista

  if $INSTALL; then
    bin="${HOME}/.local/bin"
    mkdir -p "$bin"
    cp -f "$out" "${bin}/barista$([ "$os" = "windows" ] && echo .exe)"
    echo "installed ${bin}/barista$([ "$os" = "windows" ] && echo .exe)"
  fi
done
