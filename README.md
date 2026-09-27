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
| `all_scores_zero` | Every score is 0, so there is no result to rate |
| `duplicate` | The same tournament, date, players and scores as an earlier row, in any file. The first copy is loaded |
| `column_count`, `no_tournament`, `bad_date`, `bad_score`, `player_count`, `player_gap`, `score_without_player`, `player_without_score`, `same_player_twice` | The row is malformed |

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
