# engine

The rating engine: DANeo's Glicko-2 and Whole-History Rating models, and the
calculations that feed them. It reads the loaded matches from the ratings
database and writes each set of results as a new **rating run** there.

## Where it comes from

Copied without history from
[TTA-Custom-Glicko-Community-Page](https://github.com/DANeoDev/TTA-Custom-Glicko-Community-Page)
at **`5a8fdbc`**. A change made upstream after that commit is copied across by
hand, and this line is updated to the upstream commit we are at.

Upstream imports `src` itself as the package. Here it is `tta_ratings_engine`,
in pytest's `src` layout, so `src/models/` upstream is
`src/tta_ratings_engine/models/` here.

**The rating math is upstream's, unchanged.** What changed is how the code
reads and writes: Postgres instead of SQLite, and every result goes into a
run.

| Here, under `src/tta_ratings_engine/` | Upstream, under `src/` | Changes |
|---|---|---|
| `models/glicko2/engine.py`, `adaptive_t.py`, `models/whr/engine.py`, `solver.py` | same | Import lines only |
| `models/glicko2/calculator.py`, `models/whr/calculator.py` | same | Read the run's pairs and write the run's ratings and history. Linted and formatted |
| `models/evaluation/` | same | The same, for calibration. `standard_eval.py` no longer writes a JSON cache, and walk-forward reports each month as it finishes. The command-line entry points are gone: `run.py` replaces them |
| `data/badges.py` | same | Titles go into the run's `player_titles`, not onto `players` |
| `data/pairwise.py` | `data/loader.py` | Reads `matches` from the database instead of a CSV, and carries `glicko_eligible` into each pair. The expansion is upstream's |
| `data/tournaments.py` | `data/hall_of_fame.py`, `data/parse_tournaments.py` | Reads the matches and the recorded facts instead of spreadsheets: each game's series, season and division, group standings by each league's rules, the league trophyboards and upstream's seasonal records |
| `data/series_pages.py` | `data/hall_of_fame.py` | The series counts, the World Championship, Grand Slams and Ladders boards, and the Hall of Fame's all-time table, as upstream derives them |
| `data/db.py` | same | Connections, and the reads and writes the models share. The schema is the migrations in `backend/migrations/ratings` |
| `run.py`, `progress.py` | `run_pipeline.py` | One run of every model: see below |
| `tests/` | `tests/` | Upstream's pure tests unchanged. The database tests are rewritten for Postgres |

**Tournament results come from the matches.** What a match name cannot say
is a recorded fact the load job loads from our own CSVs, each row naming its
source: a division where the name gives none (International S3 to S27 are
named `International S20`), an official placement (a season with no games,
or a podium the original site hard-codes), or a season correction. No
spreadsheet is read.

The four files upstream's math lives in, and their tests, are exempt from
`ruff` until they are refactored (`pyproject.toml` lists them).

## Running the engine

It needs a loaded ratings database (see "Load ratings data" in the main
`README.md`) and says which engine version it is:

```sh
set -a; . ../.env; set +a
ENGINE_VERSION=$(git rev-parse --short HEAD) uv run python -m tta_ratings_engine.run
```

A run computes the 16 models upstream's site shows, for all formats together
and for 2, 3 and 4 players, then tournament results and titles, then the
standard and walk-forward calibration: about 20 minutes and 1 GB of memory on
a desktop. The run's input fingerprint covers the matches and any recorded
divisions.

It logs progress, elapsed time and the time left every 30 seconds, and a
"still on" line when one step runs longer. The time left weighs each step by
how long that kind of step took in a measured run (`STEP_COST` in `run.py`),
scaled to how fast this run is going. A run ends by logging each phase's
time: if the estimates drift, update `STEP_COST` from those lines.

- **A run is never published by itself.** It ends `complete` (or `failed`,
  with the error), and the site keeps showing the current run. Add
  `--make-current` to publish it when it completes
- `--formats 2` rates only 2-player games, which is much quicker for trying
  a change
- It refuses to start with `ENGINE_VERSION` unset, or with no matches loaded

## Running the tests

The tests need a Postgres server to create throwaway databases on. They
**fail, never skip,** without one:

```sh
scripts/ci.sh --only engine          # starts one in a container, from the repo root
TEST_DATABASE_URL=postgres://user:password@host:port/db uv run pytest   # your own
```

Each test gets a fresh database with the ratings migrations applied, and drops
it afterwards. No match data is needed, and none is kept in this repository.

**The suite stays quick: seconds, not minutes.** Tests use a few synthetic
matches. A full run on real data is never part of CI.

`tests/` mirrors the package: the tests for
`src/tta_ratings_engine/models/whr/engine.py` are in
`tests/models/whr/test_engine.py`. pytest runs in `importlib` import mode, so
two test files can share a name in different folders.
