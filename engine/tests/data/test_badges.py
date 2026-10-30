from datetime import UTC, date, datetime

import pytest

from tta_ratings_engine.data.badges import (
    Leagues,
    Titles,
    badge_titles,
    derive_tournament_badges,
)
from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.players import Player
from tta_ratings_engine.data.tournaments import derive_tournaments

# The rules, on the games, records and overrides the run reads.

CUTOFF = "2023-06-01"  # a year before the newest game, 2024-06-01

IDS: dict[str, int] = {}


def player(name):
    """An invented player with this name."""
    return Player(name, IDS.setdefault(name, len(IDS) + 1))


def rl(season, division):
    return ("royal_league", season, division)


def intl(season, division):
    return ("international", season, division)


def game(day, league, *players, finished_at=None, game_id=0):
    """Returns a played game as badge_titles reads it; league is the (series, season, division) the run recorded,
    None for a game in no league."""
    series, season, division = league or (None, None, None)
    return {
        "date": day,
        "players": list(players),
        "player_ids": [player(p).id for p in players],
        "series": series,
        "season": season,
        "division": division,
        "finished_at": finished_at,
        "game_id": game_id,
    }


def newest_first(*games):
    return sorted(games, key=lambda g: g["date"], reverse=True)


def season_record(
    player, tournament, season, division, placement, medal, finish, league=(None, None)
):
    """Returns a season record; league is the series and division its games were in."""
    return {
        "player": player,
        "tournament": tournament,
        "season": season,
        "division": division,
        "placement": placement,
        "medal": medal,
        "finish_date": finish,
        "series": league[0],
        "series_division": league[1],
    }


def override(player, title, reason="r"):
    return {"player": player, "title": title, "reason": reason}


CHAMPIONSHIP = (
    "Grandmaster",
    "Diamond",
    "Master",
    "Platinum",
    "Gold",
    "Silver",
    "Bronze",
    "Wood",
)
LEAGUES = Leagues(
    tiers={
        **{(s, d): t for s in ("international", "intermezzo") for d, t in zip(CHAMPIONSHIP, (1, 1, 2, 3, 4, 5, 6, 7))},
        **{("royal_league", d): t for d, t in zip(("Emperor", "King", "Prince", "Duke", "Marquess", "Count", "Viscount",
                                                   "Baron", "Knight"), (1, 2, 3, 4, 5, 6, 7, 7, 7))},
    },
    super_gm={"international": frozenset({"Grandmaster", "Diamond"}), "intermezzo": frozenset({"Grandmaster", "Diamond"}),
              "royal_league": frozenset({"Emperor"})},
)  # fmt: skip


def rule_titles(games, records=(), overrides=(), champions=(), leagues=LEAGUES):
    """Returns badge_titles' titles for players named in games, records, overrides and champions, by name."""
    t = badge_titles(
        newest_first(*games),
        CUTOFF,
        [(player(p), year, place) for p, year, place in champions],
        [{**r, "player": player(r["player"])} for r in records],
        [{**o, "player": player(o["player"])} for o in overrides],
        leagues,
    )
    return Titles(
        {p.name: d for p, d in t.active.items()},
        {p.name: peak for p, peak in t.peaks.items()},
        {(p.name, year): title for (p, year), title in t.yearly.items()},
    )


def active(t):
    return {
        p: (d["title"], d["title_count"], d["badge_reason"])
        for p, d in t.active.items()
    }


def test_a_player_gets_the_title_of_their_best_recent_division():
    t = rule_titles(
        [
            game("2024-05-01", rl(8, "Emperor"), "Ann", "Bob"),
            game("2024-06-01", rl(9, "King"), "Ann", "Cat"),
        ]
    )

    assert active(t)["Ann"] == ("GM", 1, "Played in Royal League Emperor Season 8")
    assert active(t)["Cat"] == ("M", 0, "Played in Royal League King Season 9")


