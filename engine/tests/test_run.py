import hashlib
import json
import multiprocessing
import os
import random
import signal
import sys
from datetime import UTC, date, datetime, timedelta
from decimal import Decimal
from importlib import metadata
from pathlib import Path

import pytest

from tta_ratings_engine import run as runner
from tta_ratings_engine.data.db import connect
from tta_ratings_engine.run import FORMATS, MODEL_TYPES, STEP_COST, main, run

KEPT = {"glicko2_daneo", "glicko2_daneo_softer", "glicko2_std"}


def boom(**_):
    raise RuntimeError("boom")


def season(add_match):
    for month in (11, 12):
        add_match(
            ("Ann", 10),
            ("Bob", 5),
            day=date(2021, month, 3),
            tournament="RL_s01 - Emperor",
        )
    for month in (1, 2):
        add_match(
            ("Ann", 10),
            ("Bob", 5),
            ("Cat", 1),
            day=date(2022, month, 3),
            tournament="Cup",
        )
        add_match(("Bob", 10), ("Cat", 5), day=date(2022, month, 9), tournament="Cup")


def count(conn, table, run_id):
    return conn.execute(
        f"SELECT count(*) AS n FROM {table} WHERE run_id = %s", (run_id,)
    ).fetchone()["n"]


def the_run(conn, run_id):
    return conn.execute(
        "SELECT * FROM rating_runs WHERE run_id = %s", (run_id,)
    ).fetchone()


def current(conn):
    row = conn.execute("SELECT run_id FROM current_run").fetchone()
    return row and row["run_id"]


def test_the_model_list_is_glicko_d_both_ways_and_standard_glicko2():
    assert set(MODEL_TYPES) == KEPT


def test_a_run_writes_every_result_and_completes(conn, add_match):
    season(add_match)

    run_id = run(conn, "v-test", formats=[0, 2], log=lambda _: None)

    assert the_run(conn, run_id)["status"] == "complete"
    assert the_run(conn, run_id)["finished_at"] is not None
    types = conn.execute(
        "SELECT DISTINCT model_type FROM player_ratings WHERE run_id = %s", (run_id,)
    ).fetchall()
    assert {r["model_type"] for r in types} == KEPT
    assert count(conn, "standard_calibration", run_id) == 3 * 2
    assert count(conn, "walk_forward_calibration", run_id) > 0
    assert count(conn, "player_titles", run_id) == 2
    assert count(conn, "pairwise_matches", run_id) == 10


def test_a_run_writes_tournament_results(conn, add_match):
    season(add_match)
    add_match(
        ("Ann", 90),
        ("Bob", 80),
        ("Cat", 70),
        day=date(2022, 2, 12),
        tournament="International S30 - Grandmaster game 1",
    )
    conn.execute(
        "INSERT INTO settings (series_id, points_3p) SELECT series_id, '{5,2,0}' FROM series WHERE slug = 'rated'"
    )
    conn.commit()

    run_id = run(conn, "v-test", formats=[2], log=lambda _: None)

    def international(table):
        return conn.execute(
            f"SELECT count(*) AS n FROM {table} WHERE run_id = %s AND series = 'international'",
            (run_id,),
        ).fetchone()["n"]

    assert international("tournament_games") == 1
    assert international("tournament_standings") == 3
    assert international("tournament_boards") == 3
    assert (
        count(conn, "tournament_games", run_id) == 3
    )  # and season()'s two Royal League games
    ann = conn.execute(
        "SELECT details FROM tournament_records WHERE run_id = %s AND player_name = 'Ann' "
        "AND tournament = 'International Championship' ORDER BY position",
        (run_id,),
    ).fetchall()
    assert [r["details"] for r in ann] == [
        "Participated in 1 seasons, total score: 5 pts",
        "Season 30 International Championship Standing (1 of 3) • Record: 1W - 0L (100.0% win rate)",
    ]
    medals = conn.execute(
        "SELECT player_name, international_titles, gold, silver, bronze, accolades "
        "FROM tournament_achievements WHERE run_id = %s ORDER BY player_name",
        (run_id,),
    ).fetchall()
    # season()'s Royal League games give Ann a title and Bob a silver too
    assert [tuple(r.values()) for r in medals] == [
        (
            "Ann",
            1,
            2,
            0,
            0,
            ["1x International Champion", "1x Royal League Emperor Champion"],
        ),
        ("Bob", 0, 0, 2, 0, []),
        ("Cat", 0, 0, 0, 1, []),
    ]


