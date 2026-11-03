#!/usr/bin/env bash
# Builds the documentation site into the directory given (by default
# ./site under the repository), with Hugo and the Hextra theme.
#
# The pages are the Markdown files of docs/, written to be read on GitHub
# as well: a page starts with its "# title" and has no front matter. They
# are copied into a Hugo site in a directory of its own, each given the
# front matter Hugo needs. docs/adr, docs/architecture.md and docs/site
# are not pages; a link to them leads to GitHub (layouts/_markup).
#
# A base URL other than hugo.yaml's is given with BASE_URL, as preview.sh
# does.
set -euo pipefail

site_dir=$(cd "$(dirname "$0")" && pwd)
docs_dir=$(cd "$site_dir/.." && pwd)
repo_dir=$(cd "$docs_dir/.." && pwd)
out=${1:-"$repo_dir/site"}
mkdir -p "$out"
out=$(cd "$out" && pwd -P)
case "$out" in
  / | "$repo_dir" | "$docs_dir" | "$site_dir")
    echo "refusing to build into $out" >&2
    exit 1
    ;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cp "$site_dir/hugo.yaml" "$site_dir/go.mod" "$site_dir/go.sum" "$work/"
cp -R "$site_dir/layouts" "$site_dir/assets" "$work/"
mkdir -p "$work/content" "$work/static"

# The order of the pages in the sidebar; a page not listed comes after
# these, by title.
weight() {
  case "$1" in
    getting-started.md) echo 10 ;;
    concepts/tabs.md) echo 10 ;;
    concepts/reading.md) echo 20 ;;
    concepts/selecting.md) echo 30 ;;
    concepts/approve-and-merge.md) echo 40 ;;
    concepts/ask-claude.md) echo 50 ;;
    reference/commands.md) echo 10 ;;
    reference/configuration.md) echo 20 ;;
    reference/keys.md) echo 30 ;;
  esac
}

# The file to edit to change a page: the page itself, or for a page written
# by a test, what the test writes it from.
source_of() {
  case "$1" in
    reference/commands.md) echo internal/app/app.go ;;
    reference/configuration.md) echo internal/config/settings.go ;;
    reference/keys.md) echo internal/tui/keymap/keymap.go ;;
    *) echo "docs/$1" ;;
  esac
}

while IFS= read -r -d '' src; do
  rel=${src#"$docs_dir/"}
  target="$work/content/$rel"
  [ "$rel" = index.md ] && target="$work/content/_index.md"

  IFS= read -r heading <"$src" || heading=""
  if [[ $heading != "# "* ]]; then
    echo "$rel: a page starts with its \"# title\"" >&2
    exit 1
  fi
  title=${heading#"# "}
  title=${title//\\/\\\\}
  title=${title//\"/\\\"}

  mkdir -p "$(dirname "$target")"
  {
    echo '---'
    echo "title: \"$title\""
    w=$(weight "$rel")
    [ -n "$w" ] && echo "weight: $w"
    echo "editURL: \"https://github.com/babarot/gh-desk/edit/main/$(source_of "$rel")\""
    if [ "$rel" = index.md ]; then
      printf '%s\n' 'type: docs' 'cascade:' '  type: docs'
    fi
    echo '---'
    tail -n +2 "$src"
  } >"$target"
done < <(find "$docs_dir" \( -path "$site_dir" -o -path "$docs_dir/adr" \) -prune -o \
  -type f -name '*.md' ! -path "$docs_dir/architecture.md" -print0)

section() { # dir title weight
  printf -- '---\ntitle: "%s"\nweight: %s\n---\n' "$2" "$3" >"$work/content/$1/_index.md"
}
section concepts Concepts 20
section reference Reference 40

hugo --source "$work" --destination "$out" ${BASE_URL:+--baseURL "$BASE_URL"} \
  --cleanDestinationDir --gc --minify --panicOnWarning --printPathWarnings
