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


# The steps the rating loops share.

from tta_ratings_engine.models.glicko2.calculator import (
    CALIBRATED_SIGMA,
    GOLDEN_MP_WEIGHTS,
    SecondPass,
    count_new_games,
    drifted,
    match_id,
    month_encounters,
    new_rating,
    new_year,
    season_reset,
    split_at_threshold,
)
from tta_ratings_engine.models.glicko2.engine import Rating


def rated(rating, rd=100.0):
    return Rating(rating, rd, CALIBRATED_SIGMA)


def game(a, b, score_a, player_count=2, match_id=1, day="2022-01-05"):
    return {
        "player_a": a,
        "player_b": b,
        "outcome_a": score_a,
        "player_count": player_count,
        "match_id": match_id,
        "date": day,
    }


@pytest.mark.parametrize(
    ("weighted", "want"), [(True, GOLDEN_MP_WEIGHTS[3]), (False, 1.0)]
)
def test_an_encounter_carries_the_opponent_score_and_weight(weighted, want):
    ann, bob = rated(1600), rated(1400)
    by_name = {"Ann": ann, "Bob": bob}

    got = month_encounters(
        [game("Ann", "Bob", 1.0, player_count=3, match_id=7)],
        by_name.__getitem__,
        weighted,
        player_count=0,
    )

    assert got == {"Ann": [(bob, 1.0, want, 3, 7)], "Bob": [(ann, 0.0, want, 3, 7)]}


@pytest.mark.parametrize(("b_first", "want"), [(False, ["Ann", "Bob"]), (True, ["Bob", "Ann"])])  # fmt: skip
def test_b_first_looks_up_player_b_before_player_a(b_first, want):
    looked_up = []

    def rating_of(p):
        looked_up.append(p)
        return new_rating()

    got = month_encounters([game("Ann", "Bob", 0.5)], rating_of, True, 2, b_first)

    assert looked_up == want
    assert list(got) == ["Ann", "Bob"], "encounters stay in the order players play"


def test_a_pair_without_a_match_id_is_its_date_and_players():
    pair = {"player_a": "Ann", "player_b": "Bob", "date": "2022-01-05"}

    assert match_id(pair) == ("2022-01-05", "Ann", "Bob")


def test_a_game_with_several_pairs_counts_once():
    seen = {1}
    encounters = [(new_rating(), 1.0, 1.0, 3, g) for g in (1, 2, 2, 3)]

    assert count_new_games(seen, encounters) == (1, 3, [2, 3])
    assert seen == {1, 2, 3}


def test_a_month_splits_at_the_games_up_to_the_threshold():
    encounters = [(new_rating(), 1.0, 1.0, 3, g) for g in (2, 2, 3, 4)]

    up_to, after = split_at_threshold(encounters, [2, 3, 4], needed=2)

    assert [it[4] for it in up_to] == [2, 2, 3]
    assert [it[4] for it in after] == [4]


@pytest.mark.parametrize(
    ("previous", "month", "want"),
    [
        (None, "2022-01", False),
        ("2022-01", "2022-02", False),
        ("2021-12", "2022-01", True),
        ("2020-06", "2022-03", True),
    ],
)
def test_a_new_year_is_a_month_in_a_later_year(previous, month, want):
    assert new_year(previous, month) == want


@pytest.mark.parametrize(
    ("rd", "want_wider", "want_rd_at_most"),
    [(60.0, True, 350.0), (350.0, False, 350.0)],
)
def test_a_month_away_widens_the_rd_to_350_at_most(rd, want_wider, want_rd_at_most):
    got = drifted(rated(1700, rd))

    assert got.rating == 1700
    assert (got.rd > rd) == want_wider
    assert got.rd <= want_rd_at_most


