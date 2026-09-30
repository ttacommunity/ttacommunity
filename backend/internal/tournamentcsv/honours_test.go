package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const honoursHeader = "series,player,gold,silver,bronze,points,source\n"

func TestReadHonoursTakesEveryColumn(t *testing.T) {
	in := honoursHeader +
		"sodium_ladder,ann,5,2,0,\"8,5\",TtA Sodium Ladder - Hall of Fame.csv\n" +
		"q_and_d,bob,2,2,1,,Quick and Dirty - Hall of Fame.csv\n" +
		"mercurial_ladder,ann,3,0,0,,m\n"

	got, err := ReadHonours("h.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadHonours: %v", err)
	}
	points := "8,5"
	want := []Honour{
		{Series: "sodium_ladder", Player: "ann", Gold: 5, Silver: 2, Bronze: 0, Points: &points, Source: "TtA Sodium Ladder - Hall of Fame.csv"},
		{Series: "q_and_d", Player: "bob", Gold: 2, Silver: 2, Bronze: 1, Source: "Quick and Dirty - Hall of Fame.csv"},
		{Series: "mercurial_ladder", Player: "ann", Gold: 3, Source: "m"},
	}
	if len(got.Facts) != len(want) || len(got.Refused) != 0 {
		t.Fatalf("honours = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
	for i := range want {
		g, w := got.Facts[i], want[i]
		if g.Series != w.Series || g.Player != w.Player || g.Gold != w.Gold || g.Silver != w.Silver ||
			g.Bronze != w.Bronze || (g.Points == nil) != (w.Points == nil) ||
			(g.Points != nil && *g.Points != *w.Points) || g.Source != w.Source {
			t.Errorf("honour %d = %+v, want %+v", i, g, w)
		}
	}
}

func TestReadHonoursRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"a league series", "international,ann,1,0,0,,s", BadSeries},
		{"no player", "q_and_d,,1,0,0,,s", BadPlayer},
		{"negative gold", "q_and_d,ann,-1,0,0,,s", BadMedals},
		{"silver not a number", "q_and_d,ann,1,two,0,,s", BadMedals},
		{"no bronze", "q_and_d,ann,1,0,,,s", BadMedals},
		{"no source", "q_and_d,ann,1,0,0,,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadHonours("h.csv", strings.NewReader(honoursHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadHonours(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "h.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadHonours(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadHonoursRefusesAPlayerTwiceInASeries(t *testing.T) {
	in := honoursHeader + "q_and_d,ann,1,0,0,,s\nq_and_d,ann,2,0,0,,s\nsodium_ladder,ann,1,0,0,,s\n"
	got, err := ReadHonours("h.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadHonours: %v", err)
	}
	if want := []Refusal{{File: "h.csv", Line: 3, Reason: Duplicate}}; !slices.Equal(got.Refused, want) || len(got.Facts) != 2 {
		t.Errorf("refused = %v, facts %d; want %v and 2 facts", got.Refused, len(got.Facts), want)
	}
}
