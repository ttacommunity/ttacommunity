"""The analysis pages' counts for a run: seat results by skill tier and games and pairs per format, over the games
the run rated."""

from tta_ratings_engine.data.db import DictConnection, require_running


def derive_analysis_counts(*, conn: DictConnection, run_id: int) -> None:
    """Records the run's seat results by tier and its games per format, counted by the database's
    derive_seat_tier_stats and derive_run_format_counts. A game's tier comes from the run's Glicko-D history, so
    this runs after the models. Raises RunNotRunning."""
    require_running(conn, run_id)
    conn.execute("SELECT derive_seat_tier_stats(%s)", (run_id,))
    conn.execute("SELECT derive_run_format_counts(%s)", (run_id,))
