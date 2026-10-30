"""The ratings database: connections, run checks, and the reads and writes the models share.

The database address comes from RATINGS_DATABASE_URL. Its schema is the
goose migrations in backend/migrations/ratings.
"""

import os

import psycopg
from psycopg.rows import DictRow, dict_row

# DictConnection is a connection whose rows are dicts, as connect opens them.
DictConnection = psycopg.Connection[DictRow]


def connect(url: str | None = None) -> DictConnection:
    """Opens the ratings database at url, or at RATINGS_DATABASE_URL, with rows as dicts.

    The connection autocommits, so each `with conn.transaction()` block is a
    real transaction, committed when it ends. Raises RuntimeError if neither
    is set, and psycopg.OperationalError if the database cannot be reached.
    """
    url = url or os.environ.get("RATINGS_DATABASE_URL")
    if not url:
        raise RuntimeError("No database: pass a URL or set RATINGS_DATABASE_URL")
    return psycopg.connect(url, row_factory=dict_row, autocommit=True)


class RunNotRunning(Exception):
    """Raised on a write into a rating run that is missing or no longer running."""


def require_running(conn, run_id):
    """Raises RunNotRunning unless the run exists and is running.

    Holds a share lock on the run until the transaction ends, so it cannot
    be finished or published mid-write.
    """
    row = conn.execute(
        "SELECT status FROM rating_runs WHERE run_id = %s FOR SHARE", (run_id,)
    ).fetchone()
    if row is None:
        raise RunNotRunning(f"run {run_id} does not exist")
    if row["status"] != "running":
        raise RunNotRunning(f"run {run_id} is {row['status']}, not running")


def load_pairs(conn, run_id, player_count):
    """Returns the run's eligible pairs for a format (0 for all), in date then pair order.

    Dates come back as 'YYYY-MM-DD' strings, the form the models parse.
    """
    fmt = "AND player_count = %(fmt)s" if player_count and player_count > 0 else ""
    return conn.execute(
        f"""
        SELECT match_id, to_char(date, 'YYYY-MM-DD') AS date, tournament,
               player_a, player_b, outcome_a, weight, player_count
        FROM pairwise_matches
        WHERE run_id = %(run)s AND glicko_eligible {fmt}
        ORDER BY pairwise_matches.date, pairwise_id
        """,
        {"run": run_id, "fmt": player_count},
    ).fetchall()


RATING_COLUMNS = (
    "model_type, player_count, player_id, rating, rd, sigma, c_rating, rank, rank_delta, "
    "games_played, opponents_count, wins, losses, draws, win_rate, last_played"
)
HISTORY_COLUMNS = (
    "model_type, player_count, player_id, period_date, rating, rd, c_rating"
)


def save_model_results(
    conn, run_id, model_type, player_count, rating_rows, history_rows
):
    """Replaces one model and format's ratings and history in a run.

    Rows are tuples in RATING_COLUMNS and HISTORY_COLUMNS order, without the
    run_id. Written with COPY; the caller commits. Raises RunNotRunning.
    """
    require_running(conn, run_id)
    for table in ("player_ratings", "rating_history"):
        conn.execute(
            f"DELETE FROM {table} WHERE run_id = %s AND model_type = %s AND player_count = %s",
            (run_id, model_type, player_count),
        )
    for table, columns, rows in (
        ("player_ratings", RATING_COLUMNS, rating_rows),
        ("rating_history", HISTORY_COLUMNS, history_rows),
    ):
        with conn.cursor().copy(f"COPY {table} (run_id, {columns}) FROM STDIN") as copy:
            for row in rows:
                copy.write_row((run_id, *row))
