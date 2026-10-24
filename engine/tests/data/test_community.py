from datetime import date

import pytest

from tta_ratings_engine.data.community import (
    CommunityCodeMissing,
    CommunitySeasonUndated,
    best_eight,
    counted,
    derive_community,
    division,
    editions,
    eiffel_floor,
    event_code,
    place_points,
    ranked,
    round_name,
    round_points,
    rung_value,
    rungs,
    scored,
    table_for,
    wimbledon_round,
)
from tta_ratings_engine.data.db import RunNotRunning

POINTS = {
    ("international", 2024): {("Wood", 2): 15},
    ("international", 2025): {("Wood", 2): 20, ("Grandmaster", 1): 1000},
}


def test_a_table_is_the_latest_from_or_before_the_edition():
    assert table_for(POINTS, "international", 2024) == {("Wood", 2): 15}
    assert table_for(POINTS, "international", 2026)[("Wood", 2)] == 20


def test_an_edition_before_every_table_takes_the_earliest():
    assert table_for(POINTS, "international", 2020) == {("Wood", 2): 15}


def test_a_tournament_without_a_table_has_none():
    assert table_for(POINTS, "wimbledon", 2025) == {}


WORLDS = {
    ("Stage 1", 4): 20,
    ("Stage 3", 0): 130,
    ("Stage 3", 2): 115,
    ("Stage 3", 4): 80,
    ("Stage 8", 1): 2500,
}


@pytest.mark.parametrize(
    ("group", "place", "upper", "want"),
    [
        ("Stage 3", 2, False, 115),  # listed
        ("Stage 1", 1, False, 20),  # not listed: the next place listed
        ("Stage 3", 3, False, 80),
        ("Stage 3", 4, True, 130),  # leaving from an Upper group: the "0" row
        ("Stage 3", 2, True, 130),
        ("Stage 8", 2, False, 0),  # nothing listed from here on
        ("Stage 5", 1, False, 0),  # no such group
    ],
)
def test_place_points(group, place, upper, want):
    assert place_points(WORLDS, group, place, upper=upper) == want


def test_an_upper_group_without_a_zero_row_scores_by_place():
    assert place_points(WORLDS, "Stage 1", 4, upper=True) == 20


@pytest.mark.parametrize(
    ("group", "want"),
    [("Bronze 12", "Bronze"), ("Grandmaster", "Grandmaster"), ("Silver 3", "Silver")],
)
def test_division(group, want):
    assert division(group) == want


@pytest.mark.parametrize(
    ("players", "want"),
    [
        (256, "R256"),
        (128, "R128"),
        (32, "R32"),
        (16, "QF"),
        (8, "SF"),
        (4, "Final"),
        (5, None),
    ],
)
def test_round_name_by_players_in_the_stage(players, want):
    assert round_name(players) == want


def test_wimbledon_rounds_count_back_from_the_final():
    stages = [1, 2, 3, 4, 5, 6, 7]
    assert [wimbledon_round(s, stages) for s in stages] == [
        "R1",
        "R2",
        "R3",
        "R4",
        "R5",
        "SF",
        "Final",
    ]


@pytest.mark.parametrize(
    ("group", "want"),
    [
        ("The Top", "The Top"),
        ("The Stairs", "The Stairs"),
        ("The Gardens", "Other"),
        ("1st Floor", "Other"),
    ],
)
def test_eiffel_floor(group, want):
    assert eiffel_floor(group) == want


def line(stage, group, place, name, points=10):
    return {
        "stage": stage,
        "group": group,
        "place": place,
        "name": name,
        "points": points,
    }


