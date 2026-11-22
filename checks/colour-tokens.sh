#!/usr/bin/env bash
# Fail when web/ source draws a colour other than through a role token: a colour value, color-mix() or a
# palette token anywhere outside web/shared/styles/tokens.css. See docs/architecture.md#colours.
set -uo pipefail
# One collation for sort and comm, so the undefined-token comparison misses nothing.
export LC_ALL=C

cd "$(dirname "$0")/.." || exit 1

tokens=web/shared/styles/tokens.css

# Each allowed literal: the file, the literal, and why it is not a colour the site draws.
allowed=(
  "web/admin/src/components/EndpointSheet.tsx|#ff4500|a Discord embed's default colour: data sent to Discord"
)

mapfile -t palette < <(sed -n '/^ *\/\* Palette/,/^ *\/\* Roles/p' "$tokens" 2>/dev/null | grep -oE '^ *--[a-z0-9-]+' | tr -d ' ' | sed 's/^--//')
if [ ${#palette[@]} -eq 0 ]; then
  echo "colour-tokens: found no palette section (/* Palette … */ to /* Roles … */) in $tokens" >&2
  exit 1
fi
names=$(IFS='|'; echo "${palette[*]}")

# A hex colour has 3, 4, 6 or 8 digits. Outside CSS one is always quoted, so a bare "#135401" there is a number.
digits='#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})([^0-9a-zA-Z_-]|$)'
hex_css='(^|[[:space:]:'"'"'"(,=])'"$digits"
hex_quoted='(^|[:'"'"'"(,=])'"$digits"
func='(^|[^a-zA-Z0-9_.-])(rgba?|hsla?|hwb|lab|lch|oklab|oklch|color-mix|color)\('
token='(var\([[:space:]]*|['"'"'"])--('"$names"')([^a-zA-Z0-9_-]|$)'
colour_props='(color|background|background-color|background-image|border|border-[a-z-]+|outline|outline-color|fill|stroke|box-shadow|text-shadow|caret-color|accent-color|column-rule|text-decoration|text-decoration-color|scrollbar-color)'
named='aliceblue|antiquewhite|aqua|aquamarine|azure|beige|bisque|black|blanchedalmond|blue|blueviolet|brown|burlywood|cadetblue|chartreuse|chocolate|coral|cornflowerblue|cornsilk|crimson|cyan|darkblue|darkcyan|darkgoldenrod|darkgray|darkgreen|darkgrey|darkkhaki|darkmagenta|darkolivegreen|darkorange|darkorchid|darkred|darksalmon|darkseagreen|darkslateblue|darkslategray|darkslategrey|darkturquoise|darkviolet|deeppink|deepskyblue|dimgray|dimgrey|dodgerblue|firebrick|floralwhite|forestgreen|fuchsia|gainsboro|ghostwhite|gold|goldenrod|gray|green|greenyellow|grey|honeydew|hotpink|indianred|indigo|ivory|khaki|lavender|lavenderblush|lawngreen|lemonchiffon|lightblue|lightcoral|lightcyan|lightgoldenrodyellow|lightgray|lightgreen|lightgrey|lightpink|lightsalmon|lightseagreen|lightskyblue|lightslategray|lightslategrey|lightsteelblue|lightyellow|lime|limegreen|linen|magenta|maroon|mediumaquamarine|mediumblue|mediumorchid|mediumpurple|mediumseagreen|mediumslateblue|mediumspringgreen|mediumturquoise|mediumvioletred|midnightblue|mintcream|mistyrose|moccasin|navajowhite|navy|oldlace|olive|olivedrab|orange|orangered|orchid|palegoldenrod|palegreen|paleturquoise|palevioletred|papayawhip|peachpuff|peru|pink|plum|powderblue|purple|rebeccapurple|red|rosybrown|royalblue|saddlebrown|salmon|sandybrown|seagreen|seashell|sienna|silver|skyblue|slateblue|slategray|slategrey|snow|springgreen|steelblue|tan|teal|thistle|tomato|turquoise|violet|wheat|white|whitesmoke|yellow|yellowgreen'
named_decl='(^|[;{[:space:]])'"$colour_props"'[[:space:]]*:[^;}]*(^|[^a-zA-Z0-9_-])('"$named"')([^a-zA-Z0-9_-]|$)'

mapfile -t files < <(git ls-files -co --exclude-standard -- 'web/*/src/**' 'web/shared/**' |
  grep -E '\.(css|astro|ts|tsx|mdx)$' | grep -vE '\.test\.tsx?$|^web/shared/api/|^web/shared/styles/tokens\.css$')

if [ ${#files[@]} -eq 0 ]; then
  echo "colour-tokens: found no web source to check" >&2
  exit 1
fi

found=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  # Custom property names and comments are dropped before looking for colour words, so --gold-wash is not "gold".
  hits=$({
    case $f in *.css | *.astro) hex=$hex_css ;; *) hex=$hex_quoted ;; esac
    grep -nE -e "$hex" -e "$func" -e "$token" "$f"
    case $f in *.css | *.astro) sed -E 's#/\*.*\*/##g; s/--[a-zA-Z0-9_-]+//g' "$f" | grep -niE "$named_decl" ;; esac
  } | sort -t: -k1,1n -u)
  [ -n "$hits" ] || continue
  for a in "${allowed[@]}"; do
    IFS='|' read -r path literal _ <<<"$a"
    [ "$path" = "$f" ] && hits=$(printf '%s\n' "$hits" | grep -vF -- "$literal")
  done
  [ -n "$hits" ] || continue
  printf '%s\n' "$hits" | sed "s|^|  $f:|" >&2
  found=$((found + 1))
done

# A token that no file defines draws nothing, silently (a renamed or removed token): one used in var() without
# a fallback, or named in quotes by script (a chart reads its colours by name).
mapfile -t scripts < <(printf '%s\n' "${files[@]}" | grep -E '\.tsx?$')
mapfile -t undefined < <(comm -23 \
  <({
    cat "${files[@]}" | grep -oE 'var\([[:space:]]*--[a-zA-Z0-9_-]+[[:space:]]*\)'
    [ ${#scripts[@]} -eq 0 ] || cat "${scripts[@]}" | grep -oE "['\"]--[a-zA-Z0-9_-]+['\"]"
  } | grep -oE -- '--[a-zA-Z0-9_-]+' | sort -u) \
  <(cat "${files[@]}" "$tokens" | grep -oE -- "--[a-zA-Z0-9_-]+[[:space:]]*:|setProperty\\('--[a-zA-Z0-9_-]+" | grep -oE -- '--[a-zA-Z0-9_-]+' | sort -u))
for name in "${undefined[@]}"; do
  grep -nE "var\([[:space:]]*${name}[[:space:]]*\)|['\"]${name}['\"]" "${files[@]}" | sed "s|^|  undefined $name: |" >&2
  found=$((found + 1))
done

if [ "$found" -gt 0 ]; then
  echo "colour-tokens: $found file(s) or token(s) draw a colour that is not a defined role token." >&2
  echo "  Use a role from $tokens (or add one there, pointing at the palette)." >&2
  exit 1
fi
echo "  colour-tokens: ${#files[@]} file(s), every colour from a role token"
