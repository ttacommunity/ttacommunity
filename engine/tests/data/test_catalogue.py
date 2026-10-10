from datetime import date

import pytest

from tta_ratings_engine.data.catalogue import derive_catalogue
from tta_ratings_engine.data.db import RunNotRunning


def spans(conn, run_id):
    """The run's spans by level: series slug, then season and stage labels where the row has them."""
    rows = conn.execute(
        "SELECT t.slug, se.label AS season, st.label AS stage, c.games, c.players, c.first_game, c.last_game "
        "FROM catalogue_spans c JOIN series t USING (series_id) "
        "LEFT JOIN tournaments se USING (tournament_id) LEFT JOIN stages st USING (stage_id) "
        "WHERE c.run_id = %s",
        (run_id,),
    ).fetchall()
    return {
        (r["slug"], r["season"], r["stage"]): (
            r["games"],
            r["players"],
            r["first_game"],
            r["last_game"],
        )
        for r in rows
    }


def test_a_run_records_each_series_season_and_stage_span(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 1, 3))
    add_match(("Ann", 10), ("Cat", 5), ("Dan", 1), day=date(2024, 2, 9))
    add_match(("Eve", 10), ("Fay", 5), day=date(2024, 3, 1), placed="other")
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 4, 1), state="never_played")
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 5, 1), placed=None)

    derive_catalogue(conn=conn, run_id=run_id)

    first, last = date(2024, 1, 3), date(2024, 2, 9)
    assert spans(conn, run_id) == {
        ("rated", None, None): (2, 4, first, last),
        ("rated", "1", None): (2, 4, first, last),
        ("rated", "1", ""): (2, 4, first, last),
        ("other", None, None): (1, 2, date(2024, 3, 1), date(2024, 3, 1)),
        ("other", "1", None): (1, 2, date(2024, 3, 1), date(2024, 3, 1)),
        ("other", "1", ""): (1, 2, date(2024, 3, 1), date(2024, 3, 1)),
    }


def test_a_finished_run_gets_no_spans(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5))
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )

    with pytest.raises(RunNotRunning):
        derive_catalogue(conn=conn, run_id=run_id)