@pytest.mark.parametrize(
    "fact",
    [
        (
            "INSERT INTO tournament_group_divisions "
            "VALUES ('international', 3, 'Grandmaster', 'Ann', 'sheet')"
        ),
        (
            "INSERT INTO tournament_official_placements "
            "VALUES ('royal_league', 1, 'Emperor', 'Ann', 1, NULL, 'r', 's')"
        ),
        (
            "INSERT INTO tournament_season_corrections "
            "VALUES ('intermezzo', 8, 7, 'r', 's')"
        ),
        "INSERT INTO tournament_finish_dates VALUES ('intermezzo', 8, '2020-08-31', 's')",
        (
            "INSERT INTO tournament_career_honours "
            "VALUES ('q_and_d', 'Ann', 1, 0, 0, NULL, 's')"
        ),
        "INSERT INTO settings (series_id, timeout_penalty) SELECT series_id, -1 FROM series WHERE slug = 'rated'",
        "INSERT INTO tournament_cup_podiums VALUES ('wimbledon', '2024', '', 1, 'Ann', 's', '2025-02-24')",
        "INSERT INTO tournament_podium_overrides (tournament, season, place, player_name) VALUES ('worlds', '2025', 1, 'Ann')",
        "INSERT INTO cge_tournaments (cge_tournament, name, status) VALUES (5204, 'World Championship 2025', 'finished')",
        "INSERT INTO player_title_overrides VALUES ('Ann', 'GM', 'r')",
        "UPDATE tournaments SET rated = false",
        "UPDATE series SET rated_player_counts = '{2,3}'",
    ],
)
def test_a_recorded_fact_changes_the_input_fingerprint(conn, add_match, fact):
    season(add_match)
    ranking = conn.execute("SELECT community_ranking() AS id").fetchone()["id"]
    first = runner.input_fingerprint(conn, ranking)
    conn.execute(fact)
    conn.commit()

    assert runner.input_fingerprint(conn, ranking) != first


def test_a_run_is_not_published_unless_asked(conn, add_match):
    season(add_match)

    run(conn, "v-test", formats=[2], log=lambda _: None)

    assert current(conn) is None


def prewarmed(lines):
    return [line for line in lines if "player pages prewarmed" in line]


def test_a_run_ends_by_prewarming_the_current_runs_player_pages(conn, add_match):
    season(add_match)
    first, second = [], []

    unpublished = run(conn, "v1", formats=[2], log=first.append)
    published = run(conn, "v1", formats=[2], log=second.append, make_current=True)

    assert unpublished != published
    assert prewarmed(first) == []  # nothing is current yet
    [line] = prewarmed(second)
    assert line.startswith(f"run {published}'s player pages prewarmed: ")
    assert int(line.split(": ")[1].split()[0]) > 0


def test_a_run_can_be_published_when_it_completes(conn, add_match):
    season(add_match)

    run_id = run(conn, "v-test", formats=[2], log=lambda _: None, make_current=True)

    assert current(conn) == run_id
    assert the_run(conn, run_id)["published_at"] is not None


def test_a_run_records_its_version_and_input(conn, add_match):
    season(add_match)
    first = run(conn, "v1", formats=[2], log=lambda _: None)
    second = run(conn, "v2", formats=[2], log=lambda _: None)
    add_match(("Ann", 1), ("Cat", 2), day=date(2022, 2, 20))
    third = run(conn, "v2", formats=[2], log=lambda _: None)

    runs = [the_run(conn, r) for r in (first, second, third)]
    assert [r["engine_version"] for r in runs] == ["v1", "v2", "v2"]
    assert runs[0]["input_fingerprint"] == runs[1]["input_fingerprint"]
    assert runs[2]["input_fingerprint"] != runs[1]["input_fingerprint"]


def test_progress_counts_every_step_up_front(conn, add_match):
    season(add_match)
    lines = []

    run(conn, "v-test", formats=[0, 2], log=lines.append)

    progress = [line for line in lines if line.startswith("[")]
    # pairs + 3 models x 2 formats + tournaments + titles + 3 calibrations x 2
    # + walk-forward months (2 + 2)
    assert progress[-1].startswith("[19/19] ")


def test_a_failure_marks_the_run_failed_and_is_raised(conn, add_match, monkeypatch):
    season(add_match)
    monkeypatch.setattr(runner, "derive_tournament_badges", boom)

    with pytest.raises(RuntimeError, match="boom"):
        run(conn, "v-test", formats=[2], log=lambda _: None, make_current=True)

    row = conn.execute("SELECT status, finished_at FROM rating_runs").fetchone()
    assert row["status"] == "failed"
    assert row["finished_at"] is not None
    assert current(conn) is None


