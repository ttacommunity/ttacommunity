from datetime import date

from tta_ratings_engine.data.match_order import assembled_order


def m(mid, tournament, day):
    return {"match_id": mid, "tournament": tournament, "date": day}


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
