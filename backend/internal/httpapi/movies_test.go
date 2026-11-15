package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

func (f *fakeRatings) Movies(_ context.Context) (store.MoviesFeed, error) {
	p := "Alice"
	return store.MoviesFeed{
		Creators: []store.MovieCreator{
			{ChannelID: "UC0000000000000000000001", Name: "Alice Casts", Player: &p},
		},
		Videos: []store.MovieVideo{
			{
				VideoID:       "9vhRomv0yBc",
				ChannelID:     "UC0000000000000000000001",
				CreatorName:   "Alice Casts",
				CreatorPlayer: &p,
				Title:         "Royal League vs Arne",
				PublishedAt:   time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
				ThumbnailURL:  "https://i.ytimg.com/vi/9vhRomv0yBc/hqdefault.jpg",
			},
		},
	}, nil
}

func (readsOnly) Movies(_ context.Context) (store.MoviesFeed, error) {
	return store.MoviesFeed{}, nil
}

func (f *fakeSignIns) MarkMoviesSeen(_ context.Context, account int64, _ time.Time) error {
	f.notes = append(f.notes, "movies_seen:"+string(rune('0'+account)))
	return nil
}

func (f *fakeAdmins) AdminMovies(_ context.Context) (store.AdminMoviesState, error) {
	return store.AdminMoviesState{
		Channels: []store.AdminVideoChannel{
			{
				ChannelID:  "UC0000000000000000000001",
				Name:       "Alice Casts",
				CreatedAt:  time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC),
				VideoCount: 1,
			},
		},
		Videos: []store.AdminVideo{
			{
				VideoID:      "9vhRomv0yBc",
				ChannelID:    "UC0000000000000000000001",
				CreatorName:  "Alice Casts",
				Title:        "Royal League vs Arne",
				PublishedAt:  time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
				ThumbnailURL: "https://i.ytimg.com/vi/9vhRomv0yBc/hqdefault.jpg",
			},
		},
	}, nil
}

func (f *fakeAdmins) ResolveMovieChannel(_ context.Context, raw string) (string, error) {
	switch raw {
	case "not-a-channel":
		return "", movies.ErrNotAChannel
	case "@NobodyInvented":
		return "", movies.ErrNoSuchChannel
	case "@NoKeyInvented":
		return "", movies.ErrNoYouTube
	case "@FailingInvented":
		return "", errors.New("looking up a handle: the Data API answered HTTP 503 (backendError)")
	}
	return "UC0000000000000000000001", nil
}

func TestAddingAMovieChannelSaysWhyAChannelIsRefused(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name, channel string
		wantCode      int
		wantError     string
	}{
		{"a handle", "@InventedCaster", http.StatusCreated, ""},
		{"not a channel", "not-a-channel", http.StatusBadRequest, movies.ErrNotAChannel.Error()},
		{"no channel has the handle", "@NobodyInvented", http.StatusBadRequest, movies.ErrNoSuchChannel.Error()},
		{"a handle with no key set up", "@NoKeyInvented", http.StatusBadRequest, movies.ErrNoYouTube.Error()},
		{"the Data API failing", "@FailingInvented", http.StatusInternalServerError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/movies/channels", site,
				`{"channel": "`+tt.channel+`", "name": "Invented Caster", "include_all": false}`)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tt.wantCode)
			}
			if tt.wantError != "" {
				wantJSON(t, rec.Body.Bytes(), "", fmt.Sprintf(`{"error": %q}`, tt.wantError))
			}
			if strings.Contains(rec.Body.String(), "backendError") {
				t.Errorf("body = %s, shows the Data API's own error", rec.Body)
			}
			if tt.wantCode != http.StatusCreated && slices.ContainsFunc(f.notes, func(n string) bool { return strings.HasPrefix(n, "add channel") }) {
				t.Errorf("notes = %q, a refused channel was added", f.notes)
			}
		})
	}
}

func (f *fakeAdmins) AddMovieChannel(_ context.Context, channelID, name string, player *string, includeAll bool, now time.Time) (store.AdminVideoChannel, error) {
	f.notes = append(f.notes, "add channel "+channelID+" "+name)
	return store.AdminVideoChannel{
		ChannelID:  channelID,
		Name:       name,
		Player:     player,
		IncludeAll: includeAll,
		CreatedAt:  now,
	}, nil
}

func (f *fakeAdmins) UpdateMovieChannel(_ context.Context, channelID string, _ *string, _ *string, _ bool, _ *bool) error {
	if channelID == "UC_missing" {
		return store.ErrNoChannel
	}
	f.notes = append(f.notes, "update channel "+channelID)
	return nil
}

func (f *fakeAdmins) DeleteMovieChannel(_ context.Context, channelID string) error {
	if channelID == "UC_missing" {
		return store.ErrNoChannel
	}
	f.notes = append(f.notes, "delete channel "+channelID)
	return nil
}

func (f *fakeAdmins) SetMovieVideoHidden(_ context.Context, videoID string, hidden bool) error {
	if videoID == "missing" {
		return store.ErrNoVideo
	}
	if hidden {
		f.notes = append(f.notes, "hide video "+videoID)
	} else {
		f.notes = append(f.notes, "show video "+videoID)
	}
	return nil
}

func (f *fakeAdmins) SyncMoviesNow(_ context.Context, _ time.Time) (store.MoviesSyncResult, error) {
	f.notes = append(f.notes, "sync movies")
	return store.MoviesSyncResult{Checked: 1, Added: 2}, nil
}

func TestMoviesFeedAndMarkSeen(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/movies")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/movies = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "videos.0", `{"video_id": "9vhRomv0yBc", "creator_name": "Alice Casts"}`)
	wantJSON(t, rec.Body.Bytes(), "creators.0", `{"channel_id": "UC0000000000000000000001", "player": "Alice"}`)

	signIns := newFakeSignIns()
	cookie := sessionFor(signIns, "session-token", store.Account{ID: 7})
	w := playerDo(t, signIns, http.MethodPost, "/v1/me/movies/seen", siteOrigin, "", cookie)
	if w.Code != http.StatusNoContent {
		t.Fatalf("POST /v1/me/movies/seen = %d %s, want 204", w.Code, w.Body)
	}
}

func TestAdminMoviesEndpoints(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	f := &fakeAdmins{}

	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/movies/channels", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/admin/movies/channels = %d %s", rec.Code, rec.Body)
	}
	var state AdminMoviesState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil || len(state.Channels) != 1 || len(state.Videos) != 1 {
		t.Fatalf("unexpected admin movies state: %v %+v", err, state)
	}

	if rec := adminDoAs(t, f, org, http.MethodGet, "/v1/admin/movies/channels", "", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("organiser GET = %d, want 403", rec.Code)
	}

	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/movies/channels", site,
		`{"channel": "https://www.youtube.com/@AliceCasts", "name": "Alice Casts"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST channel = %d %s, want 201", rec.Code, rec.Body)
	}

	rec = adminDoAs(t, f, owner, http.MethodPost, "/v1/admin/movies/sync", site, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST sync = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"checked": 1, "added": 2}`)

	rec = adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/movies/videos/9vhRomv0yBc", site, `{"hidden": true}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH video = %d %s, want 204", rec.Code, rec.Body)
	}
}
