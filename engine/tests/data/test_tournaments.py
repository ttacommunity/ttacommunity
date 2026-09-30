from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.tournaments import (
    derive_tournaments,
    game_points,
    parse_international,
)


@pytest.mark.parametrize(
    "name, want",
    [
        ("International S30 - Silver 5 game 12", (30, "Silver", "Silver 5")),
        ("International S28 - Grandmaster game 1", (28, "Grandmaster", "Grandmaster")),
        ("International S33 - Master 1 game 3", (33, "Master", "Master 1")),
        ("International S20", (20, None, None)),
        ("International S9 game 4", (9, None, None)),
        ("Intermezzo S30 - Grandmaster game 1", None),
        ("Internationals S3", None),
    ],
)
def test_parse_international(name, want):
    assert parse_international(name) == want


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
