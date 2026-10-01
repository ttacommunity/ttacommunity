from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.tournaments import (
    derive_tournaments,
    game_points,
    parse_intermezzo,
    parse_international,
    parse_royal_league,
)


@pytest.mark.parametrize(
    "name, want",
    [
        ("International S30 - Silver 5 game 12", (30, "Silver", "Silver 5", False)),
        (
            "International S28 - Grandmaster game 1",
            (28, "Grandmaster", "Grandmaster", False),
        ),
        ("International S33 - Master 1 game 3", (33, "Master", "Master 1", False)),
        ("International S20", (20, None, None, False)),
        ("International S9 game 4", (9, None, None, False)),
        ("Intermezzo S30 - Grandmaster game 1", None),
        ("Internationals S3", None),
    ],
)
def test_parse_international(name, want):
    assert parse_international(name) == want


@pytest.mark.parametrize(
    "name, want",
    [
        ("Intermezzo S24 - Wood 12 game 2", (24, "Wood", "Wood 12", False)),
        (
            "Intermezzo S12 - Grandmaster 1 game 3",
            (12, "Grandmaster", "Grandmaster 1", False),
        ),
        ("Intermezzo S3", (3, None, None, False)),
        ("International S3", None),
    ],
)
def test_parse_intermezzo(name, want):
    assert parse_intermezzo(name) == want


@pytest.mark.parametrize(
    "name, want",
    [
        ("RL_s09 - Marquess 3 game 24", (9, "Marquess", "Marquess 3", False)),
        ("RL_s03 - Emperor game 5", (3, "Emperor", "Emperor", False)),
        ("RL_s01", (1, None, None, False)),
        (
            "RL_tb_03 - Emperor pl.4-6 game 1",
            (3, "Emperor", "Emperor pl.4-6", True),
        ),
        ("RL_tb_s06 - Best Baron 1 game 2", (6, "Baron", "Best Baron 1", True)),
        ("RL_2w_tb_s02 - Knight 3 game 1", (2, "Knight", "Knight 3", True)),
        ("RL_3w_tb_s02 Duke game 1", (2, "Duke", "Duke", True)),
        ("RL_tb_s01 game 1", (1, None, None, True)),
        ("RL_s09 - Emperor", (9, "Emperor", "Emperor", False)),
        ("International S3", None),
    ],
)
def test_parse_royal_league(name, want):
    assert parse_royal_league(name) == want


def test_game_points_split_ties():
    four = [6, 3, 1, 0]
    assert game_points([("a", 90), ("b", 80), ("c", 70), ("d", 60)], four) == {
        "a": 6,
        "b": 3,
        "c": 1,
        "d": 0,
    }
    assert game_points([("a", 90), ("b", 90), ("c", 70), ("d", 70)], four) == {
        "a": 4.5,
        "b": 4.5,
        "c": 0.5,
        "d": 0.5,
    }
    assert game_points([("a", 50), ("b", 50), ("c", 50)], [5, 2, 0]) == {
        "a": 7 / 3,
        "b": 7 / 3,
        "c": 7 / 3,
    }


def gm(n, season=30):
    return f"International S{season} - Grandmaster game {n}"


@pytest.fixture
def record(conn):
    """Returns a function that records players' division for a season."""

    def add(season, division, *players):
        for p in players:
            conn.execute(
                "INSERT INTO tournament_group_divisions "
                "VALUES ('international', %s, %s, %s, 'sheet')",
                (season, division, p),
            )
        conn.commit()

    return add


def standings(conn, run_id, season):
    rows = conn.execute(
        "SELECT player_name, place, points FROM tournament_standings "
        "WHERE run_id = %s AND season = %s ORDER BY group_label, place",
        (run_id, season),
    ).fetchall()
    return [(r["player_name"], r["place"], r["points"]) for r in rows]


def test_a_two_way_tie_goes_to_head_to_head(conn, add_match, run_id):
    add_match(("ann", 100), ("cat", 90), ("bob", 80), tournament=gm(1))
    add_match(("bob", 100), ("ann", 90), ("cat", 80), tournament=gm(2))
    add_match(("cat", 100), ("dan", 90), ("ann", 10), tournament=gm(3))

    derive_tournaments(conn=conn, run_id=run_id)

    # ann and cat both have 7; ann finished above cat in two of their three games.
    assert standings(conn, run_id, 30) == [
        ("ann", 1, 7.0),
        ("cat", 2, 7.0),
        ("bob", 3, 5.0),
        ("dan", 4, 2.0),
    ]


