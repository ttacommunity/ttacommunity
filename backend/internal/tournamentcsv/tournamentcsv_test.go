package tournamentcsv

import (
	"io"
	"strings"
	"testing"
)

func FuzzReaders(f *testing.F) {
	for _, seed := range []string{
		header + "international,20,Grandmaster,ann,Hall of Fame.xlsx\r\n",
		placementsHeader + "intermezzo,24,Grandmaster,ann,1,25,no games in the match data,Hall of Fame.xlsx\n",
		correctionsHeader + "intermezzo,8,7,labelled one high,Intermezzo Season 7.xlsx\nintermezzo,7,,a copy,s\n",
		datesHeader + "international,34,2026-08-14,s\n",
		honoursHeader + "sodium_ladder,ann,5,2,0,\"8,5\",s\nq_and_d,bob,2,2,1,,s\n",
		standingsHeader + "2024,1,1,ann,7657,11,s\nrace26,3,2,bob,3970,11,s\n",
		scoresHeader + "2024,ann,wrld,1600,s\n",
		cgeHeader + "5731,Cup 2026,finished,bob,2025-12-21,,2026-09-03,\"Welcome!\nOne game.\",Cup contributes,https://example.org,cup,2026\n",
		cgeStandingsHeader + "4760,2,2024 group 1,1,1,ann,101239,,5,21,1 1,1 0 0,2 0 0 0,CGE p\n",
		communityLaddersHeader + "mercurial_ladder,2024,0.8,,1 0.4 0,10,tab\nsodium_ladder,2025,0.8,4,1 0.5 0.166 0,8,tab\n",
		communityPointsHeader + "international,2025,Grandmaster,1,1000,sheet\nworlds,2025,Stage 3,0,130,sheet\n",
		communityQuartersHeader + "wimbledon,2025,2026 Q1,tab\n",
		ladderWinsHeader + "mercurial_ladder,ann,10,s\nmercurial_ladder,bob,3.5,s\n",
		notesHeader + "worlds,2023 Play Off,Stage 6,The final four.,CGE /tournaments/detail/4338/6\n",
		podiumsHeader + "wimbledon,2024,,2025-02-24,1,ann,CGE /tournaments/detail/4991/7\n",
		rulesHeader + "international,16,0,5-2-0,6-3-1-0,\"duelPositionsComparison, winnerScore / winnerScore\",s\n",
		stagesHeader + "intermezzo,26,,19,21\nwimbledon,2024,Stage 7,4991,7\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		check(t, "ReadDivisions", ReadDivisions, in)
		check(t, "ReadPlacements", ReadPlacements, in)
		check(t, "ReadCorrections", ReadCorrections, in)
		check(t, "ReadFinishDates", ReadFinishDates, in)
		check(t, "ReadHonours", ReadHonours, in)
		check(t, "ReadCommunityStandings", func(f string, r io.Reader) (Result[Standing], error) {
			return ReadCommunityStandings(f, r, editions)
		}, in)
		check(t, "ReadCommunityScores", func(f string, r io.Reader) (Result[Score], error) {
			return ReadCommunityScores(f, r, editions)
		}, in)
		check(t, "ReadCGETournaments", ReadCGETournaments, in)
		check(t, "ReadCGEStandings", ReadCGEStandings, in)
		check(t, "ReadCommunityLadders", ReadCommunityLadders, in)
		check(t, "ReadCommunityPoints", ReadCommunityPoints, in)
		check(t, "ReadCommunityQuarters", ReadCommunityQuarters, in)
		check(t, "ReadLadderWins", ReadLadderWins, in)
		check(t, "ReadSeasonNotes", ReadSeasonNotes, in)
		check(t, "ReadPodiums", ReadPodiums, in)
		check(t, "ReadRules", ReadRules, in)
		check(t, "ReadCGEStages", ReadCGEStages, in)
	})
}

// check reads in with read and reports a result whose facts and refusals do not add up to its rows.
func check[T any](t *testing.T, name string, read func(string, io.Reader) (Result[T], error), in string) {
	got, err := read("f.csv", strings.NewReader(in))
	if err != nil {
		return
	}
	if n := len(got.Facts) + len(got.Refused); n != got.Rows {
		t.Errorf("%s(%q): facts + refused = %d, want rows %d", name, in, n, got.Rows)
	}
}
