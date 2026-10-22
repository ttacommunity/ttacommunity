import pytest

from tta_ratings_engine.models.glicko2.calculator import (
    PlayerThresholds,
    compute_player_thresholds,
    get_tier_mean,
)


def pair(a, b, day, match_id):
    return {"player_a": a, "player_b": b, "date": day, "match_id": match_id}


def test_thresholds_follow_the_games_seen_so_far():
    january = [pair("Ann", "Bob", "2022-01-05", i) for i in range(5)]
    a_year_on = [pair("Cat", "Dan", "2023-03-01", 99)]
    thresholds = PlayerThresholds(standard_threshold=15, inactivity_days=365)

    thresholds.add(january)
    assert thresholds.get("Ann", 15) == 15, "five games and still active"
    thresholds.add(a_year_on)
    assert thresholds.get("Ann", 15) == 5, "five games, then a year away"
    assert thresholds.get("Eve", 15) == 15, "never seen"
    assert compute_player_thresholds(january + a_year_on) == {
        "Ann": 5,
        "Bob": 5,
        "Cat": 15,
        "Dan": 15,
    }


def test_tier_regression_rules():
    """Tests 16-tier dynamic phi-quantile hierarchy and deflation-only reset rules."""
    # 1. SuperGM 2 (z >= 3.0, >= 2025 with sigma=175) -> deflates toward SuperGM 1 lower bound (1500 + phi^2 * 175)
    mean_val = get_tier_mean(2050.0)
    assert abs(mean_val - (1500.0 + (1.61803398875**2) * 175.0)) < 1e-4

    # 2. SuperGM 1 (phi^2 <= z < 3.0) -> deflates toward GM 2 lower bound (1500 + 2.0 * 175 = 1850)
    assert abs(get_tier_mean(1980.0) - 1850.0) < 1e-4

    # 3. GM 2 (2.0 <= z < phi^2) -> deflates toward GM 1 lower bound (1500 + phi * 175 ≈ 1783.16)
    assert abs(get_tier_mean(1900.0) - (1500.0 + 1.61803398875 * 175.0)) < 1e-4

    # 4. Platinum 1 (0.0 <= z < phi^-2) -> deflates toward mean (1500.0)
    assert get_tier_mean(1520.0) == 1500.0

    # 5. Deflation-Only Invariant: Below or at mean (<= 1500), no upward regression
    assert get_tier_mean(1500.0) == 1500.0
    assert get_tier_mean(1400.0) == 1400.0
    assert get_tier_mean(1200.0) == 1200.0
    assert get_tier_mean(900.0) == 900.0


def test_daneo_golden_ratio_weights_and_invariant():
    """Tests the Golden Ratio multiplayer information weights and dimensional invariant."""
    from tta_ratings_engine.models.glicko2.calculator import GOLDEN_MP_WEIGHTS, PHI

    # Check weights
    assert abs(GOLDEN_MP_WEIGHTS[2] - 1.0) < 1e-6
    assert abs(GOLDEN_MP_WEIGHTS[3] - PHI / 2.0) < 1e-6
    assert abs(GOLDEN_MP_WEIGHTS[4] - (PHI**2) / 3.0) < 1e-6

    # Check redundancy invariant Delta(N) == phi^-2
    phi_inv_sq = 1.0 / (PHI**2)
    delta_3 = (3 - 1) - (GOLDEN_MP_WEIGHTS[3] * 2)  # 2 - phi
    delta_4 = (4 - 1) - (GOLDEN_MP_WEIGHTS[4] * 3)  # 3 - phi^2
    assert abs(delta_3 - phi_inv_sq) < 1e-6
    assert abs(delta_4 - phi_inv_sq) < 1e-6


# End to end, on Postgres: a small synthetic season through the calculator.

from datetime import date

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.glicko2.calculator import compute_glicko2_ratings


def season(add_match):
    for month in (1, 2, 3):
        add_match(("Alice", 100), ("Bob", 50), day=date(2024, month, 5))
        add_match(("Alice", 100), ("Carol", 50), day=date(2024, month, 12))
        add_match(("Bob", 70), ("Carol", 70), day=date(2024, month, 19))
    add_match(("Alice", 10), ("Bob", 20), ("Dave", 30), day=date(2024, 3, 25))