def test_a_three_way_tie_goes_to_relative_performance(conn, add_match, run_id):
    # Each wins once and is 2nd once: 7 points each. Shares of the winner's
    # score: ann 1 + 0.9 + 0.5 (floor), bob 1 + 0.5 + 0.8, cat 1 + 0.6 + 0.5.
    add_match(("ann", 100), ("bob", 50), ("cat", 10), tournament=gm(1))
    add_match(("bob", 100), ("cat", 60), ("ann", 20), tournament=gm(2))
    add_match(("cat", 100), ("ann", 90), ("bob", 80), tournament=gm(3))

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 30) == [
        ("ann", 1, 7.0),
        ("bob", 2, 7.0),
        ("cat", 3, 7.0),
    ]


def test_a_season_without_divisions_uses_the_recorded_ones(
    conn, add_match, run_id, record
):
    record(20, "Grandmaster", "ann", "bob", "cat")
    top = add_match(
        ("ann", 90), ("bob", 80), ("cat", 70), tournament="International S20"
    )
    other = add_match(
        ("ann", 90), ("dan", 80), ("cat", 70), tournament="International S20"
    )

    derive_tournaments(conn=conn, run_id=run_id)

    games = {
        r["match_id"]: r
        for r in conn.execute(
            "SELECT match_id, season, division, group_label, division_source "
            "FROM tournament_games WHERE run_id = %s",
            (run_id,),
        ).fetchall()
    }
    assert games[top]["division"] == "Grandmaster"
    assert games[top]["division_source"] == "recorded"
    # dan is not recorded, so that game's division is unknown.
    assert games[other]["division"] is None
    assert games[other]["season"] == 20
    assert standings(conn, run_id, 20) == [
        ("ann", 1, 5.0),
        ("bob", 2, 2.0),
        ("cat", 3, 0.0),
    ]


def test_a_named_division_is_never_replaced_by_a_record(
    conn, add_match, run_id, record
):
    record(30, "Silver", "ann", "bob", "cat")
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))

    derive_tournaments(conn=conn, run_id=run_id)

    row = conn.execute(
        "SELECT division, division_source FROM tournament_games WHERE run_id = %s",
        (run_id,),
    ).fetchone()
    assert (row["division"], row["division_source"]) == ("Grandmaster", "name")


def board(conn, run_id):
    rows = conn.execute(
        "SELECT player_name, gold, silver, bronze, points, title "
        "FROM tournament_boards WHERE run_id = %s AND series = 'international' "
        "ORDER BY player_name",
        (run_id,),
    ).fetchall()
    return {r["player_name"]: tuple(list(r.values())[1:]) for r in rows}


def test_the_board_counts_top_division_podiums(conn, add_match, run_id):
    for season in (30, 31):
        add_match(
            ("Weidenbaum", 90),
            ("ann", 80),
            ("bob", 70),
            ("cat", 60),
            tournament=gm(1, season),
        )
    add_match(
        ("dan", 90),
        ("eve", 80),
        ("fay", 70),
        tournament="International S31 - Gold 1 game 1",
    )

    derive_tournaments(conn=conn, run_id=run_id)

    assert board(conn, run_id) == {
        "Weidenbaum": (2, 0, 0, 12.0, "WC"),
        "ann": (0, 2, 0, 6.0, "M"),
        "bob": (0, 0, 2, 2.0, "M"),
    }


def season_records(conn, run_id):
    rows = conn.execute(
        "SELECT player_name, season, points, games, placings, last_date "
        "FROM tournament_season_records WHERE run_id = %s ORDER BY player_name",
        (run_id,),
    ).fetchall()
    return [tuple(r.values()) for r in rows]


def test_season_records_score_tied_places_in_seat_order(conn, add_match, run_id):
    # Upstream's seasonal records give a tied place the points of its seat
    # order, and count only seasons of at least 6 games.
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 90),
            ("cat", 70),
            ("dan", 60),
            tournament=gm(n),
            day=date(2025, 7, n),
        )
    add_match(
        ("ann", 90), ("eve", 80), ("fay", 70), tournament=gm(7), day=date(2025, 7, 9)
    )

    derive_tournaments(conn=conn, run_id=run_id)

    assert season_records(conn, run_id) == [
        ("ann", 30, 41.0, 7, [7, 0, 0, 0], date(2025, 7, 9)),
        ("bob", 30, 18.0, 6, [0, 6, 0, 0], date(2025, 7, 6)),
        ("cat", 30, 6.0, 6, [0, 0, 6, 0], date(2025, 7, 6)),
        ("dan", 30, 0.0, 6, [0, 0, 0, 6], date(2025, 7, 6)),
    ]


