from datetime import date

import pytest

from tta_ratings_engine.data.badges import derive_tournament_badges
from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.seats import Seat
from tta_ratings_engine.data.tournaments import (
    SERIES,
    SeriesRulesMissing,
    derive_tournaments,
    final_podium,
    parse_intermezzo,
    parse_international,
    parse_royal_league,
    rank_group,
)


@pytest.mark.parametrize(
    ("name", "want"),
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
    ("name", "want"),
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
    ("name", "want"),
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
        (
            "RL_s04 - Emperor, pl.4-6 game 1",
            (4, "Emperor", "Emperor, pl.4-6", False),
        ),
        ("International S3", None),
    ],
)
def test_parse_royal_league(name, want):
    assert parse_royal_league(name) == want


def gm(n, season=30):
    return f"International S{season} - Grandmaster game {n}"


# The series a test game's name places it in, as the load job's classifier does.
SERIES_OF = (
    ("International", "international"),
    ("Intermezzo", "intermezzo"),
    ("RL_", "royal_league"),
    ("Worlds", "worlds"),
)


@pytest.fixture
def add_match(add_match):
    """The shared add_match, placing a game in the series its name says, so its seats take that series' points."""

    def add(*players, tournament="T", placed="rated", **kw):
        if placed == "rated":
            placed = next(
                (slug for prefix, slug in SERIES_OF if tournament.startswith(prefix)),
                placed,
            )
        return add_match(*players, tournament=tournament, placed=placed, **kw)

    return add


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


def seats(*rows):
    """Returns a game's seats from (player, place, points, culture) rows, each finished."""
    return [
        Seat(p, place, float(c), "finished", float(pts)) for p, place, pts, c in rows
    ]


@pytest.mark.parametrize(
    ("series", "games", "want"),
    [
        pytest.param(
            "international",
            [
                seats(("ann", 1, 5, 100), ("cat", 2, 2, 90), ("bob", 3, 0, 80)),
                seats(("bob", 1, 5, 100), ("ann", 2, 2, 90), ("cat", 3, 0, 80)),
                seats(("cat", 1, 5, 100), ("dan", 2, 2, 90), ("ann", 3, 0, 10)),
            ],
            [("ann", 1, 7.0), ("cat", 2, 7.0), ("bob", 3, 5.0), ("dan", 4, 2.0)],
            id="a league's two-way tie goes to head-to-head: ann above cat in two of three",
        ),
        pytest.param(
            "international",
            [
                seats(("ann", 1, 5, 100), ("bob", 2, 2, 50), ("cat", 3, 0, 10)),
                seats(("bob", 1, 5, 100), ("cat", 2, 2, 60), ("ann", 3, 0, 20)),
                seats(("cat", 1, 5, 100), ("ann", 2, 2, 90), ("bob", 3, 0, 80)),
            ],
            [("ann", 1, 7.0), ("bob", 2, 7.0), ("cat", 3, 7.0)],
            id="a league's three-way tie goes to shares of the winner's score: 2.4, 2.3, 2.1",
        ),
        pytest.param(
            "royal_league",
            [
                seats(("bob", 1, 2, 100), ("ann", 2, 0, 90)),
                seats(("ann", 1, 2, 100), ("cat", 2, 0, 90)),
                seats(("cat", 1, 2, 90), ("bob", 2, 0, 50)),
                seats(("ann", 1, 2, 100), ("cat", 2, 0, 90)),
            ],
            [("ann", 1, 4.0), ("cat", 2, 2.0), ("bob", 3, 2.0)],
            id="Royal League: bob and cat on 2 points, cat beat bob",
        ),
        pytest.param(
            "royal_league",
            [
                seats(("ann", 1, 1, 100), ("bob", 1, 1, 100)),
                seats(("ann", 1, 1, 100), ("cat", 1, 1, 100)),
                seats(("ann", 1, 1, 100), ("dan", 1, 1, 100)),
                seats(("bob", 1, 2, 100), ("cat", 2, 0, 90)),
                seats(("dan", 1, 2, 100), ("bob", 2, 0, 90)),
                seats(("dan", 1, 2, 100), ("cat", 2, 0, 90)),
            ],
            [("dan", 1, 5.0), ("bob", 2, 3.0), ("ann", 3, 3.0), ("cat", 4, 1.0)],
            id="Royal League: ann and bob drew, on 3 points; bob has a win, ann three draws",
        ),
        pytest.param(
            "royal_league",
            [
                seats(("ann", 1, 1, 100), ("bob", 1, 1, 100)),
                seats(("ann", 1, 2, 100), ("cat", 2, 0, 90)),
                seats(("bob", 1, 2, 300), ("dan", 2, 0, 10)),
                seats(("cat", 1, 2, 50), ("dan", 2, 0, 40)),
            ],
            [("bob", 1, 3.0), ("ann", 2, 3.0), ("cat", 3, 2.0), ("dan", 4, 0.0)],
            id="Royal League: ann and bob drew and won once each, bob scored more",
        ),
        pytest.param(
            "international",
            [
                seats(("ann", 1, 5, 100), ("bob", 2, 2, 90), ("cat", 3, 0, 80)),
                seats(("bob", 1, 5, 100), ("ann", 2, 2, 90), ("cat", 3, 0, 80)),
            ],
            [("ann", 1, 7.0), ("bob", 1, 7.0), ("cat", 3, 0.0)],
            id="a league's tie level on head-to-head and shares is a shared place",
        ),
        pytest.param(
            "royal_league",
            [
                seats(("ann", 1, 1, 100), ("bob", 1, 1, 100)),
                seats(("ann", 1, 2, 100), ("cat", 2, 0, 90)),
                seats(("bob", 1, 2, 100), ("cat", 2, 0, 90)),
            ],
            [("ann", 1, 3.0), ("bob", 1, 3.0), ("cat", 3, 0.0)],
            id="Royal League: level on every tiebreak is a shared place",
        ),
    ],
)
def test_rank_group_breaks_ties_by_the_series_rules(series, games, want):
    got = rank_group(games, SERIES[series])

    assert [(r["player_name"], r["place"], r["points"]) for r in got] == want


