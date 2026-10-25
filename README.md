# ttacommunity

A community platform for **Through the Ages**, with three sites on one backend:

- **Ratings**: community Glicko ratings, with a public API. The rating math is
  DANeo's rating engine
- **Tournaments** (planned): players in official tournament matches agree a
  start time, get reminders, and have the game started for them
- **Blitz tournaments** (being built): organisers run a Swiss-and-knockout
  blitz tournament from admin, and the site shows who is in, the weekly
  pairings, results and standings
- **TTA Pulse** (planned): a blitz queue that pairs players near their level

The ratings site, its public API and the admin site are built, and blitz
tournaments' entrants with them; Tournaments and Pulse are not yet. How the
pieces fit, and the database schema:
[`docs/architecture.md`](docs/architecture.md). Rules for contributors and
coding agents: [`AGENTS.md`](AGENTS.md).

## Layout

| Directory | What | Language |
|---|---|---|
| `backend/` | `cmd/api` (the JSON API), `cmd/load` (the data load job), `internal/`, `migrations/`, and `api/openapi.yaml` (the API contract) | Go |
| `engine/` | The rating engine, run as a job | Python (uv) |
| `web/` | `site` (public), `admin`, and `shared` (theme, components, generated API types) | Astro, React, TypeScript (pnpm) |
| `infra/access-oidc/` | The Worker that signs admins in to Cloudflare Access with Discord | JavaScript |
| `deploy/` | What the home cluster runs (Flux applies it from `main`) | Kubernetes YAML |
| `scripts/` | Install, run, test and CI entry points; `.ps1` twins for Windows | Bash, PowerShell |
| `checks/` | Repo checks: secrets, digests, comment density, no raw HTML | Bash |
| `docs/` | Architecture | Markdown |

## Develop

### 1. Get the code and the data

The code is here; the data never is (it lives in a private repository the
maintainers share). Clone both side by side:

```bash
git clone git@github.com:ttacommunity/ttacommunity.git
git clone git@github.com:ttacommunity/ttac-ratings-data.git
```

On Windows over HTTPS, sign in to GitHub once when Git Credential Manager
asks, or use `git@github.com:` URLs with an SSH key.

### 2. Install the tools

