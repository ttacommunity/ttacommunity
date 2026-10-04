# ttacommunity

A community platform for **Through the Ages**, with three sites on one backend:

- **Ratings**: community Glicko ratings, with a public API. The rating math is
  DANeo's rating engine
- **Tournaments**: players in official tournament matches agree a start time,
  get reminders, and have the game started for them
- **TTA Pulse**: a blitz queue that pairs players near their level

**Early days.** The foundation is in place and the features are being built.
How the pieces fit: [`docs/architecture.md`](docs/architecture.md).

## Layout

| Directory | What | Language |
|---|---|---|
| `backend/` | The API and the worker (`cmd/api`, `cmd/worker`) | Go |
| `engine/` | The rating engine, run as a scheduled job | Python (uv) |
| `web/` | The public site, the admin site, and shared theme and types | Astro, React, TypeScript (pnpm) |
| `checks/` | Repo checks: secrets, digests, comment density, no raw HTML | Bash |
| `scripts/` | Run, test and CI entry points | Bash |

## Run it

**In containers**, with only Docker (or Podman with a compose provider):

```bash
scripts/dev.sh           # both databases, api, worker, the engine once, and the site
scripts/dev.sh down      # stop it; the database volumes are kept
```

Then open the site at http://127.0.0.1:4321, and the API at
http://127.0.0.1:8080/healthz. The first run creates `.env` from
`.env.example`, whose values are for local use only.

With Podman, the compose plugin talks to Podman's API socket. Turn it on once:
`systemctl --user enable --now podman.socket`.

**Natively**, for day-to-day work (Postgres still runs in a container):

```bash
scripts/dev.sh --native
```

