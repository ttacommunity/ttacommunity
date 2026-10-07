from datetime import date

import pytest

from tta_ratings_engine.data.achievements import (
    campaign,
    finish_date,
    player_achievements,
    player_records,
)


def match(tournament, *players, day, match_id):
    """A game row as derive_tournaments reads it."""
    return {
        "match_id": match_id,
        "tournament": tournament,
        "date": day,
        "player_count": len(players),
        "players": [p for p, _ in players],
        "places": [1 + sum(1 for _, o in players if o > s) for _, s in players],
        "cultures": [max(s, 0) for _, s in players],
        "outcomes": ["finished" if s >= 0 else "timed_out" for _, s in players],
        "game_points": [None for _ in players],
    }


TOP = {"international": {"Grandmaster"}, "intermezzo": {"Grandmaster"}}


def placed(player, place, points=None):
    return {"player_name": player, "place": place, "points": points}


def medals(player, gold=0, silver=0, bronze=0):
    return {"player_name": player, "gold": gold, "silver": silver, "bronze": bronze}


@pytest.mark.parametrize(
    ("name", "want"),
    [
        (
            "International S30 - Grandmaster - game 3",
            ("International Championship", "Season 30", "Grandmaster"),
        ),
        (
            "International S20 g4",
            ("International Championship", "Season 20", "Grandmaster"),
        ),
        (
            "Intermezzo S24 - Platinum 1 game 2",
            ("Intermezzo Championship", "Season 24", "Platinum 1"),
        ),
        ("RL_s01 - game 2", ("Royal League", "Season 1", "Division")),
        ("RL_s09 - Emperor game 1", ("Royal League", "Season 9", "Emperor")),
        (
            "Worlds 2025 - Final game 3",
            ("World Championship 2025", "Championship", "Championship"),
        ),
        (
            "Worlds 2026 - Stage 2 - Group A",
            ("World Championship 2026", "Stage 2", "Group A"),
        ),
        ("Sodium Season 16 - Xenon 2", ("Sodium Ladder", "Season 16", "Xenon 2")),
        ("ML_s01", ("Mercurial Ladder", "Season 1", "Ladder Division")),
        ("ML_r11", ("Mercurial Ladder", "Season 11", "Ladder Division")),
        (
            "Mercurial Season 22 - 6-Carbon 1",
            ("Mercurial Ladder", "Season 22", "6-Carbon 1"),
        ),
        ("Sodium Ladder Season 16 - Tier 1", ("Sodium Ladder", "Season 16", "Tier 1")),
        ("Premier League 2 game 5", ("Premier League", "Season 2", "Division")),
        ("Slow Burn S10 - group 1", ("Slow Burn", "Season 10", "group 1")),
        ("Eiffel Tower S03 - The Top", ("Eiffel Tower Cup", "Season 3", "The Top")),
        (
            "Survivors Cup 2026 Stage 4 - group 2 game 1",
            ("Survivors Cup", "2026", "Stage 4 - group 2"),
        ),
        ("Leaderboard Trophy 2025 Stage 3", ("Leaderboard Trophy", "2025", "Stage 3")),
        # upstream files NL_s under Sodium Ladder and calls DC_s "Diamond Cup"
        ("NL_s07", ("Nations League", "Season 7", "Division")),
        ("DC_s02", ("Duel Curve", "Season 2", "Division")),
        ("TCL Round 31", ("TCL Round 31", "Tournament", "Main")),
        ("French Open 2026 - Final", ("French Open 2026", "Final", "Main")),
        ("", ("Unknown Tournament", "Season", "Main")),
    ],
)
def test_campaign_reads_a_match_name_as_upstream(name, want):
    assert campaign(name) == want


def test_finish_date_takes_a_recorded_date_or_the_fallback():
    dates = {("international", 30): "2025-08-14", ("royal_league", 8): "2026-05-27"}
    assert (
        finish_date("International Championship", "Season 30", "2025-08-01", dates)
        == "2025-08-14"
    )
    assert finish_date("Royal League", "Season 8", "2026-05-01", dates) == "2026-05-27"
    assert (
        finish_date("International Championship", "Season 31", "2025-11-01", dates)
        == "2025-11-01"
    )
    assert (
        finish_date("TCL Round 31", "Tournament", "2024-01-02", dates) == "2024-01-02"
    )
    assert finish_date("TCL Round 31", "Tournament", None, dates) == "2026-01-01"


