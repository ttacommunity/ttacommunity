from datetime import UTC, date, datetime

from tta_ratings_engine.data.match_order import assembled_order, match_order


def m(mid, tournament, day, finished_at=None):
    return {
        "match_id": mid,
        "tournament": tournament,
        "date": day,
        "finished_at": finished_at,
    }


def test_match_order_orders_by_date_then_finish_time_then_match_id():
    day = date(2025, 1, 11)
    t1 = datetime(2025, 1, 11, 10, 0, tzinfo=UTC)
    t2 = datetime(2025, 1, 11, 14, 30, tzinfo=UTC)
    rows = [
        m(10, "RL", day, finished_at=t2),
        m(2, "Worlds", day, finished_at=t1),
        m(5, "Old Cup", day, finished_at=None),  # Pre-CGE game (date only)
        m(1, "Old Cup", day, finished_at=None),  # Earlier load order
        m(8, "Another", date(2025, 1, 10), finished_at=t2),  # Earlier date
    ]
    # Ordering:
    # 1. 2025-01-10: match 8
    # 2. 2025-01-11 None finish time: match 1, then match 5 (by match_id)
    # 3. 2025-01-11 t1 finish time: match 2
    # 4. 2025-01-11 t2 finish time: match 10
    assert [r["match_id"] for r in match_order(rows)] == [8, 1, 5, 2, 10]


def test_games_are_grouped_by_tournament_block_ordered_by_each_blocks_first_date():
    rows = [
        m(1, "RL_s03 - Knight 2 game 27", date(2025, 1, 11)),
        m(2, "RL_s03 - Knight 2 game 5", date(2025, 1, 11)),
        m(3, "Worlds 2025 Stage 2 - game 11", date(2025, 1, 11)),
        m(4, "Worlds 2024 Stage 1 - game 1", date(2024, 3, 1)),
        m(5, "RL_s01 - Knight game 1", date(2024, 6, 1)),
    ]
    # Worlds starts in March 2024, before RL (June 2024), so its block comes first.
    assert [r["match_id"] for r in assembled_order(rows)] == [4, 3, 5, 1, 2]


def test_a_block_is_sorted_by_date_keeping_the_load_order_of_a_days_games():
    rows = [
        m(1, "ML_r18", date(2024, 10, 22)),
        m(2, "Mercurial Season 19 game 2", date(2024, 10, 1)),
        m(3, "ML_r19", date(2024, 10, 22)),
    ]
    assert [r["match_id"] for r in assembled_order(rows)] == [2, 1, 3]


def test_unknown_tournaments_share_one_block_and_ties_keep_first_appearance():
    rows = [
        m(1, "Some Cup", date(2024, 1, 1)),
        m(2, "Sodium Season 1", date(2024, 1, 1)),
        m(3, "Other Open", date(2024, 1, 1)),
    ]
    # Both blocks start the same day: the one seen first comes first.
    assert [r["match_id"] for r in assembled_order(rows)] == [1, 3, 2]