Every tool version is pinned in [`mise.toml`](mise.toml) (Go, Node, uv, which
fetches Python 3.12 itself, and CI's linters), and pnpm's in
`web/package.json`, used through Node's corepack. CI installs the same.

**Linux and macOS:** install [mise](https://mise.jdx.dev), then `mise
install`. Postgres runs in a container: Docker, or Podman with a compose
provider (turn on its socket once: `systemctl --user enable --now
podman.socket`).

**Windows** (no Docker; Windows PowerShell 5.1 or PowerShell 7). Allow local
scripts once, then install:

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
scripts\install.ps1
```

`install.ps1` installs mise with winget, then Go, Node and uv, and
PostgreSQL 18 on port 55432, for your user only: no admin rights, unpacked
to `%LOCALAPPDATA%\ttacommunity`, and started by the scripts when they need
it (`pg_ctl stop` stops it). It asks once for a password for the local
`postgres` superuser, which only it uses. With `-Service` it installs
PostgreSQL as a Windows service instead (asks for admin). It is safe to run
again. `. scripts\env.ps1` loads `.env` into a PowerShell session for
running anything by hand.

Either way, turn on the git hooks, which refuse a commit or push that looks
like it holds a credential (`install.ps1` does this for you):

```bash
git config core.hooksPath .githooks
```

The first run of any script creates `.env` from `.env.example`. Its values
are for local use only. The dev scripts warn when `.env` lacks a setting
added to `.env.example` since; copy that line across.

### 3. Load the data and compute ratings

Restore the newest baseline from the data repository into the empty local
database, then compute a rating run and publish it locally:

```bash
scripts/ratings-db.sh --native ../ttac-ratings-data/baseline
cd engine && set -a && . ../.env && set +a
uv run python -m tta_ratings_engine.run --make-current
```

```powershell
scripts\ratings-db.ps1 ..\ttac-ratings-data\baseline
. scripts\env.ps1; cd engine
uv run python -m tta_ratings_engine.run --make-current
```

A full run takes about 20 minutes. `--formats 2` rates two-player games only,
for a quicker look. More in [Load ratings data](#load-ratings-data) and
[Compute ratings](#compute-ratings).

### 4. Run it

```bash
scripts/dev.sh --native    # api, site and admin here; Postgres in a container
scripts/dev.sh             # or everything in containers, the engine once too
scripts/dev.sh down        # stop the containers (data volumes are kept)
```

```powershell
scripts\dev.ps1            # api, site and admin; Ctrl-C stops them
```

The site is at http://127.0.0.1:4321, admin at http://127.0.0.1:4322 and
the API at http://127.0.0.1:8080/healthz. Locally, `ADMIN_DEV_USER` in `.env`
stands in for Cloudflare Access: a request with no Access token is that
admin, and the token `dev-<id>` is admin `<id>` (to try another role). Add
that admin once, `go run ./cmd/load -add-admin 100000000000000001 -name
"Local admin" -role admin`. The api refuses `ADMIN_DEV_USER` unless
`ADMIN_ORIGIN` and `API_ADDR` are `127.0.0.1` and it runs outside
Kubernetes, so it can never open a deployed admin. `DISCORD_DEV_USER` does
the same for players' sign-in: "Sign in with Discord" signs you in at once
as that invented Discord account, with no Discord app. The api refuses it
on the same terms, and when a Discord app is set.

### 5. Test it

```bash
scripts/test.sh    # every test: Go, the engine, web, on the local dev Postgres (else a throwaway one)
scripts/ci.sh      # everything CI runs, in the same order
```

```powershell
scripts\test.ps1   # the same tests, against the local Postgres
scripts\ci.ps1     # what CI runs, all but the few it lists
```

CI runs `scripts/ci.sh` unchanged, one job per area, so a green run here
means a green pipeline. Each script ends by listing what it did not cover.
The `windows` workflow runs `install.ps1` and `ci.ps1` on GitHub's Windows
runner when started by hand.

With Windows 11's **Smart App Control** on, Go's tests fail at random with
"An Application Control policy has blocked this file": Windows refuses the
unsigned test programs `go test` builds, and offers no exception for them.
Turn Smart App Control off while developing (Windows Security → App &
browser control → Smart App Control; on a Windows 11 with the April 2026
updates it can be turned back on there), or rely on CI for the Go tests.

### 6. Make a change

Read [`AGENTS.md`](AGENTS.md) first: how we test, comment, commit and open
pull requests. Then, by kind of change:

- **The API**: change `backend/api/openapi.yaml` first, then the handler
  in `backend/internal/httpapi`. Regenerate both sides and commit them:
  `cd backend && go generate ./internal/httpapi` and `corepack pnpm
  --filter @ttac/shared gen:api` (in `web/`). A test fails while a route and
  the spec disagree
- **The database**: add the next numbered file to
  `backend/migrations/ratings/` (goose, plain SQL, with its `-- +goose
  Down`), after `00001_schema.sql`, the whole schema at its start. Never
  edit one that has reached `main`. Apply it locally with `go
  run ./cmd/load -migrate`. The export and import must stay equal: a new
  table the data needs goes into the export too, and the data repository
  then needs a new baseline. The schema at a glance:
  [`docs/architecture.md#the-ratings-database`](docs/architecture.md#the-ratings-database)
- **The engine**: `engine/`; don't change the rating math itself
  (`engine.py`, `solver.py`) to make something else easier. `uv run pytest`,
  `uv run ruff check .`, `uv run ruff format .`
- **A page**: `web/site/src/pages/`; text pages are MDX in
  `web/site/src/content/`. Keep components thin: decisions go in a tested
  `lib/` function
- **The api or engine image**: once the code is final, add the `images`
  label to the pull request and pin the digests it prints in `deploy/` (see
  [Shipping a change](#shipping-a-change-to-the-api-or-the-engine))

Branch from `main` and open a pull request to `main`. Merging needs a
maintainer.

### Show a local build to someone: Cloudflare quick tunnels

To check the site on a phone, or show it to someone, before it is merged:
serve a production build locally and put it and the API behind
`cloudflared` quick tunnels. Quick tunnels need no Cloudflare account, and
**their URLs are public until you stop them**.

1. Stop `scripts/dev.sh`, then run the API against your local ratings
   database **with every dev sign-in off**. `.env`'s `ADMIN_DEV_USER` and
   the other `*_DEV_*` settings sign in anyone who reaches the API, and
   behind a tunnel that is anyone with its URL. Without them, player
   sign-in needs a Discord client, so give it a placeholder that signs
   nobody in, and leave Turnstile and email unset:

   ```bash
   cd backend && set -a && . ../.env && set +a &&
     ADMIN_DEV_USER= DISCORD_DEV_USER= MICROSOFT_DEV_USER= GOOGLE_DEV_USER= EMAIL_DEV_MAIL= \
     MICROSOFT_DEV_EMAIL= GOOGLE_DEV_EMAIL= DISCORD_CLIENT_ID=tunnel DISCORD_CLIENT_SECRET=tunnel \
     TURNSTILE_SECRET= RESEND_API_KEY= EMAIL_FROM= EMAIL_SUPPORT= \
     MICROSOFT_CLIENT_ID= MICROSOFT_CLIENT_SECRET= GOOGLE_CLIENT_ID= GOOGLE_CLIENT_SECRET= \
     go run ./cmd/api
   ```

   Check it: its log has no "bypassed" or "stand-in" warning, and
   `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/v1/admin/me`
   prints `401`. **Never leave the API's tunnel up while `scripts/dev.sh`
   runs**: its API listens on the same port with the dev sign-ins on. Admin
   cannot be shown this way: it signs in through Access, or `ADMIN_DEV_USER`
   on `127.0.0.1` only

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

## Load ratings data

No match or rating data is kept in this repo. The data lives outside it, and
a database starts from a **baseline**: an export of every input table (games,
CGE seats, players, the tournament catalogue, every recorded fact and admin's
settings, but no personal data) with every correction already applied. Restore one into an empty
database, then compute ratings:

```bash
scripts/ratings-db.sh ../ttac-ratings-data/baseline            # in a container
scripts/ratings-db.sh --native ../ttac-ratings-data/baseline   # with local Go
scripts\ratings-db.ps1 ..\ttac-ratings-data\baseline            # Windows
```

The script starts the ratings database if needed and restores the export
whole. It refuses a database that already holds data: to start again,
remove the database's volume. Then [compute ratings](#compute-ratings).

A new baseline is any export: **Download all data** on admin's home page
(admins and organisers; a zip of the same files), or `go run ./cmd/load
-export <new folder>` (see [Export the data, and rebuild from
it](#export-the-data-and-rebuild-from-it)). It replaces the contents of the
data repo's `baseline/` folder, whose name never changes, so the commands here
stay the same.

After pulling a new migration, apply it to a database you already loaded:

```bash
cd backend && set -a && . ../.env && set +a && go run ./cmd/load -migrate
```

### Add new data

New games and facts are loaded on top of a restored baseline, and the next
export is the next baseline.

**CGE's data extract.** CGE's export of every tournament game (one row per
player per game) is the most complete record. `-cge-extract` matches its
games to ours (by share code, else by players and finished scores), fills in
what CGE recorded (each seat's account, outcome and place, the game's times
and share code; CGE's finish date becomes the game's date), adds the games we
lack (a CGE tournament we do not link becomes a hidden, unrated series) and
the settings we lack from the settings files beside it. A game that may be a
copy of ours is held back; a file of pairs checked by hand (`share_code,
game_id,reason`) matches them, and a game an earlier load added for a
session the file gives to another game of ours is removed as a copy. A seat
CGE holds no account for (an empty seat that timed out) is kept as the
player "Unknown player", who is nobody and is never rated; the load counts
those in rated tournaments, each worth looking up (the older match files
may hold the player, as for the pair above). It prints the plan and writes
nothing unless given `-apply`; loading the same extract again changes
nothing, except that it merges a hidden series' stages back into one
tournament where an earlier load split them:

```bash
cd backend && set -a && . ../.env && set +a
go run ./cmd/load -cge-extract ../../ttac-ratings-data/cge/extract/2026-10-07 \
  -cge-settings ../../ttac-ratings-data/cge/settings [-cge-pairs <file>] [-apply]
```

The extract in the data repository is gzipped (`gzip -dk` restores it).

**A first database from match CSVs.** A directory without a
`manifest.json` is loaded as match CSVs, every `*.csv` at its top level in
name order (the format is under [The files](#the-files)). This seeds an
empty database only: it refuses one that already has matches, so nothing is
loaded twice, and `--reset` deletes every match, refused row and rating run
first. It is how the first baseline was built:

```bash
scripts/ratings-db.sh ~/tta-data/first-games
```

In a container, the job sees only that directory, so a symlink pointing out
of it does not resolve. Copy the files in instead.

Players' countries come from CSV files with a `Player` and a `Code` column
(an ISO 3166-1 alpha-2 code, or `-` for none), such as the original site's
ratings export. A country lives on the player's member (added for them if
they have none): every page's flag is the member's, and a player who has
signed in sets their own. Load the files after the matches; a later file
wins, a player who has signed in keeps their own country, and names that
are not players are counted and skipped:

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
their line and reason. A season's time-out penalty set in admin is kept when
`rules` are reloaded. The next rating run uses them:

```bash
cd backend && set -a && . ../.env && set +a
go run ./cmd/load -facts divisions ~/tta-data/new/group_divisions.csv
```

Each tournament's points per place (for 2, 3 and 4 players), its top
divisions and each season's time-out penalty are set in admin's tournament
settings. The engine scores standings with them, and refuses to run when
International, Intermezzo, Royal League or the World Championship has none.

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
not in it: the engine computes them again. **An export holds no personal
data, so it can be shared:** who the admins are, the audit of what they did,
players' sign-ins, the blitz tables and the rows a load refused are left
out, and of a member only the name, its ids and the country (what the site
shows beside the name; the manifest names the columns). Keep exports
outside the repo.

```bash
go run ./cmd/load -export <new or empty folder>
```

#### Rebuild a database from scratch

Everything a database needs comes back from an export: the schema from the
migrations, the inputs from the CSVs, the ratings from the engine. To start
again from nothing:

1. **An empty database.** Locally: `scripts/dev.sh down`, remove the
   `ratings-db` volume (`podman volume rm` or `docker volume rm`), and start
   it again; on Windows: `dropdb ratings` and `createdb -O ratings ratings`
   (as `postgres`), or rerun `install.ps1`. The cluster's:
   [Rebuild the cluster's database](#rebuild-the-clusters-database)
2. **Schema and data**, from the newest export:

   ```bash
   cd backend && go run ./cmd/load -restore ../../ttac-ratings-data/baseline
   ```

   The restore applies the migrations, then loads everything in one
   transaction. It refuses a database that already holds games or runs, a
   file changed since the export, and an export taken at another schema
   version (restore with the `load` of that version, then migrate)
3. **Admins**, which no export holds (they are personal data):
   `go run ./cmd/load -add-admin <Discord id> -name <name> -role admin`
4. **Ratings**: run the engine and publish the run
   ([Compute ratings](#compute-ratings), with `--make-current`)

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
uv run python -m tta_ratings_engine.run
```

In PowerShell: `. scripts\env.ps1; cd engine; uv run python -m tta_ratings_engine.run`.

A full run of every model and format takes about 3 minutes on a desktop
(the 16-model runs before engine 1.4.0 took 20), and logs its progress and
time left as it goes. It is
not published until you add `--make-current`. The options and
what a run writes are in [`engine/README.md`](engine/README.md).

## Publish

The cluster runs what `deploy/` lists (Flux applies it from `main`); the
sites are built and served by Cloudflare Workers from git. Nothing here holds
a Cloudflare credential.

### Shipping a change to the api or the engine

The images are pinned in the same pull request as the code, so a merge
deploys both together:

1. When the code is final, run the `images` workflow on the pull request's
   branch (Actions → images → Run workflow). Each job pushes
   `<image>:sha-<commit>` to GHCR and prints the reference in its summary
2. Resolve each digest, and pin `tag@digest` in `deploy/`. The packages are private, so log in with your `gh` token:

   ```bash
   gh auth token | crane auth login ghcr.io -u "$(gh api user -q .login)" --password-stdin
   crane digest ghcr.io/ttacommunity/ttacommunity/api:sha-<commit>
   ```

3. Nothing to do for migrations: each api pod's `migrate` init container
   (the api image run with `-migrate`, as the database's owner) applies them
   before the new api starts, one pod at a time. Migrations only add, so the
   api still running from before is unaffected while it happens

`checks/deploy-current.sh` fails if `deploy/` pins an image built from older
code than the branch's, so a late code change needs a new image and pin.

### The cluster's ratings database

Getting `kubectl` access to the cluster:
[`cluster-access`](https://github.com/ttacommunity/cluster-access)
(private; a maintainer adds you). The database is reached through
`kubectl port-forward`, as its owner
`ratings`, with the password read from its Secret into the environment only
(never onto disk or into shell history):

```bash
kubectl -n ttacommunity port-forward svc/ratings-db 55499:5432 &
export PGPASSWORD=$(kubectl -n ttacommunity get secret ratings-db -o jsonpath='{.data.password}' | base64 -d)
export RATINGS_DATABASE_URL="postgres://ratings@127.0.0.1:55499/ratings?sslmode=disable"
```

**Apply migrations by hand** only when the deploy cannot (the api's init
container normally does it), and check the version moved:

```bash
psql "$RATINGS_DATABASE_URL" -Atc "SELECT max(version_id) FROM goose_db_version"
cd backend && go run ./cmd/load -migrate
psql "$RATINGS_DATABASE_URL" -Atc "SELECT max(version_id) FROM goose_db_version"
```

**Recorded tournament facts** are in the baseline. To change one kind, write
its whole CSV in the format under [Load ratings data](#load-ratings-data) and
load it before the run that should use it; the file replaces every fact of
its kind:

```bash
cd backend
go run ./cmd/load -facts placements ~/tta-data/new/official_placements.csv
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
INSERT INTO current_run (ranking_id, run_id)
  SELECT ranking_id, run_id FROM rating_runs WHERE run_id = :run
  ON CONFLICT (ranking_id) DO UPDATE SET run_id = excluded.run_id;
UPDATE rating_runs SET published_at = now() WHERE run_id = :run;
COMMIT;
SQL
```

Admin's runs panel publishes the same way. Only a `complete` run can be
made current: the database refuses any other. Going back is the same
statement with the earlier run's id.

When done, stop the port-forward (`kill %1` in the same shell) and
`unset PGPASSWORD`.

#### Rebuild the cluster's database

The cluster's `ratings` database rebuilt from an export, with the
port-forward and `RATINGS_DATABASE_URL` above. The dev site's `ratings_dev`
is usually refreshed from production instead, with
`scripts/seed-dev-db.sh --replace`: the inputs and the current run by
default, or `--no-runs` (inputs only) or `--all-runs`. It can be rebuilt
from an export the same way: its owner is `ratings_dev`, its logins
`ratings_dev_api` and `ratings_dev_admin_api`, and its `RATINGS_DATABASE_URL`
is in the `ratings-dev-engine` Secret. The api serves errors from step 4
until step 8.

**What an export does not hold**, and where it comes from in a rebuild:

| What | Where it lives | Brought back by |
|---|---|---|
| Admins and their audit (personal data) | the database only | step 2's backup file, restored in step 7 |
| The database logins and their passwords | the server (roles) and the Secrets in [`deploy/SECRETS.md`](deploy/SECRETS.md) | untouched by a database rebuild; a new server needs them created again from the Secrets |
| The Secrets themselves | the cluster, made by hand | keep a copy outside the cluster; recreate as `SECRETS.md` describes |
| Ratings and tournament results | computed | step 8 |

1. **Keep a full copy first**, and prove it restores elsewhere (not on a
   cluster node's own disk: a large restore there can evict the database).
   It holds the admins, so keep it outside any repository, mode 600:
   `pg_dump -Fc "$RATINGS_DATABASE_URL" > ratings-<date>.dump`
2. **Back up what the export leaves out** (mode 600):
   `pg_dump -Fc --data-only -t admins -t admin_audit "$RATINGS_DATABASE_URL" > admins-<date>.dump`
3. **Note the highest run id**:
   `psql "$RATINGS_DATABASE_URL" -Atc "SELECT max(run_id) FROM rating_runs"`.
   Run-scoped API responses are cached at the edge for a year, so new runs
   must not reuse an old run's id
4. **Drop and recreate the database** with the same owner and connect
   rights (for `ratings_dev`: owner `ratings_dev`, and no `REVOKE`/`GRANT`):

   ```bash
   kubectl -n ttacommunity exec -i deploy/ratings-db -- psql -v ON_ERROR_STOP=1 -U ratings -d postgres <<'SQL'
   DROP DATABASE ratings WITH (FORCE);
   CREATE DATABASE ratings OWNER ratings;
   REVOKE CONNECT ON DATABASE ratings FROM PUBLIC;
   GRANT CONNECT ON DATABASE ratings TO ratings_api, ratings_admin_api;
   SQL
   ```

5. **Restore the newest export** (step 2 of [Rebuild a database from
   scratch](#rebuild-a-database-from-scratch)) as the database's owner. The
   export must be at the code's schema version
6. **Continue the run ids** after the highest from step 3:
   `psql "$RATINGS_DATABASE_URL" -c "SELECT setval('rating_runs_run_id_seq', <highest>)"`
7. **Restore the admins** from step 2:
   `pg_restore --data-only -d "$RATINGS_DATABASE_URL" admins-<date>.dump`
   (or add them by hand with `load -add-admin`)
8. **Compute a run, check it, and publish it** (above)

### The sites and the sign-in worker on Cloudflare Workers

Each is a Worker configured by a `wrangler.jsonc`, which also declares its
custom domain (so a merge that changes it changes what is published).
Cloudflare's Workers Builds is connected to this repository and deploys each
one from its branch:

| Worker (host) | Branch | Root directory | Build command | Deploy command |
|---|---|---|---|---|
| `ttacommunity`, the site (`ttacommunity.org`) | `main` | `/web` | `pnpm --filter @ttac/site build` | `npx wrangler@4.141.0 deploy --config site/wrangler.jsonc` |
| `ttacommunity`, non-production builds | all but `main` | `/web` | `echo "No site previews: branch dev deploys as Worker ttacommunity-dev"` | the same `echo` |
| `ttacommunity-dev`, the dev site (`dev.ttacommunity.org`) | `dev` | `/web` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then pnpm --filter @ttac/site build; else echo "ttacommunity-dev builds from branch dev only"; exit 1; fi` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then npx wrangler@4.141.0 deploy --config site/wrangler.jsonc --env dev; else echo "ttacommunity-dev builds from branch dev only"; exit 1; fi` |
| `ttac-admin` (`admin.ttacommunity.org`) | `main` | `/web` | `pnpm --filter @ttac/admin build` | `npx wrangler@4.141.0 deploy --config admin/wrangler.jsonc` |
| `ttac-admin` Previews (`dev.admin.ttacommunity.org`) | all but `main` | `/web` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then pnpm --filter @ttac/admin build; else echo "Previews are built for branch dev only"; fi` | `if [ "$WORKERS_CI_BRANCH" = dev ]; then npx wrangler@4.141.0 preview --config admin/wrangler.jsonc; else echo "Previews are built for branch dev only"; fi` |
| `ttac-auth`, Discord sign-in for Access (`auth.ttacommunity.org`) | `main` | `/infra/access-oidc` | none | `npx wrangler deploy` |
| `build-discord-notifier` (posts `main` builds to Discord with secret `DISCORD_WEBHOOK_URL`, and `dev` builds to their own channel with `DISCORD_DEV_WEBHOOK_URL`; without it, `dev` builds are not posted) | `main` | `/infra/build-notifier` | none | `npx wrangler deploy` |

**The site has no Previews**: on the zone's apex a Preview route claims every
subdomain (`*.ttacommunity.org`), the API's included. Branch `dev` is a
Worker of its own instead, `ttacommunity-dev`, from the site's `env.dev`.
**Keep the site's non-production trigger, which only echoes**: with none, a
push to another branch lands on the production trigger, and retrying that
build deploys the branch to production. The API is `api.ttacommunity.org`,
and `dev`'s `api-dev.ttacommunity.org`, through tunnels ("The API" below).

Build variables, set in each site Worker's build settings:

| Variable | Worker | Value | Why |
|---|---|---|---|
| `PUBLIC_API_URL` | `ttacommunity`, `ttacommunity-dev` | `https://api.ttacommunity.org`; on `dev`, `https://api-dev.ttacommunity.org` | Where the browser reads ratings, and the CSP's `connect-src`. Sent to the browser, so never a secret |
| `PUBLIC_ADMIN_URL` | `ttacommunity`, `ttacommunity-dev` | `https://admin.ttacommunity.org`; on `dev`, `https://dev.admin.ttacommunity.org` | Where the header, footer and home page link to admin |
| `PUBLIC_TURNSTILE_SITEKEY` | `ttacommunity`, `ttacommunity-dev` | `0x4AAAAAAFeTS4FLImf-aXyC` (the widget "ttacommunity email sign-in") | The email form's Turnstile widget. Public. Without it the build uses Cloudflare's test key, whose tokens the api refuses: email sign-in fails closed |
| `SITE_NOINDEX` | `ttacommunity-dev` | `1` | Adds `X-Robots-Tag: noindex`, so the dev site is never indexed |
| `NODE_VERSION` | the site Workers and admin | the `node` version in `mise.toml` | The build image's default is older |
| `PNPM_VERSION` | the site Workers and admin | the pnpm version in `web/package.json` | The build image does not read `packageManager` |

When importing a Worker, **turn off its non-production (preview) builds**
unless the table lists them. Admin's are for branch `dev` only, but they
have no branch filter, so both their build and deploy commands skip every
other branch: a push elsewhere runs a short build that does nothing and
passes. A preview build reports on the pull request's commit, and a failure
there blocks the merge. Previews Base has its own build variables; don't
click "Enable Preview URLs", which turns on `workers.dev`. A Preview on a
custom domain is `<branch>.<route host>`, and the route sets
`previews_enabled`. It appears only once a production deploy has attached
that route, and each Preview build also publishes `<version>.<route host>`.

**The zone answers only on a fixed list of hostnames**: a WAF custom rule on
`ttacommunity.org` blocks every other host, Preview version hosts included.
A new public hostname is added to that rule first.

`ttac-auth` needs one runtime secret, `DISCORD_CLIENT_SECRET`, set under the
Worker's Variables and Secrets (not its build variables).

`ttac-mail-forward` (`infra/mail-forward`) is the Email Worker that Email
Routing's rules for support@, privacy@ and accounts@ (the shared login for
outside services) send mail to; it forwards each
message to every address in its secret `FORWARD_TO` (comma-separated, each a
verified Email Routing destination). It has no route and no build trigger: a
maintainer deploys it by hand, from that folder, with `npx wrangler deploy`,
and sets the secret with `printf %s '<addresses>' | npx wrangler secret put
FORWARD_TO`. A new address the site gives out is a routing rule to this
Worker, and a line in its `ROUTED` list.

**`ttac-admin` is only reachable through Cloudflare Access.** Its Access
application must exist before its domain is declared, and it covers
`*.admin.ttacommunity.org` before previews are turned on: `dev`'s
preview is `dev.admin.ttacommunity.org`.

### The Secrets and Cloudflare's settings are backed up nightly

The CronJob `backup-config` (`deploy/backup-config.yaml`) runs at 03:30 UTC:
`load -backup-config-to` exports the namespace's Secrets and the Cloudflare
settings that are not in git (Access, DNS, Turnstile, Email Routing, the
zone's rulesets, the Workers' names, the R2 buckets' rules) as JSON, encrypts
each with `age` to the maintainers' key, and rclone puts them in the R2
bucket `ttacommunity-config-backup` as `secrets/<time>.json.age` and
`cloudflare/<time>.json.age`. Nothing unencrypted touches a disk. It needs
the Secrets `cloudflare-readonly` and `backup-config-r2` (`deploy/SECRETS.md`).
A copy is kept for 90 days. To take one now:

```bash
kubectl -n ttacommunity create job --from=cronjob/backup-config backup-config-manual-1
kubectl -n ttacommunity logs job/backup-config-manual-1 --all-containers -f
```

Restoring, for a holder of the private key: list the copies with
`rclone lsjson r2:ttacommunity-config-backup -R` (an `r2` remote on the
bucket's token), then
`rclone cat r2:ttacommunity-config-backup/secrets/<time>.json.age | age -d -i <key file> | kubectl apply --dry-run=server -f -`,
and the same without `--dry-run`. The Cloudflare copy decrypts to the
exact values; what each one recreates, and what is reissued by hand
instead, is in `docs/architecture.md`, "What only Cloudflare holds".

### Admin

Admin is the `ttac-admin` site and the API's `/v1/admin/` routes:

- **The site's Worker** serves the pages, and proxies `/api/*` to
  `/v1/admin/*` on `API_ORIGIN` after verifying the Access token itself.
  It does so only on `ADMIN_HOST` and refuses any other host. Both are
  vars in `web/admin/wrangler.jsonc`: production uses `api.ttacommunity.org`,
  the `dev` preview (`previews.vars`) `api-dev.ttacommunity.org`
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
live when a DNS record points at the tunnel and its ingress rule names it:

```bash
cloudflared tunnel route dns ttac-api api.ttacommunity.org
```

A WAF rate-limiting rule on that hostname comes first. cloudflared reads its
ingress rules only when it starts, so restart it after changing them:
`kubectl -n ttacommunity rollout restart deployment/ttac-api-tunnel`.

#### Find an error someone reports

The API sets an `X-Request-Id` header on every answer. Admin's proxy passes
it on; on the site only the browser's developer tools show it, as the
site's code cannot read it across origins. A server error (500) also puts
it in its body as `request_id` and in its log line, and that is where
people see it: the site and admin show it as "ref …" in the error's
message. The id is random, so it tells nothing about who asked. Find the
error by it (`app=api-dev` on dev):

```bash
kubectl -n ttacommunity logs -l app=api --tail=-1 | grep '"request_id":"<id>"'
```

#### Metrics

The api serves Prometheus metrics at `/metrics` on a listener of its own,
`METRICS_ADDR` (`127.0.0.1:8081` locally, `:8081` in the cluster), never on
the public port. They are how long requests take by route pattern and status
(`ttacapi_http_request_duration_seconds`; status `aborted` for an answer
cut off after it started, such as an export that failed part way), each
database pool's connections
(`ttacapi_db_pool_…`), and Go's runtime. A request taking a second or more
also logs a `slow request` line. In the cluster only Prometheus may reach
that port (`deploy/networkpolicy.yaml`); its scrape config and dashboard live
with the cluster's own configuration.

```bash
curl -s http://127.0.0.1:8081/metrics | grep '^ttacapi_'
```