def test_a_final_tie_the_rules_cannot_split_gives_no_podium():
    # ann and bob share both games' points and winning shares; head-to-head is level.
    games = [
        seats(("ann", 1, 2, 90), ("bob", 1, 2, 90)),
        seats(("bob", 1, 2, 80), ("ann", 1, 2, 80)),
    ]

    assert final_podium(games, SERIES["worlds"]) is None


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


def test_a_game_named_without_a_division_takes_cges_group(
    conn, add_match, run_id, record
):
    record(16, "Grandmaster", "ann", "bob", "cat")
    add_match(
        ("ann", 90),
        ("bob", 80),
        ("cat", 70),
        tournament="International S16",
        group="Gold 2",
    )

    derive_tournaments(conn=conn, run_id=run_id)

    row = conn.execute(
        "SELECT division, group_label, division_source FROM tournament_games WHERE run_id = %s",
        (run_id,),
    ).fetchone()
    assert (row["division"], row["group_label"], row["division_source"]) == (
        "Gold",
        "Gold 2",
        "cge",
    )


def test_a_world_championship_group_is_not_a_division(conn, add_match, run_id):
    add_match(
        ("ann", 90),
        ("bob", 80),
        ("cat", 70),
        tournament="Worlds 2025 Stage 2 - Upper 107",
        group="Upper 107",
    )

    derive_tournaments(conn=conn, run_id=run_id)

    row = conn.execute(
        "SELECT division, division_source FROM tournament_games WHERE run_id = %s",
        (run_id,),
    ).fetchone()
    assert (row["division"], row["division_source"]) == (None, None)


def test_placements_of_a_series_the_engine_does_not_derive_are_left_alone(
    conn, run_id, place
):
    place("nations_league", 10, "Apple", "Dahlia")

    derive_tournaments(conn=conn, run_id=run_id)

    assert (
        conn.execute(
            "SELECT count(*) AS n FROM tournament_boards WHERE run_id = %s", (run_id,)
        ).fetchone()["n"]
        == 0
    )


