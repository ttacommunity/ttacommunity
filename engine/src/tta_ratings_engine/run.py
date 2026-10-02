"""Computes one whole rating run from the loaded matches: python -m tta_ratings_engine.run.

A run is created as 'running', filled step by step, then marked 'complete', or
'failed' if any step raises. It is published only with --make-current.
"""

import argparse
import functools
import multiprocessing
import os
import time
from concurrent.futures import FIRST_EXCEPTION, ProcessPoolExecutor, wait

from tta_ratings_engine.data.badges import derive_tournament_badges
from tta_ratings_engine.data.db import connect
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.data.tournaments import derive_tournaments
from tta_ratings_engine.models.evaluation.standard_eval import (
    run_standard_calibration_all,
)
from tta_ratings_engine.models.evaluation.walk_forward import (
    run_walk_forward_evaluation,
)
from tta_ratings_engine.models.glicko2.calculator import compute_glicko2_ratings
from tta_ratings_engine.models.whr.calculator import compute_whr_ratings
from tta_ratings_engine.progress import Heartbeat, Progress

FORMATS = [0, 2, 3, 4]
WALK_FORWARD_START = "2022-01"

# Seconds per step, by phase and format, from a full run on 2026-09 data. Only
# the ratios matter: the ETA scales them to this machine as the run goes.
# Retune from the phase summary a run logs at the end.
STEP_COST = {
    ("pairs", None): 18.2,
    ("tournaments", None): 2.5,
    ("titles", None): 0.1,
    ("models", 0): 9.3, ("models", 2): 2.0, ("models", 3): 5.6, ("models", 4): 4.0,
    ("calibration", 0): 8.1, ("calibration", 2): 1.2, ("calibration", 3): 3.9, ("calibration", 4): 2.9,
    ("walk-forward", 0): 5.9, ("walk-forward", 2): 0.8, ("walk-forward", 3): 3.4, ("walk-forward", 4): 2.2,
}  # fmt: skip

_g2 = compute_glicko2_ratings
_whr = functools.partial(compute_whr_ratings, max_iter=6, tol=1e-3, verbose=False)
_softer = {"reset_mode": "softer"}
_retro = {"retro_calibrated": True}

# Upstream's pipeline models plus its retro variants: what the site shows.
MODELS = [
    ("glicko2_daneo", _g2, {"engine": "glicko2_daneo"}),
    ("glicko2_daneo_softer", _g2, {"engine": "glicko2_daneo", **_softer}),
    ("glicko2_std", _g2, {"weighted": False}),
    ("glicko2_std_retro", _g2, {"weighted": False, **_retro}),
    ("glicko2_std_softer", _g2, {"weighted": False, **_softer}),
    ("glicko2_std_softer_retro", _g2, {"weighted": False, **_softer, **_retro}),
    ("glicko2_mp", _g2, {"weighted": True}),
    ("glicko2_mp_retro", _g2, {"weighted": True, **_retro}),
    ("glicko2_mp_softer", _g2, {"weighted": True, **_softer}),
    ("glicko2_mp_softer_retro", _g2, {"weighted": True, **_softer, **_retro}),
    ("glicko2_adapt", _g2, {"engine": "glicko2_adapt"}),
    ("glicko2_adapt_retro", _g2, {"engine": "glicko2_adapt", **_retro}),
    ("glicko2_adapt_softer", _g2, {"engine": "glicko2_adapt", **_softer}),
    ("glicko2_adapt_softer_retro", _g2, {"engine": "glicko2_adapt", **_softer, **_retro}),
    ("whr", _whr, {}),
    ("whr_softer", _whr, _softer),
]  # fmt: skip
MODEL_TYPES = [name for name, _, _ in MODELS]


RESULT_TABLES = [
    "rating_runs",
    "pairwise_matches",
    "player_ratings",
    "rating_history",
    "player_titles",
    "standard_calibration",
    "walk_forward_calibration",
    "tournament_group_divisions",
    "tournament_official_placements",
    "tournament_season_corrections",
    "tournament_finish_dates",
    "tournament_career_honours",
    "tournament_season_rules",
    "tournament_cup_podiums",
    "tournaments",
    "tournament_games",
    "tournament_standings",
    "tournament_boards",
    "tournament_season_records",
    "tournament_series",
    "tournament_legends",
    "tournament_achievements",
    "tournament_records",
]


