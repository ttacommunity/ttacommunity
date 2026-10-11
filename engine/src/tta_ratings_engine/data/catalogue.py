"""The catalogue's spans for a run: each series', season's and stage's games, players and first and last game."""

from tta_ratings_engine.data.db import DictConnection, require_running


def derive_catalogue(*, conn: DictConnection, run_id: int) -> None:
    """Records the run's catalogue spans, counted by the database's derive_catalogue_spans. Raises RunNotRunning."""
    require_running(conn, run_id)
    conn.execute("SELECT derive_catalogue_spans(%s)", (run_id,))
