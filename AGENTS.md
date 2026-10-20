# AGENTS.md: working on ttacommunity

Instructions for coding agents and the people who use them. Read it before
changing anything. Commands are given for **Windows (PowerShell)** first and
Linux/macOS (bash) beside them.

## What this is

A community platform for the board game **Through the Ages**, run on one Go
backend:

- **Ratings**: a rebuild of DANeo's Glicko community ratings site, with a
  public API. The rating math is DANeo's Python code, in `engine/`
- **Tournaments**: players in official CGE tournament matches agree start
  times; reminders go out and games start through CGE's API
- **TTA Pulse**: a blitz queue that pairs players by rating

| Path | What |
|---|---|
| `backend/` | Go: `cmd/api` (JSON API), `cmd/load` (the data load job), `internal/`, `migrations/` |
| `backend/api/openapi.yaml` | The API contract. The TS types are generated from it |
| `engine/` | Python (uv): the rating engine. Runs as a job, not a server |
| `web/` | Astro + React + TypeScript (pnpm): `site` (public), `admin`, `shared` |
| `infra/access-oidc/` | The Worker that signs admins in with Discord |
| `deploy/` | Kubernetes manifests for the home cluster |
| `scripts/` | Everything you run: dev, tests, CI. `.ps1` twins for Windows |
| `checks/` | Repo checks CI runs (comment density, secrets, digests…) |

## Getting started

```powershell
# Windows, once: allow local scripts, then install everything
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
scripts\install.ps1                         # mise, Go, Node, uv, PostgreSQL 18 (user install; -Service for a Windows service), local databases
scripts\ratings-db.ps1 ..\ttac-ratings-data\baseline   # load the data
scripts\dev.ps1                             # api :8080, site :4321, admin :4322
scripts\test.ps1                            # every test
scripts\ci.ps1                              # everything CI runs that Windows can
```

```bash
# Linux / macOS (Docker or Podman for Postgres)
mise install
scripts/ratings-db.sh --native ../ttac-ratings-data/baseline
scripts/dev.sh --native
scripts/test.sh
scripts/ci.sh            # everything CI runs, in CI's order
```

- **Tool versions live in `mise.toml`** (Go, Node, uv, …) and pnpm's in
  `web/package.json`. CI installs the same. Don't install a different version
  by hand
- `.env` is created from `.env.example` on first run. Its values are for
  local use only. `scripts/dev.sh` and `scripts\dev.ps1` warn when `.env`
  lacks a setting added to `.env.example` since: copy it across. In PowerShell, always dot-source `. scripts\env.ps1` to load
  it and add mise shims (`%LOCALAPPDATA%\mise\shims`) and Postgres tools to PATH
- **Postgres startup**: `scripts\env.ps1` starts Postgres if stopped. After a
  restart or crash recovery, wait for `pg_isready` before connecting if queries
  report "the database system is starting up"
- **Data is never in this repo.** The maintainers keep a baseline export (every
  input table, no personal data) in a private repo in the ttacommunity
  organization: `ttac-ratings-data` (`git@github.com:ttacommunity/ttac-ratings-data.git`).
  Clone it beside this repository (`..\ttac-ratings-data`). Compute ratings after
  loading: see README, "Compute ratings"
- **Admin in local dev**: the dev scripts run admin at http://127.0.0.1:4322.
  With `ADMIN_DEV_USER` set in `.env`, the api signs a request with no Access
  token in as that admin, and a token `dev-<id>` as admin `<id>` (to try
  other roles; the id must be in `admins`). The api refuses to start with it
  unless `ADMIN_ORIGIN` is `http://127.0.0.1:…`, `API_ADDR` is
  `127.0.0.1:…`, and it is not in Kubernetes. Add the admin once:
  `go run ./cmd/load -add-admin 100000000000000001 -name "Local admin" -role admin`.
  An older `.env` may say `API_ADDR=:8080`: change it to `127.0.0.1:8080`
