"""A game's seats as the engine reads them from the game_lines view."""

from typing import Any, NamedTuple

from tta_ratings_engine.data.players import Player

# The game_lines columns seats_of reads, for a view aliased m.
SEAT_COLUMNS = (
    "m.players, m.player_ids, m.places, m.cultures, m.outcomes, m.game_points"
)


class Seat(NamedTuple):
    """One player's seat: place (1 is best, tied seats share it), culture (never negative), outcome (finished,
    resigned or timed_out) and points (the seat's points in its group, None where its stage has none)."""

    player: Player
    place: int
    culture: float
    outcome: str | None
    points: float | None


def players_of(row: dict[str, Any]) -> list[Player]:
    """Returns a game row's players in seat order, from its players (names) and player_ids."""
    return [
        Player(n, i) for n, i in zip(row["players"], row["player_ids"], strict=True)
    ]


def seats_of(row: dict[str, Any]) -> list[Seat]:
    """Returns a game row's seats in seat order."""
    return [
        Seat(
            p, place, float(culture), outcome, None if points is None else float(points)
        )
        for p, place, culture, outcome, points in zip(
            players_of(row),
            row["places"],
            row["cultures"],
            row["outcomes"],
            row["game_points"],
        )
    ]


def by_place(seats: list[Seat]) -> list[Seat]:
    """Returns seats best place first, a tie in seat order."""
    return sorted(seats, key=lambda s: s.place)