@pytest.mark.parametrize(
    ("kind", "lines", "table", "want"),
    [
        pytest.param(
            "league",
            [
                line(21, "Grandmaster", 1, "ann"),
                line(21, "Wood 7", 2, "bob"),
                line(21, "Wood 7", 3, "cy"),
            ],
            {("Grandmaster", 1): 1000, ("Wood", 2): 20},
            {"ann": 1000, "bob": 20, "cy": 0},
            id="a league scores each place by its division",
        ),
        pytest.param(
            "worlds",
            [
                line(1, "2025 Group 1", 1, "ann"),
                line(3, "2025 Lower 1", 2, "ann"),
                line(1, "2025 Group 1", 3, "bob"),
                line(3, "2025 Upper 4", 4, "cy"),
            ],
            {("Stage 1", 4): 20, ("Stage 3", 0): 130, ("Stage 3", 2): 115},
            {"ann": 115, "bob": 20, "cy": 130},
            id="a World Championship scores the last stage reached, the upper bracket as its own row",
        ),
        pytest.param(
            "rounds",
            [
                line(1, "group 1" if i <= 4 else "group 2", (i - 1) % 4 + 1, n)
                for i, n in enumerate("abcdefgh", start=1)
            ]
            + [line(2, "Final", i, n) for i, n in enumerate("abef", start=1)],
            {("SF", 3): 500, ("SF", 4): 500, ("Final", 1): 1000},
            {"a": 1000, "c": 500, "h": 500},
            id="a cup scores the round a player goes out in",
        ),
        pytest.param(
            "floors",
            [
                line(3, "The Legs", 24, "ann", points=3),
                line(3, "The Legs", 25, "bob", points=0),
            ],
            {("Other", 24): 5, ("Other", 25): 4},
            {"ann": 5, "bob": 0},
            id="the Eiffel Tower scores nothing for a player not above 0 points",
        ),
    ],
)
def test_a_season_scores_by_its_kind(kind, lines, table, want):
    got = scored(kind, lines, table)

    assert {p: got[p] for p in want} == want


# The database part: places from CGE's standings or the run's own, names by CGE user.


@pytest.fixture
def run_id(conn):
    rid = conn.execute(
        "INSERT INTO rating_runs (engine_version, input_fingerprint, status) VALUES ('t', 'f', 'running') RETURNING run_id"
    ).fetchone()["run_id"]
    conn.commit()
    return rid


# Each test series' community scoring, kind and sheet code, as the load job gives them; any other scores nothing.
SCORING = {
    "international": ("league", "league", "ic_{}"),
    "intermezzo": ("league", "league", "im_{}"),
    "worlds": ("worlds", "cup", "wrld"),
    "australian_open": ("rounds", "cup", "aus"),
    "wimbledon": ("knockout", "cup", "wmbl"),
    "eiffel_tower": ("floors", "league", "eif"),
    "mercurial_ladder": (None, "ladder", "ml"),
    "sodium_ladder": (None, "ladder", "sl"),
}


@pytest.fixture
def catalogue(conn):
    """Returns a function that adds a season of a tournament in the community leaderboard, its stages linked to CGE."""

    def add(slug, season, stages=(), quarter=None, scoring=None, kind=None, code=None):
        known = SCORING.get(slug, (None, "cup", None))
        scoring, kind, code = scoring or known[0], kind or known[1], code or known[2]
        t = conn.execute(
            "INSERT INTO series (slug, name, in_community_leaderboard, community_scoring, kind, community_code) "
            "VALUES (%s, %s, true, %s, %s, %s) ON CONFLICT (slug) DO UPDATE SET in_community_leaderboard = true, "
            "community_scoring = excluded.community_scoring, kind = excluded.kind, "
            "community_code = excluded.community_code RETURNING series_id",
            (slug, slug, scoring, kind, code),
        ).fetchone()["series_id"]
        s = conn.execute(
            "INSERT INTO tournaments (series_id, label) VALUES (%s, %s) RETURNING tournament_id",
            (t, season),
        ).fetchone()["tournament_id"]
        for label, cge_t, cge_s in stages:
            conn.execute(
                "INSERT INTO stages (tournament_id, label, cge_tournament, cge_stage_number) VALUES (%s, %s, %s, %s)",
                (s, label, cge_t, cge_s),
            )
        if quarter:
            conn.execute(
                "INSERT INTO community_quarters VALUES (%s, %s, %s, %s, 'tab')",
                (slug, season, quarter[0], quarter[1]),
            )
        conn.commit()

    return add


