from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning, connect
from tta_ratings_engine.data.pairwise import expand_pairwise, pairs_of


def pairs(conn, run_id):
    rows = conn.execute(
        "SELECT match_id, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible "
        "FROM pairwise_matches WHERE run_id = %s ORDER BY pairwise_id",
        (run_id,),
    ).fetchall()
    return [tuple(r.values()) for r in rows]


def new_run(conn):
    rid = conn.execute(
        "INSERT INTO rating_runs (engine_version, input_fingerprint) "
        "VALUES ('test', 'test') RETURNING run_id"
    ).fetchone()["run_id"]
    conn.commit()
    return rid


@pytest.mark.parametrize(
    ("players", "places", "cultures", "want"),
    [
        pytest.param(
            ["Alice", "Bob"],
            [1, 2],
            [120, 90],
            [("Alice", "Bob", 120, 90, 1.0, 1.0)],
            id="two players",
        ),
        pytest.param(
            ["A", "B", "C"],
            [3, 2, 1],
            [10, 20, 30],
            [
                ("A", "B", 10, 20, 0.0, 0.5),
                ("A", "C", 10, 30, 0.0, 0.5),
                ("B", "C", 20, 30, 0.0, 0.5),
            ],
            id="three players",
        ),
        pytest.param(
            ["A", "B", "C", "D"],
            [2, 3, 3, 1],
            [40, 30, 30, 50],
            [
                ("A", "B", 40, 30, 1.0, 0.333333333),
                ("A", "C", 40, 30, 1.0, 0.333333333),
                ("A", "D", 40, 50, 0.0, 0.333333333),
                ("B", "C", 30, 30, 0.5, 0.333333333),
                ("B", "D", 30, 50, 0.0, 0.333333333),
                ("C", "D", 30, 50, 0.0, 0.333333333),
            ],
            id="four players, a shared place draws",
        ),
        pytest.param(["A"], [1], [10], [], id="one player, no pairs"),
    ],
)
def test_a_game_is_a_pair_for_each_two_seats_weighted_by_its_size(
    players, places, cultures, want
):
    got = [
        (a, b, sa, sb, out, round(w, 9))
        for a, b, sa, sb, out, w in pairs_of(players, places, cultures)
    ]

    assert got == want


def test_each_game_is_written_as_its_pairs(conn, add_match, run_id):
    m = add_match(("A", 10), ("B", 20), ("C", 30))

    assert expand_pairwise(conn, run_id) == 3
    assert {p[0] for p in pairs(conn, run_id)} == {m}


def test_ineligible_match_stays_ineligible_in_its_pairs(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20), eligible=False)

    expand_pairwise(conn, run_id)
    assert [e for *_, e in pairs(conn, run_id)] == [False]


def test_pairs_keep_the_match_date_and_tournament(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20), day=date(2023, 5, 17), tournament="Cup")

    expand_pairwise(conn, run_id)
    row = conn.execute(
        "SELECT date, tournament, player_count FROM pairwise_matches WHERE run_id = %s",
        (run_id,),
    ).fetchone()
    assert row == {"date": date(2023, 5, 17), "tournament": "Cup", "player_count": 2}


def test_expanding_again_replaces_the_runs_pairs_instead_of_adding(
    conn, add_match, run_id
):
    first = add_match(("A", 10), ("B", 20))
    expand_pairwise(conn, run_id)
    second = add_match(("C", 10), ("D", 20))

    assert expand_pairwise(conn, run_id) == 2
    assert [p[0] for p in pairs(conn, run_id)] == [first, second]


def test_another_runs_pairs_are_left_alone(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20))
    other = new_run(conn)
    expand_pairwise(conn, other)

    expand_pairwise(conn, run_id)

    assert len(pairs(conn, other)) == 1
    assert len(pairs(conn, run_id)) == 1


def test_the_pairs_are_committed(conn, db_url, add_match, run_id):
    add_match(("A", 10), ("B", 20))
    expand_pairwise(conn, run_id)

    with connect(db_url) as other:
        assert len(pairs(other, run_id)) == 1


def test_a_finished_run_gets_no_pairs(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20))
    conn.execute(
        "UPDATE rating_runs SET status = 'failed' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        expand_pairwise(conn, run_id)

    assert pairs(conn, run_id) == []


def test_the_planner_knows_about_the_new_pairs(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20), ("C", 30))

    expand_pairwise(conn, run_id)

    row = conn.execute(
        "SELECT reltuples FROM pg_class WHERE relname = 'pairwise_matches'"
    ).fetchone()
    assert row["reltuples"] == 3


def test_a_days_games_are_numbered_in_upstreams_assembled_order(
    conn, add_match, run_id
):
    day = date(2025, 1, 11)
    add_match(
        ("Worlds", 1),
        ("Early", 0),
        day=date(2024, 3, 1),
        tournament="Worlds 2024 Stage 1",
    )
    rl = add_match(
        ("Kestrel", 2), ("Robin51", 1), day=day, tournament="RL_s03 - Knight 2 game 5"
    )
    worlds = add_match(
        ("Kestrel", 3),
        ("Ibis04", 1),
        day=day,
        tournament="Worlds 2025 Stage 2 - game 11",
    )

    expand_pairwise(conn, run_id)
    same_day = [p[0] for p in pairs(conn, run_id) if p[0] in (rl, worlds)]
    assert same_day == [worlds, rl]


@pytest.mark.parametrize(
    "change",
    [
        "UPDATE games SET group_id = NULL WHERE game_id = (SELECT max(game_id) FROM games)",
        "UPDATE games SET state = 'never_played' WHERE game_id = (SELECT max(game_id) FROM games)",
        "UPDATE tournaments SET rated = false",
        "UPDATE series SET rated_player_counts = '{3,4}' WHERE slug = 'rated'",
    ],
    ids=["unplaced", "never played", "season not rated", "player count not rated"],
)
def test_a_game_left_out_of_the_runs_ranking_gives_no_pairs(
    conn, add_match, run_id, change
):
    add_match(("Ann", 10), ("Bob", 5))
    conn.execute(change)
    conn.commit()

    assert expand_pairwise(conn, run_id) == 0


def test_pairs_come_only_from_the_runs_rankings_series(conn, add_match, run_id):
    rated = add_match(("Alice", 120), ("Bob", 90))
    add_match(("Cat", 120), ("Dan", 90), placed="unrated")
    three = add_match(("Ann", 10), ("Bob", 5), ("Cat", 1))

    assert expand_pairwise(conn, run_id) == 4
    assert {p[0] for p in pairs(conn, run_id)} == {rated, three}
