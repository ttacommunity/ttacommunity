from datetime import date

import pytest

from tta_ratings_engine import run as runner
from tta_ratings_engine.data.db import connect
from tta_ratings_engine.run import FORMATS, MODEL_TYPES, STEP_COST, main, run

SIXTEEN = {
    "glicko2_daneo", "glicko2_daneo_softer",
    "glicko2_std", "glicko2_std_retro", "glicko2_std_softer", "glicko2_std_softer_retro",
    "glicko2_mp", "glicko2_mp_retro", "glicko2_mp_softer", "glicko2_mp_softer_retro",
    "glicko2_adapt", "glicko2_adapt_retro", "glicko2_adapt_softer", "glicko2_adapt_softer_retro",
    "whr", "whr_softer",
}  # fmt: skip


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


def test_the_model_list_is_the_sixteen_calibration_models():
    assert set(MODEL_TYPES) == SIXTEEN


def test_a_run_writes_every_result_and_completes(conn, add_match):
    season(add_match)

    run_id = run(conn, "v-test", formats=[0, 2], log=lambda _: None)

    assert the_run(conn, run_id)["status"] == "complete"
    assert the_run(conn, run_id)["finished_at"] is not None
    types = conn.execute(
        "SELECT DISTINCT model_type FROM player_ratings WHERE run_id = %s", (run_id,)
    ).fetchall()
    assert {r["model_type"] for r in types} == SIXTEEN
    assert count(conn, "standard_calibration", run_id) == 16 * 2
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
    ],
)
def test_a_recorded_fact_changes_the_input_fingerprint(conn, add_match, fact):
    season(add_match)
    first = run(conn, "v1", formats=[2], log=lambda _: None)
    conn.execute(fact)
    conn.commit()
    second = run(conn, "v1", formats=[2], log=lambda _: None)

    assert (
        the_run(conn, second)["input_fingerprint"]
        != the_run(conn, first)["input_fingerprint"]
    )


def test_a_run_is_not_published_unless_asked(conn, add_match):
    season(add_match)

    run(conn, "v-test", formats=[2], log=lambda _: None)

    assert current(conn) is None


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
    # pairs + 16 models x 2 formats + tournaments + titles + 16 calibrations x 2
    # + walk-forward months (2 + 2)
    assert progress[-1].startswith("[71/71] ")


def test_a_failure_marks_the_run_failed_and_is_raised(conn, add_match, monkeypatch):
    season(add_match)
    monkeypatch.setattr(runner, "derive_tournament_badges", boom)

    with pytest.raises(RuntimeError, match="boom"):
        run(conn, "v-test", formats=[2], log=lambda _: None, make_current=True)

    row = conn.execute("SELECT status, finished_at FROM rating_runs").fetchone()
    assert row["status"] == "failed"
    assert row["finished_at"] is not None
    assert current(conn) is None


def test_an_empty_database_is_refused_before_a_run_starts(conn):
    with pytest.raises(runner.NoMatches):
        run(conn, "v-test", log=lambda _: None)

    assert conn.execute("SELECT count(*) AS n FROM rating_runs").fetchone()["n"] == 0


def test_the_command_needs_an_engine_version(monkeypatch, db_url):
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)
    monkeypatch.delenv("ENGINE_VERSION", raising=False)

    with pytest.raises(SystemExit, match="ENGINE_VERSION"):
        main([])


def test_the_command_runs_the_formats_asked_for(monkeypatch, db_url, conn, add_match):
    season(add_match)
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)
    monkeypatch.setenv("ENGINE_VERSION", "v-cli")

    main(["--formats", "2", "--make-current"])

    run_id = current(conn)
    assert the_run(conn, run_id)["engine_version"] == "v-cli"
    formats = conn.execute(
        "SELECT DISTINCT player_count FROM player_ratings WHERE run_id = %s", (run_id,)
    ).fetchall()
    assert [r["player_count"] for r in formats] == [2]


@pytest.mark.parametrize("table", ["player_titles", "tournament_group_divisions"])
def test_a_database_missing_a_migration_is_refused_before_a_run_starts(
    conn, add_match, table
):
    season(add_match)
    conn.execute(f"DROP TABLE {table} CASCADE")
    conn.commit()

    with pytest.raises(runner.SchemaOutOfDate, match=table):
        run(conn, "v-test", log=lambda _: None)

    assert conn.execute("SELECT count(*) AS n FROM rating_runs").fetchone()["n"] == 0


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
    real = runner.derive_tournament_badges

    def slow(**kw):
        conn.execute("SELECT pg_sleep(1.1)")
        return real(**kw)

    monkeypatch.setattr(runner, "derive_tournament_badges", slow)

    run_id = run(conn, "v-test", formats=[2], log=lambda _: None)

    took = conn.execute(
        "SELECT extract(epoch FROM finished_at - started_at) AS s FROM rating_runs WHERE run_id = %s",
        (run_id,),
    ).fetchone()["s"]
    assert took >= 1.1
