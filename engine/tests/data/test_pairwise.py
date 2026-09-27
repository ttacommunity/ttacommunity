from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning, connect
from tta_ratings_engine.data.pairwise import expand_pairwise


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


def test_two_player_match_is_one_full_weight_pair(conn, add_match, run_id):
    m = add_match(("Alice", 120), ("Bob", 90))

    assert expand_pairwise(conn, run_id) == 1
    assert pairs(conn, run_id) == [(m, "Alice", "Bob", 120, 90, 1.0, 1.0, True)]


def test_four_player_match_is_six_pairs_weighted_a_third(conn, add_match, run_id):
    add_match(("A", 40), ("B", 30), ("C", 30), ("D", 50))

    assert expand_pairwise(conn, run_id) == 6
    got = [(a, b, out, round(w, 9)) for _, a, b, _, _, out, w, _ in pairs(conn, run_id)]
    assert got == [
        ("A", "B", 1.0, 0.333333333),
        ("A", "C", 1.0, 0.333333333),
        ("A", "D", 0.0, 0.333333333),
        ("B", "C", 0.5, 0.333333333),
        ("B", "D", 0.0, 0.333333333),
        ("C", "D", 0.0, 0.333333333),
    ]


def test_three_player_match_pairs_are_weighted_a_half(conn, add_match, run_id):
    add_match(("A", 10), ("B", 20), ("C", 30))

    expand_pairwise(conn, run_id)
    assert [w for *_, w, _ in pairs(conn, run_id)] == [0.5, 0.5, 0.5]


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
