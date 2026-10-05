package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const communityLaddersHeader = "tournament,from_edition,ratio,half_steps_from,shares,rounds,source\n"

func TestReadCommunityLaddersTakesEachRow(t *testing.T) {
	in := communityLaddersHeader + "mercurial_ladder,2024,0.8,,1 0.4 0,10,tab\nsodium_ladder,2025,0.8,4,1 0.5 0.166 0,8,tab\n"
	got, err := ReadCommunityLadders("l.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityLadders: %v", err)
	}
	if len(got.Facts) != 2 || len(got.Refused) != 0 {
		t.Fatalf("got %+v, refused %v; want 2 rows", got.Facts, got.Refused)
	}
	m, s := got.Facts[0], got.Facts[1]
	if m.Tournament != "mercurial_ladder" || m.FromEdition != 2024 || m.Ratio != 0.8 || m.HalfStepsFrom != 0 || m.Rounds != 10 || !slices.Equal(m.Shares, []float64{1, 0.4, 0}) {
		t.Errorf("mercurial = %+v", m)
	}
	if s.HalfStepsFrom != 4 || !slices.Equal(s.Shares, []float64{1, 0.5, 0.166, 0}) {
		t.Errorf("sodium = %+v", s)
	}
}

func TestReadCommunityLaddersRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"not a tournament name": "Mercurial,2024,0.8,,1 0.4 0,10,s",
		"edition not a year":    "mercurial_ladder,24,0.8,,1 0.4 0,10,s",
		"ratio of 1":            "mercurial_ladder,2024,1,,1 0.4 0,10,s",
		"ratio not a number":    "mercurial_ladder,2024,x,,1 0.4 0,10,s",
		"half steps from 1":     "mercurial_ladder,2024,0.8,1,1 0.4 0,10,s",
		"no shares":             "mercurial_ladder,2024,0.8,,,10,s",
		"a share above 1":       "mercurial_ladder,2024,0.8,,1.5 0.4 0,10,s",
		"no rounds":             "mercurial_ladder,2024,0.8,,1 0.4 0,0,s",
		"no source":             "mercurial_ladder,2024,0.8,,1 0.4 0,10,",
	} {
		got, err := ReadCommunityLadders("l.csv", strings.NewReader(communityLaddersHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadCommunityLadders: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}
