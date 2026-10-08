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
        SELECT m.game_id AS match_id, m.name AS tournament, m.date, m.glicko_eligible, m.players, m.places, m.cultures, m.outcomes
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
        p_count = len(row["players"])
        for pa, pb, sa, sb, out_a, weight in pairs_of(
            row["players"], row["places"], row["cultures"], row["outcomes"]
        ):
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


def pairs_of(players, places, cultures, outcomes=None):
    """Returns a game's pairs, one for each two seats in seat order, as (player_a, player_b, culture_a, culture_b,
    outcome_a, weight): the better place wins (1.0), a shared place draws (0.5), each weighted 1/(N-1).
    A pair between two players who both timed out is omitted.
    """
    if outcomes is None:
        outcomes = [None] * len(players)
    seats = list(zip(players, places, cultures, outcomes))
    if len(seats) < 2:
        return []
    weight = 1.0 / float(len(seats) - 1)
    pairs = []
    for i, (pa, place_a, sa, oa) in enumerate(seats):
        for pb, place_b, sb, ob in seats[i + 1 :]:
            if oa == "timed_out" and ob == "timed_out":
                continue
            out_a = 1.0 if place_a < place_b else 0.0 if place_a > place_b else 0.5
            pairs.append((pa, pb, sa, sb, out_a, weight))
    return pairs