def test_season_records_leave_out_recorded_divisions(conn, add_match, run_id, record):
    record(20, "Grandmaster", "ann", "bob", "cat")
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            tournament="International S20",
            day=date(2021, 1, n),
        )

    derive_tournaments(conn=conn, run_id=run_id)

    assert season_records(conn, run_id) == []


def test_derive_needs_a_running_run(conn, run_id):
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )
    conn.commit()
    with pytest.raises(RunNotRunning):
        derive_tournaments(conn=conn, run_id=run_id)


def test_derive_replaces_the_runs_rows(conn, add_match, run_id):
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    derive_tournaments(conn=conn, run_id=run_id)
    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 30) == [
        ("ann", 1, 5.0),
        ("bob", 2, 2.0),
        ("cat", 3, 0.0),
    ]


@pytest.fixture
def place(conn):
    """Returns a function that records official placements for a season's division."""

    def add(series, season, division, *players, points=None):
        for i, p in enumerate(players, start=1):
            conn.execute(
                "INSERT INTO tournament_official_placements "
                "VALUES (%s, %s, %s, %s, %s, %s, 'test', 'sheet')",
                (series, season, division, p, i, points[i - 1] if points else None),
            )
        conn.commit()

    return add


@pytest.fixture
def correct(conn):
    """Returns a function that records a season correction."""

    def add(series, name_season, season):
        conn.execute(
            "INSERT INTO tournament_season_corrections "
            "VALUES (%s, %s, %s, 'test', 'sheet')",
            (series, name_season, season),
        )
        conn.commit()

    return add


def games(conn, run_id):
    return conn.execute(
        "SELECT match_id, season, name_season, division, tiebreak, in_records "
        "FROM tournament_games WHERE run_id = %s ORDER BY match_id",
        (run_id,),
    ).fetchall()


def boards(conn, run_id, series):
    rows = conn.execute(
        "SELECT player_name, gold, silver, bronze, points, first_season "
        "FROM tournament_boards WHERE run_id = %s AND series = %s ORDER BY player_name",
        (run_id, series),
    ).fetchall()
    return {r["player_name"]: tuple(list(r.values())[1:]) for r in rows}


def im(n, season=8, group="Grandmaster"):
    return f"Intermezzo S{season} - {group} game {n}"


def test_a_corrected_season_moves_the_results_not_the_name(
    conn, add_match, run_id, correct
):
    correct("intermezzo", 8, 7)
    correct("intermezzo", 7, None)
    moved = add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1))
    copy = add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1, 7))

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r for r in games(conn, run_id)}
    assert (got[moved]["season"], got[moved]["name_season"]) == (7, 8)
    assert (got[copy]["season"], got[copy]["name_season"], got[copy]["division"]) == (
        None,
        7,
        None,
    )
    # The left-out copy is in no standings; the moved game counts once, as season 7.
    seasons = conn.execute(
        "SELECT DISTINCT season FROM tournament_standings WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    assert [r["season"] for r in seasons] == [7]
    assert boards(conn, run_id, "intermezzo")["ann"][:3] == (1, 0, 0)


def test_official_placements_replace_the_derived_podium_on_the_board(
    conn, add_match, run_id, place
):
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1, 30))
    place("intermezzo", 30, "Grandmaster", "cat", "bob", "ann", points=[9, 4, 1])
    place("intermezzo", 31, "Grandmaster", "dan", "ann", "eve", points=[21, 17, 15])

    derive_tournaments(conn=conn, run_id=run_id)

    assert boards(conn, run_id, "intermezzo") == {
        "cat": (1, 0, 0, 9.0, 30),
        "bob": (0, 1, 0, 4.0, 30),
        "ann": (0, 1, 1, 18.0, 30),
        "dan": (1, 0, 0, 21.0, 31),
        "eve": (0, 0, 1, 15.0, 31),
    }
    # The derived standings stay as the matches give them.
    assert standings(conn, run_id, 30) == [
        ("ann", 1, 5.0),
        ("bob", 2, 2.0),
        ("cat", 3, 0.0),
    ]


