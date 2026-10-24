from datetime import UTC, date, datetime

import pytest

from tta_ratings_engine.data.badges import (
    badge_titles,
    classify_division_tier,
    derive_tournament_badges,
    parse_badge_reason,
)
from tta_ratings_engine.data.db import RunNotRunning


def test_badge_classification():
    """Tests tournament division to badge tier classification."""
    # Royal League
    assert classify_division_tier("RL_s08 - Emperor") == 1  # GM
    assert classify_division_tier("RL_s08 - King") == 2  # M
    assert classify_division_tier("RL_s08 - Prince A") == 3  # Platinum
    assert classify_division_tier("RL_s08 - Duke B") == 4  # Gold
    assert classify_division_tier("RL_s08 - Marquess C") == 5  # Silver
    assert classify_division_tier("RL_s08 - Count D") == 6  # Bronze
    assert classify_division_tier("RL_s08 - Viscount") == 7  # Wood
    assert classify_division_tier("RL_s09 - Baron 2 game 19") == 7  # Wood (Baron)

    # International Championship
    assert classify_division_tier("International S33 - Diamond") == 1  # GM
    assert classify_division_tier("International S33 - Grandmaster") == 1  # GM
    assert classify_division_tier("International S33 - Master") == 2  # M
    assert classify_division_tier("International S33 - Master 1") == 2  # M
    assert classify_division_tier("International S33 - Master 2") == 2  # M
    assert classify_division_tier("International S33 - Platinum") == 3  # P
    assert classify_division_tier("International S33 - Gold") == 4  # G
    assert classify_division_tier("International S33 - Silver") == 5  # S
    assert classify_division_tier("International S34 - Bronze 28") == 6  # B
    assert classify_division_tier("International S33 - Wood 5") == 7  # W

    # Intermezzo
    assert classify_division_tier("Intermezzo S30 - Grandmaster") == 1  # GM
    assert classify_division_tier("Intermezzo S30 - Master 1") == 2  # M
    assert classify_division_tier("Intermezzo S30 - Platinum 1") == 3  # P
    assert classify_division_tier("Intermezzo S30 - Gold 2") == 4  # G
    assert classify_division_tier("Intermezzo S30 - Silver 3") == 5  # S
    assert classify_division_tier("Intermezzo S30 - Bronze 4") == 6  # B
    assert classify_division_tier("Intermezzo S30 - Wood 5") == 7  # W


def test_royal_league_reasons_name_the_lower_ranks():
    assert (
        parse_badge_reason("RL_s09 - Baron 2 game 19", 7)
        == "Played in Royal League Baron Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Knight", 7)
        == "Played in Royal League Knight Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Viscount", 7)
        == "Played in Royal League Viscount Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Emperor", 1)
        == "Played in Royal League Emperor Season 9"
    )


# The rules, on the games, records and overrides the run reads.

CUTOFF = "2023-06-01"  # a year before the newest game, 2024-06-01


def game(
    day,
    tournament,
    *players,
    series=None,
    division=None,
    finished_at=None,
    game_id=0,
):
    """Returns a played game as badge_titles reads it."""
    return {
        "tournament": tournament,
        "date": day,
        "players": list(players),
        "series": series,
        "division": division,
        "finished_at": finished_at,
        "game_id": game_id,
    }


def newest_first(*games):
    return sorted(games, key=lambda g: g["date"], reverse=True)


def season_record(player, tournament, season, division, placement, medal, finish):
    return {
        "player_name": player,
        "tournament": tournament,
        "season": season,
        "division": division,
        "placement": placement,
        "medal": medal,
        "finish_date": finish,
    }


def override(player, title, reason="r"):
    return {"player_name": player, "title": title, "reason": reason}


def rule_titles(games, records=(), overrides=(), champions=()):
    return badge_titles(
        newest_first(*games), CUTOFF, list(champions), list(records), list(overrides)
    )


def active(t):
    return {
        p: (d["title"], d["title_count"], d["badge_reason"])
        for p, d in t.active.items()
    }


def test_a_player_gets_the_title_of_their_best_recent_division():
    t = rule_titles(
        [
            game("2024-05-01", "RL_s08 - Emperor", "Ann", "Bob"),
            game("2024-06-01", "RL_s09 - King", "Ann", "Cat"),
        ]
    )

    assert active(t)["Ann"] == ("GM", 1, "Played in Royal League Emperor Season 8")
    assert active(t)["Cat"] == ("M", 0, "Played in Royal League King Season 9")