def test_the_engine_version_is_the_one_in_pyproject():
    assert runner.engine_version() == metadata.version("tta-ratings-engine")


def test_the_command_records_the_engine_version_from_pyproject(
    monkeypatch, db_url, conn, add_match
):
    season(add_match)
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)
    monkeypatch.setenv("ENGINE_VERSION", "set-by-hand")

    main(["--formats", "2", "--make-current"])

    assert the_run(conn, current(conn))["engine_version"] == metadata.version(
        "tta-ratings-engine"
    )


def test_the_command_runs_the_formats_asked_for(monkeypatch, db_url, conn, add_match):
    season(add_match)
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)

    main(["--formats", "2", "--make-current"])

    run_id = current(conn)
    formats = conn.execute(
        "SELECT DISTINCT player_count FROM player_ratings WHERE run_id = %s", (run_id,)
    ).fetchall()
    assert [r["player_count"] for r in formats] == [2]


def test_a_run_ends_with_the_time_each_phase_took(conn, add_match):
    season(add_match)
    lines = []

    run(conn, "v-test", formats=[2], log=lines.append)

    phases = [
        line.split(":")[0]
        for line in lines
        if " steps in " in line or " step in " in line
    ]
    assert phases == [
        "pairs",
        "models 2p",
        "tournaments",
        "titles",
        "calibration 2p",
        "walk-forward 2p",
    ]


def test_every_step_has_a_cost_for_the_eta():
    for fmt in FORMATS:
        for phase in ("models", "calibration", "walk-forward"):
            assert STEP_COST[phase, fmt] > 0
    for phase in ("pairs", "tournaments", "titles"):
        assert STEP_COST[phase, None] > 0


def test_a_failed_run_keeps_the_steps_it_finished(conn, db_url, add_match, monkeypatch):
    season(add_match)
    monkeypatch.setattr(runner, "derive_tournament_badges", boom)

    with pytest.raises(RuntimeError):
        run(conn, "v-test", formats=[2], log=lambda _: None)

    with connect(db_url) as other:
        run_id = other.execute("SELECT run_id FROM rating_runs").fetchone()["run_id"]
        assert count(other, "pairwise_matches", run_id) == 10
        assert count(other, "player_ratings", run_id) > 0


def test_a_run_records_how_long_it_took(conn, add_match, monkeypatch):
    season(add_match)
    now = [datetime(2026, 1, 1, 12, 0, tzinfo=UTC)]
    real = runner.derive_tournament_badges

    def slow(**kw):
        now[0] += timedelta(minutes=7, seconds=30)
        return real(**kw)

    monkeypatch.setattr(runner, "derive_tournament_badges", slow)

    run_id = run(conn, "v-test", formats=[2], log=lambda _: None, clock=lambda: now[0])

    took = conn.execute(
        "SELECT finished_at - started_at AS took FROM rating_runs WHERE run_id = %s",
        (run_id,),
    ).fetchone()["took"]
    assert took == timedelta(minutes=7, seconds=30)


def rows_of(conn, table, run_id):
    """Every row of a run in a table, without its run and pair ids, in a stable order."""
    rows = conn.execute(
        f"SELECT * FROM {table} WHERE run_id = %s", (run_id,)
    ).fetchall()
    ids = ("run_id", "pairwise_id")
    return sorted(str({k: v for k, v in r.items() if k not in ids}) for r in rows)


def test_a_tournaments_only_run_copies_the_ratings_and_recomputes_tournaments(
    conn, add_match
):
    season(add_match)
    full = run(conn, "v1", formats=[0, 2], log=lambda _: None, make_current=True)
    conn.execute(
        "INSERT INTO tournament_official_placements VALUES ('royal_league', 1, 'Emperor', 'Bob', 1, NULL, 'r', 's')"
    )
    conn.commit()

    fast = run(conn, "v2", tournaments_only=True, log=lambda _: None, make_current=True)

    assert the_run(conn, fast)["status"] == "complete"
    assert the_run(conn, fast)["copied_from"] == full
    assert current(conn) == fast
    for table in runner.COPIED_TABLES + ["pairwise_matches"]:
        assert rows_of(conn, table, fast) == rows_of(conn, table, full), table
    boards = conn.execute(
        "SELECT player_name, gold FROM tournament_boards WHERE run_id = %s AND series = 'royal_league' ORDER BY player_name",
        (fast,),
    ).fetchall()
    assert [(r["player_name"], r["gold"]) for r in boards] == [
        ("Bob", 1)
    ]  # the recorded place replaces the derived one
    assert count(conn, "player_titles", fast) == count(conn, "player_titles", full)


