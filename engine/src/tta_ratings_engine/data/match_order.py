"""Orders matches chronologically for rating models and newcomer calibration.

Matches are ordered by date, then by CGE finish time (finished_at) when known,
with ties and pre-CGE games (date only) broken by match_id (load order).
"""

from collections import defaultdict
from datetime import UTC, datetime
from typing import Any

_EPOCH = datetime.min.replace(tzinfo=UTC)


def match_order(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Returns rows in chronological order: date, finish time (finished_at), then match_id."""
    return sorted(
        rows,
        key=lambda r: (
            r["date"],
            r.get("finished_at") or _EPOCH,
            r["match_id"],
        ),
    )


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
    """Returns rows (in load order, each with "tournament" and "date") in upstream's assembled order."""
    blocks: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for row in rows:
        blocks[block_of(row["tournament"] or "")].append(row)
    for block in blocks.values():
        block.sort(key=lambda r: r["date"])
    ordered = sorted(blocks, key=lambda b: min(r["date"] for r in blocks[b]))
    return [row for b in ordered for row in blocks[b]]