def board(conn, run_id):
    rows = conn.execute(
        "SELECT player_name, gold, silver, bronze, points, title "
        "FROM tournament_boards WHERE run_id = %s AND series = 'international' "
        "ORDER BY player_name",
        (run_id,),
    ).fetchall()
    return {r["player_name"]: tuple(list(r.values())[1:]) for r in rows}


def test_the_board_counts_top_division_podiums(conn, add_match, run_id, place):
    # A World Champion is WC only on the World Championship's board.
    place("worlds", 2023, "Final", "Willow_Tree")
    for season in (30, 31):
        add_match(
            ("Willow_Tree", 90),
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
        "Willow_Tree": (2, 0, 0, 12.0, "GM"),
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


def test_season_records_split_a_tied_places_points(conn, add_match, run_id):
    # Tied players share their places' points and the better place; only
    # seasons of at least 6 games count.
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
        ("ann", 30, 32.0, 7, [7, 0, 0, 0], date(2025, 7, 9)),
        ("bob", 30, 27.0, 6, [6, 0, 0, 0], date(2025, 7, 6)),
        ("cat", 30, 6.0, 6, [0, 0, 6, 0], date(2025, 7, 6)),
        ("dan", 30, 0.0, 6, [0, 0, 0, 6], date(2025, 7, 6)),
    ]


def test_season_records_take_a_recorded_top_division(conn, add_match, run_id, record):
    record(20, "Grandmaster", "ann", "bob", "cat")
    record(20, "Platinum", "dan", "eve", "fay")
    for n in range(1, 7):
        add_match(
            ("ann", 90), ("bob", 80), ("cat", 70),
            tournament="International S20", day=date(2021, 1, n),
        )  # fmt: skip
        add_match(
            ("dan", 90), ("eve", 80), ("fay", 70),
            tournament="International S20", day=date(2021, 1, n),
        )  # fmt: skip

    derive_tournaments(conn=conn, run_id=run_id)

    assert [r[:4] for r in season_records(conn, run_id)] == [
        ("ann", 20, 30.0, 6),
        ("bob", 20, 12.0, 6),
        ("cat", 20, 0.0, 6),
    ]


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


def test_records_keep_grandmaster_1_and_leave_out_master_1(conn, add_match, run_id):
    plain = add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1))
    numbered = add_match(
        ("ann", 90), ("bob", 80), ("cat", 70), tournament=im(1, 12, "Grandmaster 1")
    )
    master = add_match(
        ("ann", 90),
        ("bob", 80),
        ("cat", 70),
        tournament=im(1, 12, "Master 1 - Grandmaster"),
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r["in_records"] for r in games(conn, run_id)}
    assert (got[plain], got[numbered], got[master]) == (True, True, False)


def test_season_records_count_the_corrected_season(conn, add_match, run_id, correct):
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
    assert got[0][:5] == ("ann", 7, 30.0, 6, [6, 0, 0])


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