The toolchain versions are pinned in [`mise.toml`](mise.toml): Go, Node, uv
(which fetches Python itself), crane and shellcheck. With
[mise](https://mise.jdx.dev) installed, `mise install` fetches exactly those,
and CI uses the same file. pnpm comes through Node's corepack, at the version
`web/package.json` pins. On Windows, use WSL2 or the containers.

### Show a local build to someone: Cloudflare quick tunnels

To check the site on a phone, or show it to someone, before it is merged:
serve a production build locally and put it and the API behind
`cloudflared` quick tunnels. Quick tunnels need no Cloudflare account, and
**their URLs are public until you stop them**.

1. Run the API against your local ratings database:

   ```bash
   cd backend && set -a && . ../.env && set +a && go run ./cmd/api
   ```

2. Open a tunnel to it, and note the `https://….trycloudflare.com` URL it
   prints:

   ```bash
   cloudflared tunnel --no-autoupdate --url http://127.0.0.1:8080
   ```

3. Build the site against that URL (the browser calls the API directly, so
   the default `http://127.0.0.1:8080` only works on this machine), then
   serve the build and open a second tunnel to it:

   ```bash
   cd web
   PUBLIC_API_URL=https://<api-tunnel>.trycloudflare.com pnpm --filter @ttac/site build
   cd site && pnpm exec astro preview --port 4321 --host 127.0.0.1 --allowed-hosts <site-tunnel>.trycloudflare.com
   cloudflared tunnel --no-autoupdate --url http://127.0.0.1:4321
   ```

   `astro preview` refuses a host it was not told about, so start the site's
   tunnel first if you need its URL for `--allowed-hosts`, then start the
   preview.

**Any other site build overwrites the one being served**, including the one
`scripts/ci.sh` runs, with the default API URL: the page then loads, but
every request fails with a network error. Rebuild as in step 3 afterwards,
and check the page carries the tunnel's URL:

```bash
curl -s "https://<site-tunnel>.trycloudflare.com/ratings/" | grep -o 'props="[^"]*"'
```

The preview sends none of `public/_headers`, so the real CSP is only
checked on the deployed site.

## Test it

```bash
scripts/test.sh    # every test: Go (with a throwaway Postgres), the engine, and web
scripts/ci.sh      # everything CI runs, in the same order
```

CI runs `scripts/ci.sh` unchanged, so a green run here means a green pipeline.
The script ends by listing what it did not cover.

Turn on the git hooks, which refuse a commit or push that looks like it holds a
credential:

```bash
git config core.hooksPath .githooks
```

## Load ratings data

No match or rating data is kept in this repo. The ratings database is seeded
by the load job from a directory of match CSVs that you keep elsewhere:

```bash
scripts/ratings-db.sh ~/tta-data            # in a container
scripts/ratings-db.sh --native ~/tta-data   # with local Go
scripts/ratings-db.sh --reset ~/tta-data    # empty the database first
```

It starts the ratings database if needed, applies the migrations, and loads
every `*.csv` at the top of the directory (not in subdirectories), in name
order. It refuses to load into a database that already has matches, so
nothing is loaded twice: use `--reset` to start again. **`--reset` deletes
every match, refused row and rating run.**

In a container, the job sees only that directory, so a symlink pointing out
of it does not resolve. Copy the files in instead.

After pulling a new migration, apply it to a database you already loaded:

```bash
cd backend && set -a && . ../.env && set +a && go run ./cmd/load -migrate
```

Players' countries come from CSV files with a `Player` and a `Code` column
(an ISO 3166-1 alpha-2 code, or `-` for none), such as the original site's
ratings export. Load them after the matches; a later file wins, and names
that are not players are counted and skipped:

```bash
cd backend && set -a && . ../.env && set +a
go run ./cmd/load -countries ~/tta-data/ratings_overall.csv ~/tta-data/country-overrides.csv
```

What match names cannot say about a tournament is a recorded fact, kept
with the data outside the repo, one CSV per kind. `series` is
`international`, `intermezzo`, `royal_league` or `worlds` (the World
Championship, whose season is the year), and `source` says where the row
comes from:

| Kind | Header | What it records |
|---|---|---|
| `divisions` | `series,season,division,player,source` | The division a player played in a season whose match names give none |
| `placements` | `series,season,division,player,place,points,reason,source` | A place the organisers recorded, applied over the derived one: a season with no games, or a ruling. `points` may be empty |
| `corrections` | `series,name_season,season,reason,source` | The real season of games whose name gives the wrong one; an empty `season` leaves them out of tournament results (never out of the ratings) |
| `dates` | `series,season,finish_date,source` | The day a season officially finished (`YYYY-MM-DD`) |
| `honours` | `series,player,gold,silver,bronze,points,source` | A player's career medals from an organisers' Hall of Fame the matches cannot rank; `series` is `sodium_ladder`, `mercurial_ladder` or `q_and_d`, and `points` (as written there) may be empty |
| `community` | `edition,sheet_row,rank,player,points,events,source` | The community leaderboard's standings as its maintainer publishes them; `edition` is a year (2026, 2025, 2024) or `race26`, `sheet_row` is the row's place in the sheet (it orders equal ranks), and names are kept even when they are not players |
| `community_scores` | `edition,player,event,score,source` | Each player's points in each event column of an edition (`wrld`, `ic_30`, `aus`…) |

Loading a file replaces every fact of its kind, in one transaction; names
that are not players are counted and skipped, and bad rows are refused with
their line and reason. The next rating run uses them:

```bash
cd backend && set -a && . ../.env && set +a
go run ./cmd/load -facts divisions ~/tta-data/tournaments/group_divisions.csv
```

The API reads ratings as `ratings_api`, a login that can only `SELECT`.
`scripts/dev.sh` creates it, and the API refuses to start if its login can
write.

### Copy a loaded database to another server

To seed an empty ratings database elsewhere (a server, or the cluster) from
the local one, or from a `pg_dump -Fc` file:

```bash
PGPASSWORD=… RATINGS_API_PASSWORD=… \
  scripts/seed-cluster-db.sh --host db.example --port 5432 [--dump ratings.dump]
scripts/seed-cluster-db.sh --kubectl [--dump ratings.dump]   # through kubectl
```

`PGPASSWORD` is the `ratings` owner's password on the target, and
`RATINGS_API_PASSWORD` becomes `ratings_api`'s. `--kubectl` reads both from
the cluster's Secrets (`deploy/SECRETS.md`). Either way it refuses a database
that already has tables.

### Export the data, and rebuild from it

An export is every table that holds inputs (games, CGE seats, players, the
tournament catalogue and every recorded fact) as one CSV each, with a
`manifest.json` of the schema version, row counts and SHA-256s. Ratings are
not in it: the engine computes them again. Keep exports outside the repo.

```bash
go run ./cmd/load -export ~/tta-data/exports/2026-10-03    # a new or empty folder
```

To rebuild a lost database, restore into a new, empty one, then compute:

```bash
go run ./cmd/load -restore ~/tta-data/exports/2026-10-03
```

The restore applies the migrations, then loads everything in one
transaction. It refuses a database that already holds games or runs, a file
changed since the export, and an export taken at another schema version
(restore with the `load` of that version, then migrate). Then run the engine
([Compute ratings](#compute-ratings)).

For the original site (upstream), write the games as the one match CSV its
loader reads, `data/raw/all_matches.csv`:

```bash
go run ./cmd/load -export-upstream ~/tta-data/exports/all_matches.csv   # a new file
```

It holds every played game, oldest first. Games never played or unconfirmed
are left out: they are not rated here, and upstream would rate them. Replay
codes, CGE seats and the recorded tournament facts are not in that format.

### The files

Every file has this header, and one match per row:

```csv
Tournament,Date,Player1,Score1,Player2,Score2,Player3,Score3,Player4,Score4,Player_Count
```

- `Date` is `YYYY-MM-DD`
- 2 to 4 players, filled from `Player1` on, each with a score. Scores may be
  negative (penalties) or decimals
- `Player_Count` is the number of players named
- Names are matched exactly, so `ann` and `Ann` are two players

A file with any other header stops the load before anything is written.

### What gets refused

A row that is not a real game is **refused, not loaded**. It is kept in the
`refused_rows` table with its file, line, reason and the fields as read, and
the job prints a count per reason:

| Reason | The row |
|---|---|
| `standings_row` | A standings table scraped as matches: a name starting with a rank (`1.`), or containing "No finished games" |
| `bad_name` | A name with leading or trailing whitespace, or a line break or other control character |
| `duplicate` | The same tournament, date, players and scores as an earlier row, in any file. The first copy is loaded |
| `column_count`, `no_tournament`, `bad_date`, `bad_score`, `player_count`, `player_gap`, `score_without_player`, `player_without_score`, `same_player_twice` | The row is malformed |

A match where every score is 0 is **loaded**, and rated as a draw between
its players, as the original site rates it.

To see them:

```bash
psql postgres://ratings:local-only-ratings@127.0.0.1:55432/ratings \
  -c "SELECT reason, source_file, source_line FROM refused_rows ORDER BY 1, 2, 3"
```

## Compute ratings

With matches loaded, the engine computes a new rating run from them:

```bash
cd engine
set -a; . ../.env; set +a
ENGINE_VERSION=$(git rev-parse --short HEAD) uv run python -m tta_ratings_engine.run
```

A full run of every model and format takes about 20 minutes and 1 GB of
memory on a desktop, and logs its progress and time left as it goes. It is
not published until you add `--make-current`. The options and
what a run writes are in [`engine/README.md`](engine/README.md).

## Publish

The cluster runs what `deploy/` lists (Flux applies it from `main`); the
sites are built and served by Cloudflare Workers from git. Nothing here holds
a Cloudflare credential.

### Shipping a change to the api or the engine

The images are pinned in the same merge request as the code, so a merge
deploys both together:

1. When the code is final, run the manual `api-image` and `engine-image` jobs
   in the merge request's pipeline. Each pushes `<image>:sha-<commit>`
2. Resolve each digest, and pin `tag@digest` in `deploy/`. Set the engine's
   `ENGINE_VERSION` in `deploy/engine.yaml` to the same commit: every run
   records it:

   ```bash
   crane digest registry.gitlab.com/unstablehomelab/ttacommunity/api:sha-<commit>
   ```

   The registry is private: without a `crane auth login`, read the digest
   through the GitLab API instead, and check it against the job's log line
   `…:sha-<commit>: digest: sha256:…`:

   ```bash
   glab api "projects/:id/registry/repositories?per_page=50" | jq -r '.[] | "\(.id) \(.name)"'
   glab api "projects/:id/registry/repositories/<id>/tags/sha-<commit>" | jq -r .digest
   ```

3. Apply any new migration to the cluster's database **before** merging
   (see [the cluster's database](#the-clusters-ratings-database)).
   Migrations only add, so the running api is unaffected

`checks/deploy-current.sh` fails if `deploy/` pins an image built from older
code than the branch's, so a late code change needs a new image and pin.

### The cluster's ratings database

The database is reached through `kubectl port-forward`, as its owner
`ratings`, with the password read from its Secret into the environment only
(never onto disk or into shell history):

```bash
kubectl -n ttacommunity port-forward svc/ratings-db 55499:5432 &
export PGPASSWORD=$(kubectl -n ttacommunity get secret ratings-db -o jsonpath='{.data.password}' | base64 -d)
export RATINGS_DATABASE_URL="postgres://ratings@127.0.0.1:55499/ratings?sslmode=disable"
```

**Apply new migrations** (before merging the change that needs them), and
check the version moved:

```bash
psql "$RATINGS_DATABASE_URL" -Atc "SELECT max(version_id) FROM goose_db_version"
cd backend && go run ./cmd/load -migrate
psql "$RATINGS_DATABASE_URL" -Atc "SELECT max(version_id) FROM goose_db_version"
```

**Load the recorded tournament facts** (see [Load ratings data](#load-ratings-data))
when they change, before the run that should use them. Each file replaces
its kind:

```bash
cd backend
go run ./cmd/load -facts divisions ~/tta-data/tournaments/group_divisions.csv
go run ./cmd/load -facts placements ~/tta-data/tournaments/official_placements.csv
go run ./cmd/load -facts corrections ~/tta-data/tournaments/season_corrections.csv
go run ./cmd/load -facts dates ~/tta-data/tournaments/finish_dates.csv
go run ./cmd/load -facts honours ~/tta-data/tournaments/career_honours.csv
go run ./cmd/load -facts community ~/tta-data/tournaments/community_standings.csv
go run ./cmd/load -facts community_scores ~/tta-data/tournaments/community_scores.csv
```

**Compute a rating run** with the image `deploy/engine.yaml` pins. The
CronJob is suspended, so a run is started by hand as a Job from it. It takes
about 25 minutes, and ends `complete` or `failed` but **never published**:
the site keeps showing the current run.

```bash
kubectl -n ttacommunity create job --from=cronjob/engine engine-manual-<n>
kubectl -n ttacommunity logs -f job/engine-manual-<n>
psql "$RATINGS_DATABASE_URL" -c "SELECT run_id, status, engine_version, finished_at FROM rating_runs ORDER BY run_id"
```

**Check it against a local run** of the same code and data before
publishing: the two must print the same lines (a count and checksum per
table the run writes; ids that differ between databases are left out):

```bash
diff <(scripts/run-checksums.sh "$RATINGS_DATABASE_URL" <cluster_run_id>) \
     <(scripts/run-checksums.sh "<local ratings url>" <local_run_id>)
```

**Publish a complete run** when you have checked it. This is what the
engine's `--make-current` does, and the site shows the run within a minute
(the API's `/v1/runs/current` is cached for 60 seconds):

```bash
psql "$RATINGS_DATABASE_URL" -v run=<run_id> <<'SQL'
BEGIN;
INSERT INTO current_run (run_id) VALUES (:run)
  ON CONFLICT (only_row) DO UPDATE SET run_id = excluded.run_id;
UPDATE rating_runs SET published_at = now() WHERE run_id = :run;
COMMIT;
SQL
```

Only a `complete` run can be made current: the database refuses any other.
Going back is the same statement with the earlier run's id.

When done, stop the port-forward (`kill %1` in the same shell) and
`unset PGPASSWORD`.

### The sites and the sign-in worker on Cloudflare Workers

Each is a Worker configured by a `wrangler.jsonc`, which also declares its
custom domain (so a merge that changes it changes what is published).
Cloudflare's Workers Builds is connected to this repository and deploys each
from `main`:

| Worker | Root directory | Build command | Deploy command |
|---|---|---|---|
| `ttacommunity` (the site) | `/web` | `pnpm --filter @ttac/site build` | `npx wrangler@4.141.0 deploy --config site/wrangler.jsonc` |
| `ttacommunity` Previews (Settings → Builds → Previews Base) | `/web` | `pnpm --filter @ttac/site build` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then npx wrangler@4.141.0 preview --config site/wrangler.jsonc; else echo "Previews are built for branch dev only"; fi` |
| `ttac-admin` | `/web` | `pnpm --filter @ttac/admin build` | `npx wrangler@4.141.0 deploy --config admin/wrangler.jsonc` |
| `ttac-admin` Previews | `/web` | `pnpm --filter @ttac/admin build` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then npx wrangler@4.141.0 preview --config admin/wrangler.jsonc; else echo "Previews are built for branch dev only"; fi` |
| `ttac-auth` (Discord sign-in for Access) | `/infra/access-oidc` | none | `npx wrangler deploy` |

Build variables, set in each site Worker's build settings:

| Variable | Worker | Value | Why |
|---|---|---|---|
| `PUBLIC_API_URL` | `ttacommunity` | `https://ttac-api.unstableunicorn.dev`; on `dev`, `https://ttac-api-dev.unstableunicorn.dev` | Where the browser reads ratings, and the CSP's `connect-src`. Sent to the browser, so never a secret |
| `SITE_NOINDEX` | `ttacommunity`, `dev` only | `1` | Adds `X-Robots-Tag: noindex`: Cloudflare adds none to a Preview on a custom domain |
| `NODE_VERSION` | both sites | the `node` version in `mise.toml` | The build image's default is older |
| `PNPM_VERSION` | both sites | the pnpm version in `web/package.json` | The build image does not read `packageManager` |

When importing a Worker, **turn off non-production (preview) builds**, and
on `ttacommunity` keep them to branch `dev`. Previews have no branch
filter, so the Preview command skips every other branch. A preview build
reports on the merge request's commit, and a failure there blocks the
merge. Previews Base has its own build variables; don't click "Enable
Preview URLs", which turns on `workers.dev`. `dev` is served at `dev.ttacommunity.unstableunicorn.dev`: a
Preview on a custom domain is `<branch>.<route host>`, and the route sets
`previews_enabled`.

`ttac-auth` needs one runtime secret, `DISCORD_CLIENT_SECRET`, set under the
Worker's Variables and Secrets (not its build variables).

**`ttac-admin` is only reachable through Cloudflare Access.** Its Access
application must exist before its domain is declared, and it covers
`*.ttac-admin.unstableunicorn.dev` before previews are turned on: `dev`'s
preview is `dev.ttac-admin.unstableunicorn.dev`.

### Admin

Admin is the `ttac-admin` site and the API's `/v1/admin/` routes:

- **The site's Worker** serves the pages, and proxies `/api/*` to
  `/v1/admin/*` on the API for its host (`ttac-admin` to `ttac-api`,
  `dev.ttac-admin` to `ttac-api-dev`; any other host is refused) after
  verifying the Access token itself
- **The API verifies the token again** (signature, issuer, the admin app's
  audience, expiry, a Discord id), then looks the Discord id up in `admins`:
  role `admin` or `organiser`. A change must come from `ADMIN_ORIGIN`, and
  every change is written to `admin_audit`
- **The API refuses to start** without `ACCESS_TEAM`, `ACCESS_AUD`,
  `ADMIN_ORIGIN` and `RATINGS_ADMIN_DATABASE_URL`, or when the admin login
  could change games, players or a run's ratings

**Rating runs.** Admins start a run, watch it, stop it, compare it with the
published run, and publish it (or publish an older one to roll back). A
stopped engine marks its run failed as it ends; a run left running by an
engine that was killed outright shows as failed once no engine Job is going. The engine writes
its latest progress line to `run_progress` as it goes. To start a run, the API
creates a Job from the CronJob named by `ENGINE_CRONJOB`, in its own namespace,
using its service account (`deploy/dev/api-rbac.yaml`: read that CronJob, and
list, create and delete Jobs); stopping deletes that CronJob's unfinished Jobs. Without `ENGINE_CRONJOB`, as when run locally, starting a run
answers 503; the rest of admin works.

The admin login is a member of `ratings_admin`, which the migrations grant
only what admin edits. A database owner without `CREATEROLE` needs the role
made first (`scripts/seed-cluster-db.sh` does). Add someone to admin:

```bash
go run ./cmd/load -add-admin <Discord id> -name <name> -role admin   # or organiser
```

### The API

`deploy/cloudflare-tunnel.yaml` runs the tunnel's connector. The hostname goes
live when a DNS record points at the tunnel:

```bash
cloudflared tunnel route dns ttacommunity ttac-api.unstableunicorn.dev
```

A WAF rate-limiting rule on that hostname comes first.
