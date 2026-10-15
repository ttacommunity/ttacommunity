"""Each player's record in a run by format and year: opponents met, wins, losses, draws and their last game."""

from tta_ratings_engine.data.db import DictConnection, require_running


def derive_year_records(*, conn: DictConnection, run_id: int) -> None:
    """Records the run's player records by format and year, counted from its pairs by the database's
    derive_player_year_records. Raises RunNotRunning."""
    require_running(conn, run_id)
    conn.execute("SELECT derive_player_year_records(%s)", (run_id,))
