# engine

The rating engine: DANeo's Glicko-2 and Whole-History Rating models, and the
calculations that feed them. It reads and writes only its database.

## Where it comes from

Copied without history from
[TTA-Custom-Glicko-Community-Page](https://github.com/DANeoDev/TTA-Custom-Glicko-Community-Page)
at **`5a8fdbc`**. A change made upstream after that commit is copied across by
hand, and this line is updated to the upstream commit we are at.

| Here | Upstream | Changes |
|---|---|---|
| `src/models/` | `src/models/` | `standard_eval.py` no longer writes a JSON cache; results stay in the database |
| `src/data/badges.py` | same | none |
| `src/data/pairwise.py` | `src/data/loader.py` | Reads `matches` from the database instead of a CSV, and carries `glicko_eligible` into each pair. The expansion is upstream's |
| `src/data/db.py` | same | Only the engine's tables. The path comes from `ENGINE_DB_PATH`, with no default |
| `tests/test_glicko2.py`, `tests/test_whr.py` | same | none |
| `tests/test_walk_forward.py`, `test_retro_calibration.py`, `test_reset_and_badges.py` | same | Only the tests that need neither the website nor the real match data. Every test kept is unchanged |

**Not here yet:** upstream's `hall_of_fame.py`, which reads the Intermezzo and
International results from a spreadsheet. It comes back when those results are
derived from the matches. The pipeline runner comes with the move to Postgres.

Upstream's files are exempt from `ruff` until they are refactored
(`pyproject.toml` lists them).

## Running the tests

```sh
uv run pytest
```

The tests build their own small databases. No match data is needed, and none is
kept in this repository.
