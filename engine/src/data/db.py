import os
import sqlite3
from pathlib import Path

DEFAULT_DB_PATH = Path(__file__).resolve().parents[2] / 'data' / 'tta_ratings.db'

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
    replay_code TEXT
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

CREATE TABLE IF NOT EXISTS official_baseline (
    player_name TEXT PRIMARY KEY,
    rank INTEGER,
    rank_delta REAL,
    c_rating REAL,
    c_rating_delta REAL,
    rating REAL,
    rating_delta REAL,
    rd REAL,
    rd_delta REAL,
    opps INTEGER,
    opps_delta REAL,
    win_rate REAL,
    win_rate_delta REAL,
    last_played TEXT
);

CREATE TABLE IF NOT EXISTS yearly_player_stats (
    year INTEGER NOT NULL,
    player_count INTEGER NOT NULL,
    player_name TEXT NOT NULL,
    opponents_count INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    losses INTEGER NOT NULL DEFAULT 0,
    draws INTEGER NOT NULL DEFAULT 0,
    win_rate REAL NOT NULL DEFAULT 0.0,
    last_played TEXT,
    career_opps INTEGER NOT NULL DEFAULT 0,
    career_wins INTEGER NOT NULL DEFAULT 0,
    career_losses INTEGER NOT NULL DEFAULT 0,
    career_draws INTEGER NOT NULL DEFAULT 0,
    career_win_rate REAL NOT NULL DEFAULT 0.0,
    PRIMARY KEY (year, player_count, player_name)
);

CREATE TABLE IF NOT EXISTS pipeline_updates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    cutoff_date TEXT,
    matches_added INTEGER DEFAULT 0,
    description TEXT
);

CREATE INDEX IF NOT EXISTS idx_pu_date ON pipeline_updates(updated_at);
CREATE INDEX IF NOT EXISTS idx_yps_lookup ON yearly_player_stats(year, player_count, player_name);
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

CREATE TABLE IF NOT EXISTS delta_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    active_snapshot_file TEXT,
    cutoff_date TEXT,
    description TEXT,
    updated_at TEXT
);

CREATE TABLE IF NOT EXISTS webmaster_notes (
    key TEXT PRIMARY KEY,
    title TEXT,
    content TEXT,
    updated_at TEXT
);