def test_a_tournaments_only_run_refuses_changed_matches(conn, add_match):
    season(add_match)
    full = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)
    add_match(("Ann", 10), ("Cat", 5), day=date(2022, 3, 1), tournament="Cup")

    with pytest.raises(runner.MatchesChanged):
        run(conn, "v2", tournaments_only=True, log=lambda _: None, make_current=True)

    failed = conn.execute(
        "SELECT run_id, status FROM rating_runs WHERE run_id > %s", (full,)
    ).fetchall()
    assert [r["status"] for r in failed] == ["failed"]
    assert count(conn, "player_ratings", failed[0]["run_id"]) == 0
    assert current(conn) == full


def test_a_tournaments_only_run_needs_a_current_run(conn, add_match):
    season(add_match)
    with pytest.raises(runner.NoCurrentRun):
        run(conn, "v1", tournaments_only=True, log=lambda _: None)
    assert conn.execute("SELECT count(*) AS n FROM rating_runs").fetchone()["n"] == 0


def test_every_run_table_is_copied_or_recomputed(conn):
    tables = {
        r["table_name"]
        for r in conn.execute(
            "SELECT table_name FROM information_schema.columns "
            "WHERE table_schema = 'public' AND column_name = 'run_id'"
        ).fetchall()
    }
    assert tables == set(runner.COPIED_TABLES) | set(runner.RECOMPUTED_TABLES) | {
        "rating_runs",
        "current_run",
        "run_progress",
    }


def test_a_parallel_run_writes_what_a_sequential_run_writes(conn, db_url, add_match):
    season(add_match)
    sequential = run(conn, "v1", formats=[0, 2], log=lambda _: None)

    parallel = run(
        conn, "v1", formats=[0, 2], log=lambda _: None, workers=3, url=db_url
    )

    assert the_run(conn, parallel)["status"] == "complete"
    for table in runner.COPIED_TABLES + runner.RECOMPUTED_TABLES:
        assert rows_of(conn, table, parallel) == rows_of(conn, table, sequential), table
    assert count(conn, "walk_forward_calibration", parallel) > 0


def test_a_parallel_run_counts_every_task_up_front(conn, db_url, add_match):
    season(add_match)
    lines = []

    run(conn, "v-test", formats=[0, 2], log=lines.append, workers=2, url=db_url)

    progress = [line for line in lines if line.startswith("[")]
    # pairs + 3 models x 2 formats + tournaments + titles + 3 calibrations x 2
    # + one walk-forward per format
    assert progress[-1].startswith("[17/17] ")


def test_a_parallel_run_needs_the_database_url(conn, add_match):
    season(add_match)
    with pytest.raises(ValueError, match="url"):
        run(conn, "v-test", formats=[2], log=lambda _: None, workers=2)
    assert conn.execute("SELECT count(*) AS n FROM rating_runs").fetchone()["n"] == 0


def test_a_failed_task_fails_a_parallel_run(conn, db_url, add_match, monkeypatch):
    season(add_match)
    real = runner._tasks
    monkeypatch.setattr(
        runner, "_tasks", lambda formats: real(formats) + [("no-such-task", 2, None)]
    )

    with pytest.raises(ValueError, match="no-such-task"):
        run(conn, "v-test", formats=[2], log=lambda _: None, workers=2, url=db_url)

    assert (
        conn.execute("SELECT status FROM rating_runs").fetchone()["status"] == "failed"
    )


def test_the_command_runs_in_parallel_when_asked(monkeypatch, db_url, conn, add_match):
    season(add_match)
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)
    lines = []
    monkeypatch.setattr(
        "builtins.print", lambda *a, **_: lines.append(" ".join(map(str, a)))
    )

    main(["--formats", "2", "--workers", "2", "--make-current"])

    assert the_run(conn, current(conn))["status"] == "complete"
    assert any("2 workers" in line for line in lines)


def run_stats(conn, run_id):
    return conn.execute(
        "SELECT games, players, pairs, first_game::text, last_game::text FROM rating_runs WHERE run_id = %s",
        (run_id,),
    ).fetchone()


def test_a_run_records_its_games_players_and_dates(conn, add_match):
    season(add_match)

    run_id = run(conn, "v-test", formats=[2], log=lambda _: None)

    assert run_stats(conn, run_id) == {
        "games": 6,
        "players": 3,
        "pairs": 10,
        "first_game": "2021-11-03",
        "last_game": "2022-02-09",
    }


def test_a_tournaments_only_run_records_its_stats_too(conn, add_match):
    season(add_match)
    full = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)

    fast = run(conn, "v2", tournaments_only=True, log=lambda _: None)

    assert run_stats(conn, fast) == run_stats(conn, full)


