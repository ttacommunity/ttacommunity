package movies

import (
	"context"
	"io"
	"net/http"
	"strings"
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

func TestIsTTAVideo(t *testing.T) {
	tournaments := []string{"Royal League", "US Open", "Intermezzo", "World Championship"}
	cases := []struct {
		name  string
		title string
		desc  string
		kw    string
		want  bool
	}{
		{name: "ttahub hashtag in desc", title: "Stream highlights", desc: "Playing #TTAHub tonight", want: true},
		{name: "through the ages in title", title: "Through the Ages: Strategy Guide", want: true},
		{name: "whole word TTA", title: "TTA 4p Ranked Game", want: true},
		{name: "tournament name in title", title: "Royal League vs Arne", want: true},
		{name: "keywords fallback", title: "Game 5 vs Bob", kw: "Through The Ages, Boardgame", want: true},
		{name: "substring tta inside another word", title: "Attack on Titan review", desc: "great battles", want: false},
		{name: "unrelated game", title: "Terraforming Mars solo", desc: "playing board games", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsTTAVideo(tc.title, tc.desc, tc.kw, tournaments)
			if got != tc.want {
				t.Fatalf("got = %v, want %v", got, tc.want)
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

func TestResolveChannelHandle(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		body string
		want string
	}{
		{
			name: "channelId in json",
			raw:  "https://www.youtube.com/@InventedCaster",
			body: `<html>{"channelId":"UC0000000000000000000001"}</html>`,
			want: "UC0000000000000000000001",
		},
		{
			name: "bare @handle with externalId",
			raw:  "@daneo1952",
			body: `<html>{"externalId":"UC0000000000000000000001"}</html>`,
			want: "UC0000000000000000000001",
		},
		{
			name: "browseId in json",
			raw:  "https://www.youtube.com/@daneo1952",
			body: `<html>{"browseId":"UC0000000000000000000001"}</html>`,
			want: "UC0000000000000000000001",
		},
		{
			name: "canonical channel url in html",
			raw:  "https://www.youtube.com/@daneo1952",
			body: `<html><link rel="canonical" href="https://www.youtube.com/channel/UC0000000000000000000001"></html>`,
			want: "UC0000000000000000000001",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotCookie string
			f := NewFetcher(&http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					gotCookie = r.Header.Get("Cookie")
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(tc.body)),
					}, nil
				}),
			})
			got, err := f.ResolveChannel(context.Background(), tc.raw)
			if err != nil {
				t.Fatalf("ResolveChannel failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got = %q, want %q", got, tc.want)
			}
			if !strings.Contains(gotCookie, "SOCS=CAI") {
				t.Fatalf("Cookie = %q, want SOCS=CAI", gotCookie)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