CREATE TABLE IF NOT EXISTS faq_sections (
    key TEXT PRIMARY KEY,
    title TEXT,
    content TEXT,
    updated_at TEXT
);
CREATE TABLE IF NOT EXISTS cms_content_blocks (
    page_id TEXT NOT NULL,
    block_key TEXT NOT NULL,
    title TEXT,
    content_html TEXT,
    content_markdown TEXT,
    is_deleted INTEGER DEFAULT 0,
    relative_to TEXT,
    placement TEXT,
    sort_order INTEGER DEFAULT 0,
    is_custom_card INTEGER DEFAULT 0,
    updated_at TEXT,
    PRIMARY KEY (page_id, block_key)
);
"""

def get_connection(db_path=None):
    path = db_path or DEFAULT_DB_PATH
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    conn = sqlite3.connect(str(path), timeout=60.0)
    conn.row_factory = sqlite3.Row
    conn.execute('PRAGMA foreign_keys = ON;')
    conn.execute('PRAGMA busy_timeout = 60000;')
    # Do NOT force WAL mode on PythonAnywhere / NFS filesystems
    is_pa = bool(os.environ.get('PYTHONANYWHERE_DOMAIN') or os.environ.get('PYTHONANYWHERE_SITE'))
    if is_pa:
        conn.execute('PRAGMA journal_mode = DELETE;')
    return conn

def init_db(db_path=None):
    conn = get_connection(db_path)
    with conn:
        conn.executescript(SCHEMA_SQL)
        # Migrate glicko_eligible if not yet present
        try:
            conn.execute("ALTER TABLE matches ADD COLUMN glicko_eligible INTEGER NOT NULL DEFAULT 1")
        except sqlite3.OperationalError:
            pass
        try:
            conn.execute("ALTER TABLE matches ADD COLUMN replay_code TEXT")
        except sqlite3.OperationalError:
            pass
        try:
            conn.execute("ALTER TABLE pairwise_matches ADD COLUMN glicko_eligible INTEGER NOT NULL DEFAULT 1")
        except sqlite3.OperationalError:
            pass
        try:
            conn.execute("ALTER TABLE players ADD COLUMN badge_reason TEXT")
        except sqlite3.OperationalError:
            pass
        for col, col_def in [
            ('career_opps', 'INTEGER NOT NULL DEFAULT 0'),
            ('career_wins', 'INTEGER NOT NULL DEFAULT 0'),
            ('career_losses', 'INTEGER NOT NULL DEFAULT 0'),
            ('career_draws', 'INTEGER NOT NULL DEFAULT 0'),
            ('career_win_rate', 'REAL NOT NULL DEFAULT 0.0')
        ]:
            try:
                conn.execute(f"ALTER TABLE yearly_player_stats ADD COLUMN {col} {col_def}")
            except sqlite3.OperationalError:
                pass

        # Migrate cms_content_blocks columns
        for col, col_def in [
            ('is_deleted', 'INTEGER DEFAULT 0'),
            ('relative_to', 'TEXT'),
            ('placement', 'TEXT'),
            ('sort_order', 'INTEGER DEFAULT 0'),
            ('is_custom_card', 'INTEGER DEFAULT 0')
        ]:
            try:
                conn.execute(f"ALTER TABLE cms_content_blocks ADD COLUMN {col} {col_def}")
            except sqlite3.OperationalError:
                pass

        # Migrate existing faq_sections into cms_content_blocks
        try:
            conn.execute("""
                INSERT OR IGNORE INTO cms_content_blocks (page_id, block_key, title, content_html, content_markdown, updated_at)
                SELECT 'faq', key, title, content, content, updated_at FROM faq_sections
            """)
        except sqlite3.OperationalError:
            pass
    conn.close()

def get_all_cms_blocks(conn=None):
    """Returns a dictionary of all custom CMS blocks keyed by (page_id, block_key)."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        cur = conn.execute("""
            SELECT page_id, block_key, title, content_html, content_markdown,
                   is_deleted, relative_to, placement, sort_order, is_custom_card, updated_at
            FROM cms_content_blocks
        """)
        rows = cur.fetchall()
        return {(r['page_id'], r['block_key']): dict(r) for r in rows}
    finally:
        if close_after:
            conn.close()

def save_cms_block(page_id: str, block_key: str, title: str, content_html: str, content_markdown: str,
                   relative_to: str = None, placement: str = None, sort_order: int = 0,
                   is_custom_card: int = 0, is_deleted: int = 0, conn=None):
    """Persists an updated CMS block to the database."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        with conn:
            conn.execute("""
                INSERT INTO cms_content_blocks (
                    page_id, block_key, title, content_html, content_markdown,
                    is_deleted, relative_to, placement, sort_order, is_custom_card, updated_at
                )
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(page_id, block_key) DO UPDATE SET
                    title = excluded.title,
                    content_html = excluded.content_html,
                    content_markdown = excluded.content_markdown,
                    is_deleted = excluded.is_deleted,
                    relative_to = COALESCE(excluded.relative_to, cms_content_blocks.relative_to),
                    placement = COALESCE(excluded.placement, cms_content_blocks.placement),
                    sort_order = excluded.sort_order,
                    is_custom_card = excluded.is_custom_card,
                    updated_at = CURRENT_TIMESTAMP
            """, (page_id, block_key, title, content_html, content_markdown,
                  is_deleted, relative_to, placement, sort_order, is_custom_card))
    finally:
        if close_after:
            conn.close()

def delete_cms_block(page_id: str, block_key: str, conn=None):
    """Marks a card as deleted (is_deleted = 1). If it's a default template card, creates an entry with is_deleted = 1."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        with conn:
            cur = conn.execute("SELECT is_custom_card FROM cms_content_blocks WHERE page_id = ? AND block_key = ?", (page_id, block_key))
            row = cur.fetchone()
            if row:
                conn.execute("UPDATE cms_content_blocks SET is_deleted = 1, updated_at = CURRENT_TIMESTAMP WHERE page_id = ? AND block_key = ?", (page_id, block_key))
            else:
                conn.execute("""
                    INSERT INTO cms_content_blocks (page_id, block_key, title, content_html, content_markdown, is_deleted, is_custom_card, updated_at)
                    VALUES (?, ?, '', '', '', 1, 0, CURRENT_TIMESTAMP)
                """, (page_id, block_key))
            return True
    finally:
        if close_after:
            conn.close()

def restore_cms_block(page_id: str, block_key: str, conn=None):
    """Restores a deleted block (either unsets is_deleted for custom card, or deletes override for default card)."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        with conn:
            cur = conn.execute("SELECT is_custom_card FROM cms_content_blocks WHERE page_id = ? AND block_key = ?", (page_id, block_key))
            row = cur.fetchone()
            if row:
                if row['is_custom_card'] == 1:
                    conn.execute("UPDATE cms_content_blocks SET is_deleted = 0, updated_at = CURRENT_TIMESTAMP WHERE page_id = ? AND block_key = ?", (page_id, block_key))
                else:
                    conn.execute("DELETE FROM cms_content_blocks WHERE page_id = ? AND block_key = ?", (page_id, block_key))
                return True
            return False
    finally:
        if close_after:
            conn.close()

