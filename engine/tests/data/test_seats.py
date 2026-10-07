from tta_ratings_engine.data.seats import Seat, by_place, seats_of


def test_seats_of_reads_a_rows_seats_in_seat_order():
    row = {
        "players": ["ann", "bob", "cy"],
        "places": [2, 1, 3],
        "cultures": [150, 200, 0],
        "outcomes": ["finished", "finished", "timed_out"],
        "game_points": [2, 5, -1],
    }

    assert seats_of(row) == [
        Seat("ann", 2, 150.0, "finished", 2.0),
        Seat("bob", 1, 200.0, "finished", 5.0),
        Seat("cy", 3, 0.0, "timed_out", -1.0),
    ]


def test_by_place_keeps_a_tie_in_seat_order():
    seats = [
        Seat("a", 2, 0, None, None),
        Seat("b", 1, 0, None, None),
        Seat("c", 1, 0, None, None),
    ]

    assert [s.player for s in by_place(seats)] == ["b", "c", "a"]
