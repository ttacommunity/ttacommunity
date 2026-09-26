import pytest

from src.data.db import get_connection, init_db
from src.data.pairwise import expand_pairwise


@pytest.fixture
def conn(tmp_path):
    db = str(tmp_path / "engine.db")
    init_db(db)
    c = get_connection(db)
    yield c
    c.close()


def add_match(conn, match_id, *players, eligible=1):
    cols = {"player_count": len(players)}
    for i, (name, score) in enumerate(players, start=1):
        cols[f"player{i}"] = name
        cols[f"score{i}"] = score
    cols.update(
        match_id=match_id, tournament="T", date="2024-01-01", glicko_eligible=eligible
    )
    names = ", ".join(cols)
    marks = ", ".join("?" for _ in cols)
    conn.execute(
        f"INSERT INTO matches ({names}) VALUES ({marks})", tuple(cols.values())
    )
    conn.commit()


def pairs(conn):
    rows = conn.execute(
        "SELECT match_id, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible "
        "FROM pairwise_matches ORDER BY pairwise_id"
    ).fetchall()
    return [tuple(r) for r in rows]


def test_two_player_match_is_one_full_weight_pair(conn):
    add_match(conn, 1, ("Alice", 120), ("Bob", 90))

    assert expand_pairwise(conn) == 1
    assert pairs(conn) == [(1, "Alice", "Bob", 120, 90, 1.0, 1.0, 1)]


def test_four_player_match_is_six_pairs_weighted_a_third(conn):
    add_match(conn, 1, ("A", 40), ("B", 30), ("C", 30), ("D", 50))

    assert expand_pairwise(conn) == 6
    got = [(a, b, out, round(w, 9)) for _, a, b, _, _, out, w, _ in pairs(conn)]
    assert got == [
        ("A", "B", 1.0, 0.333333333),
        ("A", "C", 1.0, 0.333333333),
        ("A", "D", 0.0, 0.333333333),
        ("B", "C", 0.5, 0.333333333),
        ("B", "D", 0.0, 0.333333333),
        ("C", "D", 0.0, 0.333333333),
    ]


def test_three_player_match_pairs_are_weighted_a_half(conn):
    add_match(conn, 1, ("A", 10), ("B", 20), ("C", 30))

    expand_pairwise(conn)
    assert [w for *_, w, _ in pairs(conn)] == [0.5, 0.5, 0.5]


def test_ineligible_match_stays_ineligible_in_its_pairs(conn):
    add_match(conn, 1, ("A", 10), ("B", 20), eligible=0)

    expand_pairwise(conn)
    assert [e for *_, e in pairs(conn)] == [0]


def test_expanding_again_replaces_the_pairs_instead_of_adding(conn):
    add_match(conn, 1, ("A", 10), ("B", 20))
    expand_pairwise(conn)
    add_match(conn, 2, ("C", 10), ("D", 20))

    assert expand_pairwise(conn) == 2
    assert [p[0] for p in pairs(conn)] == [1, 2]