def test_only_the_last_year_counts():
    t = rule_titles(
        [
            game("2023-05-31", "RL_s01 - Emperor", "Old", "Bob"),
            game(CUTOFF, "RL_s02 - Duke", "Edge", "Eve"),
            game("2023-07-01", "RL_s02 - Duke", "Recent", "Dan"),
            game("2024-06-01", "RL_s09 - Duke", "Ann", "Cat"),
        ]
    )

    assert set(t.active) == {"Ann", "Cat", "Recent", "Dan", "Edge", "Eve"}


def test_a_division_the_run_recorded_gives_a_title():
    t = rule_titles(
        [
            game(
                "2024-06-01",
                "International S27",
                "Ann",
                "Bob",
                series="international",
                division="Grandmaster",
            )
        ]
    )

    assert active(t)["Ann"] == (
        "GM",
        1,
        "Played in International Championship Grandmaster Season 27",
    )


def test_games_outside_the_championships_earn_no_title():
    t = rule_titles([game("2024-06-01", "Friendly Cup", "Ann", "Bob")])

    assert t.active == {}


def test_a_player_keeps_their_career_peak_title():
    t = rule_titles(
        [
            game("2021-03-01", "RL_s03 - Emperor", "Ann", "Bob"),
            game("2022-03-01", "RL_s04 - Emperor", "Ann", "Bob"),
            game("2024-06-01", "RL_s09 - King", "Ann", "Cat"),
        ]
    )

    assert t.active["Ann"]["title"] == "M"
    assert "Bob" not in t.active
    assert t.peaks == {"Ann": ("GM", 2022), "Bob": ("GM", 2022), "Cat": ("M", 2024)}


def test_each_year_keeps_the_best_division_played():
    t = rule_titles(
        [
            game("2021-03-01", "RL_s03 - Duke", "Ann", "Bob"),
            game("2021-09-01", "RL_s04 - Emperor", "Ann", "Cat"),
            game("2022-03-01", "RL_s05 - King", "Ann", "Bob"),
            game("2023-03-01", "Friendly Cup", "Ann", "Dan"),
        ]
    )

    assert {k: v for k, v in t.yearly.items() if k[0] == "Ann"} == {
        ("Ann", 2021): "GM",
        ("Ann", 2022): "M",
    }
    assert t.yearly[("Bob", 2021)] == "G"
    assert ("Dan", 2023) not in t.yearly


def test_a_recent_top_division_win_is_super_gm():
    t = rule_titles(
        [game("2024-06-01", "RL_s09 - Emperor", "Ann", "Bob")],
        records=[
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                "1 / 8",
                "gold",
                "2024-05-27",
            ),
            season_record(
                "Bob",
                "Royal League",
                "Season 9",
                "Emperor",
                "2 / 8",
                "silver",
                "2024-05-27",
            ),
        ],
    )

    assert active(t)["Ann"] == ("SGM", 1, "Champion of Royal League Emperor (Season 9)")
    assert active(t)["Bob"] == ("GM", 1, "Played in Royal League Emperor Season 9")
    assert t.peaks == {"Ann": ("SGM", 2024), "Bob": ("GM", 2024)}


def test_an_older_top_division_win_is_a_super_gm_peak():
    ic = "International Championship"
    t = rule_titles(
        [
            game("2024-06-01", "Friendly Cup", p, "Bob")
            for p in ("Cat", "Dan", "Eve", "Fay")
        ],
        records=[
            season_record(
                "Cat", ic, "Season 20", "Grandmaster", "1 / 7", "gold", "2022-12-04"
            ),
            season_record(
                "Eve", ic, "Season 20", "Grandmaster", "3 / 7", "bronze", "2022-12-04"
            ),
            season_record(
                "Fay", ic, "Season 30", "Master 1", "1 / 4", "gold", "2025-08-14"
            ),
        ],
    )

    assert t.active == {}
    assert t.peaks == {"Cat": ("SGM", 2022)}


@pytest.mark.parametrize(
    ("placement", "medal", "want"),
    [
        ("1 / 7", "gold", "SGM"),
        ("1 / 40", "gold", "SGM"),
        ("1 / 8", None, "GM"),
        ("", None, "GM"),
    ],
)
def test_only_a_gold_medal_makes_a_top_division_win(placement, medal, want):
    t = rule_titles(
        [game("2024-06-01", "RL_s09 - Emperor", "Ann", "Bob")],
        records=[
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                placement,
                medal,
                "2024-05-27",
            )
        ],
    )

    assert (active(t)["Ann"][0], t.yearly[("Ann", 2024)]) == (want, want)