def test_only_the_last_year_counts():
    t = rule_titles(
        [
            game("2023-05-31", rl(1, "Emperor"), "Old", "Bob"),
            game(CUTOFF, rl(2, "Duke"), "Edge", "Eve"),
            game("2023-07-01", rl(2, "Duke"), "Recent", "Dan"),
            game("2024-06-01", rl(9, "Duke"), "Ann", "Cat"),
        ]
    )

    assert set(t.active) == {"Ann", "Cat", "Recent", "Dan", "Edge", "Eve"}


@pytest.mark.parametrize(
    ("league", "want"),
    [
        (intl(27, "Grandmaster"), ("GM", 1, "Played in International Championship Grandmaster Season 27")),
        (("intermezzo", 30, "Platinum"), ("P", 0, "Played in Intermezzo Championship Platinum Season 30")),
        (rl(9, "Baron"), ("W", 0, "Played in Royal League Baron Season 9")),
        (rl(9, "Viscount"), ("W", 0, "Played in Royal League Viscount Season 9")),
        (rl(None, "Duke"), ("G", 0, "Played in Royal League Duke")),
        (intl(27, "Promotion"), None),
        (("nations_league", 3, "Apple"), None),
    ],
)  # fmt: skip
def test_a_games_title_comes_from_the_division_the_run_recorded(league, want):
    t = rule_titles([game("2024-06-01", league, "Ann", "Bob")])

    assert active(t).get("Ann") == want


def test_a_divisions_tier_is_its_setting():
    t = rule_titles([game("2024-06-01", intl(27, "Gold"), "Ann", "Bob")],
                    leagues=Leagues(tiers={("international", "Gold"): 2}, super_gm={}))  # fmt: skip

    assert active(t)["Ann"][0] == "M"


def test_games_outside_the_championships_earn_no_title():
    t = rule_titles([game("2024-06-01", None, "Ann", "Bob")])

    assert t.active == {}


def test_a_player_keeps_their_career_peak_title():
    t = rule_titles(
        [
            game("2021-03-01", rl(3, "Emperor"), "Ann", "Bob"),
            game("2022-03-01", rl(4, "Emperor"), "Ann", "Bob"),
            game("2024-06-01", rl(9, "King"), "Ann", "Cat"),
        ]
    )

    assert t.active["Ann"]["title"] == "M"
    assert "Bob" not in t.active
    assert t.peaks == {"Ann": ("GM", 2022), "Bob": ("GM", 2022), "Cat": ("M", 2024)}


def test_each_year_keeps_the_best_division_played():
    t = rule_titles(
        [
            game("2021-03-01", rl(3, "Duke"), "Ann", "Bob"),
            game("2021-09-01", rl(4, "Emperor"), "Ann", "Cat"),
            game("2022-03-01", rl(5, "King"), "Ann", "Bob"),
            game("2023-03-01", None, "Ann", "Dan"),
        ]
    )

    assert {k: v for k, v in t.yearly.items() if k[0] == "Ann"} == {
        ("Ann", 2021): "GM",
        ("Ann", 2022): "M",
    }
    assert t.yearly[("Bob", 2021)] == "G"
    assert ("Dan", 2023) not in t.yearly


def test_a_recent_top_division_win_is_super_gm():
    t = rule_titles(
        [game("2024-06-01", rl(9, "Emperor"), "Ann", "Bob")],
        records=[
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                "1 / 8",
                "gold",
                "2024-05-27",
                league=("royal_league", "Emperor"),
            ),
            season_record(
                "Bob",
                "Royal League",
                "Season 9",
                "Emperor",
                "2 / 8",
                "silver",
                "2024-05-27",
                league=("royal_league", "Emperor"),
            ),
        ],
    )

    assert active(t)["Ann"] == ("SGM", 1, "Champion of Royal League Emperor (Season 9)")
    assert active(t)["Bob"] == ("GM", 1, "Played in Royal League Emperor Season 9")
    assert t.peaks == {"Ann": ("SGM", 2024), "Bob": ("GM", 2024)}


