import pytest

from tta_ratings_engine.data.badges import (
    classify_division_tier,
    derive_tournament_badges,
    parse_badge_reason,
)


def test_badge_classification():
    """Tests tournament division to badge tier classification."""
    # Royal League
    assert classify_division_tier("RL_s08 - Emperor") == 1  # GM
    assert classify_division_tier("RL_s08 - King") == 2  # M
    assert classify_division_tier("RL_s08 - Prince A") == 3  # Platinum
    assert classify_division_tier("RL_s08 - Duke B") == 4  # Gold
    assert classify_division_tier("RL_s08 - Marquess C") == 5  # Silver
    assert classify_division_tier("RL_s08 - Count D") == 6  # Bronze
    assert classify_division_tier("RL_s08 - Viscount") == 7  # Wood
    assert classify_division_tier("RL_s09 - Baron 2 game 19") == 7  # Wood (Baron)

    # International Championship
    assert classify_division_tier("International S33 - Diamond") == 1  # GM
    assert classify_division_tier("International S33 - Grandmaster") == 1  # GM
    assert classify_division_tier("International S33 - Master") == 2  # M
    assert classify_division_tier("International S33 - Master 1") == 2  # M
    assert classify_division_tier("International S33 - Master 2") == 2  # M
    assert classify_division_tier("International S33 - Platinum") == 3  # P
    assert classify_division_tier("International S33 - Gold") == 4  # G
    assert classify_division_tier("International S33 - Silver") == 5  # S
    assert classify_division_tier("International S34 - Bronze 28") == 6  # B
    assert classify_division_tier("International S33 - Wood 5") == 7  # W

    # Intermezzo
    assert classify_division_tier("Intermezzo S30 - Grandmaster") == 1  # GM
    assert classify_division_tier("Intermezzo S30 - Master 1") == 2  # M
    assert classify_division_tier("Intermezzo S30 - Platinum 1") == 3  # P
    assert classify_division_tier("Intermezzo S30 - Gold 2") == 4  # G
    assert classify_division_tier("Intermezzo S30 - Silver 3") == 5  # S
    assert classify_division_tier("Intermezzo S30 - Bronze 4") == 6  # B
    assert classify_division_tier("Intermezzo S30 - Wood 5") == 7  # W


def test_royal_league_reasons_name_the_lower_ranks():
    assert (
        parse_badge_reason("RL_s09 - Baron 2 game 19", 7)
        == "Played in Royal League Baron Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Knight", 7)
        == "Played in Royal League Knight Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Viscount", 7)
        == "Played in Royal League Viscount Season 9"
    )
    assert (
        parse_badge_reason("RL_s09 - Emperor", 1)
        == "Played in Royal League Emperor Season 9"
    )


# End to end, on Postgres: titles derived from matches and written per run.

from datetime import date

from tta_ratings_engine.data.db import RunNotRunning


