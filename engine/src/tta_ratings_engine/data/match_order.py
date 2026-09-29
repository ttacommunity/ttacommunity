"""Orders matches as upstream's scripts/assemble_all_matches.py orders all_matches.csv.

The models read a day's games in this order, and GlickoD's calibration takes a newcomer's first 15 games in it,
so the same order gives the same ratings as upstream. See docs/architecture.md#rating-runs.
"""

from collections import defaultdict
from typing import Any

# Upstream's tournament blocks, by name prefix, in its order.
TOURNAMENT_BLOCKS = [
    ("RL", ("RL_", "Royal League")),
    ("International_Championship", ("International ", "International_")),
    ("Intermezzo_Championship", ("Intermezzo ",)),
    ("mercurial_ladder", ("ML_", "Mercurial")),
    ("Sodium_Ladder", ("Sodium", "SL_")),
    ("Transcontinental_Ladder", ("TCL ", "TCL_")),
    ("Survivors_Cup", ("Survivors Cup", "Survivors_Cup")),
    ("French_Open", ("French Open", "French_Open")),
    ("DC", ("DC_",)),
    ("NL", ("NL_",)),
    ("PL", ("PL_",)),
    ("Eiffel_Tower", ("Eiffel Tower",)),
    ("Worlds", ("Worlds ", "World Championship")),
    ("Slow_Burn", ("Slow Burn",)),
    ("Wimbledon", ("Wimbledon",)),
]


def block_of(tournament: str) -> str:
    """Returns the tournament block a match belongs to, or "OTHER"."""
    for key, prefixes in TOURNAMENT_BLOCKS:
        if tournament.startswith(prefixes):
            return key
    return "OTHER"


def assembled_order(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Returns rows (in load order, each with "tournament" and "date") in upstream's assembled order.

    Rows are grouped into blocks, each block is sorted by date (a day's games keep their load order), and the
    blocks follow one another by their earliest date (blocks starting the same day, in the order first seen).
    """
    blocks: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        blocks[block_of(row["tournament"] or "")].append(row)
    for block in blocks.values():
        block.sort(key=lambda r: r["date"])
    ordered = sorted(blocks, key=lambda b: min(r["date"] for r in blocks[b]))
    return [row for b in ordered for row in blocks[b]]