def test_an_older_top_division_win_is_a_super_gm_peak():
    ic = "International Championship"
    t = rule_titles(
        [game("2024-06-01", None, p, "Bob") for p in ("Cat", "Dan", "Eve", "Fay")],
        records=[
            season_record(
                "Cat",
                ic,
                "Season 20",
                "Grandmaster",
                "1 / 7",
                "gold",
                "2022-12-04",
                league=("international", "Grandmaster"),
            ),
            season_record(
                "Eve",
                ic,
                "Season 20",
                "Grandmaster",
                "3 / 7",
                "bronze",
                "2022-12-04",
                league=("international", "Grandmaster"),
            ),
            season_record(
                "Fay",
                ic,
                "Season 30",
                "Master 1",
                "1 / 4",
                "gold",
                "2025-08-14",
                league=("international", "Master"),
            ),
        ],
    )

    assert t.active == {}
    assert t.peaks == {"Cat": ("SGM", 2022)}


@pytest.mark.parametrize(
    ("placement", "medal", "want"),
    [
        ("1 / 7", "gold", "SGM"),
        ("1 / 40", "gold", "SGM"),
        ("1 / 8", None, "GM"),
        ("", None, "GM"),
    ],
)
def test_only_a_gold_medal_makes_a_top_division_win(placement, medal, want):
    t = rule_titles(
        [game("2024-06-01", rl(9, "Emperor"), "Ann", "Bob")],
        records=[
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                placement,
                medal,
                "2024-05-27",
                league=("royal_league", "Emperor"),
            )
        ],
    )

    assert (active(t)["Ann"][0], t.yearly[("Ann", 2024)]) == (want, want)


def test_records_add_super_gm_years_and_untitled_seasons():
    t = rule_titles(
        [
            game("2024-06-01", rl(9, "King"), "Ann", "Cat"),
            game("2021-03-01", rl(3, "Duke"), "Bob", "Cat"),
        ],
        records=[
            # a season named without its division: the record gives the year's title
            season_record(
                "Ann",
                "International Championship",
                "Season 20",
                "Grandmaster",
                "5 / 7",
                None,
                "2020-12-06",
                league=("international", "Grandmaster"),
            ),
            season_record(
                "Ann",
                "Royal League",
                "Season 9",
                "Emperor",
                "2 / 8",
                "silver",
                "2024-05-27",
                league=("royal_league", "Emperor"),
            ),
            season_record(
                "Bob",
                "Intermezzo Championship",
                "Season 13",
                "Grandmaster",
                "1 / 5",
                "gold",
                "2021-11-30",
                league=("intermezzo", "Grandmaster"),
            ),
        ],
    )

    assert {k: v for k, v in t.yearly.items() if k[0] in ("Ann", "Bob")} == {
        ("Ann", 2020): "GM",
        ("Ann", 2024): "M",
        ("Bob", 2021): "SGM",
    }


@pytest.mark.parametrize(
    ("overrides", "player", "want"),
    [
        pytest.param(
            [override("Ann", "SGM")],
            "Ann",
            ("SGM", 1, "Set in admin: r"),
            id="replaces the derived title",
        ),
        pytest.param(
            [override("Bob", "")], "Bob", None, id="an empty title takes it away"
        ),
        pytest.param(
            [override("Ann", "GM", "won the 2019 final, recorded nowhere")],
            "Ann",
            ("GM", 1, "Set in admin: won the 2019 final, recorded nowhere"),
            id="gives its reason",
        ),
        pytest.param(
            [override("Cat", "GM")], "Cat", None, id="still needs a recent game"
        ),
    ],
)
def test_a_title_set_in_admin(overrides, player, want):
    t = rule_titles(
        [game("2024-06-01", rl(9, "Duke"), "Ann", "Bob")], overrides=overrides
    )

    assert active(t).get(player) == want


# End to end, on Postgres: titles derived from matches and written per run.


