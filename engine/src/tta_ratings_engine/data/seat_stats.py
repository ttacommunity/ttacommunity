"""Each player's seat statistics in a run by format and seat: games, wins, draws, culture and places, over the games
the run rated."""

from tta_ratings_engine.data.db import DictConnection, require_running


def derive_seat_stats(*, conn: DictConnection, run_id: int) -> None:
    """Records the run's seat statistics, counted from its pairs and the games' seats by the database's
    derive_player_seat_stats. Raises RunNotRunning."""
    require_running(conn, run_id)
    conn.execute("SELECT derive_player_seat_stats(%s)", (run_id,))
