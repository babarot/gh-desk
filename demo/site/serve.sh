#!/usr/bin/env bash
# Builds the site as it is published and serves it at
# http://localhost:PORT/gh-desk/ (PORT is 8080 unless given). The paths are
# the published ones, so the links between its parts are tried as they will
# work.
#
# Needs Go and Hugo: mise run site:serve gives Hugo from mise.toml.
set -euo pipefail

site=$(cd "$(dirname "$0")" && pwd)
port=${PORT:-8080}

out=$(mktemp -d)
BASE_URL="http://localhost:$port/gh-desk/docs/" "$site/build.sh" "$out/gh-desk"

echo "http://localhost:$port/gh-desk/"
cd "$out"
exec python3 -m http.server "$port" --bind 127.0.0.1