def derive(conn, run_id):
    """Derives the run's tournament results, then its titles from them."""
    derive_tournaments(conn=conn, run_id=run_id)
    derive_tournament_badges(conn=conn, run_id=run_id)


def titles(conn, run_id):
    return {
        r["player"]: (r["title"], r["title_count"], r["badge_reason"])
        for r in conn.execute(
            "SELECT name AS player, title, title_count, badge_reason FROM player_titles JOIN players USING (player_id) WHERE run_id = %s",
            (run_id,),
        )
    }


def test_the_reigning_champion_is_the_latest_seasons_winner(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Marten_Fisher", "pipit4")
    place("worlds", 2025, "Final", "alder440", "kittiwake")
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")
    derive(conn, run_id)

    add_match(
        ("alder440", 10),
        ("Marten_Fisher", 5),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )
    derive(conn, run_id)
    got = titles(conn, run_id)
    assert got["alder440"][0] == "WC"
    assert got.get("Marten_Fisher", ("",))[0] != "WC"


def test_deriving_again_replaces_the_runs_titles_peaks_and_years(
    conn, add_match, run_id
):
    add_match(("Ann", 10), ("Bob", 5), day=date(2024, 6, 1), tournament="RL_s09 - Duke")

    derive(conn, run_id)
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
        r["player"]: (r["peak_title"], r["peak_year"])
        for r in conn.execute(
            "SELECT name AS player, peak_title, peak_year FROM player_peak_titles JOIN players USING (player_id) WHERE run_id = %s",
            (run_id,),
        )
    }