def test_records_add_super_gm_years_and_untitled_seasons():
    t = rule_titles(
        [
            game("2024-06-01", "RL_s09 - King", "Ann", "Cat"),
            game("2021-03-01", "RL_s03 - Duke", "Bob", "Cat"),
        ],
        records=[
            # a season named without its division: the record gives the year's title
            season_record(
                "Ann",
                "International Championship",
                "Season 20",
                "Grandmaster",
                "5 / 7",
                None,
                "2020-12-06",
            ),
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                "2 / 8",
                "silver",
                "2024-05-27",
            ),
            season_record(
                "Bob",
                "Intermezzo Championship",
                "Season 13",
                "Grandmaster",
                "1 / 5",
                "gold",
                "2021-11-30",
            ),
        ],
    )

    assert {k: v for k, v in t.yearly.items() if k[0] in ("Ann", "Bob")} == {
        ("Ann", 2020): "GM",
        ("Ann", 2024): "M",
        ("Bob", 2021): "SGM",
    }


@pytest.mark.parametrize(
    ("overrides", "player", "want"),
    [
        pytest.param(
            [override("Ann", "SGM")],
            "Ann",
            ("SGM", 1, "Set in admin: r"),
            id="replaces the derived title",
        ),
        pytest.param(
            [override("Bob", "")], "Bob", None, id="an empty title takes it away"
        ),
        pytest.param(
            [override("Ann", "GM", "won the 2019 final, recorded nowhere")],
            "Ann",
            ("GM", 1, "Set in admin: won the 2019 final, recorded nowhere"),
            id="gives its reason",
        ),
        pytest.param(
            [override("Cat", "GM")], "Cat", None, id="still needs a recent game"
        ),
    ],
)
def test_a_title_set_in_admin(overrides, player, want):
    t = rule_titles(
        [game("2024-06-01", "RL_s09 - Duke", "Ann", "Bob")], overrides=overrides
    )

    assert active(t).get(player) == want


# End to end, on Postgres: titles derived from matches and written per run.