def test_season_records_leave_out_a_left_out_seasons_games(
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

    assert season_records(conn, run_id) == []


def test_derive_writes_every_series_counts(conn, add_match, run_id):
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    add_match(("ann", 90), ("bob", 80), tournament="Worlds 2025 Stage 8 - game 1")

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT series, games, editions FROM tournament_series WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    got = {r["series"]: (r["games"], r["editions"]) for r in rows}
    assert set(got) == {
        "international",
        "intermezzo",
        "royal_league",
        "worlds",
        "grand_slams",
        "ladders",
    }
    assert got["international"] == (1, 30)
    assert got["worlds"] == (1, 1)


def positions(conn, run_id, series):
    rows = conn.execute(
        "SELECT player_name, position, title FROM tournament_boards "
        "WHERE run_id = %s AND series = %s ORDER BY position",
        (run_id, series),
    ).fetchall()
    return [(r["player_name"], r["position"], r["title"]) for r in rows]


def test_boards_are_numbered_in_upstream_order(conn, add_match, run_id, place):
    add_match(("bob", 90), ("ann", 80), ("cat", 70), tournament=gm(1))
    place("worlds", 2024, "Final", "Marten_Fisher", "flicker")
    place("worlds", 2025, "Final", "alder440", "Grebe")

    derive_tournaments(conn=conn, run_id=run_id)

    assert positions(conn, run_id, "international") == [
        ("bob", 1, "GM"),
        ("ann", 2, "M"),
        ("cat", 3, "M"),
    ]
    assert positions(conn, run_id, "worlds") == [
        ("alder440", 1, "WC"),
        ("Marten_Fisher", 2, "WC"),
        ("Grebe", 3, "GM"),
        ("flicker", 4, "GM"),
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


def test_a_recorded_top_division_game_is_a_single_game_record(
    conn, add_match, run_id, record
):
    record(20, "Grandmaster", "ann", "bob", "cat")
    record(20, "Platinum", "dan", "eve", "fay")
    top = add_match(
        ("ann", 90), ("bob", 80), ("cat", 70), tournament="International S20"
    )
    lower = add_match(
        ("dan", 90), ("eve", 80), ("fay", 70), tournament="International S20"
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r["in_records"] for r in games(conn, run_id)}
    assert (got[top], got[lower]) == (True, False)


def test_a_cge_top_division_game_is_a_single_game_record(conn, add_match, run_id):
    top = add_match(
        ("ann", 90),
        ("bob", 80),
        ("cat", 70),
        tournament="International S20",
        group="Grandmaster",
    )
    lower = add_match(
        ("dan", 90),
        ("eve", 80),
        ("fay", 70),
        tournament="International S20",
        group="Platinum 1",
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = {r["match_id"]: r["in_records"] for r in games(conn, run_id)}
    assert (got[top], got[lower]) == (True, False)


def test_a_seasons_timeout_penalty_costs_points(conn, add_match, run_id):
    for n in range(1, 7):
        cat = (
            -5 if n <= 2 else 70
        )  # a timeout scores -5, as upstream's scraper writes it
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", cat),
            tournament=im(n),
            day=date(2020, 8, n),
        )
    conn.execute(
        "INSERT INTO settings (tournament_id, timeout_penalty) SELECT tournament_id, -1 FROM tournaments "
        "JOIN series USING (series_id) WHERE slug = 'intermezzo'"
    )
    conn.commit()

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 8) == [
        ("ann", 1, 30.0),
        ("bob", 2, 12.0),
        ("cat", 3, -2.0),
    ]
    assert [r[:3] for r in season_records(conn, run_id)] == [
        ("ann", 8, 30.0),
        ("bob", 8, 12.0),
        ("cat", 8, -2.0),
    ]


def test_a_season_without_a_rule_has_no_timeout_penalty(conn, add_match, run_id):
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", -5),
            tournament=im(n),
            day=date(2020, 8, n),
        )

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 8)[2] == ("cat", 3, 0.0)


def test_a_game_never_played_gives_no_points_place_or_game(conn, add_match, run_id):
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    add_match(
        ("ann", -5), ("bob", -5), ("cat", -5), tournament=gm(2), state="never_played"
    )

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT player_name, place, points, games FROM tournament_standings WHERE run_id = %s ORDER BY place",
        (run_id,),
    ).fetchall()
    # International's 3-player points are 5-2-0; the never-played game adds no points, place or game.
    assert [tuple(r.values()) for r in rows] == [
        ("ann", 1, 5.0, 1),
        ("bob", 2, 2.0, 1),
        ("cat", 3, 0.0, 1),
    ]


@pytest.fixture
def worlds_game(conn, add_match, move_last_game):
    """Returns a function that adds a game to a World Championship season's stage, its CGE tournament in status."""

    def add(season, stage, cge_season, status, *players):
        cge = 9000 + season
        conn.execute(
            "INSERT INTO cge_tournaments (cge_tournament, name, status) VALUES (%s, %s, %s) "
            "ON CONFLICT (cge_tournament) DO UPDATE SET status = excluded.status",
            (cge, f"World Championship {season}", status),
        )
        t = conn.execute(
            "INSERT INTO series (slug, name) VALUES ('worlds', 'World Championship') "
            "ON CONFLICT (slug) DO UPDATE SET slug = excluded.slug RETURNING series_id"
        ).fetchone()["series_id"]
        se = conn.execute(
            "INSERT INTO tournaments (series_id, label) VALUES (%s, %s) "
            "ON CONFLICT (series_id, label) DO UPDATE SET label = excluded.label RETURNING tournament_id",
            (t, str(season)),
        ).fetchone()["tournament_id"]
        conn.execute(
            "INSERT INTO stages (tournament_id, label, cge_tournament, cge_stage_number) VALUES (%s, %s, %s, %s) "
            "ON CONFLICT (tournament_id, label) DO NOTHING",
            (se, stage, cge, cge_season),
        )
        add_match(*players, tournament=f"Worlds {season} {stage} - Final", placed=None)
        move_last_game("worlds", label=str(season), stage=stage)
        conn.commit()

    return add


