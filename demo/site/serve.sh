#!/usr/bin/env bash
# Builds the site as it is published and serves it at
# http://localhost:PORT/gh-desk/. PORT given is used or refused; otherwise
# it is 8080, or the next one free. The paths are the published ones, so the
# links between its parts are tried as they will work.
#
# Needs Go and Hugo: mise run site:serve gives Hugo from mise.toml.
set -euo pipefail

site=$(cd "$(dirname "$0")" && pwd)

# Before the build, as the docs are built with the port in their addresses.
free() {
  python3 -c '
import socket, sys
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", int(sys.argv[1])))
' "$1" 2>/dev/null
}

if [ -n "${PORT:-}" ]; then
  port=$PORT
  if ! free "$port"; then
    echo "port $port is in use; give another with PORT, or leave it out to take a free one" >&2
    exit 1
  fi
else
  port=8080
  until free "$port"; do
    port=$((port + 1))
    if [ "$port" -gt 8099 ]; then
      echo "ports 8080 to 8099 are in use; give another with PORT" >&2
      exit 1
    fi
  done
  [ "$port" = 8080 ] || echo "port 8080 is in use; serving on $port"
fi

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
