from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.whr.calculator import compute_whr_ratings


def season(add_match):
    for month in (1, 2, 3):
        add_match(("Alice", 100), ("Bob", 50), day=date(2024, month, 5))
        add_match(("Alice", 100), ("Carol", 50), day=date(2024, month, 12))
        add_match(("Bob", 70), ("Carol", 70), day=date(2024, month, 19))
    add_match(("Alice", 10), ("Bob", 20), ("Dave", 30), day=date(2024, 3, 25))


def ratings(conn, run_id, model_type="whr", player_count=0):
    return conn.execute(
        "SELECT player_name, rank, rating, c_rating, wins, losses, draws, last_played "
        "FROM player_ratings WHERE run_id = %s AND model_type = %s AND player_count = %s "
        "ORDER BY rank",
        (run_id, model_type, player_count),
    ).fetchall()


def history(conn, run_id, name):
    return conn.execute(
        "SELECT period_date FROM rating_history "
        "WHERE run_id = %s AND model_type = 'whr' AND player_name = %s ORDER BY period_date",
        (run_id, name),
    ).fetchall()


def test_ratings_are_ranked_by_conservative_rating(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    rows = ratings(conn, run_id)
    assert [r["rank"] for r in rows] == [1, 2, 3, 4]
    assert [r["c_rating"] for r in rows] == sorted(
        (r["c_rating"] for r in rows), reverse=True
    )
    regulars = [r["player_name"] for r in rows if r["player_name"] != "Dave"]
    assert regulars == ["Alice", "Bob", "Carol"]


def test_player_stats_count_every_pair(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    bob = next(r for r in ratings(conn, run_id) if r["player_name"] == "Bob")
    assert (bob["wins"], bob["losses"], bob["draws"]) == (1, 4, 3)
    assert bob["last_played"] == date(2024, 3, 25)


def test_history_has_a_point_per_day_played(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    days = [r["period_date"] for r in history(conn, run_id, "Carol")]
    assert days == [date(2024, m, d) for m in (1, 2, 3) for d in (12, 19)]


def test_a_format_only_ranks_players_who_played_it(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(player_count=3, conn=conn, run_id=run_id, verbose=False)

    names = {r["player_name"] for r in ratings(conn, run_id, player_count=3)}
    assert names == {"Alice", "Bob", "Dave"}


def test_the_softer_reset_is_its_own_model(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)
    compute_whr_ratings(reset_mode="softer", conn=conn, run_id=run_id, verbose=False)

    assert len(ratings(conn, run_id, "whr")) == 4
    assert len(ratings(conn, run_id, "whr_softer")) == 4


def test_ineligible_games_do_not_move_ratings(conn, add_match, run_id):
    add_match(("Alice", 100), ("Bob", 50), day=date(2024, 1, 5))
    add_match(("Bob", 100), ("Alice", 50), day=date(2024, 1, 6), eligible=False)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    alice, bob = ratings(conn, run_id)
    assert (alice["player_name"], bob["player_name"]) == ("Alice", "Bob")
    assert alice["rating"] > 1500 > bob["rating"]


def test_computing_again_replaces_the_models_rows_in_the_run(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)
    compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    assert len(ratings(conn, run_id)) == 4
    assert len(history(conn, run_id, "Carol")) == 6


def test_a_finished_run_is_never_written_to(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        compute_whr_ratings(conn=conn, run_id=run_id, verbose=False)

    assert ratings(conn, run_id) == []
