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
dist_abs="$(cd dist && pwd)"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

if ! $INSTALL; then
  mkdir -p "$stage/completions"
  for shell in bash zsh fish powershell; do
    case "$shell" in
      zsh) comp_file="_barista" ;;
      powershell) comp_file="barista.ps1" ;;
      *) comp_file="barista.$shell" ;;
    esac
    go run ./cmd/barista completion "$shell" > "$stage/completions/$comp_file"
  done
  cp LICENSE "$stage/"
fi

for p in "${platforms[@]}"; do
  os="${p%/*}"
  arch="${p#*/}"
  bin="barista"
  [ "$os" = "windows" ] && bin="barista.exe"
  out="dist/barista-${os}-${arch}"
  [ "$os" = "windows" ] && out="${out}.exe"
  if $INSTALL; then
    echo "building ${out} (${VERSION})"
  else
    echo "building ${bin} for ${os}/${arch} (${VERSION})"
  fi
  if $INSTALL; then
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$out" ./cmd/barista
  else
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$stage/$bin" ./cmd/barista
  fi

  if $INSTALL; then
    bin_dir="${HOME}/.local/bin"
    mkdir -p "$bin_dir"
    cp -f "$out" "${bin_dir}/barista$([ "$os" = "windows" ] && echo .exe)"
    echo "installed ${bin_dir}/barista$([ "$os" = "windows" ] && echo .exe)"
    continue
  fi

  name="barista-${VERSION}-${os}-${arch}"
  if [ "$os" = "windows" ]; then
    command -v zip >/dev/null || { echo "zip is required to package windows archives" >&2; exit 1; }
    (cd "$stage" && zip -q -r "${dist_abs}/${name}.zip" "$bin" LICENSE completions)
    echo "packaged dist/${name}.zip"
  else
    tar -czf "dist/${name}.tar.gz" -C "$stage" "$bin" LICENSE completions
    echo "packaged dist/${name}.tar.gz"
  fi
  rm -f "$stage/$bin"
done
