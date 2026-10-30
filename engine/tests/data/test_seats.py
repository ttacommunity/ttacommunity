from tta_ratings_engine.data.players import Player
from tta_ratings_engine.data.seats import Seat, by_place, seats_of


def test_seats_of_reads_a_rows_seats_in_seat_order():
    row = {
        "players": ["ann", "bob", "cy"],
        "player_ids": [1, 2, 3],
        "places": [2, 1, 3],
        "cultures": [150, 200, 0],
        "outcomes": ["finished", "finished", "timed_out"],
        "game_points": [2, 5, -1],
    }

    assert seats_of(row) == [
        Seat(Player("ann", 1), 2, 150.0, "finished", 2.0),
        Seat(Player("bob", 2), 1, 200.0, "finished", 5.0),
        Seat(Player("cy", 3), 3, 0.0, "timed_out", -1.0),
    ]


def test_by_place_keeps_a_tie_in_seat_order():
    seats = [
        Seat("a", 2, 0, None, None),
        Seat("b", 1, 0, None, None),
        Seat("c", 1, 0, None, None),
    ]

    assert [s.player for s in by_place(seats)] == ["b", "c", "a"]