def titles(conn, run_id):
    return {
        r["player_name"]: (r["title"], r["title_count"], r["badge_reason"])
        for r in conn.execute(
            "SELECT player_name, title, title_count, badge_reason FROM player_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_the_reigning_champion_is_the_latest_seasons_winner(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Marten_Fisher", "pipit4")
    place("worlds", 2025, "Final", "alder440", "kittiwake")
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    derive_tournament_badges(conn=conn, run_id=run_id)
    assert "alder440" not in titles(conn, run_id)

    add_match(
        ("alder440", 10),
        ("Marten_Fisher", 5),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )
    derive_tournament_badges(conn=conn, run_id=run_id)
    got = titles(conn, run_id)
    assert got["alder440"][0] == "WC"
    assert got.get("Marten_Fisher", ("",))[0] != "WC"


def test_deriving_again_replaces_the_runs_titles_peaks_and_years(
    conn, add_match, run_id
):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive_tournament_badges(conn=conn, run_id=run_id)
    derive_tournament_badges(conn=conn, run_id=run_id)

    assert len(titles(conn, run_id)) == 2
    assert set(peaks(conn, run_id)) == {"Ann", "Bob"}
    assert yearly(conn, run_id) == {("Ann", 2024): "G", ("Bob", 2024): "G"}


def test_a_finished_run_gets_no_titles(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}


def test_no_matches_means_no_titles(conn, run_id):
    assert derive_tournament_badges(conn=conn, run_id=run_id) == {}
    assert titles(conn, run_id) == {}


def peaks(conn, run_id):
    return {
        r["player_name"]: (r["peak_title"], r["peak_year"])
        for r in conn.execute(
            "SELECT player_name, peak_title, peak_year FROM player_peak_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_world_champions_peak_at_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2022, "Final", "Willow_Tree", "Ember_Wolf", "Linnet")
    place("worlds", 2023, "Final", "Willow_Tree")
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("Willow_Tree", 10),
        ("Bob", 5),
        day=date(2024, 6, 1),
        tournament="RL_s09 - King",
    )
    add_match(
        ("alder440", 10),
        ("Ember_Wolf", 5),
        ("Linnet", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = peaks(conn, run_id)
    assert got["Willow_Tree"] == ("WC", 2023)
    assert got["alder440"] == ("WC", 2025)
    assert got["Ember_Wolf"] == ("WCS", 2022)
    assert got["Linnet"] == ("WCB", 2022)
    assert titles(conn, run_id)["Willow_Tree"][0] == "M"


def yearly(conn, run_id):
    return {
        (r["player_name"], r["year"]): r["title"]
        for r in conn.execute(
            "SELECT player_name, year, title FROM player_yearly_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_world_champions_hold_wc_in_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2023, "Final", "Willow_Tree", "SilverTree", "BronzeTree")
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("Willow_Tree", 10),
        ("Bob", 5),
        day=date(2023, 6, 1),
        tournament="RL_s09 - King",
    )
    add_match(
        ("Alder440", 10),
        ("SilverTree", 5),
        ("BronzeTree", 2),
        ("Bob", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = yearly(conn, run_id)
    assert got[("Willow_Tree", 2023)] == "WC"
    assert got[("SilverTree", 2023)] == "WCS"
    assert got[("BronzeTree", 2023)] == "WCB"
    assert got[("Alder440", 2025)] == "WC"
    assert ("Marten_Fisher", 2024) not in got


def test_the_reigning_champion_comes_from_admin_then_recorded_then_the_runs_final(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Marten_Fisher")
    conn.execute(
        "INSERT INTO tournament_standings VALUES (%s, 'worlds', 2025, 'Final', 'Final', 'alder440', 1, 16, 4, %s)",
        (run_id, [3, 1, 0, 0]),
    )
    conn.commit()
    add_match(
        ("alder440", 10),
        ("Marten_Fisher", 5),
        ("pipit4", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)
    assert titles(conn, run_id)["alder440"][0] == "WC"

    conn.execute(
        "INSERT INTO tournament_podium_overrides (tournament, season, place, player_name) VALUES ('worlds', '2025', 1, 'pipit4')"
    )
    conn.commit()
    derive_tournament_badges(conn=conn, run_id=run_id)
    got = titles(conn, run_id)
    assert got["pipit4"][0] == "WC"
    assert got.get("alder440", ("",))[0] != "WC"


def test_same_day_games_pick_later_reason_by_finish_time():
    g1 = game(
        "2024-06-01",
        "International S35 - Bronze 28",
        "Ann",
        "Bob",
        finished_at=datetime(2024, 6, 1, 10, 0, tzinfo=UTC),
        game_id=1,
    )
    g2 = game(
        "2024-06-01",
        "RL_s09 - Count D",
        "Ann",
        "Cat",
        finished_at=datetime(2024, 6, 1, 15, 0, tzinfo=UTC),
        game_id=2,
    )
    t1 = badge_titles([g1, g2], CUTOFF, [], [], [])
    assert t1.active["Ann"]["badge_reason"] == "Played in Royal League Count Season 9"

    t2 = badge_titles([g2, g1], CUTOFF, [], [], [])
    assert t2.active["Ann"]["badge_reason"] == "Played in Royal League Count Season 9"


def test_same_day_games_tiebreak_by_game_id_when_finish_time_equal_or_missing():
    g1 = game(
        "2024-06-01",
        "International S35 - Bronze 28",
        "Ann",
        "Bob",
        game_id=10,
    )
    g2 = game(
        "2024-06-01",
        "RL_s09 - Count D",
        "Ann",
        "Cat",
        game_id=20,
    )
    t1 = badge_titles([g1, g2], CUTOFF, [], [], [])
    assert t1.active["Ann"]["badge_reason"] == "Played in Royal League Count Season 9"

    t2 = badge_titles([g2, g1], CUTOFF, [], [], [])
    assert t2.active["Ann"]["badge_reason"] == "Played in Royal League Count Season 9"


def test_derive_tournament_badges_picks_later_same_day_game(conn, add_match, run_id):
    add_match(
        ("Ann", 10),
        ("Bob", 5),
        day=date(2024, 6, 1),
        finished_at=datetime(2024, 6, 1, 10, 0, tzinfo=UTC),
        tournament="International S35 - Bronze 28",
    )
    add_match(
        ("Ann", 10),
        ("Cat", 5),
        day=date(2024, 6, 1),
        finished_at=datetime(2024, 6, 1, 15, 0, tzinfo=UTC),
        tournament="RL_s09 - Count D",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)
    got = titles(conn, run_id)
    assert got["Ann"][2] == "Played in Royal League Count Season 9"