def rl(n, season=9, group="Emperor"):
    return f"RL_s{season:02d} - {group} game {n}"


def test_royal_league_head_to_head_breaks_a_tie(conn, add_match, run_id):
    add_match(("bob", 100), ("ann", 90), tournament=rl(1))
    add_match(("ann", 100), ("cat", 90), tournament=rl(2))
    add_match(("bob", 50), ("cat", 90), tournament=rl(3))
    add_match(("ann", 100), ("cat", 90), tournament=rl(4))

    derive_tournaments(conn=conn, run_id=run_id)

    # bob and cat both have 2 points; cat beat bob.
    assert standings(conn, run_id, 9) == [
        ("ann", 1, 4.0),
        ("cat", 2, 2.0),
        ("bob", 3, 2.0),
    ]


def test_royal_league_then_wins(conn, add_match, run_id):
    # ann and bob drew each other and have 3 points: bob with a win, ann with three draws.
    add_match(("ann", 100), ("bob", 100), tournament=rl(1))
    add_match(("ann", 100), ("cat", 100), tournament=rl(2))
    add_match(("ann", 100), ("dan", 100), tournament=rl(3))
    add_match(("bob", 100), ("cat", 90), tournament=rl(4))
    add_match(("dan", 100), ("bob", 90), tournament=rl(5))
    add_match(("dan", 100), ("cat", 90), tournament=rl(6))

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 9) == [
        ("dan", 1, 5.0),
        ("bob", 2, 3.0),
        ("ann", 3, 3.0),
        ("cat", 4, 1.0),
    ]


def test_royal_league_then_total_score(conn, add_match, run_id):
    # ann and bob drew each other and each won once; bob scored more in all.
    add_match(("ann", 100), ("bob", 100), tournament=rl(1))
    add_match(("ann", 100), ("cat", 90), tournament=rl(2))
    add_match(("bob", 300), ("dan", 10), tournament=rl(3))
    add_match(("cat", 50), ("dan", 40), tournament=rl(4))

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 9) == [
        ("bob", 1, 3.0),
        ("ann", 2, 3.0),
        ("cat", 3, 2.0),
        ("dan", 4, 0.0),
    ]


def test_royal_league_boards_score_medals_and_order_by_first_medal(
    conn, add_match, run_id, place
):
    place("royal_league", 1, "Emperor", "ann", "bob", "cat")
    place("royal_league", 2, "Emperor", "ann", "dan", "bob")

    derive_tournaments(conn=conn, run_id=run_id)

    assert boards(conn, run_id, "royal_league") == {
        "ann": (2, 0, 0, 20.0, 1),
        "bob": (0, 1, 1, 7.0, 1),
        "cat": (0, 0, 1, 2.0, 1),
        "dan": (0, 1, 0, 5.0, 2),
    }


def test_tiebreak_games_stay_out_of_standings_but_in_records(conn, add_match, run_id):
    regular = add_match(("ann", 90), ("bob", 80), tournament=rl(1))
    tiebreak = add_match(
        ("bob", 300), ("ann", 80), tournament="RL_tb_09 - Emperor pl.1-2 game 1"
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r for r in games(conn, run_id)}
    assert (got[tiebreak]["tiebreak"], got[tiebreak]["in_records"]) == (True, True)
    assert (got[regular]["tiebreak"], got[regular]["in_records"]) == (False, True)
    assert standings(conn, run_id, 9) == [("ann", 1, 2.0), ("bob", 2, 0.0)]


def test_intermezzo_records_leave_out_grandmaster_1_as_upstream_does(
    conn, add_match, run_id
):
    kept = add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1))
    left = add_match(
        ("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1, 12, "Grandmaster 1")
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r["in_records"] for r in games(conn, run_id)}
    assert (got[kept], got[left]) == (True, False)


def test_season_records_count_the_seasons_names_give(conn, add_match, run_id, correct):
    correct("intermezzo", 8, 7)
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            tournament=im(n),
            day=date(2020, 8, n),
        )

    derive_tournaments(conn=conn, run_id=run_id)

    got = season_records(conn, run_id)
    assert got[0][:5] == ("ann", 8, 30.0, 6, [6, 0, 0])