def revert_cms_block(page_id: str, block_key: str, conn=None):
    """Deletes an override from cms_content_blocks, reverting the block to template default."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        with conn:
            cur = conn.execute("DELETE FROM cms_content_blocks WHERE page_id = ? AND block_key = ?", (page_id, block_key))
            return cur.rowcount > 0
    finally:
        if close_after:
            conn.close()

def revert_all_cms_blocks(page_id: str = None, conn=None):
    """Deletes all overrides (optionally filtered by page_id), reverting blocks to template defaults."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        with conn:
            if page_id:
                cur = conn.execute("DELETE FROM cms_content_blocks WHERE page_id = ?", (page_id,))
            else:
                cur = conn.execute("DELETE FROM cms_content_blocks")
            return cur.rowcount
    finally:
        if close_after:
            conn.close()

def get_cms_block(page_id: str, block_key: str, conn=None):
    """Fetches a single CMS block by page_id and block_key."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        cur = conn.execute("""
            SELECT page_id, block_key, title, content_html, content_markdown,
                   is_deleted, relative_to, placement, sort_order, is_custom_card, updated_at
            FROM cms_content_blocks
            WHERE page_id = ? AND block_key = ?
        """, (page_id, block_key))
        row = cur.fetchone()
        return dict(row) if row else None
    finally:
        if close_after:
            conn.close()

def get_custom_cards(page_id: str = None, conn=None):
    """Returns all active (non-deleted) custom-created cards, optionally filtered by page_id."""
    close_after = False
    if conn is None:
        conn = get_connection()
        close_after = True
    try:
        if page_id:
            cur = conn.execute("""
                SELECT page_id, block_key, title, content_html, content_markdown,
                       relative_to, placement, sort_order, updated_at
                FROM cms_content_blocks
                WHERE page_id = ? AND is_custom_card = 1 AND (is_deleted = 0 OR is_deleted IS NULL)
                ORDER BY sort_order ASC, updated_at ASC
            """, (page_id,))
        else:
            cur = conn.execute("""
                SELECT page_id, block_key, title, content_html, content_markdown,
                       relative_to, placement, sort_order, updated_at
                FROM cms_content_blocks
                WHERE is_custom_card = 1 AND (is_deleted = 0 OR is_deleted IS NULL)
                ORDER BY sort_order ASC, updated_at ASC
            """)
        return [dict(r) for r in cur.fetchall()]
    finally:
        if close_after:
            conn.close()

def log_pipeline_update(conn, cutoff_date=None, matches_added=0, description=""):
    """Logs a webmaster update event for delta tracking."""
    with conn:
        conn.execute(
            "INSERT INTO pipeline_updates (cutoff_date, matches_added, description) VALUES (?, ?, ?)",
            (cutoff_date, matches_added, description)
        )

def populate_yearly_stats(conn):
    """Populates yearly_player_stats by aggregating pairwise matches per year and player format with full player_a / player_b symmetry and career-to-date cumulative stats."""
    with conn:
        conn.execute("DELETE FROM yearly_player_stats")
        # 1. Format-specific yearly stats (player_count 2, 3, 4)
        conn.execute("""
            WITH pp AS (
                SELECT 
                    CAST(SUBSTR(date, 1, 4) AS INTEGER) as yr,
                    player_count,
                    player_a as player_name,
                    CASE WHEN outcome_a = 1.0 THEN 1 ELSE 0 END as win,
                    CASE WHEN outcome_a = 0.0 THEN 1 ELSE 0 END as loss,
                    CASE WHEN outcome_a = 0.5 THEN 1 ELSE 0 END as draw,
                    date
                FROM pairwise_matches
                UNION ALL
                SELECT 
                    CAST(SUBSTR(date, 1, 4) AS INTEGER) as yr,
                    player_count,
                    player_b as player_name,
                    CASE WHEN outcome_a = 0.0 THEN 1 ELSE 0 END as win,
                    CASE WHEN outcome_a = 1.0 THEN 1 ELSE 0 END as loss,
                    CASE WHEN outcome_a = 0.5 THEN 1 ELSE 0 END as draw,
                    date
                FROM pairwise_matches
            ),
            season_agg AS (
                SELECT 
                    yr,
                    player_count,
                    player_name,
                    COUNT(*) as opps,
                    SUM(win) as wins,
                    SUM(loss) as losses,
                    SUM(draw) as draws,
                    ROUND((SUM(win) + 0.5 * SUM(draw)) / COUNT(*) * 100.0, 1) as win_rate,
                    MAX(date) as last_played
                FROM pp
                GROUP BY yr, player_count, player_name
            )
            INSERT INTO yearly_player_stats (
                year, player_count, player_name, opponents_count, wins, losses, draws, win_rate, last_played,
                career_opps, career_wins, career_losses, career_draws, career_win_rate
            )
            SELECT 
                yr, player_count, player_name, opps, wins, losses, draws, win_rate, last_played,
                SUM(opps) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_opps,
                SUM(wins) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_wins,
                SUM(losses) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_losses,
                SUM(draws) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_draws,
                ROUND(
                    (SUM(wins) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
                     + 0.5 * SUM(draws) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW))
                    / SUM(opps) OVER (PARTITION BY player_count, player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) * 100.0,
                    1
                ) as c_win_rate
            FROM season_agg
        """)

        # 2. All-Formats yearly stats (player_count = 0)
        conn.execute("""
            WITH pp AS (
                SELECT 
                    CAST(SUBSTR(date, 1, 4) AS INTEGER) as yr,
                    player_a as player_name,
                    CASE WHEN outcome_a = 1.0 THEN 1 ELSE 0 END as win,
                    CASE WHEN outcome_a = 0.0 THEN 1 ELSE 0 END as loss,
                    CASE WHEN outcome_a = 0.5 THEN 1 ELSE 0 END as draw,
                    date
                FROM pairwise_matches
                UNION ALL
                SELECT 
                    CAST(SUBSTR(date, 1, 4) AS INTEGER) as yr,
                    player_b as player_name,
                    CASE WHEN outcome_a = 0.0 THEN 1 ELSE 0 END as win,
                    CASE WHEN outcome_a = 1.0 THEN 1 ELSE 0 END as loss,
                    CASE WHEN outcome_a = 0.5 THEN 1 ELSE 0 END as draw,
                    date
                FROM pairwise_matches
            ),
            season_agg AS (
                SELECT 
                    yr,
                    0 as player_count,
                    player_name,
                    COUNT(*) as opps,
                    SUM(win) as wins,
                    SUM(loss) as losses,
                    SUM(draw) as draws,
                    ROUND((SUM(win) + 0.5 * SUM(draw)) / COUNT(*) * 100.0, 1) as win_rate,
                    MAX(date) as last_played
                FROM pp
                GROUP BY yr, player_name
            )
            INSERT INTO yearly_player_stats (
                year, player_count, player_name, opponents_count, wins, losses, draws, win_rate, last_played,
                career_opps, career_wins, career_losses, career_draws, career_win_rate
            )
            SELECT 
                yr, player_count, player_name, opps, wins, losses, draws, win_rate, last_played,
                SUM(opps) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_opps,
                SUM(wins) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_wins,
                SUM(losses) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_losses,
                SUM(draws) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) as c_draws,
                ROUND(
                    (SUM(wins) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)
                     + 0.5 * SUM(draws) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW))
                    / SUM(opps) OVER (PARTITION BY player_name ORDER BY yr ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) * 100.0,
                    1
                ) as c_win_rate
            FROM season_agg
        """)

def ensure_yearly_stats(conn):
    """Ensures yearly_player_stats table exists and contains data."""
    c = conn.cursor()
    c.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='yearly_player_stats'")
    if not c.fetchone():
        init_db()
    c.execute("SELECT COUNT(*) FROM yearly_player_stats")
    row = c.fetchone()
    if not row or row[0] == 0:
        populate_yearly_stats(conn)

