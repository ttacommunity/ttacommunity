package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const podiumsHeader = "cup,season,final,finished,place,player,source\n"

func TestReadPodiumsTakesEachPlace(t *testing.T) {
	in := podiumsHeader +
		"wimbledon,2024,,2025-02-24,1,sora,CGE /tournaments/detail/4991/7\n" +
		"french_open,2025,odd games,2026-05-19,3,Siskin,CGE /tournaments/detail/5459\n"

	got, err := ReadPodiums("p.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadPodiums: %v", err)
	}
	want := []Podium{
		{Cup: "wimbledon", Season: "2024", Final: "", Finished: day(2025, 2, 24), Place: 1, Player: "sora", Source: "CGE /tournaments/detail/4991/7"},
		{Cup: "french_open", Season: "2025", Final: "odd games", Finished: day(2026, 5, 19), Place: 3, Player: "Siskin", Source: "CGE /tournaments/detail/5459"},
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
		{"unknown cup", "snail_cup,2024,,2025-02-24,1,sora,s", BadSeries},
		{"no season", "wimbledon,,,2025-02-24,1,sora,s", BadSeason},
		{"no finish date", "wimbledon,2024,,,1,sora,s", BadDate},
		{"a finish date that is not a day", "wimbledon,2024,,24/02/2025,1,sora,s", BadDate},
		{"fourth place", "wimbledon,2024,,2025-02-24,4,sora,s", BadPlace},
		{"place zero", "wimbledon,2024,,2025-02-24,0,sora,s", BadPlace},
		{"no player", "wimbledon,2024,,2025-02-24,1,,s", BadPlayer},
		{"a final with spaces around it", "french_open,2025, odd games,2026-05-19,1,sora,s", BadDivision},
		{"no source", "wimbledon,2024,,2025-02-24,1,sora,", BadSource},
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
	got, err := ReadPodiums("p.csv", strings.NewReader(podiumsHeader+"wimbledon,2024,,2025-02-24,1,sora,s\nwimbledon,2024,,2025-02-24,1,wheatear,s\n"))
	if err != nil {
		t.Fatalf("ReadPodiums: %v", err)
	}
	if len(got.Facts) != 1 || !slices.Equal(got.Refused, []Refusal{{File: "p.csv", Line: 3, Reason: Duplicate}}) {
		t.Errorf("podiums %+v, refused %v; want the first row and line 3 a duplicate", got.Facts, got.Refused)
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
