#!/usr/bin/env bash
# Fail on a comment block over 8 lines, or inline commentary over 20% overall.
# DOC is comment at column 0 (file and declaration comments), INLINE is
# indented commentary. Tests, generated code and upstream's engine are exempt.
#
# Usage: checks/comment-density.sh            gate
#        checks/comment-density.sh --report   print every file, fail on nothing
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

MAX_RUN=8
MAX_INLINE=20
REPORT=0
[ "${1:-}" = "--report" ] && REPORT=1

# measure prints "<comments> <non-blank lines> <longest run> <run start> <inline>".
# $2 is the comment style: slash (//, /* */, <!-- -->) or hash (#).
measure() {
  awk -v style="$2" '
    /^[[:space:]]*$/ { next }
    NR == 1 && /^#!/ { next }
    { lines++; c = 0 }
    style == "hash" && /^[[:space:]]*#/ { c = 1 }
    style == "slash" && inblock { c = 1; if ($0 ~ /(\*\/|-->)/) inblock = 0 }
    style == "slash" && !c && /^[[:space:]]*(\/\/|\/\*|<!--)/ {
      c = 1
      if ($0 ~ /^[[:space:]]*\/\*/ && $0 !~ /\*\//) inblock = 1
      if ($0 ~ /^[[:space:]]*<!--/ && $0 !~ /-->/) inblock = 1
    }
    c { comments++; if ($0 ~ /^[[:space:]]+/) inline++; run++
        if (run > maxrun) { maxrun = run; at = NR - run + 1 }; next }
    { run = 0 }
    END { printf "%d %d %d %d %d\n", comments+0, lines+0, maxrun+0, at+0, inline+0 }
  ' "$1"
}

mapfile -t files < <(git ls-files -co --exclude-standard |
  grep -E '\.(go|ts|tsx|js|mjs|astro|py|sh)$' |
  grep -vE '_test\.go$|\.test\.tsx?$|^engine/tests/|^engine/src/|^web/shared/api/|(^|/)node_modules/')

found=0; over=0; total=0; total_c=0; total_i=0
for f in "${files[@]}"; do
  [ -f "$f" ] || continue
  case $f in *.py|*.sh) style="hash" ;; *) style="slash" ;; esac
  read -r c l run at inl < <(measure "$f" "$style")
  [ "$l" -eq 0 ] && continue
  found=$((found + 1)); total=$((total + l)); total_c=$((total_c + c)); total_i=$((total_i + inl))
  if [ "$REPORT" = 1 ]; then
    printf '  doc+inline %3d%%  inline %3d%%  run %-3d %s\n' $((c * 100 / l)) $((inl * 100 / l)) "$run" "$f"
  elif [ "$run" -gt "$MAX_RUN" ]; then
    printf '  %s:%d  %d consecutive comment lines (max %d)\n' "$f" "$at" "$run" "$MAX_RUN" >&2
    over=$((over + 1))
  fi
done

if [ "$found" -eq 0 ]; then
  echo "comment-density: found no files to check" >&2
  exit 1
fi
pct=$((total_c * 100 / total)); ipct=$((total_i * 100 / total))
summary="$found files, ${pct}% comment, ${ipct}% inline (max ${MAX_INLINE}%), longest block max $MAX_RUN"
[ "$REPORT" = 1 ] && { echo "comment-density: $summary"; exit 0; }

fail=0
[ "$over" -gt 0 ] && { echo "comment-density: $over file(s) carry a block over $MAX_RUN lines." >&2; fail=1; }
[ "$ipct" -gt "$MAX_INLINE" ] && { echo "comment-density: inline commentary is ${ipct}%, over ${MAX_INLINE}%." >&2; fail=1; }
if [ "$fail" = 1 ]; then
  echo "  Say what a file or function does; a one-liner for the rest. Reasoning goes in docs/." >&2
  exit 1
fi
echo "  comment-density: $summary"