- **Player sign-in in local dev**: with `DISCORD_DEV_USER` set in `.env`,
  "Sign in with Discord" on the site signs you in at once as that invented
  Discord account, with no Discord app. The api refuses it on the same
  terms as `ADMIN_DEV_USER`, and when `DISCORD_CLIENT_ID` is set
- **A local database from before 2026-10-06** (schema numbered past 1) does
  not take the new schema: rebuild it from the baseline (README, "Rebuild a
  database from scratch")
- **On a machine that is also a cluster node**, keep big local databases
  off its disk: a full restore there can make Kubernetes evict the cluster's
  own database. Remove throwaway containers with `podman rm -f -v <name>`,
  never a host-wide `prune`
- The Windows scripts are twins of the bash ones. **Change one, change the
  other.** CI only parses the `.ps1` files on Linux; the `windows` workflow
  runs them for real when started by hand

## Related repositories

Both private to the `ttacommunity` organisation; ask a maintainer for access.

- **[`ttac-ratings-data`](https://github.com/ttacommunity/ttac-ratings-data)**:
  the data a local database is restored from. Clone it beside this
  repository (`../ttac-ratings-data`)
- **[`cluster-access`](https://github.com/ttacommunity/cluster-access)**: how
  a maintainer reaches the home cluster with `kubectl` (tools, sign-in, the
  kubeconfig). Follow it rather than copying its steps here

## How we work

- **Boring and obvious beats clever.** A contributor who knows one of Go,
  Python or TypeScript should be able to change a file without learning a
  local framework. No custom DSLs, no code generation beyond the OpenAPI
  types, no clever abstractions
- **A new dependency is a decision.** Say why in the pull request. The
  standard library and what is already used come first
- **Vertical slices**: storage → API → page for one user need, deployable on
  its own. A change that only adds a layer is not a slice. A story brings its
  own migration
- **A rule about players applies wherever a player's name is returned.**
  List those responses from `backend/api/openapi.yaml` and check each,
  boards and records included
- **A new allowed value goes everywhere values are checked**: the spec's
  enum, Go, TypeScript and the database's CHECK constraints (a
  migration). Search for an existing value to find them all
  (`git grep -n "'SGM'"`), and test that the store accepts the new one
- **A move that should change nothing proves it.** Save the API's JSON
  for every case before and after, and compare
- **Ask before building when something is unclear.** A question answered
  mid-implementation is a guess
- **Everything in this repo is public**, including comments and commit
  messages

### Speed

- **Make it as efficient as you reasonably can**, so the project runs
  well wherever it is hosted, small servers included. A host can be
  several times slower than your machine, and production logs every
  request over 1 s. Aim for under 0.1 s a route on your machine; over
  0.5 s is too slow. Past 0.1 s, speed a route up only where it pays
- **Know the sizes**: about 4,100 players, 205,000 games, 577,000 seats
  and 414,000 rated pairs per run. Code that scans every player for each
  player takes seconds here: sort once, use a map, or let Postgres do it
- **A page makes a handful of requests, each once.** Ask the API for what
  the page shows, not for everything to filter in the browser. Data
  several parts of a page need, such as the current run, comes from one
  shared request, never one per component: the browser's cache does not
  merge them everywhere. We fetch without a framework, so the client
  deduplicates requests, caches responses and avoids waterfalls itself,
  as [React's docs](https://react.dev/learn/synchronizing-with-effects#what-are-good-alternatives-to-data-fetching-in-effects)
  advise

### Comments: less comment than code

- A file comment says what the file does, in 1–3 lines. A function comment
  says what it does, accepts and returns (Go: start with the name; Python: a
  docstring; TS: a TSDoc line)
- Anything else is a one-liner, and only for a non-obvious *why* or a warning
- No decision narratives or incident stories in code
- `checks/comment-density.sh` measures it (about 20% inline at most, no block
  over 8 lines)

### Tests: TDD, on everything

- Write the failing test, **watch it fail**, make it pass, refactor. A test
  that passes before the code exists tests nothing
- **Test the failure paths**: the refused input, the expired token, the wrong
  audience
- **Control what you depend on**: inject the clock, config and network
  clients. No `time.Now()` below the wiring layer
- **Tests that need a database fail, never skip, when it is missing**
- **Test what it does, not how it looks.** A test changes only when behaviour
  changes, never when markup, styling, wording or data changes. No render or
  DOM tests of UI components: keep a component thin and put its decisions in
  a `lib/` function with its own test
- **A test checks what it is about**, not a whole struct or every field
- **One function, several cases: one table of cases**, not a test per case
  ([Go's guide](https://go.dev/wiki/TableDrivenTests); pytest
  `parametrize`, Vitest `it.each`). A table holds one kind of case: when
  cases need different fields, write a separate test
- **Hard-to-write tests mean hard-to-use code**: heavy setup or many fakes
  is a reason to change the code. Prefer simple stubs to mocks, test through
  the public interface
  ([anti-patterns](https://quii.gitbook.io/learn-go-with-tests/meta/anti-patterns))
- **Never loosen a test to make it pass.** Dropping an assertion, a
  status code or a "refused requests never reach the store" check
  changes what is tested: name it in the PR
- **Test data is invented.** Never a real Discord id, handle, email or TTA
  username, and never a slice of real match data. Use ids like
  `100000000000000001`
- **Invented names in code too.** No real player as a test name, a
  default or a fallback, however well known
- **Coverage is a prompt to look, never a target.** `scripts/ci.sh` prints
  Go's. The engine's and web's are run by hand when you want them (commands
  under Engine and Web), and add no dependency. A low number is not a
  reason to write a test that asserts nothing

### Safety rules that are not negotiable

- **No credential in git, ever.** Not in code, docs, tests or commit
  messages. The git hooks (`git config core.hooksPath .githooks`; install
  scripts set it) and CI scan for them
- **Never invent a value you did not look up**: an image digest, an id, a
  version. Resolve it with a command. A plausible-looking digest is the worst
  kind of bug
- **Never log personal data**: no Discord id, handle, email, TTA username or
  IP, at any level. Log the matched route (`r.Pattern`), never the path
- **Fail closed.** Missing config, an empty audience, a missing role: refuse
  to start, never default to open. An unknown id, slug or edition is a 404,
  never a page built from defaults
- **Never render data as HTML** in the sites: no `set:html`,
  `dangerouslySetInnerHTML` or `innerHTML` with API data. Player names are
  scraped. A check fails CI on these
- **Verify a control by using it.** A header, a CSP, a permission check: prove
  it with a real request or a test that would fail without it

## Before you push

Run these in order and report each in the PR's **How to test it**. An
agent runs them itself.

1. **Run CI's checks**: `scripts\ci.ps1` (Linux, macOS: `scripts/ci.sh`).
   Red means something to fix before pushing
2. **See each new test fail.** Commit your change, then put the code it
   tests back as on `main` (`git checkout main -- <file>`), run the test
   and see it fail, and restore yours (`git checkout HEAD -- <file>`).
   A test that passes on `main`'s code tests nothing
3. **Time it on the full data.** With the baseline loaded, time each
   route you added or changed:
   `curl.exe -s -o NUL -w "%{time_total}\n" http://127.0.0.1:8080/v1/…`
   (Linux, macOS: `curl -s -o /dev/null -w …`). Aim for under 0.1 s;
   over 0.5 s is too slow (see "Speed"). Open each page you changed and count its API requests
   in the browser's network tab
4. **Read your own diff.** Every file in `git diff main...HEAD --stat` is
   explained in **What it does**. A migration, an `openapi.yaml` change
   and a change to what an existing page shows are always named
5. **Walk the issue's "Done when"** line by line and say how each is met.
   If something is not done, write `Part of #<n>`, not `Closes #<n>`

## Go (`backend/`)

- **The spec comes first.** Change `backend/api/openapi.yaml`, then the
  handler. `go generate ./internal/httpapi` regenerates the Go; the TS types
  are regenerated with `pnpm --filter @ttac/shared gen:api`. Commit both. A
  test checks every route is in the spec and vice versa
- JSON only; no HTML from Go. `net/http` `ServeMux` patterns, no router
  dependency. Explicit server timeouts
- **One connection pool per database role**: public routes read with a
  `SELECT`-only login; admin writes with its own narrow login
- State-changing routes check `Origin` against an exact allowlist. CORS is an
  exact allowlist, never `*` with credentials
- **Migrations**: goose, numbered plain `.sql` files in
  `backend/migrations/<db>/`. `00001_schema.sql` is the ratings schema at
  its start; a change is the next number after it. One that has reached
  `main` is never edited; fix forward. Write the `-- +goose Down`, or mark it
  `-- irreversible` with why. The schema's shape:
  `docs/architecture.md#the-ratings-database`
- Errors: wrap with `%w` and context. An error crossing a trust boundary is
  rewritten, not wrapped (a database error can carry hosts and users)
- `slog`, JSON to stdout. A type holding a secret gets a redacting `String()`
- Tests: table tests with `name` and `t.Run`; messages read
  `got = %q, want %q`. Integration tests sit behind `//go:build integration`.
  Tests touching shared tables live in `internal/store`
- Times are `timestamptz` in UTC plus an IANA zone name per player, never an
  offset. Test a DST change in both hemispheres
- `gofmt`, `go vet` and `go tool staticcheck ./...` must be clean;
  `scripts/ci.sh` and `scripts\ci.ps1` run all three. The pre-commit hook
  checks staged Go with gofmt, and refuses when gofmt is not on PATH (in
  PowerShell, dot-source `scripts\env.ps1` first). staticcheck and govulncheck are `tool`
  lines in `go.mod`, so they build with the project's Go. Fix the code rather than add a `//lint:ignore`; one that is truly
  needed says why on the same line

## Engine (`engine/`)

- **The numbers are the product.** `engine.py` and `solver.py` are DANeo's
  math: **do not edit them** to make something else easier. Storage changes
  go in the adapter layer. A change that *intends* to move ratings is a
  maintainers' decision, made explicitly
- **A change that moves ratings is a new engine version.** Bump `version` in
  `engine/pyproject.toml` and add its hash to `engine/tests/golden.json`
  (the golden test's failure prints it). CI fails a change that moves
  ratings without a new version, and a version's hash never changes. Every
  run records the version
- The engine reads only the database: no web code, scraping, CSV or
  spreadsheet parsing. Loading and checking data is the Go load job's work
- **Runs, not overwrites**: each computation writes a new run; publishing is
  a separate step
- Tournament results are derived from matches, with organisers' recorded
  rulings applied on top and the derived result kept visible
- Python 3.12 via uv; `uv run pytest`, `uv run ruff check .`,
  `uv run ruff format .`, `uv run mypy`. mypy checks our code (`data/`,
  `run.py`, `progress.py`), not upstream's `models/`: fix a finding in our
  code, never in the math. Slow tests are marked `slow` and run on `main`.
  Tests that touch the database require `TEST_DATABASE_URL` (which `scripts/test.ps1`
  and `scripts/test.sh` set automatically); without it, those tests fail
- `tests/` mirrors the package: one `test_<module>.py` per module
- **Coverage, by hand.** `--with` adds pytest-cov for this run only; it is
  not in `uv.lock`. The database is the tests' own, made by `install.ps1` or
  the first `scripts/test.sh` (use your `.env`'s port if it is not 55432):

  ```powershell
  cd engine
  $env:TEST_DATABASE_URL = 'postgres://ttac:local-only@127.0.0.1:55432/ttac_test?sslmode=disable'
  uv run --frozen --with pytest-cov pytest -m 'not slow' --cov=tta_ratings_engine --cov-branch
  ```

  ```bash
  cd engine
  TEST_DATABASE_URL='postgres://ttac:local-only@127.0.0.1:55432/ttac_test?sslmode=disable' \
    uv run --frozen --with pytest-cov pytest -m 'not slow' --cov=tta_ratings_engine --cov-branch
  ```

  Upstream's `models/` is in the report; we do not add tests to it

## Web (`web/`)

- pnpm through corepack: `corepack pnpm install`, `corepack pnpm test`,
  `corepack pnpm lint`, `corepack pnpm check`, `corepack pnpm build`
- **Static by default**: every page is prerendered. A page rendered on
  request needs the maintainers' agreement first
- `.astro` for layout and static content; React only where something is
  interactive. TypeScript `strict`, no `any` in shared code
- **Plain CSS**: tokens as custom properties in `shared`, scoped `<style>` per
  component. No Tailwind, no CSS-in-JS, **no inline `style="…"`**
- **The ratings pages are a faithful visual copy of DANeo's site.** Follow
  upstream's look; don't redesign it without the maintainers' say-so
- Content pages are **MDX** in `src/content/`. Maths is TeX (`$…$`, `$$…$$`),
  rendered by KaTeX as MathML
- Times are shown in the viewer's zone, labelled. The API sends UTC
- `web/shared/api/` is generated: never edit it by hand
- The CSP has no `'unsafe-inline'` for scripts; our scripts and styles are
  never inlined
- Accessibility: semantic HTML, keyboard reachable, table headers and
  captions, a text alternative for every chart
- Cloudflare builds and hosts both sites from `main`. CI building both sites
  is the real gate
- **Coverage, by hand.** `dlx` fetches the coverage package for this run
  only, at the installed vitest's version, which it must match. The report
  lists the files the tests import: components are not tested, so look at
  `lib/`:

  ```powershell
  cd web
  $v = (Get-Content node_modules/vitest/package.json | ConvertFrom-Json).version
  corepack pnpm "--package=vitest@$v" "--package=@vitest/coverage-v8@$v" dlx vitest run --coverage.enabled --coverage.reporter=text
  ```

  ```bash
  cd web
  v=$(node -p "require('./node_modules/vitest/package.json').version")
  corepack pnpm "--package=vitest@$v" "--package=@vitest/coverage-v8@$v" dlx vitest run --coverage.enabled --coverage.reporter=text
  ```

## Git and pull requests

- Branch from the latest `main`; never commit to `main` directly. The
  pre-push hook refuses a push to `main`
- **Work on an issue starts from it**: `gh issue develop <n> --base main
  --checkout` (or "Create a branch" on the issue) makes a branch linked to
  it. End the PR's **What it does** with `Closes #<n>`, so merging to `main`
  closes the issue. Branch names alone link nothing
- **Commit messages**: one line, imperative, capitalised, no full stop, about
  50 characters (72 at most). Describe the change in plain words:
  `Add sign-in with Discord to the API`. No AI or tool attribution trailers
- **Pull requests have two sections only**: **What it does** and **How to
  test it** (commands and URLs). Merging needs a maintainer
- **One issue per pull request.** Work for another issue goes on its own
  branch. A side fix rides along only when **What it does** names it
- **No `WIP` commits reach `main`.** Fold fix-ups, such as formatting or
  a forgotten file, into the commit they fix before the image build
- **In PowerShell, backticks are escapes**, so `--body "…"` loses its
  code formatting. Write the description to a file and pass
  `gh pr create --body-file pr.md`
- **Review your own PR before merging**, as a reviewer would: the steps
  in "Before you push", then each changed function against the input
  that would break it
- **CI runs `scripts/ci.sh`**, one job per area (go, engine, web, checks),
  plus secret and code scanning. If it passes locally it should pass in CI;
  `scripts\ci.ps1` runs the same checks on Windows, all but the few it lists.
  Locally, red means something to fix: what only CI can check (our private
  images without `read:packages`, a branch's `deploy/` pins) is listed under
  "Not covered here" instead. Green is not the whole story: say what the
  change was *not* tested against
- Image builds: add the `images` label to a pull request once the code is
  final; it builds `sha-<commit>` images to pin in `deploy/`
- **Issues** open through a form: Bug, Data (a wrong or missing game, score
  or result), Feature, Decision, Task. Label the area: `ratings`,
  `tournaments`, `site`, `admin`, `build` (CI, checks, images) or `infra`
  (hosting, Cloudflare, backups)