# What a completed run records about its input, for the site's header and footer.
RUN_STATS = ["games", "players", "pairs", "first_game", "last_game"]

# A tournaments-only run copies these from the current run, and recomputes the rest.
COPIED_TABLES = [
    "player_ratings",
    "rating_history",
    "standard_calibration",
    "walk_forward_calibration",
]
RECOMPUTED_TABLES = [
    "pairwise_matches", "player_titles", "player_peak_titles", "player_yearly_titles",
    "tournament_games", "tournament_standings", "tournament_boards", "tournament_season_records",
    "tournament_series", "tournament_legends", "tournament_achievements", "tournament_records",
]  # fmt: skip


class NoMatches(Exception):
    """Raised when there are no matches to rate."""


class NoCurrentRun(Exception):
    """Raised when a tournaments-only run has no current run to copy."""


class MatchesChanged(Exception):
    """Raised when the matches changed since the run a tournaments-only run would copy."""


class SchemaOutOfDate(Exception):
    """Raised when a table the run writes is missing: the migrations are behind."""


def check_schema(conn):
    """Raises SchemaOutOfDate naming every table in RESULT_TABLES, and every RUN_STATS column, that does not exist."""
    missing = [
        t
        for t in RESULT_TABLES
        if conn.execute("SELECT to_regclass(%s) AS t", (t,)).fetchone()["t"] is None
    ]
    if not missing:
        have = set(_columns(conn, "rating_runs"))
        missing = [f"rating_runs.{c}" for c in RUN_STATS if c not in have]
    if missing:
        raise SchemaOutOfDate(
            f"missing {', '.join(missing)}: apply the ratings migrations first"
        )


def input_fingerprint(conn):
    """Returns an md5 of the run's input, or None with no matches.

    That is every match's content in load order, and with any recorded
    tournament facts, those too. Without them it is the matches' md5 alone,
    as before facts were recorded.
    """
    return conn.execute("""
        WITH m AS (
            SELECT md5(string_agg(
                (tournament, date, player1, score1, player2, score2,
                 player3, score3, player4, score4, glicko_eligible)::text,
                E'\\n' ORDER BY match_id)) AS md5
            FROM matches
        ), f AS (
            SELECT string_agg(r, E'\\n' ORDER BY r) AS facts FROM (
                SELECT 'd' || t::text AS r FROM tournament_group_divisions t
                UNION ALL SELECT 'p' || t::text FROM tournament_official_placements t
                UNION ALL SELECT 'c' || t::text FROM tournament_season_corrections t
                UNION ALL SELECT 'f' || t::text FROM tournament_finish_dates t
                UNION ALL SELECT 'h' || t::text FROM tournament_career_honours t
                UNION ALL SELECT 'r' || t::text FROM tournament_season_rules t
                UNION ALL SELECT 'u' || t::text FROM tournament_cup_podiums t
            ) x
        )
        SELECT CASE WHEN f.facts IS NULL THEN m.md5 ELSE md5(m.md5 || md5(f.facts)) END
               AS fingerprint
        FROM m, f
    """).fetchone()["fingerprint"]


def eval_months(conn, run_id, player_count):
    """Returns how many months walk-forward will evaluate for a format."""
    fmt = "AND player_count = %(fmt)s" if player_count else ""
    return conn.execute(
        f"""
        SELECT count(DISTINCT to_char(date, 'YYYY-MM')) AS n FROM pairwise_matches
        WHERE run_id = %(run)s AND glicko_eligible
          AND to_char(date, 'YYYY-MM') >= %(start)s {fmt}
        """,
        {"run": run_id, "fmt": player_count, "start": WALK_FORWARD_START},
    ).fetchone()["n"]


def _label(fmt):
    return f"{fmt}p" if fmt else "all"


