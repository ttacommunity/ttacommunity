from datetime import date

from tta_ratings_engine.data.series_pages import (
    grand_slams_board,
    ladders_board,
    legends,
    series_metrics,
    worlds_board,
)


def match(tournament, *players, day=date(2025, 1, 1), match_id=0):
    """A matches row as derive_tournaments reads it."""
    row = {
        "match_id": match_id,
        "tournament": tournament,
        "date": day,
        "player_count": len(players),
    }
    for i in range(1, 5):
        name, score = players[i - 1] if i <= len(players) else (None, None)
        row[f"player{i}"], row[f"score{i}"] = name, score
    return row


def numbered(*rows):
    return [{**r, "match_id": i} for i, r in enumerate(rows, start=1)]


def test_series_metrics_count_as_upstream():
    rows = numbered(
        match("International S30 - Silver 5 game 1", ("a", 1), ("b", 2), ("c", 3)),
        match("International S20", ("a", 1), ("b", 2), ("c", 3)),
        match("Intermezzo S12 - Grandmaster 1 game 1", ("a", 1), ("b", 2), ("c", 3)),
        match("RL_s09 - Emperor game 1", ("a", 1), ("b", 2)),
        match("RL_tb_s10 - King pl.7-8 game 1", ("a", 1), ("b", 2)),
        match("Worlds 2025 Stage 8 - game 1", ("a", 1), ("b", 2)),
        match("Worlds 2 - finals game 1", ("a", 1), ("b", 2)),
        match("Wimbledon 2025 Stage 10 game 1", ("a", 1), ("b", 2)),
        match("Wimbledon 2025 Stage 9 game 1", ("a", 1), ("b", 2)),
        match("Survivors Cup 2026 Final game 1", ("a", 1), ("b", 2), ("c", 3)),
        match("ML_r11 game 1", ("a", 1), ("b", 2), ("c", 3)),
        match("TCL Round 12", ("a", 1), ("b", 2), ("c", 3)),
        match("NL_s10", ("a", 1), ("b", 2), ("c", 3)),
    )
    assert series_metrics(rows) == {
        "international": (2, 30),
        "intermezzo": (1, 12),
        "royal_league": (2, 10),
        "worlds": (2, 2),
        "grand_slams": (3, 2),
        "ladders": (2, 16),
    }


def test_the_world_championship_board_lists_recent_champions_first():
    placements = [
        {"season": 2023, "player_name": "Weidenbaum", "place": 1},
        {"season": 2025, "player_name": "a440", "place": 1},
        {"season": 2025, "player_name": "Grozz", "place": 2},
        {"season": 2024, "player_name": "Martin_Pecheur", "place": 1},
        {"season": 2024, "player_name": "frotes", "place": 2},
        {"season": 2025, "player_name": "silent_x111", "place": 3},
    ]
    got = worlds_board(placements)
    assert [
        (b["player_name"], b["title"], b["gold"], b["silver"], b["bronze"]) for b in got
    ] == [
        ("a440", "WC", 1, 0, 0),
        ("Martin_Pecheur", "WC", 1, 0, 0),
        ("Weidenbaum", "WC", 1, 0, 0),
        ("Grozz", "GM", 0, 1, 0),
        ("frotes", "GM", 0, 1, 0),
        ("silent_x111", "GM", 0, 0, 1),
    ]


def test_grand_slams_count_each_cup_edition_once():
    rows = numbered(
        match("Wimbledon 2023 stage 1 game 1", ("a", 1), ("b", 2)),
        match("Wimbledon 2023 stage 7 game 1", ("a", 1), ("b", 2)),
        match("Wimbledon 2025 Stage 9 game 1", ("a", 1), ("b", 2)),
        match("Survivors Cup 2026 Final game 1", ("a", 1), ("b", 2)),
        match("Survivors Cup 2026 Final game 2", ("a", 1), ("b", 2)),
        match("French Open 2025 (Odd Games) - R1 game 1", ("a", 1), ("b", 2)),
        match("French Open 2025 (Even Games) - R1 game 1", ("a", 1), ("b", 2)),
        match("Slow Burn 1", ("a", 1), ("b", 2)),
        match("Slow Burn S1 - game 2", ("a", 1), ("b", 2)),
        match("Slow Burn S15 - final", ("a", 1), ("b", 2)),
        match("Eiffel Tower S3 - Group A game 4", ("a", 1), ("b", 2)),
        match("Eiffel Tower S3 - Group B game 1", ("a", 1), ("b", 2)),
    )
    # Wimbledon 2023 and 2025, Survivors 2026, French Open 2025, Slow Burn 1 and 15, Eiffel Tower 3
    assert series_metrics(rows)["grand_slams"] == (12, 7)