def worlds_podium(conn, run_id, season):
    rows = conn.execute(
        "SELECT player_name, place, points FROM tournament_standings "
        "WHERE run_id = %s AND series = 'worlds' AND season = %s AND division = 'Final' ORDER BY place",
        (run_id, season),
    ).fetchall()
    return [(r["player_name"], r["place"], r["points"]) for r in rows]


def world_titles(conn, run_id):
    return {
        r["player_name"]: r["world_titles"]
        for r in conn.execute(
            "SELECT player_name, world_titles FROM tournament_achievements WHERE run_id = %s AND world_titles > 0",
            (run_id,),
        ).fetchall()
    }


def podium_from(conn, slug, source):
    conn.execute("UPDATE series SET podium_source = %s WHERE slug = %s", (source, slug))
    conn.commit()


def a_final(worlds_game, season=2025, status="finished"):
    worlds_game(
        season, "Stage 7", 7, status, ("ann", 90), ("bob", 80), ("cat", 70), ("dan", 60)
    )
    # The final, by CGE's points: 4p 6/3/1/0, 3p 5/2/0, 2p 4/0.
    worlds_game(
        season, "Stage 8", 8, status, ("bob", 90), ("ann", 80), ("cat", 70), ("dan", 60)
    )
    worlds_game(season, "Stage 8", 8, status, ("cat", 90), ("bob", 80), ("ann", 70))
    worlds_game(season, "Stage 8", 8, status, ("bob", 90), ("dan", 80))


def test_a_finished_world_championship_takes_its_podium_from_the_final(
    conn, worlds_game, run_id
):
    podium_from(conn, "worlds", "final_stage")
    a_final(worlds_game)

    derive_tournaments(conn=conn, run_id=run_id)

    assert worlds_podium(conn, run_id, 2025) == [
        ("bob", 1, 12.0),
        ("cat", 2, 6.0),
        ("ann", 3, 3.0),
        ("dan", 4, 0.0),
    ]
    assert world_titles(conn, run_id) == {"bob": 1}


def test_a_world_championship_still_running_has_no_podium(conn, worlds_game, run_id):
    podium_from(conn, "worlds", "final_stage")
    a_final(worlds_game, status="running")

    derive_tournaments(conn=conn, run_id=run_id)

    assert worlds_podium(conn, run_id, 2025) == []
    assert world_titles(conn, run_id) == {}


def test_a_recorded_podium_beats_the_derived_one(conn, worlds_game, run_id, place):
    podium_from(conn, "worlds", "final_stage")
    a_final(worlds_game)
    place("worlds", 2025, "Final", "dan", "ann", "cat")

    derive_tournaments(conn=conn, run_id=run_id)

    assert [p[0] for p in worlds_podium(conn, run_id, 2025)][:1] == ["bob"]
    assert world_titles(conn, run_id) == {"dan": 1}


def test_an_admin_podium_beats_a_recorded_one(conn, worlds_game, run_id, place):
    podium_from(conn, "worlds", "final_stage")
    a_final(worlds_game)
    place("worlds", 2025, "Final", "dan", "ann", "cat")
    for p, name in enumerate(("ann", "dan", "bob"), start=1):
        conn.execute(
            "INSERT INTO tournament_podium_overrides (tournament, season, place, player_name) VALUES ('worlds', '2025', %s, %s)",
            (p, name),
        )
    conn.commit()

    derive_tournaments(conn=conn, run_id=run_id)

    assert world_titles(conn, run_id) == {"ann": 1}


