from datetime import date

import pytest

from tta_ratings_engine.data.analysis_counts import derive_analysis_counts
from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise


def formats(conn, run_id):
    rows = conn.execute(
        "SELECT player_count, games, pairs FROM run_format_counts WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    return {r["player_count"]: (r["games"], r["pairs"]) for r in rows}


def seats_by_tier(conn, run_id):
    rows = conn.execute(
        "SELECT tier, sum(seats)::int AS n FROM seat_tier_stats WHERE run_id = %s GROUP BY tier",
        (run_id,),
    ).fetchall()
    return {r["tier"]: r["n"] for r in rows}


def test_a_run_counts_its_games_per_format_and_seats_per_tier(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 1, 1))
    add_match(("Ann", 7), ("Bob", 7), day=date(2024, 2, 1))
    add_match(("Bob", 12), ("Ann", 4), ("Cat", 1), day=date(2024, 3, 1))
    add_match(("Ann", 20), ("Bob", 1), day=date(2024, 4, 1), state="never_played")
    expand_pairwise(conn, run_id)

    derive_analysis_counts(conn=conn, run_id=run_id)

    assert formats(conn, run_id) == {2: (2, 2), 3: (1, 3)}
    assert seats_by_tier(conn, run_id) == {"advanced": 7}


def test_a_finished_run_gets_no_analysis_counts(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5))
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )

    with pytest.raises(RunNotRunning):
        derive_analysis_counts(conn=conn, run_id=run_id)
