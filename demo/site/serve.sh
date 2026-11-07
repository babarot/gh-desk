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
# no-store: every build has the same addresses, so a page kept by the browser
# would hide what changed.
exec python3 -c '
import http.server, sys

class Handler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-store")
        super().end_headers()

http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
' "$port"
