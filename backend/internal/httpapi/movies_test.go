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

	"ttacommunity/backend/internal/enginejob"
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
				VideoID:       "vid00000001",
				ChannelID:     "UC0000000000000000000001",
				CreatorName:   "Alice Casts",
				CreatorPlayer: &p,
				Title:         "Royal League vs Invented Rival",
				PublishedAt:   time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
				ThumbnailURL:  "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg",
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
				ChannelID:   "UC0000000000000000000001",
				Name:        "Alice Casts",
				CreatedAt:   time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC),
				VideoCount:  1,
				VideosFound: 3,
			},
		},
		Videos: []store.AdminVideo{
			{
				VideoID:      "vid00000001",
				ChannelID:    "UC0000000000000000000001",
				CreatorName:  "Alice Casts",
				Title:        "Royal League vs Invented Rival",
				PublishedAt:  time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
				ThumbnailURL: "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg",
				TTAReason:    "no mention of TTA",
				TTAOverride:  &shownByAdmin,
				Shown:        true,
			},
		},
	}, nil
}

var shownByAdmin = true

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
			wantNotes := []string(nil)
			if tt.wantCode == http.StatusCreated {
				wantNotes = []string{"add channel UC0000000000000000000001 Invented Caster"}
			}
			if !slices.Equal(f.notes, wantNotes) {
				t.Errorf("notes = %q, want %q: a refused channel is not added", f.notes, wantNotes)
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

func (f *fakeAdmins) StartMovieJob(_ context.Context, channelID string) (string, error) {
	if f.moviesErr != nil {
		return "", f.moviesErr
	}
	if channelID == "UC_missing" {
		return "", store.ErrNoChannel
	}
	f.notes = append(f.notes, "start movies job for "+channelID)
	return "movies-admin-x1y2z", nil
}

func (f *fakeAdmins) MovieJobGoing(context.Context) (bool, error) {
	if f.moviesErr == enginejob.ErrNotSetUp {
		return false, enginejob.ErrNotSetUp
	}
	return f.moviesGoing, nil
}

func TestAnAdminStartsTheMoviesJob(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, as, body string
		err            error
		wantCode       int
		wantNote       string
		wantDetail     string
	}{
		{"every creator", owner, "", nil, http.StatusAccepted, "start movies job for ", `{"channel":""}`},
		{"one creator", owner, `{"channel": "UC0000000000000000000001"}`, nil, http.StatusAccepted, "start movies job for UC0000000000000000000001", `{"channel":"UC0000000000000000000001"}`},
		{"a channel not linked", owner, `{"channel": "UC_missing"}`, nil, http.StatusNotFound, "", ""},
		{"a job already running", owner, "", enginejob.ErrRunning, http.StatusConflict, "", ""},
		{"no job set up here", owner, "", enginejob.ErrNotSetUp, http.StatusServiceUnavailable, "", ""},
		{"an organiser", org, "", nil, http.StatusForbidden, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{moviesErr: tt.err}
			rec := adminDoAs(t, f, tt.as, http.MethodPost, "/v1/admin/movies/run", site, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tt.wantCode)
			}
			if tt.wantCode == http.StatusAccepted {
				wantJSON(t, rec.Body.Bytes(), "", `{"job": "movies-admin-x1y2z"}`)
			}
			wantNotes, wantAudits, wantDetails := []string(nil), []string(nil), []string(nil)
			if tt.wantNote != "" {
				wantNotes = []string{tt.wantNote}
				wantAudits, wantDetails = []string{owner + " movie.run movies-admin-x1y2z"}, []string{tt.wantDetail}
			}
			if !slices.Equal(f.notes, wantNotes) || !slices.Equal(f.audits, wantAudits) || !slices.Equal(f.details, wantDetails) {
				t.Errorf("notes %q, audits %q %q; want %q, %q %q", f.notes, f.audits, f.details, wantNotes, wantAudits, wantDetails)
			}
		})
	}
}

func TestAdminSeesWhetherTheMoviesJobIsRunning(t *testing.T) {
	for _, tt := range []struct {
		name string
		f    *fakeAdmins
		want string
	}{
		{"running", &fakeAdmins{moviesGoing: true}, "true"},
		{"not running", &fakeAdmins{}, "false"},
		{"not set up here", &fakeAdmins{moviesErr: enginejob.ErrNotSetUp}, "false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminDoAs(t, tt.f, "111111111111111111", http.MethodGet, "/v1/admin/movies/channels", "", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d %s, want 200", rec.Code, rec.Body)
			}
			wantJSON(t, rec.Body.Bytes(), "job_running", tt.want)
		})
	}
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

func (f *fakeAdmins) SetMovieVideoOverride(_ context.Context, videoID string, override *bool) error {
	if videoID == "missing" {
		return store.ErrNoVideo
	}
	switch {
	case override == nil:
		f.notes = append(f.notes, "video "+videoID+" left to its verdict")
	case *override:
		f.notes = append(f.notes, "video "+videoID+" shown")
	default:
		f.notes = append(f.notes, "video "+videoID+" kept off")
	}
	return nil
}

func TestAnAdminOverridesAVideosVerdictOrLeavesItBe(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name, video, body string
		wantCode          int
		wantNote          string
		wantDetail        string
	}{
		{"shown", "vid00000001", `{"override": true}`, http.StatusNoContent, "video vid00000001 shown", `{"override":true}`},
		{"kept off", "vid00000001", `{"override": false}`, http.StatusNoContent, "video vid00000001 kept off", `{"override":false}`},
		{"left to its verdict", "vid00000001", `{"override": null}`, http.StatusNoContent, "video vid00000001 left to its verdict", `{"override":null}`},
		{"no such video", "missing", `{"override": true}`, http.StatusNotFound, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/movies/videos/"+tt.video, site, tt.body)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body, tt.wantCode)
			}
			if tt.wantNote != "" && !slices.Contains(f.notes, tt.wantNote) {
				t.Errorf("notes = %q, want %q", f.notes, tt.wantNote)
			}
			wantAudits, wantDetails := []string(nil), []string(nil)
			if tt.wantDetail != "" {
				wantAudits, wantDetails = []string{owner + " movie.video.override " + tt.video}, []string{tt.wantDetail}
			}
			if !slices.Equal(f.audits, wantAudits) || !slices.Equal(f.details, wantDetails) {
				t.Errorf("audits = %q %q, want %q %q", f.audits, f.details, wantAudits, wantDetails)
			}
		})
	}
}

func TestAdminSeesEachVideosVerdictAndOverride(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/movies/channels", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "videos.0", `{"is_tta": false, "tta_reason": "no mention of TTA", "tta_override": true, "shown": true}`)
	wantJSON(t, rec.Body.Bytes(), "channels.0", `{"video_count": 1, "videos_found": 3}`)
}

func TestMoviesFeedAndMarkSeen(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/movies")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/movies = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "videos.0", `{"video_id": "vid00000001", "creator_name": "Alice Casts"}`)
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

	rec = adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/movies/videos/vid00000001", site, `{"override": false}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH video = %d %s, want 204", rec.Code, rec.Body)
	}
}
