"""Expands every match into the one-on-one results the rating models read."""

import psycopg

from tta_ratings_engine.data.db import require_running

PAIR_COLUMNS = (
    "run_id, match_id, date, tournament, player_count, "
    "player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible"
)


def expand_pairwise(conn: psycopg.Connection, run_id: int) -> int:
    """Rebuilds the run's pairwise_matches from matches, commits, and returns how many pairs it wrote.

    An N-player match becomes N*(N-1)/2 pairs, each weighted 1/(N-1). The
    higher score wins the pair, and equal scores draw. Pairs are numbered in
    match order, which the models rely on to order a day's games. Raises
    RunNotRunning unless the run is running.
    """
    with conn.transaction():
        return _expand(conn, run_id)


def _expand(conn, run_id):
    require_running(conn, run_id)
    rows = conn.execute("""
        SELECT match_id, tournament, date, glicko_eligible,
               player1, score1, player2, score2, player3, score3, player4, score4
        FROM matches
        ORDER BY match_id
    """).fetchall()

    pairwise_to_insert = []
    for row in rows:
        participants = []
        for i in range(1, 5):
            p_name = row[f"player{i}"]
            score = row[f"score{i}"]
            if p_name and score is not None:
                participants.append((p_name, float(score)))

        p_count = len(participants)
        if p_count < 2:
            continue

        weight = 1.0 / float(p_count - 1)
        for i in range(p_count):
            for j in range(i + 1, p_count):
                pa, sa = participants[i]
                pb, sb = participants[j]
                if sa > sb:
                    out_a = 1.0
                elif sa < sb:
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
    with conn.cursor().copy(f"COPY pairwise_matches ({PAIR_COLUMNS}) FROM STDIN") as copy:
        for pair in pairwise_to_insert:
            copy.write_row(pair)

    return len(pairwise_to_insert)