def test_grand_slams_board_takes_each_cup_as_upstream():
    rows = numbered(
        match(
            "Survivors Cup 2026 Final game 1",
            ("ann", 90),
            ("bob", 80),
            ("cat", 70),
            day=date(2026, 8, 1),
        ),
        match(
            "Survivors Cup 2026 Final game 0",
            ("zed", 99),
            ("yan", 1),
            ("xi", 0),
            day=date(2026, 7, 1),
        ),
        match("Slow Burn S15 Final game 1", ("bob", 90), ("dan", 80), ("eve", 70)),
        match("Slow Burn S15 Final game 2", ("bob", 90), ("eve", 85), ("dan", 70)),
        match("Wimbledon 2023 stage 7 game 1", ("fay", 90), ("gus", 80)),
        match("Wimbledon 2023 stage 7 game 2", ("fay", 90), ("gus", 80)),
        match("Wimbledon 2025 Stage 10 game 1", ("ann", 90), ("gus", 80)),
        match("Wimbledon 2025 Stage 10 game 2", ("gus", 90), ("hal", 80)),
        match("Wimbledon 2025 Stage 10 game 3", ("ann", 90), ("hal", 80)),
        match("Eiffel Tower S4 - The Top game 1", ("ivy", 90), ("jo", 80)),
        match("Eiffel Tower S4 - The Top game 2", ("ivy", 90), ("jo", 80)),
        match("Eiffel Tower S4 - The Top game 3", ("jo", 90), ("ivy", 80)),
        match("Eiffel Tower S4 - The Top game 4", ("kim", 90), ("jo", 80)),
        match("Eiffel Tower S4 - The Top game 5", ("lu", 90), ("jo", 80)),
    )
    got = grand_slams_board(rows)
    # Only the latest Survivors final counts; a Wimbledon player without a win
    # gets no medal (gus in 2023, hal in 2025); Eiffel takes the top three
    # winners of "The Top" in first-seen order among equals.
    assert [
        (b["player_name"], b["gold"], b["silver"], b["bronze"], b["details"])
        for b in got
    ] == [
        ("ann", 2, 0, 0, "2026 Survivors Cup Champion, 2025 Wimbledon TTA Champion"),
        ("bob", 1, 1, 0, "2026 Survivors Cup Runner-Up, Slow Burn S15 Champion"),
        ("fay", 1, 0, 0, "2023 Wimbledon TTA Champion"),
        ("ivy", 1, 0, 0, "Eiffel Tower Pinnacle Champion (2 Top Victories)"),
        ("jo", 1, 0, 0, "Eiffel Tower Pinnacle Champion (1 Top Victories)"),
        ("kim", 1, 0, 0, "Eiffel Tower Pinnacle Champion (1 Top Victories)"),
        ("eve", 0, 1, 0, "Slow Burn S15 Runner-Up"),
        ("gus", 0, 1, 0, "2025 Wimbledon TTA Runner-Up"),
        ("cat", 0, 0, 1, "2026 Survivors Cup 3rd Place"),
        ("dan", 0, 0, 1, "Slow Burn S15 3rd Place"),
    ]


def test_ladders_board_counts_ladder_wins_for_players_of_30_games():
    rows = []
    for n in range(30):
        # ann wins 20 of 30, bob 10; cat plays 29 games and is left out.
        winner = ("ann", 90) if n < 20 else ("bob", 90)
        loser = ("bob", 80) if n < 20 else ("ann", 80)
        third = [("cat", 70)] if n < 29 else [("dan", 70)]
        rows.append(
            match(f"Mercurial Season 30 - 0-Tier {n} game 1", winner, loser, *third)
        )
    rows.append(match("ML_s01 game 1", ("bob", 90), ("ann", 80)))
    got = ladders_board(numbered(*rows))
    assert [(b["player_name"], b["games"], b["wins"], b["win_rate"]) for b in got] == [
        ("ann", 30, 20, 66.7),
        ("bob", 30, 10, 33.3),
    ]


def medal(player, gold=0, silver=0, bronze=0):
    return {"player_name": player, "gold": gold, "silver": silver, "bronze": bronze}


def test_legends_sum_the_four_major_circuits_as_upstream():
    boards = {
        "worlds": [medal("wc", gold=1), medal("vice", silver=1)],
        "international": [
            medal("ann", gold=2),
            medal("wc", gold=1, bronze=1),
            medal("one", bronze=1),
        ],
        "intermezzo": [medal("bob", gold=2), medal("ann", silver=1)],
        "royal_league": [medal("cat", silver=1, bronze=1)],
        "grand_slams": [medal("cup", gold=9)],
    }
    got = legends(boards)
    # Titles first (golds), World titles break ties, then medals; a player
    # with no title needs two medals; ties keep the order first seen.
    assert [
        (
            r["player_name"],
            r["world_titles"],
            r["international_titles"],
            r["intermezzo_titles"],
            r["royal_league_titles"],
            r["gold"],
            r["silver"],
            r["bronze"],
        )
        for r in got
    ] == [
        ("wc", 1, 1, 0, 0, 2, 0, 1),
        ("ann", 0, 2, 0, 0, 2, 1, 0),
        ("bob", 0, 0, 2, 0, 2, 0, 0),
        ("cat", 0, 0, 0, 0, 0, 1, 1),
    ]
