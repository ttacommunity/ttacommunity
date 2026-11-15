package movies

import (
	"testing"
	"time"
)

func TestParseChannelID(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "bare id", raw: "UC0000000000000000000001", want: "UC0000000000000000000001", ok: true},
		{name: "channel url", raw: "https://www.youtube.com/channel/UC0000000000000000000001?sub_confirmation=1", want: "UC0000000000000000000001", ok: true},
		{name: "handle url", raw: "https://www.youtube.com/@InventedCaster", want: "", ok: false},
		{name: "invalid", raw: "not-a-channel", want: "", ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseChannelID(tc.raw)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestParseFeed(t *testing.T) {
	xmlFeed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns:yt="http://www.youtube.com/xml/schemas/2015" xmlns:media="http://search.yahoo.com/mrss/" xmlns="http://www.w3.org/2005/Atom">
 <entry>
  <yt:videoId>9vhRomv0yBc</yt:videoId>
  <title>Royal League vs Arne</title>
  <published>2026-11-14T14:17:33+00:00</published>
  <media:group>
   <media:thumbnail url="https://i2.ytimg.com/vi/9vhRomv0yBc/hqdefault.jpg"/>
   <media:description>Match commentary #TTAHub</media:description>
  </media:group>
 </entry>
</feed>`
	entries, err := ParseFeed([]byte(xmlFeed))
	if err != nil {
		t.Fatalf("ParseFeed failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.VideoID != "9vhRomv0yBc" || e.Title != "Royal League vs Arne" || e.Description != "Match commentary #TTAHub" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	wantTime := time.Date(2026, 11, 14, 14, 17, 33, 0, time.UTC)
	if !e.PublishedAt.Equal(wantTime) {
		t.Fatalf("got published %v, want %v", e.PublishedAt, wantTime)
	}
}

func TestClassify(t *testing.T) {
	tournaments := []string{"Royal League", "Cup"}
	tests := []struct {
		name       string
		text       VideoText
		includeAll bool
		want       Verdict
	}{
		{"a channel whose every video counts", VideoText{Title: "Chess stream"}, true, Verdict{true, "the channel's every video counts"}},
		{"the game's name", VideoText{Title: "Through the Ages: strategy guide"}, false, Verdict{true, "mentions Through the Ages"}},
		{"a hashtag in the description", VideoText{Title: "Highlights", Description: "Tonight #TTAHub"}, false, Verdict{true, "mentions Through the Ages"}},
		{"the word TTA", VideoText{Title: "TTA 4p ranked game"}, false, Verdict{true, "mentions TTA"}},
		{"a tag", VideoText{Title: "Game 5", Tags: []string{"boardgame", "Through The Ages"}}, false, Verdict{true, "mentions Through the Ages"}},
		{"a tournament's name", VideoText{Title: "Royal League round 3"}, false, Verdict{true, "names the tournament Royal League"}},
		{"a tournament name too short to trust", VideoText{Title: "World Cup final"}, false, Verdict{false, "no mention of TTA"}},
		{"tta inside another word", VideoText{Title: "Attack on Titan review"}, false, Verdict{false, "no mention of TTA"}},
		{"another game", VideoText{Title: "Terraforming Mars solo"}, false, Verdict{false, "no mention of TTA"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.text, tt.includeAll, tournaments); got != tt.want {
				t.Errorf("Classify(%+v) = %+v, want %+v", tt.text, got, tt.want)
			}
		})
	}
}
