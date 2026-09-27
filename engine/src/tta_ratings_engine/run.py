"""Computes one whole rating run from the loaded matches: python -m tta_ratings_engine.run.

A run is created as 'running', filled step by step, then marked 'complete', or
'failed' if any step raises. It is published only with --make-current.
"""

import argparse
import functools
import os

from tta_ratings_engine.data.badges import derive_tournament_badges
from tta_ratings_engine.data.db import connect
from tta_ratings_engine.data.pairwise import expand_pairwise
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


class NoMatches(Exception):
    """Raised when there are no matches to rate."""


def input_fingerprint(conn):
    """Returns an md5 of every match's content in load order, or None with no matches."""
    return conn.execute("""
        SELECT md5(string_agg(
            (tournament, date, player1, score1, player2, score2,
             player3, score3, player4, score4, glicko_eligible)::text,
            E'\\n' ORDER BY match_id)) AS fingerprint
        FROM matches
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


def run(conn, engine_version, *, formats=FORMATS, log=print, make_current=False):
    """Computes a whole run and returns its run_id. Raises NoMatches, or whatever failed the run."""
    with conn.transaction():
        fingerprint = input_fingerprint(conn)
        if fingerprint is None:
            raise NoMatches("no matches are loaded: nothing to rate")
        run_id = conn.execute(
            "INSERT INTO rating_runs (engine_version, input_fingerprint) "
            "VALUES (%s, %s) RETURNING run_id",
            (engine_version, fingerprint),
        ).fetchone()["run_id"]
    log(
        f"run {run_id}: engine {engine_version}, input {fingerprint}, formats {formats}"
    )

    try:
        _compute(conn, run_id, formats, log)
    except BaseException:
        conn.rollback()
        with conn.transaction():
            _finish(conn, run_id, "failed")
        log(f"run {run_id}: failed")
        raise

    with conn.transaction():
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
        progress.total = 1 + 2 * len(MODELS) * len(formats) + 1 + sum(months.values())
        progress.advance("pairs")

        on_run = {"conn": conn, "run_id": run_id}
        for fmt in formats:
            for name, compute, kwargs in MODELS:
                progress.working_on(f"{name} {_label(fmt)}")
                compute(player_count=fmt, **kwargs, **on_run)
                progress.advance(f"{name} {_label(fmt)}")

        progress.working_on("titles")
        derive_tournament_badges(**on_run)
        progress.advance("titles")

        for fmt in formats:
            for name in MODEL_TYPES:
                progress.working_on(f"calibration {name} {_label(fmt)}")
                run_standard_calibration_all(
                    verbose=False, model_filter=[name], formats=[fmt], **on_run
                )
                progress.advance(f"calibration {name} {_label(fmt)}")

        for fmt in formats:
            label = f"walk-forward {_label(fmt)}"
            progress.working_on(label)
            run_walk_forward_evaluation(
                player_count=fmt,
                start_eval_month=WALK_FORWARD_START,
                verbose=False,
                on_month=lambda month, label=label: progress.advance(
                    f"{label} {month}"
                ),
                **on_run,
            )


def _finish(conn, run_id, status):
    conn.execute(
        "UPDATE rating_runs SET status = %s, finished_at = now() WHERE run_id = %s",
        (status, run_id),
    )


def _publish(conn, run_id):
    conn.execute(
        "INSERT INTO current_run (run_id) VALUES (%s) "
        "ON CONFLICT (only_row) DO UPDATE SET run_id = excluded.run_id",
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
        )


if __name__ == "__main__":
    main()