def run(
    conn, engine_version, *, formats=FORMATS, log=print, make_current=False, tournaments_only=False,
    workers=1, url=None,
):  # fmt: skip
    """Computes a run and returns its run_id.

    With tournaments_only, it copies the current run's ratings and
    calibration and recomputes only tournament results and titles, for
    trying a change to those quickly. With workers > 1, the steps after the
    pairs run in that many processes, each connecting to url; the results
    are the same. Raises ValueError for workers without a url, and
    SchemaOutOfDate, NoMatches or NoCurrentRun before creating a run, or
    whatever failed the run (MatchesChanged if the matches differ from the
    copied run's).
    """
    if workers < 1 or (workers > 1 and not url):
        raise ValueError(
            "a parallel run needs one or more workers and the database url"
        )
    with conn.transaction():
        check_schema(conn)
        fingerprint = input_fingerprint(conn)
        if fingerprint is None:
            raise NoMatches("no matches are loaded: nothing to rate")
        source = None
        if tournaments_only:
            row = conn.execute(
                "SELECT run_id FROM current_run WHERE ranking_id = community_ranking()"
            ).fetchone()
            if row is None:
                raise NoCurrentRun("no current run to copy the ratings from")
            source = row["run_id"]
        run_id = conn.execute(
            "INSERT INTO rating_runs (engine_version, input_fingerprint, copied_from) "
            "VALUES (%s, %s, %s) RETURNING run_id",
            (engine_version, fingerprint, source),
        ).fetchone()["run_id"]
    if source is None:
        log(
            f"run {run_id}: engine {engine_version}, input {fingerprint}, formats {formats}"
            + (f", {workers} workers" if workers > 1 else "")
        )
    else:
        log(
            f"run {run_id}: engine {engine_version}, input {fingerprint}, tournaments only, ratings copied from run {source}"
        )

    try:
        if source is None and workers > 1:
            _compute_parallel(conn, run_id, formats, log, workers, url)
        elif source is None:
            _compute(conn, run_id, formats, log)
        else:
            _recompute_tournaments(conn, run_id, source, log)
    except BaseException:
        conn.rollback()
        with conn.transaction():
            _finish(conn, run_id, "failed")
        log(f"run {run_id}: failed")
        raise

    with conn.transaction():
        _record_stats(conn, run_id)
        _finish(conn, run_id, "complete")
        if make_current:
            _publish(conn, run_id)
    log(f"run {run_id}: complete" + (", and now current" if make_current else ""))
    return run_id


def _compute(conn, run_id, formats, log):
    progress = Progress(1, log=log)
    progress.working_on("pairs")
    with Heartbeat(progress):
        expand_pairwise(conn, run_id)
        months = {fmt: eval_months(conn, run_id, fmt) for fmt in formats}
        progress.total = 1 + 2 * len(MODELS) * len(formats) + 2 + sum(months.values())
        progress.total_cost = (
            STEP_COST["pairs", None]
            + STEP_COST["tournaments", None]
            + STEP_COST["titles", None]
            + sum(
                len(MODELS) * (STEP_COST["models", f] + STEP_COST["calibration", f])
                + months[f] * STEP_COST["walk-forward", f]
                for f in formats
            )
        )
        progress.advance("pairs", STEP_COST["pairs", None], "pairs")

        on_run = {"conn": conn, "run_id": run_id}
        for fmt in formats:
            for name, compute, kwargs in MODELS:
                progress.working_on(f"{name} {_label(fmt)}")
                compute(player_count=fmt, **kwargs, **on_run)
                progress.advance(
                    f"{name} {_label(fmt)}",
                    STEP_COST["models", fmt],
                    f"models {_label(fmt)}",
                )

        progress.working_on("tournaments")
        derive_tournaments(**on_run)
        progress.advance("tournaments", STEP_COST["tournaments", None], "tournaments")

        progress.working_on("titles")
        derive_tournament_badges(**on_run)
        progress.advance("titles", STEP_COST["titles", None], "titles")

        for fmt in formats:
            for name in MODEL_TYPES:
                progress.working_on(f"calibration {name} {_label(fmt)}")
                run_standard_calibration_all(
                    verbose=False, model_filter=[name], formats=[fmt], **on_run
                )
                progress.advance(
                    f"calibration {name} {_label(fmt)}",
                    STEP_COST["calibration", fmt],
                    f"calibration {_label(fmt)}",
                )

        for fmt in formats:
            label = f"walk-forward {_label(fmt)}"
            progress.working_on(label)

            def month_done(month, label=label, fmt=fmt):
                progress.advance(
                    f"{label} {month}", STEP_COST["walk-forward", fmt], label
                )

            run_walk_forward_evaluation(
                player_count=fmt,
                start_eval_month=WALK_FORWARD_START,
                verbose=False,
                on_month=month_done,
                **on_run,
            )
    progress.summary()


