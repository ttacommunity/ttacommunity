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
| `data/achievements.py` | `data/hall_of_fame.py`, `data/parse_tournaments.py` | Each player's medal summary from the four major circuits' boards, and the records their achievements page lists, by upstream's rules, from the standings, the matches and the recorded facts |
| `data/series_pages.py` | `data/hall_of_fame.py` | The series counts, the World Championship, Grand Slams and Ladders boards, and the Hall of Fame's all-time table, as upstream derives them |
| `data/db.py` | same | Connections, and the reads and writes the models share. The schema is the migrations in `backend/migrations/ratings` |
| `run.py`, `progress.py` | `run_pipeline.py` | One run of every model: see below |
| `tests/` | `tests/` | Upstream's pure tests unchanged. The database tests are rewritten for Postgres |

**Tournament results come from the matches.** What a match name cannot say
is a recorded fact the load job loads from our own CSVs, each row naming its
source: a division where the name gives none (International S3 to S27 are
named `International S20`), an official placement (a season with no games,
or a podium the original site hard-codes), a season correction, a season's
finish date, or a player's career honours from an organisers' Hall of Fame.
No spreadsheet is read.

The four files upstream's math lives in, and their tests, are exempt from
`ruff` until they are refactored (`pyproject.toml` lists them).

## Running the engine

It needs a loaded ratings database (see "Load ratings data" in the main
`README.md`):

```sh
set -a; . ../.env; set +a
uv run python -m tta_ratings_engine.run
```

A run computes three models (`MODELS` in `run.py`): Glicko-D continuous and
with a season reset, which the site shows, and standard Glicko-2 as the
benchmark; for all formats together and for 2, 3 and 4 players, then
tournament results and titles, then the standard and walk-forward
calibration: about 3 minutes on a desktop (the 16 models of engine 1.3.0 and
before, upstream's full set, took 20). The run's input fingerprint covers the
matches and every recorded tournament fact.

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
- `--tournaments-only` copies the current run's ratings and calibration and
  recomputes only tournament results and titles: 2m28s against ~22 min, for trying
  a tournament change. It refuses if the matches changed since that run
  (its pairs must be identical), and the run records which run it copied
  (`copied_from`)
- `--workers 4` runs the steps after the pairs (each format's models,
  calibration and walk-forward, and the tournaments) in 4 processes: 9m17s
  against 22m13s on 8 cores, about 2.3 GiB for the four together, and every
  table identical to a run with one worker. Walk-forward's months stay in
  order: each month starts from the last one's ratings
- It refuses to start with no matches loaded

## Versions

Every run records the engine's version, from `pyproject.toml`. A change that
moves ratings is a new version: bump it, and add the new version's hash to
`tests/golden.json`. `test_the_results_match_the_engine_versions_golden_hash`
rates a small invented season in every format and hashes every result, so a
change that moves ratings without a new version fails, and its message gives
the hash. A refactor keeps the hash. A version's hash never changes once it
is on `main` (`checks/engine-golden.sh`).

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