def points_table(conn, slug, edition, cells):
    for (group, place), pts in cells.items():
        conn.execute(
            "INSERT INTO community_points VALUES (%s, %s, %s, %s, %s, 'sheet')",
            (slug, edition, group, place, pts),
        )
    conn.commit()


def cge_line(conn, t, s, group, place, player, user, games=6, points=10):
    conn.execute(
        "INSERT INTO cge_standings (cge_tournament, cge_season, group_name, row_number, place, player_name, cge_user, "
        "banned, games, points, source) VALUES (%s, %s, %s, %s, %s, %s, %s, false, %s, %s, 'CGE')",
        (t, s, group, place, place, player, user, games, points),
    )
    conn.commit()


def event_points(conn, run_id):
    rows = conn.execute(
        "SELECT tournament, season, player_name, points FROM community_event_points WHERE run_id = %s "
        "ORDER BY tournament, season, player_name",
        (run_id,),
    ).fetchall()
    return [(r["tournament"], r["season"], r["player_name"], r["points"]) for r in rows]


def test_a_league_season_scores_cge_places_by_division(conn, run_id, catalogue):
    catalogue("international", "30", [("", 21, 21)], quarter=(2025, 3))
    points_table(
        conn, "international", 2025, {("Grandmaster", 1): 1000, ("Wood", 2): 20}
    )
    cge_line(conn, 21, 21, "Grandmaster", 1, "ann", 1)
    cge_line(conn, 21, 21, "Wood 7", 2, "bob", 2)
    cge_line(conn, 21, 21, "Wood 7", 3, "cy", 3)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [
        ("international", "30", "ann", 1000),
        ("international", "30", "bob", 20),
        ("international", "30", "cy", 0),
    ]


def test_the_seasons_quarter_picks_the_table(conn, run_id, catalogue):
    catalogue("international", "27", [("", 21, 18)], quarter=(2024, 4))
    points_table(conn, "international", 2024, {("Wood", 2): 15})
    points_table(conn, "international", 2025, {("Wood", 2): 20})
    cge_line(conn, 21, 18, "Wood 1", 2, "bob", 2)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [("international", "27", "bob", 15)]


def test_a_deleted_cge_account_keeps_the_name_our_games_have(
    conn, run_id, catalogue, add_match
):
    catalogue("intermezzo", "25", [("", 19, 20)], quarter=(2025, 1))
    points_table(conn, "intermezzo", 2025, {("Gold", 1): 185})
    add_match(("Mallard", 200), ("ann", 100), placed=None)
    conn.execute(
        "INSERT INTO player_accounts (cge_user, player_id, name) SELECT 38126, player_id, 'deleted_user_1000001' FROM players WHERE name = 'Mallard'"
    )
    conn.execute(
        "UPDATE seats SET cge_user = 38126, outcome = 'finished' WHERE seat = 1"
    )
    cge_line(conn, 19, 20, "Gold 3", 1, "deleted_user_1000001", 38126)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [("intermezzo", "25", "Mallard", 185)]


def test_a_league_season_without_cge_standings_takes_the_runs_own(
    conn, run_id, catalogue
):
    catalogue("international", "33", [("", 21, 24)], quarter=(2026, 2))
    points_table(conn, "international", 2025, {("Master", 2): 410})
    conn.execute(
        "INSERT INTO tournament_standings VALUES (%s, 'international', 33, 'Master', 'Master 1', 'ann', 2, 9, 6, '{0,0,0,0}')",
        (run_id,),
    )
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [("international", "33", "ann", 410)]


def test_a_player_cge_lists_with_no_game_scores_nothing(conn, run_id, catalogue):
    catalogue("international", "30", [("", 21, 21)], quarter=(2025, 3))
    points_table(conn, "international", 2025, {("Wood", 1): 27})
    cge_line(conn, 21, 21, "Wood 1", 1, "quit", 9, games=0)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == []


