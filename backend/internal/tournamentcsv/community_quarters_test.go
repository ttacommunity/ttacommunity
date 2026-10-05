package tournamentcsv

import (
	"slices"
	"strings"
	"testing"
)

const communityQuartersHeader = "tournament,season,quarter,source\n"

func TestReadCommunityQuartersTakesEachRow(t *testing.T) {
	in := communityQuartersHeader + "wimbledon,2025,2026 Q1,tab\ninternational,30,2025 Q3,tab\n"
	got, err := ReadCommunityQuarters("q.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityQuarters: %v", err)
	}
	want := []CommunityQuarter{
		{Tournament: "wimbledon", Season: "2025", Year: 2026, Quarter: 1, Source: "tab"},
		{Tournament: "international", Season: "30", Year: 2025, Quarter: 3, Source: "tab"},
	}
	if !slices.Equal(got.Facts, want) || len(got.Refused) != 0 {
		t.Errorf("quarters = %+v, refused %v; want %+v", got.Facts, got.Refused, want)
	}
}

func TestReadCommunityQuartersRefusesBadRows(t *testing.T) {
	for name, row := range map[string]string{
		"not a tournament name": "Wimbledon,2025,2026 Q1,s",
		"no season":             "wimbledon,,2026 Q1,s",
		"no quarter":            "wimbledon,2025,,s",
		"a fifth quarter":       "wimbledon,2025,2026 Q5,s",
		"a quarter as a month":  "wimbledon,2025,2026-01,s",
		"no source":             "wimbledon,2025,2026 Q1,",
	} {
		got, err := ReadCommunityQuarters("q.csv", strings.NewReader(communityQuartersHeader+row+"\n"))
		if err != nil {
			t.Fatalf("%s: ReadCommunityQuarters: %v", name, err)
		}
		if len(got.Facts) != 0 || len(got.Refused) != 1 {
			t.Errorf("%s: %+v accepted, want it refused", name, got.Facts)
		}
	}
}

func TestReadCommunityQuartersRefusesASeasonGivenTwice(t *testing.T) {
	in := communityQuartersHeader + "wimbledon,2025,2026 Q1,s\nwimbledon,2025,2025 Q4,s\n"
	got, err := ReadCommunityQuarters("q.csv", strings.NewReader(in))
	if err != nil {
		t.Fatalf("ReadCommunityQuarters: %v", err)
	}
	if len(got.Facts) != 1 || len(got.Refused) != 1 || got.Refused[0].Reason != Duplicate {
		t.Errorf("got %d rows, refused %v; want the second refused as a duplicate", len(got.Facts), got.Refused)
	}
}