def test_world_champions_peak_at_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2022, "Final", "Willow_Tree", "Ember_Wolf", "Linnet")
    place("worlds", 2023, "Final", "Willow_Tree")
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("Willow_Tree", 10),
        ("Bob", 5),
        day=date(2024, 6, 1),
        tournament="RL_s09 - King",
    )
    add_match(
        ("alder440", 10),
        ("Ember_Wolf", 5),
        ("Linnet", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive(conn, run_id)

    got = peaks(conn, run_id)
    assert got["Willow_Tree"] == ("WC", 2023)
    assert got["alder440"] == ("WC", 2025)
    assert got["Ember_Wolf"] == ("WCS", 2022)
    assert got["Linnet"] == ("WCB", 2022)
    assert titles(conn, run_id)["Willow_Tree"][0] == "M"


def yearly(conn, run_id):
    return {
        (r["player"], r["year"]): r["title"]
        for r in conn.execute(
            "SELECT name AS player, year, title FROM player_yearly_titles JOIN players USING (player_id) WHERE run_id = %s",
            (run_id,),
        )
    }


def test_world_champions_hold_wc_in_their_title_year(conn, add_match, run_id, place):
    place("worlds", 2023, "Final", "Willow_Tree", "SilverTree", "BronzeTree")
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("Willow_Tree", 10),
        ("Bob", 5),
        day=date(2023, 6, 1),
        tournament="RL_s09 - King",
    )
    add_match(
        ("alder440", 10),
        ("SilverTree", 5),
        ("BronzeTree", 2),
        ("Bob", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive(conn, run_id)

    got = yearly(conn, run_id)
    assert got[("Willow_Tree", 2023)] == "WC"
    assert got[("SilverTree", 2023)] == "WCS"
    assert got[("BronzeTree", 2023)] == "WCB"
    assert got[("alder440", 2025)] == "WC"
    assert ("Marten_Fisher", 2024) not in got


def test_the_reigning_champion_comes_from_admin_then_recorded(
    conn, add_match, run_id, place
):
    place("worlds", 2024, "Final", "Marten_Fisher")
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("alder440", 10),
        ("Marten_Fisher", 5),
        ("pipit4", 1),
        day=date(2024, 6, 2),
        tournament="Friendly Cup",
    )

    derive(conn, run_id)
    assert titles(conn, run_id)["alder440"][0] == "WC"

    conn.execute(
        "INSERT INTO tournament_podium_overrides (tournament, season, place, player_id) "
        "SELECT 'worlds', '2025', 1, player_id FROM players WHERE name = 'pipit4'"
    )
    conn.commit()
    derive(conn, run_id)
    got = titles(conn, run_id)
    assert got["pipit4"][0] == "WC"
    assert got.get("alder440", ("",))[0] != "WC"


def test_same_day_games_pick_later_reason_by_finish_time():
    g1 = game(
        "2024-06-01",
        intl(35, "Bronze"),
        "Ann",
        "Bob",
        finished_at=datetime(2024, 6, 1, 10, 0, tzinfo=UTC),
        game_id=1,
    )
    g2 = game(
        "2024-06-01",
        rl(9, "Count"),
        "Ann",
        "Cat",
        finished_at=datetime(2024, 6, 1, 15, 0, tzinfo=UTC),
        game_id=2,
    )
    t1 = badge_titles([g1, g2], CUTOFF, [], [], [], LEAGUES)
    assert (
        t1.active[player("Ann")]["badge_reason"]
        == "Played in Royal League Count Season 9"
    )

    t2 = badge_titles([g2, g1], CUTOFF, [], [], [], LEAGUES)
    assert (
        t2.active[player("Ann")]["badge_reason"]
        == "Played in Royal League Count Season 9"
    )


def test_same_day_games_tiebreak_by_game_id_when_finish_time_equal_or_missing():
    g1 = game(
        "2024-06-01",
        intl(35, "Bronze"),
        "Ann",
        "Bob",
        game_id=10,
    )
    g2 = game(
        "2024-06-01",
        rl(9, "Count"),
        "Ann",
        "Cat",
        game_id=20,
    )
    t1 = badge_titles([g1, g2], CUTOFF, [], [], [], LEAGUES)
    assert (
        t1.active[player("Ann")]["badge_reason"]
        == "Played in Royal League Count Season 9"
    )

    t2 = badge_titles([g2, g1], CUTOFF, [], [], [], LEAGUES)
    assert (
        t2.active[player("Ann")]["badge_reason"]
        == "Played in Royal League Count Season 9"
    )


def test_derive_tournament_badges_picks_later_same_day_game(conn, add_match, run_id):
    add_match(
        ("Ann", 10),
        ("Bob", 5),
        day=date(2024, 6, 1),
        finished_at=datetime(2024, 6, 1, 10, 0, tzinfo=UTC),
        tournament="International S35 - Bronze 28",
    )
    add_match(
        ("Ann", 10),
        ("Cat", 5),
        day=date(2024, 6, 1),
        finished_at=datetime(2024, 6, 1, 15, 0, tzinfo=UTC),
        tournament="RL_s09 - Count D",
    )

    derive(conn, run_id)
    got = titles(conn, run_id)
    assert got["Ann"][2] == "Played in Royal League Count Season 9"


def test_a_podium_name_is_matched_to_a_player_exactly(conn, add_match, run_id, place):
    place("worlds", 2025, "Final", "alder440")
    add_match(
        ("Alder440", 10), ("Bob", 5), day=date(2024, 6, 2), tournament="Friendly Cup"
    )

    derive(conn, run_id)

    got = (
        titles(conn, run_id).get("Alder440"),
        peaks(conn, run_id).get("Alder440"),
        yearly(conn, run_id).get(("Alder440", 2025)),
    )
    assert got == (None, None, None)


def test_an_undated_top_division_win_is_a_peak_but_no_recent_or_year_title(
    conn, add_match, run_id, place
):
    place("royal_league", 1, "Emperor", "Ann", "Bob")
    add_match(("Ann", 10), ("Bob", 5), day=date(2026, 6, 1), tournament="RL_s09 - King")

    derive(conn, run_id)

    assert (
        titles(conn, run_id)["Ann"][0],
        peaks(conn, run_id)["Ann"],
        yearly(conn, run_id)[("Ann", 2026)],
    ) == (
        "M",
        ("SGM", None),
        "M",
    )