def _tasks(formats):
    """Returns the steps after the pairs as (kind, format, model), longest first.

    Each reads only the run's pairs and the loaded data, never another
    step's results, so they can run in any order.
    """
    tasks = [("walk-forward", fmt, None) for fmt in formats]
    tasks += [("calibration", fmt, name) for fmt in formats for name in MODEL_TYPES]
    tasks += [("models", fmt, name) for fmt in formats for name in MODEL_TYPES]
    return tasks + [("tournaments", None, None)]


def _run_task(url, run_id, kind, fmt, name):
    """Runs one task from _tasks on its own connection to url, in a worker process.

    Returns the seconds it took.
    """
    start = time.monotonic()
    with connect(url) as conn:
        on_run = {"conn": conn, "run_id": run_id}
        if kind == "models":
            _, compute, kwargs = next(m for m in MODELS if m[0] == name)
            compute(player_count=fmt, **kwargs, **on_run)
        elif kind == "calibration":
            run_standard_calibration_all(
                verbose=False, model_filter=[name], formats=[fmt], **on_run
            )
        elif kind == "walk-forward":
            run_walk_forward_evaluation(
                player_count=fmt, start_eval_month=WALK_FORWARD_START, verbose=False, **on_run
            )  # fmt: skip
        elif kind == "tournaments":
            derive_tournaments(**on_run)
            derive_tournament_badges(**on_run)
        else:
            raise ValueError(f"unknown engine task: {kind}")
    return time.monotonic() - start


def _compute_parallel(conn, run_id, formats, log, workers, url):
    """As _compute, but the steps after the pairs run in worker processes."""
    progress = Progress(1, log=log)
    progress.working_on("pairs")
    with Heartbeat(progress):
        expand_pairwise(conn, run_id)
        months = {fmt: eval_months(conn, run_id, fmt) for fmt in formats}
        tasks = _tasks(formats)
        progress.total = 1 + len(tasks) + 1  # tournaments and titles are reported apart
        progress.total_cost = (
            STEP_COST["pairs", None]
            + STEP_COST["tournaments", None]
            + STEP_COST["titles", None]
            + sum(
                len(MODELS) * (STEP_COST["models", f] + STEP_COST["calibration", f])
                + months[f] * STEP_COST["walk-forward", f]
                for f in formats
            )
        )
        progress.advance("pairs", STEP_COST["pairs", None], "pairs")
        progress.working_on(f"{len(tasks)} steps in {workers} workers")

        # spawn, not fork: a forked child would share the parent's database socket.
        pool = ProcessPoolExecutor(
            workers, mp_context=multiprocessing.get_context("spawn")
        )
        try:
            pending = {pool.submit(_run_task, url, run_id, *t): t for t in tasks}
            while pending:
                done, _ = wait(pending, return_when=FIRST_EXCEPTION)
                for future in done:
                    kind, fmt, name = pending.pop(future)
                    _report(progress, months, kind, fmt, name, future.result())
        finally:
            pool.shutdown(cancel_futures=True)
    progress.summary()


def _report(progress, months, kind, fmt, name, seconds):
    """Advances progress for one finished task from _tasks that took seconds."""
    if kind == "tournaments":
        cost = STEP_COST["tournaments", None]
        progress.advance("tournaments", cost, "tournaments", seconds=seconds)
        progress.advance("titles", STEP_COST["titles", None], "titles", seconds=0.0)
    elif kind == "walk-forward":
        label = f"walk-forward {_label(fmt)}"
        cost = months[fmt] * STEP_COST["walk-forward", fmt]
        progress.advance(label, cost, label, seconds=seconds)
    else:
        label = name if kind == "models" else f"calibration {name}"
        progress.advance(
            f"{label} {_label(fmt)}",
            STEP_COST[kind, fmt],
            f"{kind} {_label(fmt)}",
            seconds=seconds,
        )


def _columns(conn, table, leave_out=("run_id",)):
    return [
        r["column_name"]
        for r in conn.execute(
            "SELECT column_name FROM information_schema.columns "
            "WHERE table_schema = 'public' AND table_name = %s ORDER BY ordinal_position",
            (table,),
        ).fetchall()
        if r["column_name"] not in leave_out
    ]