def add_ranking(conn, slug, *tournaments):
    """Adds ranking slug holding test tournaments already made by add_match."""
    rid = conn.execute(
        "INSERT INTO rankings (slug, name) VALUES (%s, %s) RETURNING ranking_id",
        (slug, slug),
    ).fetchone()["ranking_id"]
    for t in tournaments:
        conn.execute(
            "INSERT INTO ranking_series (ranking_id, series_id) "
            "SELECT %s, series_id FROM series WHERE slug = %s",
            (rid, t),
        )
    conn.commit()


def test_a_run_rates_the_ranking_it_names_and_publishes_it_alone(conn, add_match):
    season(add_match)
    community = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)
    for month in (3, 4):
        add_match(("Eve", 10), ("Fay", 5), day=date(2022, month, 1), placed="blitz_cup")
    add_ranking(conn, "blitz", "blitz_cup")

    blitz = run(
        conn, "v1", formats=[2], log=lambda _: None, make_current=True, ranking="blitz"
    )

    players = conn.execute(
        "SELECT DISTINCT player_name FROM player_ratings WHERE run_id = %s ORDER BY 1",
        (blitz,),
    ).fetchall()
    assert [p["player_name"] for p in players] == ["Eve", "Fay"]
    currents = conn.execute(
        "SELECT k.slug, c.run_id FROM current_run c JOIN rankings k USING (ranking_id) ORDER BY 1"
    ).fetchall()
    assert [(c["slug"], c["run_id"]) for c in currents] == [
        ("blitz", blitz),
        ("community", community),
    ]


def test_a_rankings_tournaments_are_part_of_its_input(conn, add_match):
    season(add_match)
    add_match(("Eve", 10), ("Fay", 5), placed="other")
    first = run(conn, "v1", formats=[2], log=lambda _: None)
    conn.execute(
        "INSERT INTO ranking_series (ranking_id, series_id) "
        "SELECT community_ranking(), series_id FROM series WHERE slug = 'other'"
    )
    conn.commit()

    second = run(conn, "v1", formats=[2], log=lambda _: None)

    assert (
        the_run(conn, first)["input_fingerprint"]
        != the_run(conn, second)["input_fingerprint"]
    )


def test_a_game_never_played_is_not_counted_as_a_game(conn, add_match):
    season(add_match)
    add_match(("Ann", -5), ("Zed", -5), day=date(2022, 3, 1), state="never_played")

    run_id = run(conn, "v1", formats=[2], log=lambda _: None)

    stats = the_run(conn, run_id)
    assert stats["games"] == 6
    assert stats["last_game"] == date(2022, 2, 9)


def test_a_games_state_is_part_of_the_input(conn, add_match):
    season(add_match)
    m = add_match(("Ann", -5), ("Bob", -5), day=date(2022, 3, 1))
    first = run(conn, "v1", formats=[2], log=lambda _: None)
    conn.execute("UPDATE games SET state = 'never_played' WHERE game_id = %s", (m,))
    conn.commit()

    second = run(conn, "v1", formats=[2], log=lambda _: None)

    assert (
        the_run(conn, first)["input_fingerprint"]
        != the_run(conn, second)["input_fingerprint"]
    )


def test_a_run_records_its_latest_progress_line_for_admin(conn, db_url, add_match):
    season(add_match)
    lines = []

    run_id = run(conn, "v-test", formats=[2], log=lines.append, workers=2, url=db_url)

    got = conn.execute(
        "SELECT line FROM run_progress WHERE run_id = %s", (run_id,)
    ).fetchone()["line"]
    progress = [line for line in lines if line.startswith("[")]
    assert got == progress[-1]


def progress_line(conn, run_id):
    row = conn.execute(
        "SELECT line FROM run_progress WHERE run_id = %s", (run_id,)
    ).fetchone()
    return row and row["line"]


@pytest.mark.skipif(
    sys.platform == "win32",
    reason="Windows ends a process on SIGTERM; the engine runs on Linux",
)
def test_a_stopped_run_is_marked_failed_and_says_so(conn, db_url, add_match):
    season(add_match)
    previous = signal.getsignal(signal.SIGTERM)
    runner.stop_on_sigterm()

    def stop_after_a_task(line):
        if (
            line.startswith("[")
            and not line.startswith("[0/")
            and not line.startswith("[1/")
        ):
            os.kill(os.getpid(), signal.SIGTERM)

    try:
        with pytest.raises(runner.Stopped):
            run(
                conn,
                "v-test",
                formats=[2],
                log=stop_after_a_task,
                workers=2,
                url=db_url,
            )
    finally:
        signal.signal(signal.SIGTERM, previous)

    run_id = conn.execute("SELECT run_id FROM rating_runs").fetchone()["run_id"]
    assert the_run(conn, run_id)["status"] == "failed"
    assert progress_line(conn, run_id) == "stopped"
    assert multiprocessing.active_children() == []


