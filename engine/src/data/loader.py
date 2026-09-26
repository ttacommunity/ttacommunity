"""High-performance CSV data loader and match expander for TTA-Glicko2-WHR."""
import csv
import sqlite3
from pathlib import Path
from typing import Optional, Dict, Any, List

from src.data.db import get_connection, init_db

DEFAULT_MATCHES_CSV = Path(__file__).resolve().parents[2] / 'data' / 'raw' / 'all_matches.csv'
DEFAULT_RATINGS_CSV = Path(__file__).resolve().parents[2] / 'data' / 'raw' / 'ratings_overall.csv'

def load_players_metadata(csv_path: Optional[Path] = None, conn: Optional[sqlite3.Connection] = None) -> int:
    path = csv_path or DEFAULT_RATINGS_CSV
    close_conn = False
    if conn is None:
        conn = get_connection()
        close_conn = True

    players_data = []
    with open(path, mode='r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            name = (row.get('Player') or '').strip()
            if not name:
                continue
            code = (row.get('Code') or '').strip()
            if code == '-' or not code:
                code = None
            title = (row.get('Title') or '').strip()
            if title == '-' or not title:
                title = None
            try:
                title_count = int(float(row.get('Title Count') or 0))
            except ValueError:
                title_count = 0
            last_played = (row.get('Last Played') or '').strip()
            KNOWN_COUNTRY_OVERRIDES = {
                'dellcan': 'CN',
                'tianren4561367': 'CN',
                'tinaren': 'CN',
            }
            if name.lower() in KNOWN_COUNTRY_OVERRIDES:
                code = KNOWN_COUNTRY_OVERRIDES[name.lower()]

            players_data.append((name, code, title, title_count, last_played))

    with conn:
        conn.executemany("""
            INSERT INTO players (name, country_code, title, title_count, last_played)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(name) DO UPDATE SET
                country_code = excluded.country_code,
                title = excluded.title,
                title_count = excluded.title_count,
                last_played = excluded.last_played
        """, players_data)

    if close_conn:
        conn.close()
    return len(players_data)


def load_official_baseline(csv_path: Optional[Path] = None, conn: Optional[sqlite3.Connection] = None) -> int:
    path = csv_path or DEFAULT_RATINGS_CSV
    close_conn = False
    if conn is None:
        conn = get_connection()
        close_conn = True

    baseline_data = []
    with open(path, mode='r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            name = (row.get('Player') or '').strip()
            if not name:
                continue
            
            def to_float(val, default=0.0):
                if not val or val == '-' or val == 'unretired':
                    return default
                try:
                    return float(val)
                except ValueError:
                    return default

            def to_int(val, default=None):
                if not val or val == '-' or val == 'unretired':
                    return default
                try:
                    return int(float(val))
                except ValueError:
                    return default

            rank = to_int(row.get('Rank'))
            rank_delta = to_float(row.get('Rank Δ'))
            c_rating = to_float(row.get('C Rating'))
            c_rating_delta = to_float(row.get('C Rating Δ'))
            rating = to_float(row.get('Rating'))
            rating_delta = to_float(row.get('Rating Δ'))
            rd = to_float(row.get('RD'))
            rd_delta = to_float(row.get('RD Δ'))
            opps = to_int(row.get('Opps'), default=0)
            opps_delta = to_float(row.get('Opps Δ'))
            win_rate = to_float(row.get('W%'))
            win_rate_delta = to_float(row.get('W% Δ'))
            last_played = (row.get('Last Played') or '').strip()
            if last_played == '-':
                last_played = None

            baseline_data.append((
                name, rank, rank_delta, c_rating, c_rating_delta,
                rating, rating_delta, rd, rd_delta, opps, opps_delta,
                win_rate, win_rate_delta, last_played
            ))

    with conn:
        conn.execute('DELETE FROM official_baseline;')
        conn.executemany("""
            INSERT INTO official_baseline (
                player_name, rank, rank_delta, c_rating, c_rating_delta,
                rating, rating_delta, rd, rd_delta, opps, opps_delta,
                win_rate, win_rate_delta, last_played
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, baseline_data)

    if close_conn:
        conn.close()
    return len(baseline_data)


def load_matches_and_expand_pairwise(csv_path: Optional[Path] = None, conn: Optional[sqlite3.Connection] = None) -> Dict[str, int]:
    path = csv_path or DEFAULT_MATCHES_CSV
    close_conn = False
    if conn is None:
        conn = get_connection()
        close_conn = True

    matches_to_insert = []
    pairwise_to_insert = []

    match_id = 0
    with open(path, mode='r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            match_id += 1
            tournament = (row.get('Tournament') or '').strip()
            date = (row.get('Date') or '').strip()
            
            # Extract player-score pairs
            participants = []
            for i in range(1, 5):
                p_name = (row.get(f'Player{i}') or '').strip()
                s_val = row.get(f'Score{i}')
                if p_name and s_val is not None and s_val != '':
                    try:
                        score = float(s_val)
                    except ValueError:
                        score = 0.0
                    participants.append((p_name, score))

            p_count = len(participants)
            if p_count < 2:
                continue

            p1 = participants[0][0]
            s1 = participants[0][1]
            p2 = participants[1][0]
            s2 = participants[1][1]
            p3 = participants[2][0] if p_count > 2 else None
            s3 = participants[2][1] if p_count > 2 else None
            p4 = participants[3][0] if p_count > 3 else None
            s4 = participants[3][1] if p_count > 3 else None

            matches_to_insert.append((
                match_id, tournament, date, p_count,
                p1, s1, p2, s2, p3, s3, p4, s4
            ))

            # Pairwise decomposition
            # Weight: w = 1.0 / (N - 1)
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

                    pairwise_to_insert.append((
                        match_id, date, tournament, p_count,
                        pa, pb, sa, sb, out_a, weight
                    ))

    with conn:
        conn.execute('DELETE FROM pairwise_matches;')
        conn.execute('DELETE FROM matches;')
        
        conn.executemany("""
            INSERT INTO matches (
                match_id, tournament, date, player_count,
                player1, score1, player2, score2, player3, score3, player4, score4
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, matches_to_insert)

        conn.executemany("""
            INSERT INTO pairwise_matches (
                match_id, date, tournament, player_count,
                player_a, player_b, score_a, score_b, outcome_a, weight
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """, pairwise_to_insert)

    if close_conn:
        conn.close()

    return {
        'matches_count': len(matches_to_insert),
        'pairwise_count': len(pairwise_to_insert)
    }

def run_ingestion():
    init_db()
    conn = get_connection()
    try:
        p_count = load_players_metadata(conn=conn)
        print(f'Loaded {p_count} players metadata from ratings_overall.csv')
        b_count = load_official_baseline(conn=conn)
        print(f'Loaded {b_count} official baseline records from ratings_overall.csv')
        res = load_matches_and_expand_pairwise(conn=conn)
        print(f"Loaded {res['matches_count']} matches and {res['pairwise_count']} pairwise games.")
    finally:
        conn.close()

if __name__ == '__main__':
    run_ingestion()