def titles(conn, run_id):
    return {
        r["player_name"]: (r["title"], r["title_count"], r["badge_reason"])
        for r in conn.execute(
            "SELECT player_name, title, title_count, badge_reason FROM player_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_a_player_gets_the_title_of_their_best_recent_division(conn, add_match, run_id):
    add_match(
        ("Ann", 10), ("Bob", 5), day=date(2024, 5, 1), tournament="RL_s08 - Emperor"
    )
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - King")

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = titles(conn, run_id)
    assert got["Ann"] == ("GM", 1, "Played in Royal League Emperor Season 8")
    assert got["Cat"] == ("M", 0, "Played in Royal League King Season 9")


def test_only_the_last_year_counts(conn, add_match, run_id):
    add_match(
        ("Old", 10), ("Bob", 5), day=date(2023, 5, 31), tournament="RL_s01 - Emperor"
    )
    add_match(
        ("Recent", 10), ("Dan", 5), day=date(2023, 7, 1), tournament="RL_s02 - Duke"
    )
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert set(titles(conn, run_id)) == {"Ann", "Cat", "Recent", "Dan"}


def test_a_division_the_run_recorded_gives_a_title(conn, add_match, run_id):
    match_id = add_match(
        ("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="International S27"
    )
    conn.execute(
        "INSERT INTO tournament_games (run_id, match_id, series, season, name_season, "
        "division, group_label, division_source) "
        "VALUES (%s, %s, 'international', 27, 27, 'Grandmaster', 'Grandmaster', 'recorded')",
        (run_id, match_id),
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id)["Ann"] == (
        "GM",
        1,
        "Played in International Championship Grandmaster Season 27",
    )


def test_games_outside_the_championships_earn_no_title(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="Friendly Cup")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}


def test_the_reigning_champion_is_the_latest_seasons_winner(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Martin_Pecheur", "pv4")
    place("worlds", 2025, "Final", "a440", "kyshantry")
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    derive_tournament_badges(conn=conn, run_id=run_id)
    assert "a440" not in titles(conn, run_id)

    add_match(
        ("a440", 10),
        ("Martin_Pecheur", 5),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )
    derive_tournament_badges(conn=conn, run_id=run_id)
    got = titles(conn, run_id)
    assert got["a440"][0] == "WC"
    assert got.get("Martin_Pecheur", ("",))[0] != "WC"


def test_deriving_again_replaces_the_runs_titles_peaks_and_years(
    conn, add_match, run_id
):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive_tournament_badges(conn=conn, run_id=run_id)
    derive_tournament_badges(conn=conn, run_id=run_id)

    assert len(titles(conn, run_id)) == 2
    assert set(peaks(conn, run_id)) == {"Ann", "Bob"}
    assert yearly(conn, run_id) == {("Ann", 2024): "G", ("Bob", 2024): "G"}


def test_a_finished_run_gets_no_titles(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    conn.execute(
        "UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}


def test_no_matches_means_no_titles(conn, run_id):
    assert derive_tournament_badges(conn=conn, run_id=run_id) == {}
    assert titles(conn, run_id) == {}


def peaks(conn, run_id):
    return {
        r["player_name"]: (r["peak_title"], r["peak_year"])
        for r in conn.execute(
            "SELECT player_name, peak_title, peak_year FROM player_peak_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_a_player_keeps_their_career_peak_title(conn, add_match, run_id):
    add_match(
        ("Ann", 10), ("Bob", 5), day=date(2021, 3, 1), tournament="RL_s03 - Emperor"
    )
    add_match(
        ("Ann", 10), ("Bob", 5), day=date(2022, 3, 1), tournament="RL_s04 - Emperor"
    )
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - King")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id)["Ann"][0] == "M"
    got = peaks(conn, run_id)
    assert got["Ann"] == ("GM", 2022)
    assert got["Bob"] == ("GM", 2022)
    assert got["Cat"] == ("M", 2024)
    assert "Bob" not in titles(conn, run_id)


def test_world_champions_peak_at_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2022, "Final", "Ender_Wiggin", "LaoHuang")
    place("worlds", 2023, "Final", "Weidenbaum")
    place("worlds", 2025, "Final", "a440")
    add_match(
        ("Weidenbaum", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - King"
    )
    add_match(
        ("a440", 10),
        ("Ender_Wiggin", 5),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = peaks(conn, run_id)
    assert got["Weidenbaum"] == ("WC", 2023)
    assert got["a440"] == ("WC", 2025)
    assert got["Ender_Wiggin"] == ("WC", 2022)
    assert titles(conn, run_id)["Weidenbaum"][0] == "M"


def yearly(conn, run_id):
    return {
        (r["player_name"], r["year"]): r["title"]
        for r in conn.execute(
            "SELECT player_name, year, title FROM player_yearly_titles WHERE run_id = %s",
            (run_id,),
        )
    }


def test_each_year_keeps_the_best_division_played(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2021, 3, 1), tournament="RL_s03 - Duke")
    add_match(
        ("Ann", 10), ("Cat", 5), day=date(2021, 9, 1), tournament="RL_s04 - Emperor"
    )
    add_match(("Ann", 10), ("Bob", 5), day=date(2022, 3, 1), tournament="RL_s05 - King")
    add_match(("Ann", 10), ("Dan", 5), day=date(2023, 3, 1), tournament="Friendly Cup")

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = yearly(conn, run_id)
    assert {k: v for k, v in got.items() if k[0] == "Ann"} == {
        ("Ann", 2021): "GM",
        ("Ann", 2022): "M",
    }
    assert got[("Bob", 2021)] == "G"
    assert ("Dan", 2023) not in got


def test_world_champions_hold_wc_in_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2023, "Final", "Weidenbaum")
    place("worlds", 2025, "Final", "a440")
    add_match(
        ("Weidenbaum", 10), ("Bob", 5), day=date(2023, 6, 1), tournament="RL_s09 - King"
    )
    add_match(("A440", 10), ("Bob", 5), day=date(2024, 6, 2), tournament="Friendly Cup")

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = yearly(conn, run_id)
    assert got[("Weidenbaum", 2023)] == "WC"
    assert got[("A440", 2025)] == "WC"
    assert ("Martin_Pecheur", 2024) not in got


def add_record(
    conn, run_id, player, tournament, season, division, placement, medal, finish
):
    """A tournament_records row, as the tournaments step writes it before titles."""
    position = conn.execute(
        "SELECT count(*) + 1 AS n FROM tournament_records WHERE run_id = %s", (run_id,)
    ).fetchone()["n"]
    conn.execute(
        "INSERT INTO tournament_records VALUES (%s, %s, %s, %s, %s, %s, %s, '', %s, '', %s, false)",
        (
            run_id,
            player,
            position,
            tournament,
            season,
            division,
            placement,
            medal,
            finish,
        ),
    )


def test_a_recent_top_division_win_is_super_gm(conn, add_match, run_id):
    add_match(
        ("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Emperor"
    )
    add_record(
        conn,
        run_id,
        "Ann",
        "Royal League",
        "Season 9",
        "Emperor",
        "1 / 8",
        "gold",
        "2024-05-27",
    )
    add_record(
        conn,
        run_id,
        "Bob",
        "Royal League",
        "Season 9",
        "Emperor",
        "2 / 8",
        "silver",
        "2024-05-27",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = titles(conn, run_id)
    assert got["Ann"] == ("SGM", 1, "Champion of Royal League Emperor (Season 9)")
    assert got["Bob"] == ("GM", 1, "Played in Royal League Emperor Season 9")
    assert peaks(conn, run_id) == {"Ann": ("SGM", 2024), "Bob": ("GM", 2024)}


def test_an_older_top_division_win_is_a_super_gm_peak(conn, add_match, run_id):
    for p in ("Cat", "Dan", "Eve", "Fay"):
        add_match((p, 10), ("Bob", 5), day=date(2024, 6, 1), tournament="Friendly Cup")
    add_record(
        conn,
        run_id,
        "Cat",
        "International Championship",
        "Season 20",
        "Grandmaster",
        "1 / 7",
        "gold",
        "2022-12-04",
    )
    # upstream counts a campaign row too, whatever group it ranked first in
    add_record(
        conn,
        run_id,
        "Dan",
        "International Championship",
        "Season 14",
        "Grandmaster",
        "1 / 735",
        "gold",
        "2021-03-02",
    )
    add_record(
        conn,
        run_id,
        "Eve",
        "International Championship",
        "Season 20",
        "Grandmaster",
        "3 / 7",
        "bronze",
        "2022-12-04",
    )
    add_record(
        conn,
        run_id,
        "Fay",
        "International Championship",
        "Season 30",
        "Master 1",
        "1 / 4",
        "gold",
        "2025-08-14",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert titles(conn, run_id) == {}
    assert peaks(conn, run_id) == {"Cat": ("SGM", 2022), "Dan": ("SGM", 2021)}


def test_records_add_super_gm_years_and_untitled_seasons(conn, add_match, run_id):
    add_match(("Ann", 10), ("Cat", 5), day=date(2024, 6, 1), tournament="RL_s09 - King")
    add_match(("Bob", 10), ("Cat", 5), day=date(2021, 3, 1), tournament="RL_s03 - Duke")
    # a season named without its division: the record gives the year's title
    add_record(
        conn,
        run_id,
        "Ann",
        "International Championship",
        "Season 20",
        "Grandmaster",
        "5 / 7",
        None,
        "2020-12-06",
    )
    add_record(
        conn,
        run_id,
        "Ann",
        "Royal League",
        "Season 9",
        "Emperor",
        "2 / 8",
        "silver",
        "2024-05-27",
    )
    add_record(
        conn,
        run_id,
        "Bob",
        "Intermezzo Championship",
        "Season 13",
        "Grandmaster",
        "1 / 5",
        "gold",
        "2021-11-30",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)

    got = yearly(conn, run_id)
    assert {k: v for k, v in got.items() if k[0] in ("Ann", "Bob")} == {
        ("Ann", 2020): "GM",
        ("Ann", 2024): "M",
        ("Bob", 2021): "SGM",
    }


def test_the_reigning_champion_comes_from_admin_then_recorded_then_the_runs_final(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Martin_Pecheur")
    conn.execute(
        "INSERT INTO tournament_standings VALUES (%s, 'worlds', 2025, 'Final', 'Final', 'a440', 1, 16, 4, %s)",
        (run_id, [3, 1, 0, 0]),
    )
    conn.commit()
    add_match(
        ("a440", 10),
        ("Martin_Pecheur", 5),
        ("pv4", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive_tournament_badges(conn=conn, run_id=run_id)
    assert titles(conn, run_id)["a440"][0] == "WC"

    conn.execute(
        "INSERT INTO tournament_podium_overrides (tournament, season, place, player_name) VALUES ('worlds', '2025', 1, 'pv4')"
    )
    conn.commit()
    derive_tournament_badges(conn=conn, run_id=run_id)
    got = titles(conn, run_id)
    assert got["pv4"][0] == "WC"
    assert got.get("a440", ("",))[0] != "WC"


def override_title(conn, name, title, reason="the organisers' ruling"):
    conn.execute(
        "INSERT INTO player_title_overrides VALUES (%s, %s, %s)", (name, title, reason)
    )
    conn.commit()


def test_a_title_set_in_admin_replaces_the_derived_one(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    derive_tournament_badges(conn=conn, run_id=run_id)
    derived = titles(conn, run_id)
    assert "Ann" in derived and derived["Ann"][0] != "SGM"

    override_title(conn, "Ann", "SGM")
    override_title(conn, "Bob", "")
    derive_tournament_badges(conn=conn, run_id=run_id)

    got = titles(conn, run_id)
    assert got["Ann"][0] == "SGM"
    assert "Bob" not in got


def test_a_title_set_in_admin_gives_its_reason(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    override_title(conn, "Ann", "GM", "won the 2019 final, recorded nowhere")

    derive_tournament_badges(conn=conn, run_id=run_id)

    reason = conn.execute(
        "SELECT badge_reason FROM player_titles WHERE run_id = %s AND player_name = 'Ann'",
        (run_id,),
    ).fetchone()["badge_reason"]
    assert reason == "Set in admin: won the 2019 final, recorded nowhere"


def test_a_title_set_in_admin_still_needs_a_recent_game(conn, add_match, run_id):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    override_title(conn, "Cat", "GM")

    derive_tournament_badges(conn=conn, run_id=run_id)

    assert "Cat" not in titles(conn, run_id)