def test_player_achievements_sum_the_four_circuits_boards():
    boards = {
        "worlds": [medals("ann", gold=1), medals("bob", silver=1)],
        "international": [medals("bob", gold=2, bronze=1), medals("cat", silver=1)],
        "intermezzo": [medals("ann", gold=1, silver=1)],
        "royal_league": [medals("ann", gold=1)],
        "grand_slams": [medals("dan", gold=3)],
    }
    worlds = {(2024, "ann"): 1, (2025, "bob"): 2}

    got = player_achievements(boards, worlds)

    assert got == [
        {"player_name": "ann", "world_titles": 1, "international_titles": 0, "intermezzo_titles": 1,
         "royal_league_titles": 1, "gold": 3, "silver": 1, "bronze": 0,
         "accolades": ["2024 World Champion", "1x Intermezzo Champion", "1x Royal League Emperor Champion"]},
        {"player_name": "bob", "world_titles": 0, "international_titles": 2, "intermezzo_titles": 0,
         "royal_league_titles": 0, "gold": 2, "silver": 1, "bronze": 1,
         "accolades": ["2025 Vice-Champion", "2x International Champion"]},
        {"player_name": "cat", "world_titles": 0, "international_titles": 0, "intermezzo_titles": 0,
         "royal_league_titles": 0, "gold": 0, "silver": 1, "bronze": 0, "accolades": []},
    ]  # fmt: skip


def test_player_achievements_feature_six_accolades_at_most():
    boards = {"international": [medals("ann", gold=1)], "intermezzo": [medals("ann", gold=1)],
              "royal_league": [medals("ann", gold=1)]}  # fmt: skip
    worlds = {(y, "ann"): 1 for y in (2021, 2022, 2023, 2024)}
    got = player_achievements({**boards, "worlds": [medals("ann", gold=4)]}, worlds)
    assert got[0]["accolades"] == [
        "2024 World Champion", "2023 World Champion", "2022 World Champion", "2021 World Champion",
        "1x International Champion", "1x Intermezzo Champion",
    ]  # fmt: skip


def record(
    position,
    tournament,
    season,
    division,
    placement,
    points,
    medal,
    details,
    finish,
    career=False,
):
    return {"position": position, "tournament": tournament, "season": season, "division": division,
            "placement": placement, "points": points, "medal": medal, "details": details,
            "finish_date": finish, "career": career}  # fmt: skip


@pytest.fixture
def records():
    rows = [
        match("TCL Round 31", ("ann", 80), ("dan", 60), day=date(2024, 1, 2), match_id=1),
        match("International S30 - Grandmaster game 1", ("ann", 60), ("bob", 50), ("cat", 40),
              day=date(2025, 8, 1), match_id=2),
        match("International S30 - Grandmaster game 2", ("bob", 70), ("ann", 30), ("cat", 20),
              day=date(2025, 8, 10), match_id=3),
        match("International S30 - Master 1 game 1", ("dan", 50), ("eve", 40), ("fay", 30), ("gil", 20),
              day=date(2025, 8, 2), match_id=4),
        match("International S30 - Master 1 game 2", ("dan", 45), ("fay", 44), ("eve", 10), ("gil", 5),
              day=date(2025, 8, 9), match_id=5),
    ]  # fmt: skip
    standings = {
        ("international", 29, "Grandmaster"): [placed("ann", 1, 25.0), placed("bob", 2, 10.0), placed("cat", 3, 5.0)],
        # bob won the tie on 20 points by the tiebreak; upstream's rows rank by the sheet's order instead
        ("international", 30, "Grandmaster"): [placed("bob", 1, 20.0), placed("ann", 2, 20.0), placed("cat", 3, 10.0)],
        ("international", 30, "Master"): [placed("dan", 1, 12.0)],
    }  # fmt: skip
    official = {
        ("worlds", 2024, "Final"): [placed("ann", 1)],
        ("worlds", 2025, "Final"): [placed("bob", 2)],
        ("royal_league", 1, "Emperor"): [placed("cat", 1), placed("dan", 2)],
    }
    boards = {"international": [medals("ann", gold=1, silver=1), medals("bob", gold=1, silver=1),
                                medals("cat", bronze=2)]}  # fmt: skip
    honours = [
        {
            "series": "q_and_d",
            "player_name": "ann",
            "gold": 2,
            "silver": 0,
            "bronze": 1,
            "points": None,
        }
    ]
    dates = {
        ("international", 30): "2025-08-14",
        ("royal_league", 1): "2024-08-11",
        ("worlds", 2024): "2024-12-15",
    }
    got = player_records(rows=rows, top_divisions=TOP, standings=standings, official=official, boards=boards,
                         honours=honours, dates=dates)  # fmt: skip
    by_player = {}
    for r in got:
        by_player.setdefault(r.pop("player_name"), []).append(r)
    return by_player