def _pairs_md5(conn, run_id, columns):
    cols = ", ".join(columns)
    return conn.execute(
        f"SELECT md5(string_agg(row({cols})::text, E'\\n' ORDER BY pairwise_id)) AS md5 "
        "FROM pairwise_matches WHERE run_id = %s",
        (run_id,),
    ).fetchone()["md5"]


def _recompute_tournaments(conn, run_id, source, log):
    """Copies the source run's ratings and calibration into the run, then
    derives its tournament results and titles. The run's pairs must equal the
    source's, or the copied ratings would not be the matches' ratings.
    """
    progress = Progress(4, log=log)
    progress.working_on("pairs")
    expand_pairwise(conn, run_id)
    pair_columns = _columns(conn, "pairwise_matches", ("run_id", "pairwise_id"))
    if _pairs_md5(conn, run_id, pair_columns) != _pairs_md5(conn, source, pair_columns):
        raise MatchesChanged(f"the matches changed since run {source}: make a full run")
    progress.advance("pairs", 1, "pairs")

    progress.working_on("copy")
    with conn.transaction():
        for table in COPIED_TABLES:
            cols = ", ".join(_columns(conn, table))
            conn.execute(
                f"INSERT INTO {table} (run_id, {cols}) SELECT %s, {cols} FROM {table} WHERE run_id = %s",
                (run_id, source),
            )
    progress.advance("copy", 1, "copy")

    on_run = {"conn": conn, "run_id": run_id}
    progress.working_on("tournaments")
    derive_tournaments(**on_run)
    progress.advance("tournaments", 1, "tournaments")
    progress.working_on("titles")
    derive_tournament_badges(**on_run)
    progress.advance("titles", 1, "titles")
    progress.summary()


def _record_stats(conn, run_id):
    """Records the run's games, players and pairs, and its first and last games' dates, from its pairs."""
    conn.execute(
        """
        UPDATE rating_runs SET (games, players, pairs, first_game, last_game) = (
            SELECT count(DISTINCT match_id), (
                SELECT count(*) FROM (
                    SELECT player_a FROM pairwise_matches WHERE run_id = %(run)s
                    UNION SELECT player_b FROM pairwise_matches WHERE run_id = %(run)s) n),
                count(*), min(date), max(date)
            FROM pairwise_matches WHERE run_id = %(run)s)
        WHERE run_id = %(run)s
        """,
        {"run": run_id},
    )


def _finish(conn, run_id, status):
    conn.execute(
        "UPDATE rating_runs SET status = %s, finished_at = now() WHERE run_id = %s",
        (status, run_id),
    )


def _publish(conn, run_id):
    conn.execute(
        "INSERT INTO current_run (run_id) VALUES (%s) "
        "ON CONFLICT (ranking_id) DO UPDATE SET run_id = excluded.run_id",
        (run_id,),
    )
    conn.execute(
        "UPDATE rating_runs SET published_at = now() WHERE run_id = %s", (run_id,)
    )


def main(argv=None):
    """Runs the engine from the command line. Needs RATINGS_DATABASE_URL and ENGINE_VERSION."""
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "--formats", nargs="+", type=int, choices=FORMATS, default=FORMATS,
        help="player counts to rate, 0 for all formats together (default: all)",
    )  # fmt: skip
    parser.add_argument(
        "--make-current", action="store_true",
        help="publish the run when it completes",
    )  # fmt: skip
    parser.add_argument(
        "--workers", type=int, default=1,
        help="processes for the steps after the pairs (default: 1, one after another)",
    )  # fmt: skip
    parser.add_argument(
        "--tournaments-only", action="store_true",
        help="copy the current run's ratings and recompute only tournament results and titles",
    )  # fmt: skip
    args = parser.parse_args(argv)
    version = os.environ.get("ENGINE_VERSION")
    if not version:
        raise SystemExit(
            "ENGINE_VERSION is not set: say which engine this run comes from"
        )
    with connect() as conn:
        run(
            conn,
            version,
            formats=args.formats,
            log=functools.partial(print, flush=True),
            make_current=args.make_current,
            tournaments_only=args.tournaments_only,
            workers=args.workers,
            url=os.environ.get("RATINGS_DATABASE_URL"),
        )


if __name__ == "__main__":
    main()
