#!/usr/bin/env bash
# Builds the demo on the site into demo/site/web: gh-desk.wasm, the
# wasm_exec.js of the Go that built it, and play/, the page on the whole
# window.
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
  -o web/gh-desk.wasm ./wasm
install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js
# play/ is the playground on the whole window: the same page, which sees
# where it is served, with a base so its files are read from above.
mkdir -p web/play
awk '{ print } /<head>/ { print "<base href=\"../\">" }' web/index.html >web/play/index.html
ls -lh web/gh-desk.wasm
