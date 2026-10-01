package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const podiumsHeader = "cup,season,final,place,player,source\n"

func TestReadPodiumsTakesEachPlace(t *testing.T) {
	in := podiumsHeader +
		"wimbledon,2024,,1,saru,CGE /tournaments/detail/4991/7\n" +
		"french_open,2025,odd games,3,Sylvain,CGE /tournaments/detail/5459\n"

	got, err := ReadPodiums("p.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadPodiums: %v", err)
	}
	want := []Podium{
		{Cup: "wimbledon", Season: "2024", Final: "", Place: 1, Player: "saru", Source: "CGE /tournaments/detail/4991/7"},
		{Cup: "french_open", Season: "2025", Final: "odd games", Place: 3, Player: "Sylvain", Source: "CGE /tournaments/detail/5459"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("podiums = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadPodiumsRefusesBadRows(t *testing.T) {
	tests := []struct {
		name string
		row  string
		want Reason
	}{
		{"unknown cup", "snail_cup,2024,,1,saru,s", BadSeries},
		{"no season", "wimbledon,,,1,saru,s", BadSeason},
		{"fourth place", "wimbledon,2024,,4,saru,s", BadPlace},
		{"place zero", "wimbledon,2024,,0,saru,s", BadPlace},
		{"no player", "wimbledon,2024,,1,,s", BadPlayer},
		{"a final with spaces around it", "french_open,2025, odd games,1,saru,s", BadDivision},
		{"no source", "wimbledon,2024,,1,saru,", BadSource},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadPodiums("p.csv", strings.NewReader(podiumsHeader+tt.row+"\n"))
			if err != nil {
				t.Fatalf("ReadPodiums(%q): %v", tt.row, err)
			}
			want := []Refusal{{File: "p.csv", Line: 2, Reason: tt.want}}
			if !slices.Equal(got.Refused, want) || len(got.Facts) != 0 {
				t.Errorf("ReadPodiums(%q): refused = %v; want %v", tt.row, got.Refused, want)
			}
		})
	}
}

func TestReadPodiumsRefusesAPlaceTwice(t *testing.T) {
	got, err := ReadPodiums("p.csv", strings.NewReader(podiumsHeader+"wimbledon,2024,,1,saru,s\nwimbledon,2024,,1,whsvin,s\n"))
	if err != nil {
		t.Fatalf("ReadPodiums: %v", err)
	}
	if len(got.Facts) != 1 || !slices.Equal(got.Refused, []Refusal{{File: "p.csv", Line: 3, Reason: Duplicate}}) {
		t.Errorf("podiums %+v, refused %v; want the first row and line 3 a duplicate", got.Facts, got.Refused)
	}
}
