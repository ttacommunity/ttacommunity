"""The engine's SQLite schema and connections. The database path comes from ENGINE_DB_PATH."""

import os
import sqlite3
from pathlib import Path

import psycopg
from psycopg.rows import dict_row

SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS players (
    player_id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    country_code TEXT,
    title TEXT,
    title_count INTEGER DEFAULT 0,
    badge_reason TEXT,
    last_played TEXT
);

CREATE INDEX IF NOT EXISTS idx_players_name ON players(name);
CREATE INDEX IF NOT EXISTS idx_players_nocase ON players(name COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS matches (
    match_id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament TEXT,
    date TEXT NOT NULL,
    player_count INTEGER NOT NULL,
    player1 TEXT NOT NULL,
    score1 REAL NOT NULL,
    player2 TEXT NOT NULL,
    score2 REAL NOT NULL,
    player3 TEXT,
    score3 REAL,
    player4 TEXT,
    score4 REAL,
    replay_code TEXT,
    glicko_eligible INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_matches_date ON matches(date);
CREATE INDEX IF NOT EXISTS idx_matches_tournament ON matches(tournament);
CREATE INDEX IF NOT EXISTS idx_matches_p1 ON matches(player1, date);
CREATE INDEX IF NOT EXISTS idx_matches_p2 ON matches(player2, date);
CREATE INDEX IF NOT EXISTS idx_matches_p3 ON matches(player3, date);
CREATE INDEX IF NOT EXISTS idx_matches_p4 ON matches(player4, date);

CREATE TABLE IF NOT EXISTS pairwise_matches (
    pairwise_id INTEGER PRIMARY KEY AUTOINCREMENT,
    match_id INTEGER NOT NULL,
    date TEXT NOT NULL,
    tournament TEXT,
    player_count INTEGER NOT NULL,
    player_a TEXT NOT NULL,
    player_b TEXT NOT NULL,
    score_a REAL NOT NULL,
    score_b REAL NOT NULL,
    outcome_a REAL NOT NULL,
    weight REAL NOT NULL,
    glicko_eligible INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (match_id) REFERENCES matches(match_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_pw_date ON pairwise_matches(date);
CREATE INDEX IF NOT EXISTS idx_pw_player_a ON pairwise_matches(player_a);
CREATE INDEX IF NOT EXISTS idx_pw_player_b ON pairwise_matches(player_b);
CREATE INDEX IF NOT EXISTS idx_pw_match_id ON pairwise_matches(match_id);

CREATE TABLE IF NOT EXISTS player_ratings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_type TEXT NOT NULL,
    player_count INTEGER DEFAULT 0,
    player_name TEXT NOT NULL,
    rating REAL NOT NULL,
    rd REAL NOT NULL,
    sigma REAL NOT NULL,
    c_rating REAL NOT NULL,
    rank INTEGER,
    rank_delta INTEGER DEFAULT 0,
    games_played INTEGER DEFAULT 0,
    opponents_count INTEGER DEFAULT 0,
    wins INTEGER DEFAULT 0,
    losses INTEGER DEFAULT 0,
    draws INTEGER DEFAULT 0,
    win_rate REAL DEFAULT 0.0,
    last_played TEXT,
    UNIQUE(model_type, player_count, player_name)
);

CREATE INDEX IF NOT EXISTS idx_pr_model_fmt_rank ON player_ratings(model_type, player_count, rank);
CREATE INDEX IF NOT EXISTS idx_pr_model_fmt_player ON player_ratings(model_type, player_count, player_name);

CREATE TABLE IF NOT EXISTS rating_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_type TEXT NOT NULL,
    player_count INTEGER DEFAULT 0,
    player_name TEXT NOT NULL,
    period_date TEXT NOT NULL,
    rating REAL NOT NULL,
    rd REAL NOT NULL,
    c_rating REAL NOT NULL
);

-- Covering index for high-performance player history queries
CREATE INDEX IF NOT EXISTS idx_rh_player_fast ON rating_history(player_name, player_count, period_date, rating);
CREATE INDEX IF NOT EXISTS idx_rh_date ON rating_history(model_type, player_count, period_date);

CREATE TABLE IF NOT EXISTS walk_forward_calibration (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_type TEXT NOT NULL,
    player_count INTEGER DEFAULT 0,
    period_month TEXT NOT NULL,
    matches_evaluated INTEGER NOT NULL,
    brier_score REAL NOT NULL,
    ece REAL NOT NULL,
    log_loss REAL,
    accuracy REAL,
    bin_data_json TEXT,
    UNIQUE(model_type, player_count, period_month)
);

CREATE INDEX IF NOT EXISTS idx_wfc_lookup ON walk_forward_calibration(model_type, player_count, period_month);
CREATE INDEX IF NOT EXISTS idx_wfc_pc_month ON walk_forward_calibration(player_count, period_month);

CREATE TABLE IF NOT EXISTS standard_calibration (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_type TEXT NOT NULL,
    player_count INTEGER DEFAULT 0,
    period_month TEXT NOT NULL DEFAULT 'AGGREGATED',
    matches_evaluated INTEGER NOT NULL,
    brier_score REAL NOT NULL,
    ece REAL NOT NULL,
    log_loss REAL,
    accuracy REAL,
    bin_data_json TEXT,
    UNIQUE(model_type, player_count, period_month)
);

CREATE INDEX IF NOT EXISTS idx_std_calib_lookup ON standard_calibration(model_type, player_count, period_month);
CREATE INDEX IF NOT EXISTS idx_std_calib_pc_month ON standard_calibration(player_count, period_month);
"""


def connect(url=None):
    """Opens the ratings database at url, or at RATINGS_DATABASE_URL, with rows as dicts.

    Raises RuntimeError if neither is set, and psycopg.OperationalError if the
    database cannot be reached.
    """
    url = url or os.environ.get("RATINGS_DATABASE_URL")
    if not url:
        raise RuntimeError("No database: pass a URL or set RATINGS_DATABASE_URL")
    return psycopg.connect(url, row_factory=dict_row)


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
    "model_type, player_count, player_name, rating, rd, sigma, c_rating, rank, rank_delta, "
    "games_played, opponents_count, wins, losses, draws, win_rate, last_played"
)
HISTORY_COLUMNS = "model_type, player_count, player_name, period_date, rating, rd, c_rating"


def save_model_results(conn, run_id, model_type, player_count, rating_rows, history_rows):
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


def get_connection(db_path=None):
    """Opens the database at db_path, or at ENGINE_DB_PATH. Raises RuntimeError if neither is set."""
    path = db_path or os.environ.get("ENGINE_DB_PATH")
    if not path:
        raise RuntimeError("No database: pass db_path or set ENGINE_DB_PATH")
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(path), timeout=60.0)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA foreign_keys = ON;")
    conn.execute("PRAGMA busy_timeout = 60000;")
    return conn


def init_db(db_path=None):
    """Creates any missing engine tables and indexes, keeping existing rows."""
    conn = get_connection(db_path)
    with conn:
        conn.executescript(SCHEMA_SQL)
    conn.close()
