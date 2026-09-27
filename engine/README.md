# engine

The rating engine: DANeo's Glicko-2 and Whole-History Rating models, and the
calculations that feed them. It reads and writes only its database.

## Where it comes from

Copied without history from
[TTA-Custom-Glicko-Community-Page](https://github.com/DANeoDev/TTA-Custom-Glicko-Community-Page)
at **`5a8fdbc`**. A change made upstream after that commit is copied across by
hand, and this line is updated to the upstream commit we are at.

Upstream imports `src` itself as the package. Here it is `tta_ratings_engine`,
in pytest's `src` layout, so `src/models/` upstream is
`src/tta_ratings_engine/models/` here. Only the import lines differ.

| Here, under `src/tta_ratings_engine/` | Upstream, under `src/` | Changes |
|---|---|---|
| `models/` | `models/` | `standard_eval.py` no longer writes a JSON cache; results stay in the database |
| `data/badges.py` | same | none |
| `data/pairwise.py` | `data/loader.py` | Reads `matches` from the database instead of a CSV, and carries `glicko_eligible` into each pair. The expansion is upstream's |
| `data/db.py` | same | Only the engine's tables. The path comes from `ENGINE_DB_PATH`, with no default |
| `tests/` | `tests/` | Only the tests that need neither the website nor the real match data. Every test kept is unchanged, but moved into the layout below |

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

`tests/` mirrors the package: the tests for
`src/tta_ratings_engine/models/whr/engine.py` are in
`tests/models/whr/test_engine.py`. pytest runs in `importlib` import mode, so
two test files can share a name in different folders.