def test_league_points_come_from_the_tournament_settings(conn, add_match, run_id):
    conn.execute(
        "UPDATE settings SET points_3p = '{9,1,0}' WHERE series_id = (SELECT series_id FROM series WHERE slug = 'international')"
    )
    add_match(("ann", 100), ("bob", 90), ("cat", 80), tournament=gm(1))

    derive_tournaments(conn=conn, run_id=run_id)

    assert standings(conn, run_id, 30) == [
        ("ann", 1, 9.0),
        ("bob", 2, 1.0),
        ("cat", 3, 0.0),
    ]


def test_top_divisions_come_from_the_tournament_settings(conn, add_match, run_id):
    conn.execute(
        "UPDATE series SET top_divisions = '{Gold}' WHERE slug = 'international'"
    )
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    add_match(
        ("dan", 90),
        ("eve", 80),
        ("fay", 70),
        tournament="International S30 - Gold 1 game 1",
    )

    derive_tournaments(conn=conn, run_id=run_id)

    assert set(board(conn, run_id)) == {"dan", "eve", "fay"}


def test_the_world_championship_final_is_scored_from_its_settings(
    conn, worlds_game, run_id
):
    podium_from(conn, "worlds", "final_stage")
    conn.execute(
        "UPDATE settings SET points_4p = '{1,1,1,0}', points_3p = '{1,1,0}', points_2p = '{1,0}' WHERE series_id = (SELECT series_id FROM series WHERE slug = 'worlds')"
    )
    a_final(worlds_game)

    derive_tournaments(conn=conn, run_id=run_id)

    assert [p[1:] for p in worlds_podium(conn, run_id, 2025)][:1] == [(1, 3.0)]


@pytest.mark.parametrize(
    "change",
    [
        "DELETE FROM settings WHERE series_id = (SELECT series_id FROM series WHERE slug = 'intermezzo')",
        "UPDATE series SET top_divisions = '{}' WHERE slug = 'royal_league'",
        "DELETE FROM settings WHERE series_id = (SELECT series_id FROM series WHERE slug = 'worlds')",
        "DELETE FROM series WHERE slug = 'international'",
        "UPDATE series SET results_rules = NULL WHERE slug = 'intermezzo'",
        "UPDATE series SET podium_source = NULL WHERE slug = 'international'",
        "UPDATE series SET podium_source = 'final_stage' WHERE slug = 'intermezzo'",
        "UPDATE series SET podium_source = 'top_divisions' WHERE slug = 'worlds'",
    ],
)
def test_derive_refuses_a_series_without_its_rules(conn, run_id, change):
    conn.execute(change)
    conn.commit()

    with pytest.raises(SeriesRulesMissing):
        derive_tournaments(conn=conn, run_id=run_id)


def test_derive_leaves_its_tables_analysed(conn, add_match, run_id):
    add_match(("ann", 100), ("bob", 90), ("cat", 80), tournament=gm(1))

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT reltuples FROM pg_class WHERE relname = 'tournament_games'"
    ).fetchone()
    assert rows["reltuples"] >= 1


def test_slow_burn_records_name_its_edition_and_stage_from_the_catalogue(
    conn, add_match, run_id
):
    for winner, other in (("ann", "bob"), ("bob", "ann")):
        add_match(
            (winner, 90),
            (other, 80),
            ("cat", 70),
            tournament="Slow Burn S10 - group 1 game 1",
            placed="slow_burn",
            group="group 1",
        )
    conn.execute("UPDATE series SET name = 'Slow Burn' WHERE slug = 'slow_burn'")
    conn.execute(
        "UPDATE tournaments se SET label = '1 (2024)' FROM series t WHERE t.series_id = se.series_id AND t.slug = 'slow_burn'"
    )
    conn.execute("UPDATE stages st SET label = 'Season 10' FROM tournaments se JOIN series t USING (series_id) "
                 "WHERE st.tournament_id = se.tournament_id AND t.slug = 'slow_burn'")  # fmt: skip
    conn.commit()

    derive_tournaments(conn=conn, run_id=run_id)

    rows = conn.execute(
        "SELECT DISTINCT tournament, season, division FROM tournament_records WHERE run_id = %s AND tournament LIKE 'Slow Burn%%'",
        (run_id,),
    ).fetchall()
    assert [tuple(r.values()) for r in rows] == [
        ("Slow Burn 1 (2024)", "Season 10", "group 1")
    ]


