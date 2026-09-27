#!/usr/bin/env bash
# Run everything CI runs, in CI's order. CI calls this script unchanged.
# Ends by listing what it did not cover.
#
# Usage: scripts/ci.sh              every step
#        scripts/ci.sh --tests-only only the tests (what scripts/test.sh runs)
#        scripts/ci.sh --slow       also the engine's slow tests (on in CI on main)
#        scripts/ci.sh --only <area> one area: go, engine, web or checks (one CI job each)
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
ROOT=$PWD

TESTS_ONLY=0
SLOW=0
ONLY=""
[ "${CI_COMMIT_BRANCH:-}" = main ] && SLOW=1
while [ $# -gt 0 ]; do
  case $1 in
    --tests-only) TESTS_ONLY=1 ;;
    --slow) SLOW=1 ;;
    --only)
      ONLY=${2:-}
      shift
      case $ONLY in
        go|engine|web|checks) ;;
        *) echo "--only takes go, engine, web or checks" >&2; exit 64 ;;
      esac
      ;;
    *) echo "unknown argument: $1" >&2; exit 64 ;;
  esac
  shift
done

PG_IMAGE=docker.io/library/postgres:18-alpine@sha256:77f585114c32fbca283dc835b0596f4e52b51b4c6662d7810b2f4084f60a1873
PG_NAME=ttac-ci-pg
failures=0
not_covered=()

step() { printf '\n\033[1m== %s\033[0m\n' "$1"; }
fail() { echo "FAILED: $1" >&2; failures=$((failures + 1)); }
run() { "$@" || fail "$*"; }
need() {
  command -v "$1" >/dev/null 2>&1 && return 0
  fail "$1 is not installed (needed for: $2)"
  return 1
}
full() { [ "$TESTS_ONLY" = 0 ]; }
area() { [ -z "$ONLY" ] || [ "$ONLY" = "$1" ]; }
stop_pg() { command -v podman >/dev/null 2>&1 && podman rm -f "$PG_NAME" >/dev/null 2>&1; }

# start_test_db sets TEST_DATABASE_URL, starting a throwaway Postgres if CI has not.
start_test_db() {
  [ -n "${TEST_DATABASE_URL:-}" ] && return 0
  need podman "a throwaway Postgres for integration tests" || return 1
  stop_pg
  podman run -d --name "$PG_NAME" -e POSTGRES_USER=ttac -e POSTGRES_PASSWORD=local-only \
    -e POSTGRES_DB=ttac_test -p 127.0.0.1:55439:5432 "$PG_IMAGE" >/dev/null || return 1
  for _ in $(seq 1 60); do
    if podman exec "$PG_NAME" pg_isready -U ttac -d ttac_test >/dev/null 2>&1; then
      export TEST_DATABASE_URL='postgres://ttac:local-only@127.0.0.1:55439/ttac_test?sslmode=disable'
      return 0
    fi
    sleep 1
  done
  echo "Postgres never became ready" >&2
  return 1
}
trap stop_pg EXIT

if area go; then
step "go"
if need go "the backend"; then
  cd "$ROOT/backend" || exit 1
  if full; then
    unformatted=$(gofmt -l .)
    [ -z "$unformatted" ] || fail "not gofmt-clean: $unformatted"
    run go vet ./...
    run go vet -tags=integration ./...
  fi
  run go test -race -count=1 ./...
  if start_test_db; then
    run go test -tags=integration -count=1 ./...
  else
    fail "integration tests: no database"
  fi
  full && run go build -o /dev/null ./...
  cd "$ROOT" || exit 1
fi

if full; then
  step "spec"
  # Routes against the spec: TestRoutesMatchTheSpec, in the Go tests above.
  # The generated Go must be what the committed spec produces.
  (cd backend && go generate ./internal/httpapi) || fail "generating Go from the spec"
  git diff --quiet -- backend/internal/httpapi/api.gen.go ||
    fail "backend/internal/httpapi/api.gen.go differs from the spec: run go generate ./internal/httpapi and commit it"
fi
fi

if area engine; then
step "engine"
if need uv "the engine"; then
  cd "$ROOT/engine" || exit 1
  run uv sync --frozen --quiet
  if full; then
    run uv run --frozen ruff check .
    run uv run --frozen ruff format --check .
  fi
  # No skip without a database: the tests fail on a missing TEST_DATABASE_URL.
  start_test_db || fail "engine tests: no database"
  if [ "$SLOW" = 1 ]; then
    run uv run --frozen pytest -q
  else
    run uv run --frozen pytest -q -m "not slow"
    not_covered+=("engine: slow tests (run with --slow; CI runs them on main)")
  fi
  cd "$ROOT" || exit 1
fi
fi

if area web; then
step "web"
if need corepack "the web workspace"; then
  cd "$ROOT/web" || exit 1
  export COREPACK_ENABLE_DOWNLOAD_PROMPT=0
  # The workspace scripts call pnpm by name, so it must be on PATH, not only `corepack pnpm`.
  shims=$(mktemp -d)
  corepack enable --install-directory "$shims" pnpm && export PATH="$shims:$PATH"
  run corepack pnpm install --frozen-lockfile --silent
  if full; then
    run corepack pnpm --filter @ttac/shared gen:api
    git diff --quiet -- shared/api ||
      fail "web/shared/api differs from the spec: run pnpm --filter @ttac/shared gen:api and commit it"
    run corepack pnpm lint
    run corepack pnpm check
  fi
  run corepack pnpm test
  if full; then
    run corepack pnpm build
    for site in site admin; do
      [ -f "$site/dist/index.html" ] || fail "$site: the build produced no dist/index.html"
      [ -f "$site/dist/_headers" ] || fail "$site: _headers did not reach the output root"
      grep -q '^  Content-Security-Policy:' "$site/dist/_headers" 2>/dev/null || fail "$site: _headers sets no CSP"
      run node "$ROOT/checks/csp-inline.mjs" "$site/dist"
      want=$(cat "$site/.node-version" 2>/dev/null)
      have=$(node --version | sed 's/^v\([0-9]*\).*/\1/')
      [ "$want" = "$have" ] || fail "$site/.node-version says '$want' but this is Node $have"
    done
  fi
  cd "$ROOT" || exit 1
fi
fi

if full && area checks; then
  step "checks"
  run bash checks/comment-density.sh
  run bash checks/no-raw-html.sh
  run bash checks/secret-scan.sh --all
  run bash checks/verify-digests.sh

  step "shell"
  if need shellcheck "the shell scripts"; then
    run shellcheck scripts/*.sh checks/*.sh .githooks/*
  fi

  if [ -z "${CI:-}" ]; then
    step "git hooks"
    if [ "$(git config core.hooksPath)" = .githooks ]; then
      echo "  enabled"
    else
      fail "git hooks are not enabled: git config core.hooksPath .githooks"
    fi
  fi
fi

[ -n "$ONLY" ] && not_covered+=("every area but '$ONLY': each runs in its own CI job")
printf '\n\033[1mNot covered here:\033[0m\n'
for item in "${not_covered[@]}" \
  "Cloudflare's own Workers build (check its build log after a merge to main)" \
  "container images: built by the CI image jobs, not by this script" \
  "secret_detection and semgrep-sast: GitLab templates, CI only"; do
  echo "  - $item"
done

if [ "$failures" -gt 0 ]; then
  printf '\n\033[31m%d step(s) FAILED\033[0m\n' "$failures"
  exit 1
fi
printf '\n\033[32mall steps passed\033[0m\n'
