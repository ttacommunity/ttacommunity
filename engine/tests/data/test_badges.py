import pytest
from tta_ratings_engine.data.badges import classify_division_tier, derive_tournament_badges

def test_badge_classification():
    """Tests tournament division to badge tier classification."""
    # Royal League
    assert classify_division_tier("RL_s08 - Emperor") == 1  # GM
    assert classify_division_tier("RL_s08 - King") == 2     # M
    assert classify_division_tier("RL_s08 - Prince A") == 3 # Platinum
    assert classify_division_tier("RL_s08 - Duke B") == 4   # Gold
    assert classify_division_tier("RL_s08 - Marquess C") == 5 # Silver
    assert classify_division_tier("RL_s08 - Count D") == 6  # Bronze
    assert classify_division_tier("RL_s08 - Viscount") == 7 # Wood

    # International Championship
    assert classify_division_tier("International S33 - Diamond") == 1  # GM
    assert classify_division_tier("International S33 - Platinum") == 2 # M
    assert classify_division_tier("International S33 - Gold") == 3     # Platinum
    assert classify_division_tier("International S33 - Silver") == 4   # Gold
    assert classify_division_tier("International S33 - Bronze") == 5   # Silver

    # Intermezzo
    assert classify_division_tier("Intermezzo S30 - Master 1") == 1    # GM
    assert classify_division_tier("Intermezzo S30 - Division 2") == 2


# End to end, on Postgres: titles derived from matches and written per run.

from datetime import date

from tta_ratings_engine.data.badges import REIGNING_WC_PLAYERS
from tta_ratings_engine.data.db import RunNotRunning


def titles(conn, run_id):
    return {
        r["player_name"]: (r["title"], r["title_count"], r["badge_reason"])
        for r in conn.execute(
            "SELECT player_name, title, title_count, badge_reason FROM player_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_a_player_gets_the_title_of_their_best_recent_division(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 5, 1), tournament="RL_s08 - Emperor")
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - King")

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = titles(conn, run_id)
    assert got["Ann"] == ("GM", 1, "Played in Royal League Emperor Season 8")
    assert got["Cat"] == ("M", 0, "Played in Royal League King Season 9")


def test_only_the_last_six_months_count(conn, add_match, run_id):
    add_match(("Old", 10), ("Bob", 5), day=date(2023, 1, 1), tournament="RL_s01 - Emperor")
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert set(titles(conn, run_id)) == {"Ann", "Cat"}


def test_games_outside_the_championships_earn_no_title(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="Friendly Cup")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}


def test_the_reigning_champion_is_titled_only_if_a_known_player(conn, add_match, run_id):
    champion = next(iter(REIGNING_WC_PLAYERS))
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    derive_tournament_badges(conn=conn, run_id=run_id)
    assert champion not in titles(conn, run_id)

    add_match((champion, 10), ("Bob", 5), day=date(2024, 6, 2), tournament="Friendly Cup")
    derive_tournament_badges(conn=conn, run_id=run_id)
    assert titles(conn, run_id)[champion][0] == "WC"


def test_deriving_again_replaces_the_runs_titles(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive_tournament_badges(conn=conn, run_id=run_id)
    derive_tournament_badges(conn=conn, run_id=run_id)

    assert len(titles(conn, run_id)) == 2


def test_a_finished_run_gets_no_titles(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    conn.execute("UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,))
    conn.commit()

    with pytest.raises(RunNotRunning):
        derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}


def test_no_matches_means_no_titles(conn, run_id):
    assert derive_tournament_badges(conn=conn, run_id=run_id) == {}
    assert titles(conn, run_id) == {}