def test_player_records_list_a_champions_seasons_as_upstream(records):
    assert records["ann"] == [
        record(1, "World Championship", "2024", "Championship Final", "1 / 4", "Gold Medal & World Title", "gold",
               "Through the Ages World Championship 2024 Winner & Reigning World Champion", "2024-12-15"),
        record(2, "International Championship", "Career (2 seasons)", "Grandmaster / Hall of Fame",
               "1x 1st, 1x 2nd, 0x 3rd", "45 pts", "gold", "Participated in 2 seasons, total score: 45 pts",
               "Career / All-Time", career=True),
        record(3, "International Championship", "Season 29", "Grandmaster", "1 / 3", "25 pts", "gold",
               "Season 29 International Championship Standing (1 of 3)", "2026-01-01"),
        record(4, "International Championship", "Season 30", "Grandmaster", "1 / 3", "20 pts", "gold",
               "Season 30 International Championship Standing (1 of 3) • Record: 1W - 1L (50.0% win rate)",
               "2025-08-14"),
        record(13, "Quick and Dirty (Q&D)", "Career Trophy Hall of Fame", "Championship",
               "2 Gold, 0 Silver, 1 Bronze", "3 Medals", "gold",
               "Q&D Hall of Fame Medalist: 2x Gold, 0x Silver, 1x Bronze", "Career / All-Time", career=True),
        record(14, "TCL Round 31", "Tournament", "Main", "1 / 2", "80 pts", None,
               "Competed in Main: 1 matches played, 1 victories (100.0% win rate)", "2024-01-02"),
    ]  # fmt: skip


def test_player_records_break_a_season_tie_by_the_sheets_order(records):
    s30 = [r for r in records["bob"] if r["season"] == "Season 30"]
    assert s30 == [
        record(7, "International Championship", "Season 30", "Grandmaster", "2 / 3", "20 pts", "silver",
               "Season 30 International Championship Standing (2 of 3) • Record: 1W - 1L (50.0% win rate)",
               "2025-08-14"),
    ]  # fmt: skip


def test_player_records_rank_other_groups_by_wins_as_upstream(records):
    assert records["dan"] == [
        record(12, "Royal League", "Season 1", "Emperor", "2 / 8", "Silver Medal", "silver",
               "Royal League Emperor Division Runner-up (Season 1)", "2024-08-11"),
        record(15, "TCL Round 31", "Tournament", "Main", "2 / 2", "60 pts", None,
               "Competed in Main: 1 matches played, 0 victories (0.0% win rate)", "2024-01-02"),
        record(16, "International Championship", "Season 30", "Master 1", "1 / 4", "2W - 0L (100.0%)", None,
               "Competed in Master 1: 2 matches played, 2 victories (100.0% win rate)", "2025-08-14"),
    ]  # fmt: skip
    assert [(r["position"], r["placement"], r["medal"]) for r in records["fay"]] == [
        (18, "2 / 4", None)
    ]
    assert (
        records["cat"][-1]["details"]
        == "Royal League Emperor Division Winner (Season 1)"
    )


