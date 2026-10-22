"""The Go API's copy of the rating update (player page ΔR) is checked against
these cases. Each one is recomputed here with the engine's code, so a change
to the math fails until the cases, and the Go copy, are updated with it.
Regenerate with UPDATE_GO_CASES=1."""

import json
import os
from pathlib import Path

from tta_ratings_engine.models.glicko2.engine import Rating, update_rating

CASES = (
    Path(__file__).parents[4]
    / "backend/internal/ratingdelta/testdata/engine_cases.json"
)

INPUTS = [
    # Glickman's paper example.
    {
        "name": "paper example",
        "kind": "standard",
        "tau": 0.5,
        "player": [1500, 200, 0.06],
        "matches": [
            [1400, 30, 0.06, 1.0, 1.0, 2],
            [1550, 100, 0.06, 0.0, 1.0, 2],
            [1700, 300, 0.06, 0.0, 1.0, 2],
        ],
    },
    {
        "name": "2P win, settled player",
        "kind": "standard",
        "tau": 0.3,
        "player": [2000, 27.2, 0.03],
        "matches": [[1850, 40, 0.03, 1.0, 1.0, 2]],
    },
    {
        "name": "2P loss, settled player",
        "kind": "standard",
        "tau": 0.3,
        "player": [2000, 27.2, 0.03],
        "matches": [[1850, 40, 0.03, 0.0, 1.0, 2]],
    },
    {
        "name": "2P draw",
        "kind": "standard",
        "tau": 0.3,
        "player": [1700, 60, 0.03],
        "matches": [[1720, 55, 0.03, 0.5, 1.0, 2]],
    },
    {
        "name": "newcomer beats a strong player",
        "kind": "standard",
        "tau": 0.3,
        "player": [1500, 350, 0.03],
        "matches": [[2100, 30, 0.03, 1.0, 1.0, 2]],
    },
    {
        "name": "unrated opponent",
        "kind": "standard",
        "tau": 0.3,
        "player": [1650, 90, 0.03],
        "matches": [[1500, 350, 0.03, 0.0, 1.0, 2]],
    },
    {
        "name": "3P weighted, second place",
        "kind": "weighted",
        "tau": 0.3,
        "player": [1800, 45, 0.03],
        "matches": [[1900, 35, 0.03, 0.0, 0.5, 3], [1600, 80, 0.03, 1.0, 0.5, 3]],
    },
    {
        "name": "4P golden weights, first place",
        "kind": "weighted",
        "tau": 0.3,
        "player": [1750, 70, 0.03],
        "matches": [
            [1800, 50, 0.03, 1.0, 0.8726779962499649, 4],
            [1700, 65, 0.03, 1.0, 0.8726779962499649, 4],
            [1900, 30, 0.03, 1.0, 0.8726779962499649, 4],
        ],
    },
    {
        "name": "weights ignored when unweighted",
        "kind": "standard",
        "tau": 0.3,
        "player": [1800, 45, 0.03],
        "matches": [[1900, 35, 0.03, 0.0, 0.5, 3], [1600, 80, 0.03, 1.0, 0.5, 3]],
    },
]


def compute(case: dict) -> list[float]:
    """Returns [rating, rd, sigma] after the case's update, by the engine's code."""
    player = Rating(*case["player"])
    matches = [(Rating(r, rd, s), out, w) for r, rd, s, out, w, _ in case["matches"]]
    got = update_rating(
        player, matches, tau=case["tau"], weighted=case["kind"] == "weighted"
    )
    return [got.rating, got.rd, got.sigma]


def test_go_port_cases_match_the_engine():
    if os.environ.get("UPDATE_GO_CASES") == "1":
        CASES.parent.mkdir(parents=True, exist_ok=True)
        CASES.write_text(
            json.dumps([{**c, "want": compute(c)} for c in INPUTS], indent=1) + "\n"
        )

    stored = json.loads(CASES.read_text(encoding="utf-8"))
    assert [c["name"] for c in stored] == [c["name"] for c in INPUTS]
    for case in stored:
        assert {k: v for k, v in case.items() if k != "want"} == next(
            c for c in INPUTS if c["name"] == case["name"]
        )
        got = compute(case)
        for g, w in zip(got, case["want"]):
            assert abs(g - w) < 1e-9, f"{case['name']}: got {got}, want {case['want']}"