def test_a_failed_run_tells_admin_what_failed(conn, db_url, add_match, monkeypatch):
    season(add_match)
    monkeypatch.setattr(runner, "derive_tournament_badges", boom)

    with pytest.raises(RuntimeError):
        run(conn, "v-test", formats=[2], log=lambda _: None, url=db_url)

    run_id = conn.execute("SELECT run_id FROM rating_runs").fetchone()["run_id"]
    assert progress_line(conn, run_id) == "failed: RuntimeError"


def test_a_parallel_run_counts_each_step_as_it_finishes(
    conn, db_url, add_match, monkeypatch
):
    season(add_match)
    real_wait = runner.wait
    finished_together = []

    def counting_wait(futures, **kwargs):
        done, not_done = real_wait(futures, **kwargs)
        finished_together.append(len(done))
        return done, not_done

    monkeypatch.setattr(runner, "wait", counting_wait)

    run(conn, "v-test", formats=[2], log=lambda _: None, workers=2, url=db_url)

    assert len(finished_together) > 1, "every step was counted at once, at the end"


def _drop(sql):
    return lambda conn, add_match: conn.execute(sql)


@pytest.mark.parametrize(
    ("seeded", "change", "ranking", "error", "match"),
    [
        pytest.param(
            False,
            None,
            "community",
            runner.NoMatches,
            "no matches",
            id="empty database",
        ),
        pytest.param(
            True,
            _drop("DROP TABLE player_titles CASCADE"),
            "community",
            runner.SchemaOutOfDate,
            "player_titles",
            id="missing player_titles",
        ),
        pytest.param(
            True,
            _drop("DROP TABLE tournament_group_divisions CASCADE"),
            "community",
            runner.SchemaOutOfDate,
            "tournament_group_divisions",
            id="missing tournament_group_divisions",
        ),
        pytest.param(
            True,
            _drop("DROP TABLE tournament_series CASCADE"),
            "community",
            runner.SchemaOutOfDate,
            "tournament_series",
            id="missing tournament_series",
        ),
        pytest.param(
            True,
            _drop("ALTER TABLE rating_runs DROP COLUMN games"),
            "community",
            runner.SchemaOutOfDate,
            "rating_runs.games",
            id="missing run stats",
        ),
        pytest.param(
            True,
            lambda conn, add_match: add_match(("Ann", 10), ("Bob", 5), placed=None),
            "community",
            runner.GamesUnplaced,
            "1 game",
            id="a game placed nowhere",
        ),
        pytest.param(
            True, None, "snail", runner.UnknownRanking, "snail", id="unknown ranking"
        ),
    ],
)
def test_a_run_is_refused_before_it_starts(
    conn, add_match, seeded, change, ranking, error, match
):
    if seeded:
        season(add_match)
    if change:
        change(conn, add_match)
    conn.commit()

    with pytest.raises(error, match=match):
        run(conn, "v-test", formats=[2], log=lambda _: None, ranking=ranking)

    assert conn.execute("SELECT count(*) AS n FROM rating_runs").fetchone()["n"] == 0


def run_tables(conn):
    return [
        r["t"]
        for r in conn.execute(
            "SELECT conrelid::regclass::text AS t FROM pg_constraint "
            "WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' AND conparentid = 0"
        ).fetchall()
    ]


def result_rows(conn, run_id):
    return conn.execute(
        "SELECT result_rows FROM rating_runs WHERE run_id = %s", (run_id,)
    ).fetchone()["result_rows"]


@pytest.mark.parametrize("kind", ["full", "tournaments only"])
def test_a_run_records_the_rows_it_wrote_to_each_table(conn, add_match, kind):
    season(add_match)
    run_id = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)
    if kind == "tournaments only":
        run_id = run(conn, "v2", tournaments_only=True, log=lambda _: None)

    got = result_rows(conn, run_id)

    assert got["pairwise_matches"] > 0
    assert {t: got.get(t, 0) for t in run_tables(conn)} == {
        t: count(conn, t, run_id) for t in run_tables(conn)
    }


