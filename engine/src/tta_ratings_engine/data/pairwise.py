"""Expands every match into the one-on-one results the rating models read."""

import psycopg

from tta_ratings_engine.data.db import require_running
from tta_ratings_engine.data.match_order import assembled_order

PAIR_COLUMNS = (
    "run_id, match_id, date, tournament, player_count, "
    "player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible"
)


def expand_pairwise(conn: psycopg.Connection, run_id: int) -> int:
    """Rebuilds the run's pairwise_matches from the matches of its ranking's tournaments, commits, and returns how many pairs it wrote.

    An N-player match becomes N*(N-1)/2 pairs, each weighted 1/(N-1). The
    higher score wins the pair, and equal scores draw. Pairs are numbered in
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
        SELECT m.match_id, m.tournament, m.date, m.glicko_eligible,
               m.player1, m.score1, m.player2, m.score2, m.player3, m.score3, m.player4, m.score4
        FROM matches m
        JOIN tournament_stages st USING (stage_id)
        JOIN tournament_seasons s USING (season_id)
        JOIN ranking_tournaments rt ON rt.tournament_id = s.tournament_id
        JOIN rating_runs r ON r.ranking_id = rt.ranking_id AND r.run_id = %s
        WHERE m.state = 'played'
        ORDER BY m.match_id
        """,
        (run_id,),
    ).fetchall()
    rows = assembled_order(rows)

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
    with conn.cursor().copy(
        f"COPY pairwise_matches ({PAIR_COLUMNS}) FROM STDIN"
    ) as copy:
        for pair in pairwise_to_insert:
            copy.write_row(pair)
    # Without fresh statistics, the models' first queries get a plan for an empty table.
    conn.execute("ANALYZE pairwise_matches")

    return len(pairwise_to_insert)