def test_a_tournament_out_of_the_leaderboard_scores_nothing(conn, run_id, catalogue):
    catalogue("international", "30", [("", 21, 21)], quarter=(2025, 3))
    conn.execute("UPDATE series SET in_community_leaderboard = false")
    points_table(conn, "international", 2025, {("Wood", 1): 27})
    cge_line(conn, 21, 21, "Wood 1", 1, "ann", 1)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == []


def test_derive_community_needs_a_running_run(conn, run_id):
    conn.execute(
        "UPDATE rating_runs SET status = 'failed' WHERE run_id = %s", (run_id,)
    )
    conn.commit()
    with pytest.raises(RunNotRunning):
        derive_community(conn=conn, run_id=run_id)


def test_derive_community_replaces_the_runs_rows(conn, run_id, catalogue):
    catalogue("international", "30", [("", 21, 21)], quarter=(2025, 3))
    points_table(conn, "international", 2025, {("Wood", 1): 27})
    cge_line(conn, 21, 21, "Wood 1", 1, "ann", 1)
    derive_community(conn=conn, run_id=run_id)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [("international", "30", "ann", 27)]


def test_a_cup_takes_every_stage_of_its_cge_tournament(conn, run_id, catalogue):
    # Its early stages came from the match files, unlinked; CGE's tables still hold them.
    catalogue("wimbledon", "2025", [("Stage 3", 5495, 3)], quarter=(2026, 1))
    points_table(
        conn,
        "wimbledon",
        2026,
        {("R1", 2): 5, ("SF", 2): 480, ("Final", 1): 1000, ("Final", 2): 770},
    )
    cge_line(conn, 5495, 1, "group 1", 1, "ann", 1)
    cge_line(conn, 5495, 1, "group 1", 2, "bob", 2)
    cge_line(conn, 5495, 2, "SF 1", 1, "ann", 1)
    cge_line(conn, 5495, 2, "SF 1", 2, "cy", 3)
    cge_line(conn, 5495, 3, "Final", 1, "ann", 1)
    cge_line(conn, 5495, 3, "Final", 2, "dee", 4)
    derive_community(conn=conn, run_id=run_id)
    got = {p: pts for _, _, p, pts in event_points(conn, run_id)}
    assert got == {"ann": 1000, "bob": 5, "cy": 480, "dee": 770}


def test_a_league_kind_takes_only_its_own_seasons_stage(conn, run_id, catalogue):
    catalogue("eiffel_tower", "3", [("", 3947, 3)], quarter=(2025, 4))
    points_table(conn, "eiffel_tower", 2025, {("The Top", 1): 1000})
    cge_line(conn, 3947, 2, "The Top", 1, "old", 1)
    cge_line(conn, 3947, 3, "The Top", 1, "ann", 2)
    derive_community(conn=conn, run_id=run_id)
    assert event_points(conn, run_id) == [("eiffel_tower", "3", "ann", 1000)]


# Editions, best 8 and ranks.


def test_editions_are_calendar_years_and_rolling_years_to_each_complete_quarter():
    got = editions(first=(2024, 3), latest=date(2025, 2, 10))
    assert got["2024"] == {(2024, 1), (2024, 2), (2024, 3), (2024, 4)}
    assert got["2025"] == {(2025, 1), (2025, 2), (2025, 3), (2025, 4)}
    assert got["rolling 2024 Q4"] == {(2024, 1), (2024, 2), (2024, 3), (2024, 4)}
    assert got["rolling 2024 Q3"] == {(2023, 4), (2024, 1), (2024, 2), (2024, 3)}
    assert "rolling 2025 Q1" not in got  # the quarter is not over
    assert "rolling 2024 Q2" not in got  # before the first event


def test_best_eight_sums_the_eight_best_and_counts_the_events_that_scored():
    assert best_eight({"a": 100, "b": 0, **{f"e{i}": i for i in range(1, 10)}}) == (
        100 + 9 + 8 + 7 + 6 + 5 + 4 + 3,
        10,
    )


