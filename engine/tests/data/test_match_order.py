from datetime import UTC, date, datetime

from tta_ratings_engine.data.match_order import match_order


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
