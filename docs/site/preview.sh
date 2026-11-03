#!/usr/bin/env bash
# Builds the site as it is published, the playground with the docs under
# it, and serves it at http://localhost:PORT/gh-desk/ (PORT is 8080 unless
# given). The paths are the published ones, so the links between the
# playground and the docs are tried as they will work.
#
# Needs Go and Hugo: mise run docs:serve gives Hugo from mise.toml.
set -euo pipefail

site_dir=$(cd "$(dirname "$0")" && pwd)
repo_dir=$(cd "$site_dir/../.." && pwd)
port=${PORT:-8080}

out=$(mktemp -d)
"$repo_dir/demo/site/build.sh"
mkdir -p "$out/gh-desk"
cp -R "$repo_dir/demo/site/web/." "$out/gh-desk/"
BASE_URL="http://localhost:$port/gh-desk/docs/" "$site_dir/build.sh" "$out/gh-desk/docs"

echo "http://localhost:$port/gh-desk/"
cd "$out"
exec python3 -m http.server "$port" --bind 127.0.0.1