def test_counted_are_the_eight_best_events():
    events = {"wrld": 100, **{f"ic_{i}": i for i in range(1, 10)}}
    assert counted(events) == {"wrld", *(f"ic_{i}" for i in range(3, 10))}


def test_of_equal_points_the_event_in_the_sheets_earlier_column_counts():
    filler = {f"rl_{i:02d}": 500 for i in range(1, 8)}
    assert "ic_16" in counted({**filler, "im_13": 92, "ic_16": 92})
    assert "wrld" in counted({**filler, "ic_1": 92, "wrld": 92})
    assert "aus" in counted({**filler, "sl": 92, "aus": 92})
    assert "sl" in counted({**filler, "ml": 92, "sl": 92})


def test_seasons_compare_as_numbers_not_text():
    filler = {f"rl_{i:02d}": 500 for i in range(1, 8)}
    assert "im_9" in counted({**filler, "im_10": 92, "im_9": 92})


def test_an_event_the_sheet_has_no_column_for_comes_last():
    filler = {f"rl_{i:02d}": 500 for i in range(1, 8)}
    assert "ldrb" in counted({**filler, "zzz": 92, "ldrb": 92})


def test_equal_points_share_a_rank():
    assert ranked({"ann": 50, "bob": 70, "cy": 50, "dee": 10}) == [
        (1, "bob"),
        (2, "ann"),
        (2, "cy"),
        (4, "dee"),
    ]


@pytest.mark.parametrize(
    ("code", "season", "want"),
    [("ic_{}", "30", "ic_30"), ("im_{}", "27", "im_27"), ("rl_{:02d}", "5", "rl_05"),
     ("wrld", "2025", "wrld"), ("eif", "3", "eif")],
)  # fmt: skip
def test_event_code_is_the_series_code_with_its_season(code, season, want):
    assert event_code(code, season) == want


def rankings(conn, run_id, edition):
    rows = conn.execute(
        "SELECT rank, player_name, points, events FROM community_rankings WHERE run_id = %s AND edition = %s ORDER BY rank, player_name",
        (run_id, edition),
    ).fetchall()
    return [(r["rank"], r["player_name"], r["points"], r["events"]) for r in rows]


def test_an_edition_ranks_its_seasons_and_the_sheets_other_events(
    conn, run_id, catalogue, add_match
):
    catalogue("international", "30", [("", 21, 21)], quarter=(2025, 3))
    catalogue("international", "31", [("", 21, 22)], quarter=(2025, 4))
    catalogue("international", "29", [("", 21, 20)], quarter=(2025, 2))
    points_table(conn, "international", 2025, {("Wood", 1): 27, ("Wood", 2): 20})
    cge_line(conn, 21, 21, "Wood 1", 1, "ann", 1)
    cge_line(conn, 21, 22, "Wood 1", 2, "ann", 1)
    cge_line(conn, 21, 22, "Wood 1", 1, "Bob", 2)
    cge_line(conn, 21, 20, "Wood 1", 1, "cy", 3)
    add_match(("Bob", 1), ("x", 0), day=date(2026, 1, 5), placed=None)
    # Royal League is not computed: the sheet's points stand, under our spelling of the name.
    conn.execute(
        "INSERT INTO community_scores VALUES ('2025', 'BOB', 'rl_05', 30, 'sheet')"
    )
    conn.execute(
        "INSERT INTO community_scores VALUES ('2025', 'BOB', 'ic_30', 999, 'sheet')"
    )  # computed: ours win
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    assert rankings(conn, run_id, "2025") == [
        (1, "Bob", 57, 2),
        (2, "ann", 47, 2),
        (3, "cy", 27, 1),
    ]
    # The sheet has no such edition, so only computed events count.
    assert rankings(conn, run_id, "rolling 2025 Q4") == [
        (1, "ann", 47, 2),
        (2, "Bob", 27, 1),
        (2, "cy", 27, 1),
    ]


# Ladders: round points by rung and place, summed over an edition's rounds, ranked into the grid.