def test_player_records_write_each_honour_series_in_upstreams_order():
    honours = [
        {
            "series": "mercurial_ladder",
            "player_name": "ann",
            "gold": 0,
            "silver": 0,
            "bronze": 0,
            "points": None,
        },
        {
            "series": "sodium_ladder",
            "player_name": "ann",
            "gold": 5,
            "silver": 2,
            "bronze": 0,
            "points": "8,5",
        },
        {
            "series": "sodium_ladder",
            "player_name": "bob",
            "gold": 0,
            "silver": 1,
            "bronze": 0,
            "points": None,
        },
    ]
    got = player_records(
        rows=[],
        top_divisions=TOP,
        standings={},
        official={},
        boards={},
        honours=honours,
        dates={},
    )
    assert [(r["player_name"], r["tournament"], r["division"], r["placement"], r["points"], r["medal"], r["details"])
            for r in got] == [
        ("ann", "Sodium Ladder", "Tier 1", "5 Titles, 2 Runner-ups", "8,5 income tax pts", "gold",
         "Sodium Ladder Career Standings: 5 T1 wins, 2 2nd places"),
        ("bob", "Sodium Ladder", "Tier 1", "0 Titles, 1 Runner-ups", "", "silver",
         "Sodium Ladder Career Standings: 0 T1 wins, 1 2nd places"),
        ("ann", "Mercurial Ladder", "Premium Tier 1", "0 Championships", "0 Titles", None,
         "Mercurial Ladder Career: 0 Premium Tier 1 wins"),
    ]  # fmt: skip
    assert {(r["season"], r["finish_date"], r["career"]) for r in got} == {
        ("Career Hall of Fame", "Career / All-Time", True)
    }


def test_player_records_give_a_cup_one_row_each_ranked_by_the_furthest_stage():
    rows = [
        match("Survivors Cup 2026 Stage 1 - group 1 game 1", ("ann", 50), ("bob", 40), ("cat", 30),
              day=date(2026, 3, 1), match_id=1),
        match("Survivors Cup 2026 Stage 2 - group 1 game 1", ("bob", 60), ("ann", 20),
              day=date(2026, 4, 1), match_id=2),
        match("Survivors Cup 2026 Stage 3 - group 1 game 1", ("bob", 70), ("dan", 10),
              day=date(2026, 5, 1), match_id=3),
    ]  # fmt: skip
    got = player_records(
        rows=rows,
        top_divisions=TOP,
        standings={},
        official={},
        boards={},
        honours=[],
        dates={},
    )
    assert [(r["player_name"], r["season"], r["division"], r["placement"], r["points"], r["medal"]) for r in got] == [
        ("bob", "2026", "Championship", "1 / 4", "170 pts", "gold"),
        ("dan", "2026", "Championship", "2 / 4", "10 pts", "silver"),
        ("ann", "2026", "Championship", "3 / 4", "70 pts", "bronze"),
        ("cat", "2026", "Championship", "4 / 4", "30 pts", None),
    ]  # fmt: skip
    assert got[0]["details"] == (
        "Advanced to Stage 3 • 3 matches played, 2 victories (66.7% win rate) • Cumulative Score: 170 pts"
    )
    assert got[0]["finish_date"] == "2026-05-01"


def slow_burn(edition, stage, group, *players, match_id):
    """A Slow Burn matches row, with where the catalogue places it."""
    row = match(
        f"Slow Burn game {match_id}",
        *players,
        day=date(2025, 1, match_id),
        match_id=match_id,
    )
    return {**row, "group_label": group, "catalogue_slug": "slow_burn", "catalogue_name": "Slow Burn",
            "catalogue_season": edition, "catalogue_stage": stage}  # fmt: skip


def test_slow_burn_records_take_its_edition_and_stage_from_the_catalogue():
    rows = [
        slow_burn(
            "1 (2024)",
            "Season 10",
            "group 1",
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            match_id=1,
        ),
        slow_burn(
            "1 (2024)",
            "Season 10",
            "group 1",
            ("bob", 90),
            ("ann", 80),
            ("cat", 70),
            match_id=2,
        ),
        slow_burn(
            "2 (2026)",
            "Season 1",
            "group 4",
            ("dan", 90),
            ("eve", 80),
            ("fay", 70),
            match_id=3,
        ),
        slow_burn(
            "2 (2026)",
            "Season 1",
            "group 4",
            ("eve", 90),
            ("dan", 80),
            ("fay", 70),
            match_id=4,
        ),
    ]
    got = player_records(
        rows=rows,
        top_divisions=TOP,
        standings={},
        official={},
        boards={},
        honours=[],
        dates={},
    )

    assert {(r["tournament"], r["season"], r["division"]) for r in got} == {
        ("Slow Burn 1 (2024)", "Season 10", "group 1"),
        ("Slow Burn 2 (2026)", "Season 1", "group 4"),
    }
    assert {r["medal"] for r in got} == {None}
