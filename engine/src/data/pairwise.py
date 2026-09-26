"""Expands every match into the one-on-one results the rating models read."""

import sqlite3


def expand_pairwise(conn: sqlite3.Connection) -> int:
    """Rebuilds pairwise_matches from matches and returns how many pairs it wrote.

    An N-player match becomes N*(N-1)/2 pairs, each weighted 1/(N-1). The
    higher score wins the pair, and equal scores draw.
    """
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

    with conn:
        conn.execute("DELETE FROM pairwise_matches;")
        conn.executemany(
            """
            INSERT INTO pairwise_matches (
                match_id, date, tournament, player_count,
                player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
            pairwise_to_insert,
        )

    return len(pairwise_to_insert)