MERCURIAL = {"ratio": 0.8, "half_steps_from": None, "shares": [1, 0.4, 0]}
SODIUM = {"ratio": 0.8, "half_steps_from": 4, "shares": [1, 0.5, 0.166, 0]}


def test_rungs_count_down_from_the_top_tier_by_element_number():
    groups = [
        "Mercurial Tier",
        "25-Manganese",
        "23-Vanadium 1",
        "23-Vanadium 2",
        "1-Hydrogen 4",
    ]
    assert rungs(groups) == {
        "Mercurial Tier": 1,
        "25-Manganese": 2,
        "23-Vanadium 1": 3,
        "23-Vanadium 2": 3,
        "1-Hydrogen 4": 4,
    }


@pytest.mark.parametrize(
    ("rule", "rung", "want"),
    [
        (MERCURIAL, 1, 1000),
        (MERCURIAL, 5, 409.6),
        (SODIUM, 4, 512),
        (SODIUM, 5, 457.95),
        (SODIUM, 6, 409.6),
    ],
)
def test_rung_value(rule, rung, want):
    assert rung_value(rung, rule) == pytest.approx(want, abs=0.01)


def test_round_points_round_each_share_and_average_a_tie_unrounded():
    games = [("19-Potassium 2", [("jay_t", 1), ("Bramble", 1), ("Bright", 3)])]
    # Rung 1 here: 1000 and 400 averaged is 700; with 19 a lower rung the same rule holds.
    assert round_points(games, MERCURIAL) == {
        "jay_t": 700,
        "Bramble": 700,
        "Bright": 0,
    }
    games = [
        ("Mercurial Tier", [("a", 1)]),
        ("5-Boron", [("b", 1), ("c", 2), ("d", 3)]),
    ]
    assert round_points(games, MERCURIAL) == {"a": 1000, "b": 800, "c": 320, "d": 0}


def test_a_ladder_edition_ranks_its_rounds_totals_into_the_grid(
    conn, run_id, catalogue, add_match, move_last_game
):
    for r in ("1", "2"):
        catalogue("mercurial_ladder", r, [("", None, None)])
    conn.execute(
        "INSERT INTO community_ladders VALUES ('mercurial_ladder', 2024, 0.8, NULL, '{1,0.4,0}', 10, 'tab')"
    )
    points_table(
        conn,
        "mercurial_ladder",
        2024,
        {("leaderboard", 1): 1000, ("leaderboard", 2): 750, ("leaderboard", 3): 640},
    )
    for r, day in (("1", date(2025, 3, 1)), ("2", date(2025, 6, 1))):
        add_match(
            ("ann", 300),
            ("bob", 200),
            ("cy", 100),
            day=day,
            placed=None,
            group="Mercurial Tier",
        )
        move_last_game("mercurial_ladder", "Mercurial Tier", label=r)
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    got = {
        (r["player_name"], r["points"])
        for r in conn.execute(
            "SELECT player_name, points FROM community_edition_points WHERE run_id = %s AND edition = '2025' AND event = 'ml'",
            (run_id,),
        )
    }
    assert got == {("ann", 1000), ("bob", 750)}  # cy scored nothing, so is not ranked


def test_a_ladder_round_counts_in_the_year_cge_finished_it(
    conn, run_id, catalogue, add_match, move_last_game
):
    catalogue("sodium_ladder", "1", [("", None, None)])
    conn.execute(
        "INSERT INTO community_ladders VALUES ('sodium_ladder', 2025, 0.8, 4, '{1,0.5,0.166,0}', 8, 'tab')"
    )
    points_table(conn, "sodium_ladder", 2025, {("leaderboard", 1): 1000})
    # The match file dates the round in 2024; CGE finished it in 2025.
    add_match(
        ("ann", 300),
        ("bob", 200),
        day=date(2024, 10, 13),
        placed=None,
        group="Sodium Tier",
    )
    move_last_game("sodium_ladder", "Sodium Tier", cge_finished=date(2025, 1, 20))
    add_match(("x", 1), ("y", 0), day=date(2025, 3, 1), placed=None)
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    got = {
        (r["edition"], r["player_name"])
        for r in conn.execute(
            "SELECT edition, player_name FROM community_edition_points WHERE run_id = %s AND event = 'sl'",
            (run_id,),
        )
    }
    assert ("2025", "ann") in got
    assert ("2024", "ann") not in got