@pytest.mark.parametrize("kind", ["full", "parallel", "tournaments only"])
def test_a_run_counts_its_players_seats(conn, db_url, add_match, kind):
    season(add_match)
    run_id = run(
        conn, "v1", formats=[2], log=lambda _: None, make_current=True,
        workers=2 if kind == "parallel" else 1, url=db_url,
    )  # fmt: skip
    if kind == "tournaments only":
        run_id = run(conn, "v2", tournaments_only=True, log=lambda _: None)

    got = conn.execute(
        "SELECT games, wins FROM player_seat_stats WHERE run_id = %s AND player_name = 'Ann' "
        "AND player_count = 2 AND seat = 1",
        (run_id,),
    ).fetchone()

    assert got == {"games": 2, "wins": 2}


def recorded_bytes(conn, run_id):
    return conn.execute(
        "SELECT result_bytes, disk_bytes FROM rating_runs WHERE run_id = %s", (run_id,)
    ).fetchone()


def table_bytes(conn, table, run_id):
    """Returns the bytes of the run's rows in table, and what the run's partition of it takes on disk."""
    r = conn.execute(
        f"SELECT coalesce(sum(pg_column_size(t.*)), 0) AS b, pg_total_relation_size(%s::regclass) AS disk "
        f"FROM {table} t WHERE run_id = %s",
        (f"{table}_run{run_id}", run_id),
    ).fetchone()
    return r["b"], r["disk"]


@pytest.mark.parametrize("kind", ["full", "tournaments only"])
def test_a_run_records_the_bytes_of_its_rows_and_what_it_takes_on_disk(
    conn, add_match, kind
):
    season(add_match)
    run_id = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)
    if kind == "tournaments only":
        run_id = run(conn, "v2", tournaments_only=True, log=lambda _: None)

    got = recorded_bytes(conn, run_id)

    sizes = [table_bytes(conn, t, run_id) for t in run_tables(conn)]
    assert got["result_bytes"] > 0
    assert got["result_bytes"] == sum(rows for rows, _ in sizes)
    assert got["disk_bytes"] == sum(disk for _, disk in sizes) > got["result_bytes"]


def test_a_failed_run_records_the_rows_it_left(conn, add_match, monkeypatch):
    season(add_match)
    monkeypatch.setattr(runner, "derive_tournament_badges", boom)
    with pytest.raises(RuntimeError, match="boom"):
        run(conn, "v-test", formats=[2], log=lambda _: None)
    run_id = conn.execute("SELECT run_id FROM rating_runs").fetchone()["run_id"]

    got = result_rows(conn, run_id)

    assert {t: got.get(t, 0) for t in run_tables(conn)} == {
        t: count(conn, t, run_id) for t in run_tables(conn)
    }


GOLDEN = Path(__file__).with_name("golden.json")


def golden_season(add_match):
    """Seven months of invented games in every format, with a timeout and a resignation."""
    months = [
        (2021, 10),
        (2021, 11),
        (2021, 12),
        (2022, 1),
        (2022, 2),
        (2022, 3),
        (2022, 4),
    ]
    for i, (year, month) in enumerate(months):
        swing = (i * 17) % 40
        add_match(
            ("Ann", 120 - swing),
            ("Bob", 95 + swing),
            day=date(year, month, 2),
            tournament="RL_s01 - Emperor",
        )
        add_match(
            ("Cat", 88 + swing),
            ("Dan", 101),
            day=date(year, month, 5),
            tournament="RL_s01 - Emperor",
        )
        add_match(
            ("Eve", 110),
            ("Fay", 70 + swing),
            ("Ann", 99),
            day=date(year, month, 9),
            tournament="Cup",
        )
        add_match(("Bob", 130 - swing), ("Cat", 125), ("Dan", -1), ("Eve", 80 + swing),
                  day=date(year, month, 14), tournament="Cup")  # fmt: skip
        add_match(("Fay", 90), ("Dan", -2), day=date(year, month, 20), tournament="Cup")


def stable(value):
    """A value as text, floats to 9 significant digits so the machine's rounding cannot change it."""
    if isinstance(value, float | Decimal):
        return f"{float(value):.9g}"
    return str(value)


def results_hash(conn, run_id):
    """A hash of every row a run computed, table by table, in a stable order."""
    digest = hashlib.sha256()
    for table in sorted(runner.COPIED_TABLES + runner.RECOMPUTED_TABLES):
        rows = conn.execute(
            f"SELECT * FROM {table} WHERE run_id = %s", (run_id,)
        ).fetchall()
        lines = sorted(
            "|".join(
                f"{k}={stable(v)}"
                for k, v in sorted(r.items())
                if k not in ("run_id", "pairwise_id")
            )
            for r in rows
        )
        digest.update(f"{table} {len(lines)}\n".encode())
        for line in lines:
            digest.update(f"{line}\n".encode())
    return digest.hexdigest()


