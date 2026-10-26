"""Orders matches chronologically for rating models and newcomer calibration.

Matches are ordered by date, then by CGE finish time (finished_at) when known,
with ties and pre-CGE games (date only) broken by match_id (load order).
"""

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
