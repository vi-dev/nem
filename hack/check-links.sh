#!/usr/bin/env bash
set -euo pipefail

public=${1:?public dir}
base=${2:-/nem}
broken=$(mktemp)
trap 'rm -f "$broken"' EXIT

while IFS= read -r -d '' page; do
  dir=$(dirname "$page")
  { grep -oE 'href=("[^"]*"|[^ >]+)' "$page" || :; } | sed 's/^href=//; s/^"//; s/"$//' | while IFS= read -r href; do
    case "$href" in
      http://*|https://*|mailto:*|\#*|javascript:*) continue ;;
    esac
    fragment=""
    case "$href" in *\#*) fragment=${href#*#} ;; esac
    href=${href%%#*}
    href=${href%%\?*}
    [ -z "$href" ] && continue
    if [[ "$href" == /* ]]; then
      if [[ "$href" != "$base/"* && "$href" != "$base" ]]; then
        echo "link outside the site base $base in ${page#"$public"/}: $href" >&2
        echo x >> "$broken"
        continue
      fi
      target="$public${href#"$base"}"
    else
      target="$dir/$href"
    fi
    if [ -d "$target" ]; then
      target="$target/index.html"
    fi
    if [ ! -f "$target" ]; then
      echo "broken link in ${page#"$public"/}: $href" >&2
      echo x >> "$broken"
    elif [ -n "$fragment" ] && ! grep -qE "id=\"?$fragment\"?[ >]" "$target"; then
      echo "broken fragment in ${page#"$public"/}: $href#$fragment" >&2
      echo x >> "$broken"
    fi
  done
done < <(find "$public" -name index.html -print0)

if [ -s "$broken" ]; then
  echo "check-links: $(wc -l < "$broken" | tr -d ' ') broken link(s)" >&2
  exit 1
fi
echo "check-links: ok ($(find "$public" -name index.html | wc -l | tr -d ' ') pages)"
