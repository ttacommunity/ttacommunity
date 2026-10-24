from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.data.seat_stats import derive_seat_stats


def stats(conn, run_id):
    """The run's seat statistics by player, format and seat: games, wins, draws, culture and each place's count."""
    rows = conn.execute(
        "SELECT player_name, player_count, seat, games, wins, draws, avg_culture, p1, p2, p3, p4 "
        "FROM player_seat_stats WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    return {
        (r["player_name"], r["player_count"], r["seat"]): (
            r["games"], r["wins"], r["draws"], float(r["avg_culture"]), r["p1"], r["p2"], r["p3"], r["p4"],
        )
        for r in rows
    }  # fmt: skip


def test_a_run_counts_each_players_seats_in_the_games_it_rated(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 1, 1))
    add_match(("Ann", 7), ("Bob", 7), day=date(2024, 2, 1))
    add_match(("Bob", 12), ("Ann", 4), ("Cat", 1), day=date(2024, 3, 1))
    add_match(("Ann", 20), ("Bob", 1), day=date(2024, 4, 1), state="never_played")
    expand_pairwise(conn, run_id)

    derive_seat_stats(conn=conn, run_id=run_id)

    assert stats(conn, run_id) == {
        ("Ann", 2, 1): (2, 1, 1, 8.5, 2, 0, 0, 0),
        ("Bob", 2, 2): (2, 0, 1, 6.0, 1, 1, 0, 0),
        ("Bob", 3, 1): (1, 1, 0, 12.0, 1, 0, 0, 0),
        ("Ann", 3, 2): (1, 0, 0, 4.0, 0, 1, 0, 0),
        ("Cat", 3, 3): (1, 0, 0, 1.0, 0, 0, 1, 0),
    }


def test_a_finished_run_gets_no_seat_stats(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5))
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )

    with pytest.raises(RunNotRunning):
        derive_seat_stats(conn=conn, run_id=run_id)
