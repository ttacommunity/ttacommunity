#!/usr/bin/env bash
# Tests checks/colour-tokens.sh in throwaway git repos: each case writes one source file and expects the check's exit.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start colour-tokens.test

tokens=':root {
  /* Palette: colours by look, used only in this file. */
  --gold: #e5a93c;
  --gold-light: #ffde8a;
  /* Roles: what a colour is for. */
  --text-accent: var(--gold);
  --gold-standard-edge: var(--gold-light);
}'

# case: name, the file to write, its one line, the exit wanted.
cases=(
  "a role in CSS|web/site/src/a.css|.a { color: var(--text-accent); }|0"
  "a role whose name starts like a palette colour|web/site/src/a.css|.a { border-color: var(--gold-standard-edge); }|0"
  "a hex colour in CSS|web/site/src/a.css|.a { color: #e5a93c; }|1"
  "a short hex colour in CSS|web/site/src/a.css|.a { color: #fff; }|1"
  "rgba in CSS|web/site/src/a.css|.a { background: rgba(0, 0, 0, 0.5); }|1"
  "hsl in CSS|web/site/src/a.css|.a { color: hsl(40 80% 50%); }|1"
  "color-mix in CSS|web/site/src/a.css|.a { color: color-mix(in srgb, var(--text-accent) 50%, transparent); }|1"
  "a named colour in CSS|web/site/src/a.css|.a { color: white; }|1"
  "a named colour in a shadow|web/shared/styles/components/b.css|.b { box-shadow: 0 0 4px black; }|1"
  "a colour word in a comment|web/site/src/a.css|/* a gold edge on the left */|0"
  "transparent and currentColor|web/site/src/a.css|.a { border-color: transparent; fill: currentColor; }|0"
  "a palette token in CSS|web/site/src/a.css|.a { color: var(--gold); }|1"
  "a longer palette token in CSS|web/site/src/a.css|.a { color: var(--gold-light); }|1"
  "a palette token in a style block|web/site/src/pages/p.astro|<style>.p { color: var(--gold); }</style>|1"
  "a hex string in TypeScript|web/site/src/lib/c.ts|export const c = '#a855f7';|1"
  "rgba in TypeScript|web/site/src/lib/c.ts|export const c = 'rgba(56, 189, 248, 0.12)';|1"
  "a palette token read in TypeScript|web/site/src/lib/c.ts|const g = read('--gold');|1"
  "a role read in TypeScript|web/site/src/lib/c.ts|const g = read('--text-accent');|0"
  "a variable named for a colour in TypeScript|web/site/src/lib/c.ts|const t = { color: gold, border: white };|0"
  "a link to an anchor in MDX|web/site/src/content/faq/a.mdx|<a href=\"#whr\">Whole-History Rating</a>|0"
  "an HTML entity in MDX|web/site/src/content/faq/a.mdx|<strong>&#128172; Hover</strong>|0"
  "a hex colour in MDX|web/site/src/content/faq/a.mdx|<span data-c=\"#ff80ab\">x</span>|1"
  "a match number in MDX prose|web/site/src/content/faq/a.mdx|<strong>Match #135401 and #33943</strong>|0"
  "a match number in TSX text|web/site/src/components/A.tsx|<p>Match #135401</p>|0"
  "a hex colour after a space in a style block|web/site/src/pages/p.astro|<style>.p { border: 1px solid #fff; }</style>|1"
  "a test file is not checked|web/site/src/lib/c.test.ts|expect(c).toBe('#a855f7');|0"
  "the generated API is not checked|web/shared/api/types.ts|// '#123456'|0"
  "the allowed Discord embed default|web/admin/src/components/EndpointSheet.tsx|thread_id: '', colour: '#ff4500', text: 'full',|0"
  "the same literal elsewhere is refused|web/admin/src/components/Other.tsx|colour: '#ff4500',|1"
  "a token no file defines|web/site/src/a.css|.a { color: var(--accent-gold); }|1"
  "a token defined in the same file|web/site/src/a.css|.a { --row: var(--text-accent); background: var(--row); }|0"
  "a token with a fallback|web/site/src/a.css|.a { top: var(--header-height, 68px); }|0"
  "a token named in TypeScript that no file defines|web/site/src/lib/c.ts|const band = '--band-nowhere';|1"
  "a token named in TypeScript that tokens define|web/site/src/lib/c.ts|const g = read('--text-accent');|0"
)

# repo makes a repo with the check and the tokens file, and prints its path.
repo() {
  local dir=$work/$1
  mkdir -p "$dir/checks" "$dir/web/shared/styles"
  cp "$root/checks/colour-tokens.sh" "$dir/checks/"
  printf '%s\n' "$tokens" >"$dir/web/shared/styles/tokens.css"
  mkdir -p "$dir/web/site/src" && printf '.clean { color: var(--text-accent); }\n' >"$dir/web/site/src/clean.css"
  git -C "$dir" init -q
  echo "$dir"
}

n=0
for c in "${cases[@]}"; do
  IFS='|' read -r name file line want <<<"$c"
  n=$((n + 1))
  dir=$(repo "case$n")
  mkdir -p "$dir/$(dirname "$file")"
  printf '%s\n' "$line" >"$dir/$file"
  bash "$dir/checks/colour-tokens.sh" >/dev/null 2>&1
  got=$?
  [ "$got" = "$want" ] || fail "$name: got exit $got, want $want ($file: $line)"
done

dir=$(repo nopalette)
printf ':root { --text-accent: #e5a93c; }\n' >"$dir/web/shared/styles/tokens.css"
bash "$dir/checks/colour-tokens.sh" >/dev/null 2>&1 && fail "a tokens file with no palette section: got exit 0, want 1"

# A token set from script: one file sets it, another uses it.
dir=$(repo fromscript)
printf "el.style.setProperty('--bar-top', '10%%');\n" >"$dir/web/site/src/b.tsx"
printf '.a { top: var(--bar-top); }\n' >"$dir/web/site/src/a.css"
bash "$dir/checks/colour-tokens.sh" >/dev/null 2>&1 || fail "a token set from script: got exit 1, want 0"

testlib_finish
