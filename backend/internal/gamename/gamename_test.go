package gamename

import "testing"

func TestParseReadsEachNameShape(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Placement
	}{
		{"International, a game", "International S24 - Bronze 3 game 7", Placement{"international", "24", "", "Bronze 3", 7}},
		{"International, a dash before the game", "International S28 - Silver 2 - game 4", Placement{"international", "28", "", "Silver 2", 4}},
		{"International, the season alone", "International S10", Placement{"international", "10", "", "", 0}},
		{"Intermezzo, a top division without a number", "Intermezzo S6 - Grandmaster game 9", Placement{"intermezzo", "6", "", "Grandmaster", 9}},
		{"Royal League", "RL_s03 - Baron 1 game 1", Placement{"royal_league", "3", "", "Baron 1", 1}},
		{"Royal League, the season alone", "RL_s01", Placement{"royal_league", "1", "", "", 0}},
		{"Royal League tiebreak", "RL_tb_s06 - Best Baron 1 game 1", Placement{"royal_league", "6", "Tiebreak", "Best Baron 1", 1}},
		{"Royal League tiebreak, no s", "RL_tb_03 - Best Count 3-way game 1", Placement{"royal_league", "3", "Tiebreak", "Best Count 3-way", 1}},
		{"Royal League 2-way tiebreak", "RL_2w_tb_s02 - Baron 1 - game 1", Placement{"royal_league", "2", "2-way tiebreak", "Baron 1", 1}},
		{"Royal League 3-way tiebreak, no dash", "RL_3w_tb_s02 Viscount 1 game 2", Placement{"royal_league", "2", "3-way tiebreak", "Viscount 1", 2}},
		{"Royal League 3-way tiebreak, no game", "RL_3w_tb_s02 Marquess 5", Placement{"royal_league", "2", "3-way tiebreak", "Marquess 5", 0}},
		{"Royal League, places in the group", "RL_s05 - Marquess 2 pl.4-6 game 1", Placement{"royal_league", "5", "", "Marquess 2 pl.4-6", 1}},
		{"World Championship 1", "Worlds 1 stage 3", Placement{"worlds", "2022 (Age A)", "Stage 3", "", 0}},
		{"World Championship 2, a stage", "Worlds 2 - stage 2", Placement{"worlds", "2023", "Stage 2", "", 0}},
		{"World Championship 2, finals", "Worlds 2 - finals", Placement{"worlds", "2023", "Finals", "", 0}},
		{"World Championship by year", "Worlds 2024 Stage 4 - Lower 12 game 3", Placement{"worlds", "2024", "Stage 4", "Lower 12", 3}},
		{"World Championship play-off", "Worlds 2023 Play Off Stage 6 - Finals game 2", Placement{"worlds", "2023 Play Off", "Stage 6", "Finals", 2}},
		{"World Championship, a group with a dash", "Worlds 2025 Stage 1 - Group 7 - game 2", Placement{"worlds", "2025", "Stage 1", "Group 7", 2}},
		{"Wimbledon, lower-case stage", "Wimbledon 2023 stage 2", Placement{"wimbledon", "2023", "Stage 2", "", 0}},
		{"Wimbledon", "Wimbledon 2024 Stage 7 - Final game 5", Placement{"wimbledon", "2024", "Stage 7", "Final", 5}},
		{"Wimbledon without stages", "Wimbledon 2025 - Lower QF 2 game 1", Placement{"wimbledon", "2025", "", "Lower QF 2", 1}},
		{"French Open", "French Open 2026 - group 12 game 3", Placement{"french_open", "2026", "", "group 12", 3}},
		{"French Open, an added stage", "French Open 2025 (Odd Games) Stage 2 - group 1 game 1", Placement{"french_open", "2025", "Odd Games Stage 2", "group 1", 1}},
		{"French Open, a game with no group", "French Open 2025 -  game 1", Placement{"french_open", "2025", "", "", 1}},
		{"Australian Open", "Australian Open 2026 Stage 7 - Final game 1", Placement{"australian_open", "2026", "Stage 7", "Final", 1}},
		{"Survivors Cup", "Survivors Cup 2026 Stage 1 - group 100 game 1", Placement{"survivors_cup", "2026", "Stage 1", "group 100", 1}},
		{"Leaderboard Trophy, a game with no group", "Leaderboard Trophy 2025 Stage 1 - game 11", Placement{"leaderboard_trophy", "2025", "Stage 1", "", 11}},
		{"Slow Burn, before the S", "Slow Burn 2", Placement{"slow_burn", "1 (2024)", "Stage 2", "", 0}},
		{"Slow Burn", "Slow Burn S10 - group 1 game 3", Placement{"slow_burn", "1 (2024)", "Stage 10", "group 1", 3}},
		{"Slow Burn, its final", "Slow Burn S15 - Finals game 1", Placement{"slow_burn", "1 (2024)", "Stage 15", "Finals", 1}},
		{"Slow Burn, its second season", "Slow Burn Season 2 Stage 1 - group 4 game 2", Placement{"slow_burn", "2 (2026)", "Stage 1", "group 4", 2}},
		{"Eiffel Tower", "Eiffel Tower S3 - 1st Floor game 1", Placement{"eiffel_tower", "3", "", "1st Floor", 1}},
		{"Mercurial Ladder, 2023", "ML_s01", Placement{"mercurial_ladder", "1", "", "", 0}},
		{"Mercurial Ladder, 2024", "ML_r11", Placement{"mercurial_ladder", "11", "", "", 0}},
		{"Mercurial Ladder", "Mercurial Season 19 - -1 Tier - - game 1", Placement{"mercurial_ladder", "19", "", "-1 Tier", 1}},
		{"Mercurial Ladder, a tier", "Mercurial Season 20 - 3-Neon 2 game 4", Placement{"mercurial_ladder", "20", "", "3-Neon 2", 4}},
		{"Sodium Ladder", "Sodium Season 10 - 10-Neon 1 game 1", Placement{"sodium_ladder", "10", "", "10-Neon 1", 1}},
		{"Sodium Ladder, its later name", "Sodium Ladder Season 16 - Sodium Ladder 10-Neon 1 game 1", Placement{"sodium_ladder", "16", "", "Sodium Ladder 10-Neon 1", 1}},
		{"Transcontinental Ladder", "TCL Round 100", Placement{"transcontinental_ladder", "100", "", "", 0}},
		{"Transcontinental Ladder, a rung", "TCL Round 130 - Rung 2 game 1", Placement{"transcontinental_ladder", "130", "", "Rung 2", 1}},
		{"Premier League", "Premier League 3", Placement{"premier_league", "3", "", "", 0}},
		{"Premier League, its first edition", "Premier League First Edition - game 12", Placement{"premier_league", "First Edition (2023)", "", "", 12}},
		{"Premier League, PL_s01", "PL_s01", Placement{"premier_league", "1", "PL_s01", "", 0}},
		{"Duel Curve", "DC_s02", Placement{"duel_curve", "2", "", "", 0}},
		{"Duel Curve Grand Prix", "DC Grand Prix 2023 - game 21", Placement{"duel_curve", "Grand Prix 2023", "", "", 21}},
		{"Nations League", "NL_s10", Placement{"nations_league", "10", "", "", 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Parse(tt.in)
			if !ok {
				t.Fatalf("Parse(%q) refused it", tt.in)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseRefusesNamesItDoesNotKnow(t *testing.T) {
	for _, in := range []string{
		"",
		"Friendly game",
		"International",
		"International S",
		"Intermezzo S0 - Wood 1 game 1",
		"Worlds 3 stage 1",
		"Slow Burn Season 3 Stage 1",
		"Quick and Dirty S1",
		"International S24 - Bronze 3 game 0",
		"XInternational S24",
	} {
		if got, ok := Parse(in); ok {
			t.Errorf("Parse(%q) = %+v, want it refused", in, got)
		}
	}
}

func TestTournamentsNamesEveryTournamentParseReturns(t *testing.T) {
	seen := map[string]bool{}
	for _, tr := range Tournaments {
		if seen[tr.Slug] {
			t.Errorf("tournament %q listed twice", tr.Slug)
		}
		seen[tr.Slug] = true
	}
	for _, r := range rules {
		if !seen[r.tournament] {
			t.Errorf("a name rule returns %q, which Tournaments does not list", r.tournament)
		}
	}
}
