#!/usr/bin/env bash
# Cross-compiles the extractor stub for every target into the embed directory.
# Pure Go (CGO disabled), so it runs on any host — macOS, Linux, or Windows.
set -euo pipefail
cd "$(dirname "$0")/.."
out="internal/stubassets/stubs"
mkdir -p "$out"

build() {
  local goos="$1" goarch="$2" name="$3"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w" -o "$out/$name" ./stub
  echo "  built $name"
}

echo "Building extractor stubs -> $out"
build windows amd64 loci-stub-windows-amd64.exe
build darwin  arm64 loci-stub-darwin-arm64
build darwin  amd64 loci-stub-darwin-amd64
build linux   amd64 loci-stub-linux-amd64
echo "Done."