def ratings(conn, run_id, model_type, player_count):
    return conn.execute(
        "SELECT player_name, rank, wins, losses, draws, opponents_count, last_played, rating, c_rating "
        "FROM player_ratings WHERE run_id = %s AND model_type = %s AND player_count = %s ORDER BY rank",
        (run_id, model_type, player_count),
    ).fetchall()


def test_ratings_are_ranked_by_conservative_rating(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(player_count=0, conn=conn, run_id=run_id)

    rows = ratings(conn, run_id, "glicko2_std", 0)
    assert [r["rank"] for r in rows] == [1, 2, 3, 4]
    assert [r["c_rating"] for r in rows] == sorted(
        (r["c_rating"] for r in rows), reverse=True
    )
    regulars = [r["player_name"] for r in rows if r["player_name"] != "Dave"]
    assert regulars == ["Alice", "Bob", "Carol"]


def test_player_stats_count_every_pair(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(player_count=0, conn=conn, run_id=run_id)

    bob = next(
        r for r in ratings(conn, run_id, "glicko2_std", 0) if r["player_name"] == "Bob"
    )
    # 3 losses to Alice, 3 draws with Carol, and a 3-player game: beat Alice, lost to Dave.
    assert (bob["wins"], bob["losses"], bob["draws"], bob["opponents_count"]) == (
        1,
        4,
        3,
        8,
    )
    assert bob["last_played"] == date(2024, 3, 25)


def test_a_format_only_ranks_players_who_played_it(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(player_count=3, conn=conn, run_id=run_id)

    names = {r["player_name"] for r in ratings(conn, run_id, "glicko2_std", 3)}
    assert names == {"Alice", "Bob", "Dave"}


def test_history_has_a_point_per_player_per_month(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(player_count=2, conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT player_name, period_date FROM rating_history "
        "WHERE run_id = %s AND model_type = 'glicko2_std' AND player_count = 2 AND player_name = 'Alice' "
        "ORDER BY period_date",
        (run_id,),
    ).fetchall()
    assert [r["period_date"] for r in rows] == [
        date(2024, 1, 19),
        date(2024, 2, 19),
        date(2024, 3, 19),
    ]


def test_the_model_type_names_the_engine_and_reset(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(
        engine="glicko2_daneo", reset_mode="softer", conn=conn, run_id=run_id
    )
    compute_glicko2_ratings(
        weighted=True, retro_calibrated=True, conn=conn, run_id=run_id
    )

    types = {
        r["model_type"]
        for r in conn.execute("SELECT DISTINCT model_type FROM player_ratings")
    }
    assert types == {"glicko2_daneo_softer", "glicko2_mp_retro"}


def test_ineligible_games_do_not_move_ratings(conn, add_match, run_id):
    add_match(("Alice", 100), ("Bob", 50), day=date(2024, 1, 5))
    add_match(("Bob", 100), ("Alice", 50), day=date(2024, 1, 6), eligible=False)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(conn=conn, run_id=run_id)

    alice, bob = ratings(conn, run_id, "glicko2_std", 0)
    assert (alice["player_name"], bob["player_name"]) == ("Alice", "Bob")
    assert alice["rating"] > 1500 > bob["rating"]


def test_computing_again_replaces_the_models_rows_in_the_run(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    compute_glicko2_ratings(conn=conn, run_id=run_id)
    compute_glicko2_ratings(conn=conn, run_id=run_id)

    assert len(ratings(conn, run_id, "glicko2_std", 0)) == 4


def test_another_runs_results_are_left_alone(conn, add_match, run_id):
    season(add_match)
    other = conn.execute(
        "INSERT INTO rating_runs (engine_version, input_fingerprint) VALUES ('old', 'old') RETURNING run_id"
    ).fetchone()["run_id"]
    conn.commit()
    expand_pairwise(conn, other)
    expand_pairwise(conn, run_id)
    compute_glicko2_ratings(conn=conn, run_id=other)

    compute_glicko2_ratings(conn=conn, run_id=run_id)

    assert len(ratings(conn, other, "glicko2_std", 0)) == 4
    assert len(ratings(conn, run_id, "glicko2_std", 0)) == 4


def test_a_finished_run_is_never_written_to(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        compute_glicko2_ratings(conn=conn, run_id=run_id)

    assert conn.execute("SELECT count(*) AS n FROM player_ratings").fetchone()["n"] == 0


def test_a_run_that_does_not_exist_is_refused(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    with pytest.raises(RunNotRunning):
        compute_glicko2_ratings(conn=conn, run_id=run_id + 1000)