def test_a_ladder_round_counts_in_the_year_the_organisers_recorded(
    conn, run_id, catalogue, add_match, move_last_game
):
    # Sodium's first round finished in 2024; the organisers count it in 2025.
    catalogue("sodium_ladder", "1", [("", None, None)], quarter=(2025, 1))
    conn.execute(
        "INSERT INTO community_ladders VALUES ('sodium_ladder', 2025, 0.8, 4, '{1,0.5,0.166,0}', 8, 'tab')"
    )
    points_table(conn, "sodium_ladder", 2025, {("leaderboard", 1): 1000})
    add_match(
        ("ann", 300),
        ("bob", 200),
        day=date(2024, 11, 21),
        placed=None,
        group="Sodium Tier",
    )
    move_last_game("sodium_ladder", "Sodium Tier")
    add_match(("x", 1), ("y", 0), day=date(2025, 3, 1), placed=None)
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    got = {
        (r["edition"], r["player_name"])
        for r in conn.execute(
            "SELECT edition, player_name FROM community_edition_points WHERE run_id = %s AND event = 'sl'",
            (run_id,),
        )
    }
    assert ("2025", "ann") in got
    assert ("2024", "ann") not in got


def test_a_ladder_edition_before_its_first_rule_keeps_the_sheets_points(
    conn, run_id, catalogue, add_match, move_last_game
):
    catalogue("mercurial_ladder", "1", [("", None, None)])
    conn.execute(
        "INSERT INTO community_ladders VALUES ('mercurial_ladder', 2025, 0.8, NULL, '{1,0.4,0}', 10, 'tab')"
    )
    points_table(conn, "mercurial_ladder", 2025, {("leaderboard", 1): 1000})
    add_match(
        ("ann", 300),
        ("bob", 200),
        day=date(2024, 6, 1),
        placed=None,
        group="Mercurial Tier",
    )
    move_last_game("mercurial_ladder", "Mercurial Tier")
    conn.execute(
        "INSERT INTO community_scores VALUES ('2024', 'ann', 'ml', 640, 'sheet')"
    )
    conn.commit()
    derive_community(conn=conn, run_id=run_id)
    got = {
        (r["player_name"], r["points"], r["source"])
        for r in conn.execute(
            "SELECT player_name, points, source FROM community_edition_points WHERE run_id = %s AND edition = '2024' AND event = 'ml'",
            (run_id,),
        )
    }
    assert got == {("ann", 640, "sheet")}


def test_a_season_left_out_of_the_community_leaderboard_scores_nothing(
    conn, run_id, catalogue, add_match
):
    catalogue(
        "worlds",
        "2023",
        [("Stage 1", 4149, 1), ("Stage 3", 4149, 3)],
        quarter=(2023, 4),
    )
    catalogue("worlds", "2023 Play Off", [("Stage 1", 4150, 1)], quarter=(2023, 4))
    conn.execute(
        "UPDATE tournaments SET in_community = false WHERE label = '2023 Play Off'"
    )
    points_table(conn, "worlds", 2023, {("Stage 1", 4): 20, ("Stage 3", 2): 115})
    cge_line(conn, 4149, 1, "2023 Group 1", 1, "ann", 1)
    cge_line(conn, 4149, 3, "2023 Lower 1", 2, "ann", 1)
    cge_line(conn, 4150, 1, "2023 Group 1", 1, "ann", 1)
    add_match(("ann", 1), ("x", 0), day=date(2024, 3, 1), placed=None)

    derive_community(conn=conn, run_id=run_id)

    got = conn.execute(
        "SELECT points FROM community_edition_points WHERE run_id = %s AND edition = '2023' AND player_name = 'ann' AND event = 'wrld'",
        (run_id,),
    ).fetchall()
    assert [r["points"] for r in got] == [115]
    assert [s for _, s, _, _ in event_points(conn, run_id)] == ["2023"]