def test_a_rule_set_derives_the_results_of_the_series_that_holds_it(
    conn, add_match, run_id
):
    conn.execute("UPDATE series SET slug = 'kings_league' WHERE slug = 'royal_league'")
    conn.commit()
    add_match(("ann", 90), ("bob", 80), tournament=rl(1))

    derive_tournaments(conn=conn, run_id=run_id)

    got = conn.execute(
        "SELECT DISTINCT series FROM tournament_standings WHERE run_id = %s", (run_id,)
    ).fetchall()
    assert [r["series"] for r in got] == ["kings_league"]


def test_a_players_records_are_stored_together(conn, add_match, run_id):
    for season in (30, 31):
        add_match(("cat", 90), ("Ann", 80), ("bob", 70), tournament=gm(1, season))

    derive_tournaments(conn=conn, run_id=run_id)

    stored = [
        r["p"]
        for r in conn.execute(
            "SELECT lower(player_name) AS p FROM tournament_records WHERE run_id = %s ORDER BY ctid",
            (run_id,),
        ).fetchall()
    ]
    assert len(stored) > 3
    assert stored == sorted(stored)


def medals_and_records(conn, run_id):
    """The run's international board medals and its top-division season records, by player."""
    board_medals = {
        r["player_name"]: (r["gold"], r["silver"], r["bronze"])
        for r in conn.execute(
            "SELECT player_name, gold, silver, bronze FROM tournament_boards WHERE run_id = %s AND series = 'international'",
            (run_id,),
        ).fetchall()
    }
    record_medals = {
        r["player_name"]: r["medal"]
        for r in conn.execute(
            "SELECT player_name, medal FROM tournament_records WHERE run_id = %s AND NOT career "
            "AND tournament = 'International Championship' AND medal IS NOT NULL",
            (run_id,),
        ).fetchall()
    }
    seasons = conn.execute(
        "SELECT count(*) AS n FROM tournament_season_records WHERE run_id = %s",
        (run_id,),
    ).fetchone()["n"]
    return board_medals, record_medals, seasons


@pytest.mark.parametrize(
    ("state", "dated", "finished"),
    [
        ("running", False, False),
        ("prepared", False, False),
        ("never_played", False, True),
        ("running", True, True),
    ],
)
def test_a_seasons_results_wait_for_its_top_division_to_finish(
    conn, add_match, run_id, state, dated, finished
):
    for n in range(1, 7):
        add_match(("ann", 90), ("bob", 80), ("cat", 70), ("dan", 60), tournament=gm(n))
    add_match(
        ("ann", 0), ("bob", 0), ("cat", 0), ("dan", 0), tournament=gm(7), state=state
    )
    if dated:
        conn.execute(
            "INSERT INTO tournament_finish_dates VALUES ('international', 30, '2024-02-01', 'test')"
        )
        conn.commit()

    derive_tournaments(conn=conn, run_id=run_id)

    got = medals_and_records(conn, run_id)
    if finished:
        assert got == (
            {"ann": (1, 0, 0), "bob": (0, 1, 0), "cat": (0, 0, 1)},
            {"ann": "gold", "bob": "silver", "cat": "bronze"},
            4,
        )
    else:
        assert got == ({}, {}, 0)
    assert len(standings(conn, run_id, 30)) == 4


def test_a_finished_season_without_a_date_is_dated_by_its_last_game(
    conn, add_match, run_id
):
    for n in range(1, 7):
        add_match(
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            ("dan", 60),
            tournament=gm(n),
            day=date(2024, 1, n),
        )
    add_match(
        ("eve", 90),
        ("fay", 80),
        ("gil", 70),
        tournament="International S30 - Gold game 1",
        day=date(2024, 2, 9),
    )

    derive_tournaments(conn=conn, run_id=run_id)

    got = conn.execute(
        "SELECT DISTINCT finish_date FROM tournament_records WHERE run_id = %s AND NOT career "
        "AND tournament = 'International Championship' AND division = 'Grandmaster'",
        (run_id,),
    ).fetchall()
    assert [r["finish_date"] for r in got] == ["2024-02-09"]


