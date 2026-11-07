#!/usr/bin/env bash
# Builds the whole site into the directory given (by default ./site under
# the repository, which is ignored and made anew): the pages in
# demo/site/web, and what they need that is not kept in git. In play/,
# gh-desk.wasm and the wasm_exec.js of the Go that built it; in
# play/settings/, the settings playground; in docs/, the docs, which
# docs/site/build.sh makes.
#
# Bubble Tea and atotto/clipboard have no files for js, so each is copied
# out of the module cache with the file in demo/site/patch added, and the build
# takes the copies through a go.mod of its own; demo/site/go.mod is left
# as it is.
set -euo pipefail

site=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$site/../.." && pwd)
if [ $# -eq 0 ]; then
  rm -rf "$repo/site"
fi
out=${1:-"$repo/site"}
mkdir -p "$out"
out=$(cd "$out" && pwd -P)
cp -R "$site/web/." "$out/"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$site"

patched() { # module dir-in-patch
  local dir
  dir=$(go list -m -f '{{.Dir}}' "$1")
  if [ -z "$dir" ]; then
    go mod download "$1"
    dir=$(go list -m -f '{{.Dir}}' "$1")
  fi
  cp -R "$dir" "$work/$2"
  chmod -R u+w "$work/$2"
  cp "$site/patch/$2"/*.go "$work/$2/"
  echo "-replace=$1=$work/$2"
}

cp go.mod "$work/go.mod"
cp go.sum "$work/go.sum"
go mod edit -modfile="$work/go.mod" \
  "$(patched charm.land/bubbletea/v2 bubbletea)" \
  "$(patched github.com/atotto/clipboard clipboard)"

GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -modfile="$work/go.mod" -trimpath -ldflags="-s -w" \
  -o "$out/play/gh-desk.wasm" ./wasm
install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$out/play/wasm_exec.js"
go run ./settings "$out/play/settings/index.html"
"$repo/docs/site/build.sh" "$out/docs"
ls -lh "$out/play/gh-desk.wasm"
