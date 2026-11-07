#!/usr/bin/env bash
# Builds what the pages of the site need into demo/site/web: in play/,
# gh-desk.wasm and the wasm_exec.js of the Go that built it, and in
# play/settings/, the settings playground.
#
# Bubble Tea and atotto/clipboard have no files for js, so each is copied
# out of the module cache with the file in demo/site/patch added, and the build
# takes the copies through a go.mod of its own; demo/site/go.mod is left
# as it is.
set -euo pipefail

site=$(cd "$(dirname "$0")" && pwd)
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
  -o web/play/gh-desk.wasm ./wasm
install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/play/wasm_exec.js
go run ./settings web/play/settings/index.html
ls -lh web/play/gh-desk.wasm
