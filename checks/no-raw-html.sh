#!/usr/bin/env bash
# Fail when web/ source renders a string as HTML or carries an inline style.
# Player names come from scraped data, so an HTML sink is an injection path,
# and the CSP allows no inline styles.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

sinks='set:html|dangerouslySetInnerHTML|\.innerHTML|\.outerHTML|insertAdjacentHTML|document\.write'
inline_style='[[:space:]]style=("|\{)'

mapfile -t files < <(git ls-files -co --exclude-standard -- 'web/*/src/**' 'web/shared/**' |
  grep -E '\.(astro|ts|tsx|js|jsx|mdx)$' | grep -vE '\.test\.tsx?$')

if [ ${#files[@]} -eq 0 ]; then
  echo "no-raw-html: found no web source to check" >&2
  exit 1
fi

found=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  if hits=$(grep -nE -e "$sinks" -e "$inline_style" "$f"); then
    printf '%s\n' "$hits" | sed "s|^|  $f:|" >&2
    found=$((found + 1))
  fi
done

if [ "$found" -gt 0 ]; then
  echo "no-raw-html: $found file(s) render raw HTML or use an inline style." >&2
  echo "  Render data as text, and add a token or a component class instead of style=." >&2
  exit 1
fi
echo "  no-raw-html: ${#files[@]} file(s), no HTML sinks or inline styles"
