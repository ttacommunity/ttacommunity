from tta_ratings_engine.data.players import Player, players


def test_players_gives_every_player_by_id(conn):
    ids = {
        r["name"]: r["player_id"]
        for r in conn.execute(
            "INSERT INTO players (name) VALUES ('Ann'), ('bob') RETURNING player_id, name"
        ).fetchall()
    }

    assert players(conn) == {
        ids["Ann"]: Player("Ann", ids["Ann"]),
        ids["bob"]: Player("bob", ids["bob"]),
    }


def test_players_sort_by_name_whatever_their_ids():
    assert sorted([Player("cat", 1), Player("Ann", 3), Player("bob", 2)]) == [
        Player("Ann", 3),
        Player("bob", 2),
        Player("cat", 1),
    ]
