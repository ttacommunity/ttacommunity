#!/usr/bin/env bash
# Tests checks/shared-look.mjs in throwaway trees: each case writes one page file beside a shared stylesheet and expects a warning or none, and the exit always 0.
set -uo pipefail

# shellcheck source=checks/testlib.sh
. "$(dirname "$0")/testlib.sh"
testlib_start shared-look.test

shared='.badge { padding: 0.15rem 0.45rem; } .view-bar { display: flex; } .chip.active { color: red; }'

# case: name, the page file, its one line, "warn" or "quiet".
cases=(
  "a page recolours a shared class|site/src/components/A.css|.badge.badge-x { color: var(--text-muted); }|warn"
  "a page sizes a shared class inside its own|site/src/components/A.css|.a .badge { font-size: 0.85rem; }|warn"
  "a page places a shared class|site/src/components/A.css|.a .badge { margin-left: 0.3rem; width: 4rem; }|quiet"
  "a page styles a label holding a shared piece|site/src/components/A.css|.a label:has(> .badge) { color: var(--text-soft); }|quiet"
  "a page styles its own class|site/src/components/A.css|.a-thing { color: var(--text-soft); padding: 1rem; }|quiet"
  "a shared state word is not a piece|site/src/components/A.css|.a.active { color: var(--text-soft); }|quiet"
  "a page restyles a field by its tag|admin/src/components/B.css|.b input { font: inherit; }|warn"
  "a page restyles a button by its tag|admin/src/components/B.css|.b button:disabled { opacity: 0.5; }|warn"
  "inside a media block|site/src/components/A.css|@media (max-width: 768px) { .badge.badge-x { padding: 0; } }|warn"
  "in an Astro style block|site/src/pages/p.astro|<style>.badge.badge-p { color: var(--text-muted); }</style>|warn"
  "a kept rule|site/src/components/ViewBar.css|.view-bar { padding: 0.5rem; }|quiet"
)

# tree makes a web root with the shared stylesheet, and prints its path.
tree() {
  local dir=$work/$1
  mkdir -p "$dir/web/shared/styles/components" "$dir/web/site/src" "$dir/web/admin/src"
  printf '%s\n' "$shared" >"$dir/web/shared/styles/components/shared.css"
  echo "$dir/web"
}

n=0
for c in "${cases[@]}"; do
  IFS='|' read -r name file line want <<<"$c"
  n=$((n + 1))
  web=$(tree "case$n")
  mkdir -p "$web/$(dirname "$file")"
  printf '%s\n' "$line" >"$web/$file"
  out=$(node "$root/checks/shared-look.mjs" "$web" 2>&1)
  code=$?
  [ "$code" = 0 ] || fail "$name: exit $code, want 0 (a warning never fails)"
  if [ "$want" = warn ]; then
    grep -qF "$file:1:" <<<"$out" || fail "$name: no warning for $file ($line); got: $out"
  else
    grep -q 'restyles' <<<"$out" && fail "$name: warned for $file ($line): $out"
  fi
done

testlib_finish
