from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.data.year_records import derive_year_records


def records(conn, run_id):
    """The run's records by format, year and player: opponents, wins, losses, draws and last game."""
    rows = conn.execute(
        "SELECT player_count, year, name AS player, opponents, wins, losses, draws, last_played "
        "FROM player_year_records JOIN players USING (player_id) WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    return {
        (r["player_count"], r["year"], r["player"]): (
            r["opponents"],
            r["wins"],
            r["losses"],
            r["draws"],
            r["last_played"],
        )
        for r in rows
    }


def test_a_run_records_each_players_year_in_each_format(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2023, 5, 1))
    add_match(("Ann", 5), ("Bob", 5), day=date(2024, 3, 1))
    add_match(("Ann", 10), ("Bob", 5), ("Cat", 1), day=date(2024, 6, 1))
    expand_pairwise(conn, run_id)

    derive_year_records(conn=conn, run_id=run_id)

    assert records(conn, run_id) == {
        (2, 2023, "Ann"): (1, 1, 0, 0, date(2023, 5, 1)),
        (2, 2023, "Bob"): (1, 0, 1, 0, date(2023, 5, 1)),
        (2, 2024, "Ann"): (1, 0, 0, 1, date(2024, 3, 1)),
        (2, 2024, "Bob"): (1, 0, 0, 1, date(2024, 3, 1)),
        (3, 2024, "Ann"): (2, 2, 0, 0, date(2024, 6, 1)),
        (3, 2024, "Bob"): (2, 1, 1, 0, date(2024, 6, 1)),
        (3, 2024, "Cat"): (2, 0, 2, 0, date(2024, 6, 1)),
    }


def test_a_finished_run_gets_no_records(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5))
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )

    with pytest.raises(RunNotRunning):
        derive_year_records(conn=conn, run_id=run_id)