def test_a_ladder_round_left_out_of_the_community_leaderboard_scores_nothing(
    conn, run_id, catalogue, add_match, move_last_game
):
    catalogue("sodium_ladder", "1", [("", None, None)])
    conn.execute("UPDATE tournaments SET in_community = false")
    conn.execute(
        "INSERT INTO community_ladders VALUES ('sodium_ladder', 2025, 0.8, 4, '{1,0.5,0.166,0}', 8, 'tab')"
    )
    points_table(conn, "sodium_ladder", 2025, {("leaderboard", 1): 1000})
    add_match(
        ("ann", 300),
        ("bob", 200),
        day=date(2025, 1, 20),
        placed=None,
        group="Sodium Tier",
    )
    move_last_game("sodium_ladder", "Sodium Tier")
    add_match(("x", 1), ("y", 0), day=date(2025, 3, 1), placed=None)
    conn.commit()

    derive_community(conn=conn, run_id=run_id)

    got = conn.execute(
        "SELECT count(*) AS n FROM community_edition_points WHERE run_id = %s AND event = 'sl'",
        (run_id,),
    ).fetchone()
    assert got["n"] == 0


def test_a_series_scores_as_its_community_scoring_says_not_by_its_slug(
    conn, run_id, catalogue
):
    catalogue(
        "grass_cup",
        "2025",
        [("Stage 2", 5496, 2)],
        quarter=(2026, 1),
        scoring="knockout",
        kind="cup",
        code="grs",
    )
    points_table(conn, "grass_cup", 2026, {("SF", 2): 480, ("Final", 1): 1000})
    cge_line(conn, 5496, 1, "SF 1", 2, "bob", 2)
    cge_line(conn, 5496, 2, "Final", 1, "ann", 1)
    derive_community(conn=conn, run_id=run_id)
    got = {p: pts for _, _, p, pts in event_points(conn, run_id)}
    assert got == {"ann": 1000, "bob": 480}


def test_a_season_with_no_quarter_and_no_games_is_refused_by_name(
    conn, run_id, catalogue
):
    catalogue("international", "30", [("", 21, 21)])
    points_table(conn, "international", 2025, {("Grandmaster", 1): 1000})
    cge_line(conn, 21, 21, "Grandmaster", 1, "ann", 1)

    with pytest.raises(CommunitySeasonUndated, match="international season 30"):
        derive_community(conn=conn, run_id=run_id)


def test_a_scoring_series_with_no_community_code_is_refused_by_name(
    conn, run_id, catalogue
):
    catalogue(
        "grass_league",
        "30",
        [("", 21, 21)],
        quarter=(2025, 3),
        scoring="league",
        kind="league",
    )
    points_table(conn, "grass_league", 2025, {("Grandmaster", 1): 1000})
    cge_line(conn, 21, 21, "Grandmaster", 1, "ann", 1)

    with pytest.raises(CommunityCodeMissing, match="grass_league"):
        derive_community(conn=conn, run_id=run_id)


def test_a_ladder_with_no_community_code_is_refused_by_name(
    conn, run_id, catalogue, add_match, move_last_game
):
    catalogue("grass_ladder", "1", [("", None, None)], kind="ladder")
    conn.execute(
        "INSERT INTO community_ladders VALUES ('grass_ladder', 2024, 0.8, NULL, '{1,0.4,0}', 10, 'tab')"
    )
    points_table(conn, "grass_ladder", 2024, {("leaderboard", 1): 1000})
    add_match(
        ("ann", 300), ("bob", 200), day=date(2025, 3, 1), placed=None, group="Tier 1"
    )
    move_last_game("grass_ladder", "Tier 1", label="1")
    conn.commit()

    with pytest.raises(CommunityCodeMissing, match="grass_ladder"):
        derive_community(conn=conn, run_id=run_id)
