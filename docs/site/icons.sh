#!/usr/bin/env bash
# Writes the icons of the docs site into static/ from the two drawings of
# the logo: favicon.svg (the small mark, every pill solid) for the 16 and
# 32 px icons, and assets/logo.svg (the mark with the pulled pill solid)
# on the site's blue for the icons of a home screen. Run it again after
# changing either drawing; the PNGs and favicon.ico are kept in git.
#
# Needs rsvg-convert (librsvg) and magick (ImageMagick), for one run:
#   nix shell nixpkgs#librsvg nixpkgs#imagemagick -c docs/site/icons.sh
set -euo pipefail

site_dir=$(cd "$(dirname "$0")" && pwd)
static="$site_dir/static"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for tool in rsvg-convert magick; do
  if ! command -v "$tool" >/dev/null; then
    echo "icons.sh needs $tool; see the comment at the top" >&2
    exit 1
  fi
done

# The small mark, in the light theme's blue (a PNG has no dark mode).
for size in 16 32; do
  rsvg-convert -w "$size" -h "$size" "$static/favicon.svg" -o "$static/favicon-${size}x${size}.png"
done
magick "$static/favicon-16x16.png" "$static/favicon-32x32.png" "$static/favicon.ico"

# A home screen's icon: the logo in white on the blue, filling the square,
# since the system rounds the corners itself.
mark=$(sed -e 's/currentColor/#ffffff/g' -e 's/<svg [^>]*>//' -e 's#</svg>##' "$site_dir/assets/logo.svg")
cat >"$work/app.svg" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" fill="#2f6fde"/><g transform="translate(5.6 5.6) scale(0.65)" fill="#ffffff">$mark</g></svg>
EOF
rsvg-convert -w 180 -h 180 "$work/app.svg" -o "$static/apple-touch-icon.png"
for size in 192 512; do
  rsvg-convert -w "$size" -h "$size" "$work/app.svg" -o "$static/android-chrome-${size}x${size}.png"
done

ls -1 "$static"
