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
[ "${GITHUB_EVENT_NAME:-}" = push ] && [ "${GITHUB_REF_NAME:-}" = main ] && SLOW=1
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

# dev_test_db sets TEST_DATABASE_URL to a ttac_test database on .env's local dev server, as scripts\test.ps1
# does, creating its ttac login the first time; it fails when that server is not running.
dev_test_db() {
  [ -z "${CI:-}" ] && [ -f "$ROOT/.env" ] && command -v psql >/dev/null 2>&1 || return 1
  local dev host
  # shellcheck disable=SC1091
  dev=$(set -a; . "$ROOT/.env"; printf '%s' "${RATINGS_DATABASE_URL:-}")
  host=$(printf '%s' "$dev" | sed -n 's|^postgres://[^@]*@\([^/]*\)/.*|\1|p')
  [ -n "$host" ] && psql "$dev" -qAtc 'SELECT 1' >/dev/null 2>&1 || return 1
  psql "$dev" -qAt -v ON_ERROR_STOP=1 >/dev/null <<'SQL' || return 1
SELECT format('CREATE ROLE ttac LOGIN SUPERUSER PASSWORD %L', 'local-only') WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ttac') \gexec
SELECT 'CREATE DATABASE ttac_test OWNER ttac' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'ttac_test') \gexec
SQL
  export TEST_DATABASE_URL="postgres://ttac:local-only@$host/ttac_test?sslmode=disable"
  echo "  tests use the local dev server's ttac_test database"
}

# start_test_db sets TEST_DATABASE_URL: CI's, else the local dev server's, else a throwaway Postgres.
start_test_db() {
  [ -n "${TEST_DATABASE_URL:-}" ] && return 0
  dev_test_db && return 0
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
  # The Access OIDC worker has its own npm lockfile; wrangler is dev-only.
  cd "$ROOT/infra/access-oidc" || exit 1
  run npm ci --omit=dev --ignore-scripts --no-audit --no-fund --silent
  run npm test
  # The build notifier's tests need nothing installed.
  cd "$ROOT/infra/build-notifier" || exit 1
  run npm test
  cd "$ROOT" || exit 1
fi
fi

if full && area checks; then
  step "checks"
  run bash checks/comment-density.sh
  run bash checks/no-raw-html.sh
  run bash checks/secret-scan.sh --all
  if need kustomize "deploy/'s env values" && need yq "deploy/'s env values"; then
    run bash checks/deploy-env-strings.test.sh
    run bash checks/deploy-env-strings.sh
  fi
  run bash checks/deploy-current.test.sh
  if need jq "the engine's golden hashes"; then
    run bash checks/engine-golden.test.sh
    run bash checks/engine-golden.sh
  fi
  # Locally, red means something to fix here: a gh login that cannot read our packages leaves our images to CI.
  if [ -z "${CI:-}" ] && [ -z "${GITHUB_TOKEN:-}" ] && ! gh auth status 2>&1 | grep -qE 'read:packages|write:packages'; then
    run bash checks/verify-digests.sh --skip-own
    not_covered+=("our own GHCR images' digests: your gh login cannot read them (gh auth refresh -s read:packages); CI checks them")
  else
    run bash checks/verify-digests.sh
  fi
  # dev deploys from deploy/dev. main's pins are checked too, but not while dev's MR to main is a draft.
  target=${GITHUB_BASE_REF:-${GITHUB_REF_NAME:-$(git rev-parse --abbrev-ref HEAD)}}
  source=${GITHUB_HEAD_REF:-$target}
  if [ -z "${CI:-}" ] && [ "$source" != main ] && [ "$source" != dev ]; then
    # A branch's images are built by CI (the images label), so its pins cannot be brought up to date here.
    not_covered+=("deploy/ pins built from this branch's code: CI checks them; pin the images the images label builds")
  else
    if [ "$source" = dev ] || [ "$target" = dev ]; then
      run bash checks/deploy-current.sh deploy/dev
    fi
    if [ "$target" != dev ] && ! { [ "$source" = dev ] && [ "${PR_DRAFT:-}" = true ]; }; then
      run bash checks/deploy-current.sh
    fi
  fi

  step "workflows"
  if need actionlint "the GitHub workflows" && need zizmor "the GitHub workflows' security"; then
    run actionlint .github/workflows/*.yml
    run zizmor --quiet .github/workflows
  fi

  step "powershell"
  # The Windows twins run only on a manual Windows job; parsing them here is the cheap net.
  if need pwsh "the PowerShell scripts"; then
    # Parsing needs no locale data, and the CI image has no libicu.
    run env DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1 pwsh -NoProfile -File checks/ps-parse.ps1
  fi

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
  "gitleaks and semgrep: their own CI jobs (secrets, sast)"; do
  echo "  - $item"
done

if [ "$failures" -gt 0 ]; then
  printf '\n\033[31m%d step(s) FAILED\033[0m\n' "$failures"
  exit 1
fi
printf '\n\033[32mall steps passed\033[0m\n'
