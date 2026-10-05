package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const rulesHeader = "series,season,timeout_penalty,scoring_3p,scoring_4p,tiebreaks,source\n"

func TestReadRulesTakesEachSeasonsRules(t *testing.T) {
	in := rulesHeader +
		"intermezzo,6,-1,5-2-0,,,CGE Intermezzo Championship season 1\n" +
		"international,16,0,5-2-0,6-3-1-0,\"duelPositionsComparison, winnerScore / winnerScore\",CGE International Championship season 7\n"

	got, err := ReadRules("r.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Fatalf("rules = %+v, refused %v", got.Facts, got.Refused)
	}
	first, second := got.Facts[0], got.Facts[1]
	if first.Series != "intermezzo" || first.Season != 6 || first.TimeoutPenalty != -1 || first.Scoring3p != "5-2-0" ||
		first.Scoring4p != "" || first.Source != "CGE Intermezzo Championship season 1" {
		t.Errorf("rule 1 = %+v", first)
	}
	if second.TimeoutPenalty != 0 || second.Scoring4p != "6-3-1-0" || second.Tiebreaks != "duelPositionsComparison, winnerScore / winnerScore" {
		t.Errorf("rule 2 = %+v", second)
	}
}

func TestReadRulesRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown series", "snail_cup,1,0,,,,s", BadSeries},
		{"season zero", "intermezzo,0,0,,,,s", BadSeason},
		{"penalty not a number", "intermezzo,6,minus one,,,,s", BadPoints},
		{"a penalty that adds points", "intermezzo,6,1,,,,s", BadPoints},
		{"no source", "intermezzo,6,-1,,,,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadRules("r.csv", strings.NewReader(rulesHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadRules(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "r.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadRules(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadRulesRefusesASeasonTwice(t *testing.T) {
	got, err := ReadRules("r.csv", strings.NewReader(rulesHeader+"intermezzo,6,-1,,,,s\nintermezzo,6,0,,,,t\n"))
	if err != nil {
		t.Fatalf("ReadRules: %v", err)
	}
	if len(got.Facts) != 1 || !slices.Equal(got.Refused, []Refusal{{File: "r.csv", Line: 3, Reason: Duplicate}}) {
		t.Errorf("rules %+v, refused %v; want the first row and line 3 a duplicate", got.Facts, got.Refused)
	}
}
