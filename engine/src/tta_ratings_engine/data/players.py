"""Players as the engine holds them: by id, with the name results are listed by."""

from typing import NamedTuple

from tta_ratings_engine.data.db import DictConnection


class Player(NamedTuple):
    """A player: their current name and their id. Players sort by name, as results list them; the id is what a
    run stores."""

    name: str
    id: int


def players(conn: DictConnection) -> dict[int, Player]:
    """Returns every player by id."""
    return {
        r["player_id"]: Player(r["name"], r["player_id"])
        for r in conn.execute("SELECT player_id, name FROM players")
    }
