"""Expands every match into the one-on-one results the rating models read."""

import psycopg

from tta_ratings_engine.data.db import require_running
from tta_ratings_engine.data.match_order import assembled_order

PAIR_COLUMNS = (
    "run_id, match_id, date, tournament, player_count, "
    "player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible"
)


def expand_pairwise(conn: psycopg.Connection, run_id: int) -> int:
    """Rebuilds the run's pairwise_matches from the played matches of its ranking's tournaments, in rated seasons
    and rated player counts, commits, and returns how many pairs it wrote.

    An N-player match becomes N*(N-1)/2 pairs, each weighted 1/(N-1). The
    better place wins the pair, and a shared place draws; each side's culture is kept with it. Pairs are numbered in
    upstream's assembled match order (match_order), which the models rely on
    to order a day's games. Raises RunNotRunning unless the run is running.
    """
    with conn.transaction():
        return _expand(conn, run_id)


def _expand(conn, run_id):
    require_running(conn, run_id)
    # Only played games whose tournament is in the run's ranking.
    rows = conn.execute(
        """
        SELECT m.game_id AS match_id, m.name AS tournament, m.date, m.glicko_eligible, m.players, m.places, m.cultures
        FROM game_lines m
        JOIN tournaments s ON s.tournament_id = m.tournament_id
        JOIN series t ON t.series_id = m.series_id
        JOIN ranking_series rt ON rt.series_id = m.series_id
        JOIN rating_runs r ON r.ranking_id = rt.ranking_id AND r.run_id = %s
        WHERE m.state = 'played' AND s.rated AND m.player_count = ANY(t.rated_player_counts)
        ORDER BY m.game_id
        """,
        (run_id,),
    ).fetchall()
    rows = assembled_order(rows)

    pairwise_to_insert = []
    for row in rows:
        participants = list(zip(row["players"], row["places"], row["cultures"]))

        p_count = len(participants)
        if p_count < 2:
            continue

        weight = 1.0 / float(p_count - 1)
        for i in range(p_count):
            for j in range(i + 1, p_count):
                pa, place_a, sa = participants[i]
                pb, place_b, sb = participants[j]
                if place_a < place_b:
                    out_a = 1.0
                elif place_a > place_b:
                    out_a = 0.0
                else:
                    out_a = 0.5

                pairwise_to_insert.append(
                    (
                        run_id,
                        row["match_id"],
                        row["date"],
                        row["tournament"],
                        p_count,
                        pa,
                        pb,
                        sa,
                        sb,
                        out_a,
                        weight,
                        row["glicko_eligible"],
                    )
                )

    conn.execute("DELETE FROM pairwise_matches WHERE run_id = %s", (run_id,))
    with conn.cursor().copy(
        f"COPY pairwise_matches ({PAIR_COLUMNS}) FROM STDIN"
    ) as copy:
        for pair in pairwise_to_insert:
            copy.write_row(pair)
    # Without fresh statistics, the models' first queries get a plan for an empty table.
    conn.execute("ANALYZE pairwise_matches")

    return len(pairwise_to_insert)