def test_royal_league_season_records_are_wins_draws_losses(conn, add_match, run_id):
    for n in range(1, 5):
        add_match(("ann", 90), ("bob", 80), tournament=rl(n), day=date(2026, 7, n))
    add_match(("ann", 90), ("bob", 90), tournament=rl(5), day=date(2026, 7, 5))

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r[0]: r for r in season_records(conn, run_id)}
    assert got["ann"][:5] == ("ann", 9, 9.0, 5, [4, 1, 0])
    assert got["bob"][:5] == ("bob", 9, 1.0, 5, [0, 1, 4])


def test_a_league_game_short_of_players_scores_as_upstream(conn, add_match, run_id):
    # A 2-player game in a 3-player league: 5 and 2, the top of the 3-player table.
    add_match(("ann", 90), ("bob", 80), tournament=im(1, 23, "Platinum 1"))

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 23) == [("ann", 1, 5.0), ("bob", 2, 2.0)]


def test_season_records_keep_a_left_out_seasons_games_as_upstream(
    conn, add_match, run_id, correct
):
    correct("intermezzo", 7, None)
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            tournament=im(n, 7),
            day=date(2020, 5, n),
        )

    derive_tournaments(conn=conn, run_id=run_id)

    row = conn.execute(
        "SELECT season, division, points FROM tournament_season_records "
        "WHERE run_id = %s AND player_name = 'ann'",
        (run_id,),
    ).fetchone()
    assert (row["season"], row["division"], row["points"]) == (7, "Grandmaster", 30.0)


def test_derive_writes_every_series_counts(conn, add_match, run_id):
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    add_match(("ann", 90), ("bob", 80), tournament="Worlds 2025 Stage 8 - game 1")

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT series, games, editions FROM tournament_series WHERE run_id = %s ORDER BY series",
        (run_id,),
    ).fetchall()
    assert [tuple(r.values()) for r in rows] == [
        ("grand_slams", 0, 0),
        ("intermezzo", 0, 0),
        ("international", 1, 30),
        ("ladders", 0, 0),
        ("royal_league", 0, 0),
        ("worlds", 1, 1),
    ]


def positions(conn, run_id, series):
    rows = conn.execute(
        "SELECT player_name, position, title FROM tournament_boards "
        "WHERE run_id = %s AND series = %s ORDER BY position",
        (run_id, series),
    ).fetchall()
    return [(r["player_name"], r["position"], r["title"]) for r in rows]


def test_boards_are_numbered_in_upstream_order(conn, add_match, run_id, place):
    add_match(("bob", 90), ("ann", 80), ("cat", 70), tournament=gm(1))
    place("worlds", 2024, "Final", "Martin_Pecheur", "frotes")
    place("worlds", 2025, "Final", "a440", "Grozz")

    derive_tournaments(conn=conn, run_id=run_id)

    assert positions(conn, run_id, "international") == [
        ("bob", 1, "GM"),
        ("ann", 2, "M"),
        ("cat", 3, "M"),
    ]
    assert positions(conn, run_id, "worlds") == [
        ("a440", 1, "WC"),
        ("Martin_Pecheur", 2, "WC"),
        ("Grozz", 3, "GM"),
        ("frotes", 4, "GM"),
    ]


def test_world_championship_games_make_records_but_no_seasons(conn, add_match, run_id):
    # Upstream's records take names with Stage 4, Final, Upper or Lower.
    names = {
        "Worlds 2024 Stage 4": True,
        "Worlds 2025 Stage 6 - Lower 1 game 3": True,
        "Worlds 2 - finals game 1": True,
        "Worlds 2025 Stage 1 - game 1": False,
        "Worlds 2025 Stage 8 - game 1": False,
    }
    ids = {
        add_match(
            ("ann", 300), ("bob", 80), tournament=name, day=date(2025, 1, n)
        ): want
        for n, (name, want) in enumerate(names.items(), start=1)
    }

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r["in_records"] for r in games(conn, run_id)}
    assert {i: got[i] for i in ids} == ids
    assert season_records(conn, run_id) == []


def test_derive_writes_the_all_time_table(conn, add_match, run_id, place):
    add_match(("bob", 90), ("ann", 80), ("cat", 70), tournament=gm(1))
    place("worlds", 2025, "Final", "ann")

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT position, player_name, world_titles, international_titles, gold, silver, bronze "
        "FROM tournament_legends WHERE run_id = %s ORDER BY position",
        (run_id,),
    ).fetchall()
    # ann: World title and an International silver; bob: an International title.
    assert [tuple(r.values()) for r in rows] == [
        (1, "ann", 1, 0, 1, 1, 0),
        (2, "bob", 0, 1, 1, 0, 0),
    ]