def test_the_results_match_the_engine_versions_golden_hash(conn, add_match):
    golden_season(add_match)

    run_id = run(conn, "golden", log=lambda _: None)

    rated = conn.execute(
        "SELECT DISTINCT model_type, player_count FROM player_ratings WHERE run_id = %s",
        (run_id,),
    ).fetchall()
    assert {(r["model_type"], r["player_count"]) for r in rated} == {
        (m, f) for m in MODEL_TYPES for f in FORMATS
    }
    version, got = runner.engine_version(), results_hash(conn, run_id)
    want = json.loads(GOLDEN.read_text()).get(version)
    assert got == want, (
        f"engine {version}'s results hash to {got}, not {want}. If this change is meant to move ratings, "
        f'bump the version in engine/pyproject.toml and add "<new version>": "{got}" to tests/golden.json. '
        "A version's hash never changes."
    )


def league(add_match, seed, players, months, games):
    """Loads a synthetic two-player league and returns each player's true strength.

    Strengths are spread evenly from 1200 to 1900; each month every player plays `games` games
    against random opponents, won with the Elo probability of the strength gap.
    """
    rng = random.Random(seed)
    names = [f"Sweep{i:02d}" for i in range(players)]
    strength = {n: 1200 + 700 * i / (players - 1) for i, n in enumerate(names)}
    for month in range(months):
        day = date(2022 + month // 12, month % 12 + 1, 10)
        for _ in range(players * games // 2):
            a, b = rng.sample(names, 2)
            a_wins = rng.random() < 1 / (1 + 10 ** ((strength[b] - strength[a]) / 400))
            winner, loser = (a, b) if a_wins else (b, a)
            add_match((winner, 10), (loser, 5), day=day)
    return strength


def rank_correlation(xs, ys):
    """Returns Spearman's rank correlation of two equal-length lists without ties."""
    rx = {v: r for r, v in enumerate(sorted(xs))}
    ry = {v: r for r, v in enumerate(sorted(ys))}
    a = [rx[x] for x in xs]
    b = [ry[y] for y in ys]
    n = len(a)
    return 1 - 6 * sum((p - q) ** 2 for p, q in zip(a, b, strict=True)) / (
        n * (n * n - 1)
    )


@pytest.mark.slow
@pytest.mark.parametrize("seed", [1, 2, 3])
@pytest.mark.parametrize(
    ("months", "games", "min_correlation", "max_rd"),
    [
        pytest.param(6, 4, 0.8, 160, id="6-months-4-games"),
        pytest.param(18, 8, 0.95, 80, id="18-months-8-games"),
    ],
)
def test_ratings_converge_on_true_strength(
    conn, add_match, seed, months, games, min_correlation, max_rd
):
    strength = league(add_match, seed, players=24, months=months, games=games)

    run_id = run(conn, "v-sweep", formats=[2], log=lambda _: None)

    wrong = []
    for model in sorted(KEPT):
        rows = conn.execute(
            "SELECT player_name, rating, rd FROM player_ratings "
            "WHERE run_id = %s AND model_type = %s AND player_count = 2",
            (run_id, model),
        ).fetchall()
        got = rank_correlation(
            [strength[r["player_name"]] for r in rows], [r["rating"] for r in rows]
        )
        widest = max(r["rd"] for r in rows)
        if len(rows) != len(strength) or got < min_correlation or widest > max_rd:
            wrong.append(
                f"{model}: {len(rows)} players, rank correlation {got:.3f}, widest RD {widest:.0f}"
            )
    assert not wrong, (
        f"seed {seed}, {months} months of {games} games: want correlation >= {min_correlation} "
        f"and RD <= {max_rd} for all {len(strength)} players; got " + "; ".join(wrong)
    )


def manual_vacuums(conn, table):
    conn.execute("SELECT pg_stat_clear_snapshot()")
    row = conn.execute(
        "SELECT vacuum_count FROM pg_stat_user_tables WHERE relname = %s", (table,)
    ).fetchone()
    return row and row["vacuum_count"]


@pytest.mark.parametrize("kind", ["full", "tournaments only"])
def test_a_run_vacuums_its_partition_of_every_run_table(conn, add_match, kind):
    season(add_match)
    run_id = run(conn, "v1", formats=[2], log=lambda _: None, make_current=True)
    if kind == "tournaments only":
        run_id = run(conn, "v2", tournaments_only=True, log=lambda _: None)

    assert {t: manual_vacuums(conn, f"{t}_run{run_id}") for t in run_tables(conn)} == {
        t: 1 for t in run_tables(conn)
    }