@pytest.mark.parametrize(
    ("source", "recorded", "want"),
    [
        ("top_divisions", False, {"ann", "bob", "cat"}),
        ("recorded", False, set()),
        ("recorded", True, {"dan", "eve", "fay"}),
        ("none", True, set()),
    ],
)
def test_a_leagues_podium_source_decides_its_podium(
    conn, add_match, run_id, place, source, recorded, want
):
    podium_from(conn, "international", source)
    add_match(("ann", 90), ("bob", 80), ("cat", 70), tournament=gm(1))
    if recorded:
        place("international", 30, "Grandmaster", "dan", "eve", "fay")

    derive_tournaments(conn=conn, run_id=run_id)

    assert set(board(conn, run_id)) == want


@pytest.mark.parametrize(
    ("source", "recorded", "want"),
    [
        ("final_stage", False, {"bob": 1}),
        ("final_stage", True, {"dan": 1}),
        ("recorded", False, {}),
        ("recorded", True, {"dan": 1}),
        ("none", True, {}),
    ],
)
def test_the_world_championships_podium_source_decides_its_podium(
    conn, worlds_game, run_id, place, source, recorded, want
):
    podium_from(conn, "worlds", source)
    a_final(worlds_game)
    if recorded:
        place("worlds", 2025, "Final", "dan", "ann", "cat")

    derive_tournaments(conn=conn, run_id=run_id)

    assert world_titles(conn, run_id) == want


def admin_podium(conn, season, *places):
    for place, name in places:
        conn.execute(
            "INSERT INTO tournament_podium_overrides (tournament, season, place, player_name) VALUES ('worlds', %s, %s, %s)",
            (str(season), place, name),
        )
    conn.commit()


@pytest.mark.parametrize(
    ("source", "recorded", "admin", "want"),
    [
        ("final_stage", (), [(1, "dan")], {"dan": "WC", "cat": "WCS", "ann": "WCB"}),
        (
            "recorded",
            ("dan", "ann", "cat"),
            [(2, "bob")],
            {"dan": "WC", "bob": "WCS", "cat": "WCB"},
        ),
        (
            "final_stage",
            ("dan",),
            [(3, "bob")],
            {"dan": "WC", "cat": "WCS", "bob": "WCB"},
        ),
    ],
)
def test_each_world_championship_place_comes_from_admin_then_recorded_then_the_final(
    conn, worlds_game, run_id, place, source, recorded, admin, want
):
    podium_from(conn, "worlds", source)
    a_final(worlds_game)
    if recorded:
        place("worlds", 2025, "Final", *recorded)
    admin_podium(conn, 2025, *admin)

    derive_tournaments(conn=conn, run_id=run_id)
    derive_tournament_badges(conn=conn, run_id=run_id)

    medal = {(1, 0, 0): "WC", (0, 1, 0): "WCS", (0, 0, 1): "WCB"}
    board = {p: medal[b[:3]] for p, b in boards(conn, run_id, "worlds").items()}
    years = {
        r["player_name"]: r["title"]
        for r in conn.execute(
            "SELECT player_name, title FROM player_yearly_titles WHERE run_id = %s AND year = 2025",
            (run_id,),
        ).fetchall()
        if r["title"].startswith("WC")
    }
    assert (board, years) == (want, want)


def test_a_shared_place_gives_no_medal_until_a_ruling(conn, add_match, run_id, place):
    add_match(("ann", 100), ("bob", 90), ("cat", 80), tournament=gm(1))
    add_match(("bob", 100), ("ann", 90), ("cat", 80), tournament=gm(2))

    derive_tournaments(conn=conn, run_id=run_id)
    shared = medals_and_records(conn, run_id)[:2]
    place("international", 30, "Grandmaster", "bob", "ann", "cat")
    derive_tournaments(conn=conn, run_id=run_id)
    ruled = medals_and_records(conn, run_id)[:2]

    assert shared == ({"cat": (0, 0, 1)}, {"cat": "bronze"})
    assert ruled == (
        {"bob": (1, 0, 0), "ann": (0, 1, 0), "cat": (0, 0, 1)},
        {"bob": "gold", "ann": "silver", "cat": "bronze"},
    )