@pytest.mark.parametrize(
    ("name", "rating", "eligible"),
    [("ineligible", rated(2100, 40), False), ("unrated", rated(2100, 350.0), True)],
)
def test_a_season_reset_leaves_out_players_it_should(name, rating, eligible):
    ratings = {"Ann": rating, "Bob": rated(1500, 40), "Cat": rated(1400, 40)}
    without_ann = {"Bob": rated(1500, 40), "Cat": rated(1400, 40)}

    season_reset(ratings, lambda p: eligible or p != "Ann")
    season_reset(without_ann, lambda p: True)

    assert ratings["Ann"] == rating, name
    assert {p: ratings[p] for p in without_ann} == without_ann, (
        f"{name}: not in the population"
    )


def test_a_season_reset_deflates_only_players_above_the_mean():
    ratings = {"Ann": rated(1900, 80), "Bob": rated(1500, 80), "Cat": rated(1100, 30)}

    season_reset(ratings, lambda p: True)

    assert ratings["Ann"].rating < 1900
    assert ratings["Cat"].rating == 1100
    assert ratings["Cat"].rd > 30, "the most certain player's RD still widens"


def test_a_retro_player_is_held_at_their_prior_until_their_threshold():
    prior = rated(1800, 60)
    model = SecondPass({"Ann": prior}, {"Ann": 2}, weighted=True, retro=True, player_count=2)  # fmt: skip

    model.rate_month([game("Ann", "Bob", 0.0, match_id=1)])
    model.rate_month([game("Ann", "Bob", 0.0, match_id=2)])
    held = model.ratings["Ann"]
    model.rate_month([game("Ann", "Bob", 0.0, match_id=g) for g in (3, 4)])

    assert held == prior, "held through game 2, the threshold"
    assert model.ratings["Ann"].rating < prior.rating, "games 3 and 4 are past it"


@pytest.mark.parametrize(
    ("retro", "want_drift"),
    [(False, True), (True, False)],
)
def test_only_a_calibrated_player_drifts_in_a_month_away(retro, want_drift):
    model = SecondPass({}, {"Ann": 15}, weighted=retro, retro=retro, player_count=2)
    model.rate_month([game("Ann", "Bob", 1.0)])
    before = model.ratings["Ann"]

    model.rate_month([game("Cat", "Dan", 1.0, match_id=2)])

    assert (model.ratings["Ann"].rd > before.rd) == want_drift


# End to end, on Postgres: a small synthetic season through the calculator.

from datetime import date

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.glicko2.calculator import (
    compute_glicko2_ratings,
    model_name,
)


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
    compute_glicko2_ratings(engine="glicko2_std", conn=conn, run_id=run_id)

    types = {
        r["model_type"]
        for r in conn.execute("SELECT DISTINCT model_type FROM player_ratings")
    }
    assert types == {"glicko2_daneo_softer", "glicko2_std"}


@pytest.mark.parametrize(
    ("engine", "reset_mode", "want"),
    [
        ("glicko2_daneo", "continuous", "glicko2_daneo"),
        ("glicko2_daneo", "softer", "glicko2_daneo_softer"),
        ("glicko2_std", "continuous", "glicko2_std"),
        ("glicko2_std", "softer", "glicko2_std_softer"),
    ],
)
def test_a_models_name_is_its_engine_and_reset(engine, reset_mode, want):
    assert model_name(engine, reset_mode) == want


@pytest.mark.parametrize(
    ("engine", "reset_mode"),
    [
        ("glicko2_daneo", "soft"),
        ("glicko2_daneo", "amplified"),
        ("glicko2_std", ""),
        ("glicko2_mp", "continuous"),
        ("whr", "continuous"),
    ],
)
def test_an_unknown_engine_or_reset_is_refused(engine, reset_mode):
    with pytest.raises(ValueError, match="unknown"):
        model_name(engine, reset_mode)


def test_an_unknown_reset_writes_nothing(conn, add_match, run_id):
    season(add_match)
    expand_pairwise(conn, run_id)

    with pytest.raises(ValueError, match="unknown"):
        compute_glicko2_ratings(
            engine="glicko2_daneo", reset_mode="soft", conn=conn, run_id=run_id
        )

    assert conn.execute("SELECT count(*) AS n FROM player_ratings").fetchone()["n"] == 0


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
