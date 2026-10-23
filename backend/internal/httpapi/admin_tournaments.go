// Admin's tournaments: the list, one tournament, and changes to a tournament or one of its seasons.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/store"
)

// refused are the store's answers to a change it will not make, each a request the admin can correct.
var refused = []error{store.ErrBadScoring, store.ErrBadLevelName, store.ErrBadPlayerCounts, store.ErrBadKind, store.ErrBadPodiumSource, store.ErrGroupCycle,
	store.ErrBadGroup, store.ErrNotAGroup, store.ErrBadPenalty, store.ErrBadPodium, store.ErrBadCard, store.ErrBadStory,
	store.ErrSeasonHasGames, store.ErrSeasonExists}

// refusal returns the status and message for a store error a change can meet: 400 for a refused change, 404 for no
// such tournament or season, or 0 for an error that is not the admin's to correct.
func refusal(err error) (int, string) {
	if errors.Is(err, store.ErrNoTournament) {
		return http.StatusNotFound, "no such tournament or season"
	}
	for _, r := range refused {
		if errors.Is(err, r) {
			return http.StatusBadRequest, r.Error()
		}
	}
	return 0, ""
}

func (s *server) GetAdminTournaments(ctx context.Context, _ GetAdminTournamentsRequestObject) (GetAdminTournamentsResponseObject, error) {
	rows, err := s.admins.TournamentRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminTournamentRow, len(rows))
	for i, r := range rows {
		out[i] = AdminTournamentRow{Slug: r.Slug, Name: r.Name, Kind: TournamentKind(r.Kind), Parent: r.Parent, IsGroup: r.IsGroup,
			Description: r.Description, CgeAbout: r.CGEAbout, Visible: r.Visible, Rated: r.Rated, Community: r.Community,
			Games: r.Games, FirstGame: dateOf(r.FirstGame), LastGame: dateOf(r.LastGame)}
	}
	return GetAdminTournaments200JSONResponse{Body: out, Headers: GetAdminTournaments200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminTournament(ctx context.Context, req GetAdminTournamentRequestObject) (GetAdminTournamentResponseObject, error) {
	t, err := s.admins.Tournament(ctx, req.Tournament)
	if errors.Is(err, store.ErrNoTournament) {
		return GetAdminTournament404JSONResponse{notFound404("no such tournament")}, nil
	}
	if err != nil {
		return nil, err
	}
	seasons := make([]AdminSeason, len(t.Seasons))
	for j, se := range t.Seasons {
		stages := make([]AdminStage, len(se.Stages))
		for k, st := range se.Stages {
			stages[k] = AdminStage{Label: st.Label, OriginalName: st.OriginalName, Note: st.Note, Number: st.Number, Games: st.Games,
				Finished: st.Finished, Players: st.Players}
		}
		seasons[j] = AdminSeason{Label: se.Label, OriginalName: se.OriginalName, Note: se.Note, HasGames: se.HasGames, Rated: se.Rated,
			InChampions: se.InChampions, InCommunity: se.InCommunity, TimeoutPenalty: se.TimeoutPenalty, Stages: stages,
			Podium: podiumEntries(se.Podium), PodiumNote: se.PodiumNote, SuggestedPodium: podiumEntries(se.Suggested),
			SuggestedFrom: AdminSeasonSuggestedFrom(se.SuggestedFrom), Games: se.Games, Finished: se.Finished, Players: se.Players}
	}
	var ps *AdminTournamentPodiumSource
	if t.PodiumSource != "" {
		s := AdminTournamentPodiumSource(t.PodiumSource)
		ps = &s
	}
	out := AdminTournament{Slug: t.Slug, Name: t.Name, OriginalName: t.OriginalName, Parent: t.Parent, Description: t.Description,
		IsGroup: t.IsGroup, Games: t.Games, FirstGame: dateOf(t.FirstGame), LastGame: dateOf(t.LastGame), Kind: TournamentKind(t.Kind),
		PodiumSource: ps,
		TopDivisions: append([]string{}, t.TopDivisions...), Visible: t.Visible, Rated: t.Rated, Community: t.Community, Blitz: t.Blitz,
		Points2p: append(PlacePoints{}, t.Points[2]...), Points3p: append(PlacePoints{}, t.Points[3]...),
		Points4p: append(PlacePoints{}, t.Points[4]...), SeasonName: t.SeasonName, StageName: t.StageName, Card: tournamentCard(t.Card),
		Stories: storyList(t.Stories), RatedPlayerCounts: append([]int{}, t.RatedPlayerCounts...), Regulations: t.Regulations,
		CgeRegulations: t.CGERegulations, Seasons: seasons}
	return GetAdminTournament200JSONResponse{Body: out, Headers: GetAdminTournament200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// tournamentSettings returns the store's settings from a patch, and whether it asks for any of them.
func tournamentSettings(b *TournamentSettings) (store.TournamentSettings, bool) {
	settings := store.TournamentSettings{Visible: b.Visible, Rated: b.Rated, Community: b.Community, Blitz: b.Blitz,
		SeasonName: b.SeasonName, StageName: b.StageName, Parent: b.Parent, Description: b.Description, Name: b.Name}
	if b.PodiumSource != nil {
		s := string(*b.PodiumSource)
		settings.PodiumSource = &s
	}
	if b.RatedPlayerCounts != nil {
		settings.RatedPlayerCounts = *b.RatedPlayerCounts
	}
	if b.Kind != nil {
		settings.Kind = string(*b.Kind)
	}
	if b.TopDivisions != nil {
		settings.TopDivisions = *b.TopDivisions
	}
	for players, points := range map[int]*PlacePoints{2: b.Points2p, 3: b.Points3p, 4: b.Points4p} {
		if points != nil {
			if settings.Points == nil {
				settings.Points = map[int][]float64{}
			}
			settings.Points[players] = append([]float64{}, *points...)
		}
	}
	asked := b.Visible != nil || b.Rated != nil || b.Community != nil || b.Blitz != nil || b.RatedPlayerCounts != nil || b.Kind != nil ||
		b.PodiumSource != nil || b.TopDivisions != nil || settings.Points != nil || b.SeasonName != nil || b.StageName != nil ||
		b.Parent != nil || b.Description != nil || b.Name != nil
	return settings, asked
}

func (s *server) PatchAdminTournament(ctx context.Context, req PatchAdminTournamentRequestObject) (PatchAdminTournamentResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	b := req.Body
	if b == nil {
		return PatchAdminTournament400JSONResponse{badRequest400("nothing to change")}, nil
	}
	settings, anySetting := tournamentSettings(b)
	if !anySetting && b.Card == nil && b.Regulations == nil && b.Stories == nil && b.Members == nil {
		return PatchAdminTournament400JSONResponse{badRequest400("nothing to change")}, nil
	}
	if b.Regulations != nil && len(*b.Regulations) > 10000 {
		return PatchAdminTournament400JSONResponse{badRequest400("regulations are at most 10,000 characters")}, nil
	}
	if b.Stories != nil && len(*b.Stories) > 12 {
		return PatchAdminTournament400JSONResponse{badRequest400("at most 12 stories")}, nil
	}
	change := store.TournamentChange{Regulations: b.Regulations, Members: b.Members}
	if anySetting {
		change.Settings = &settings
	}
	if c := b.Card; c != nil {
		change.Card = &store.Card{Icon: c.Icon, Tagline: c.Tagline, Summary: c.Summary, Subtitle: c.Subtitle, Cadence: c.Cadence}
	}
	if b.Stories != nil {
		list := make([]store.Story, len(*b.Stories))
		for i, st := range *b.Stories {
			list[i] = store.Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
		}
		change.Stories = &list
	}
	err := s.admins.ChangeTournament(ctx, req.Tournament, change)
	switch code, msg := refusal(err); code {
	case http.StatusNotFound:
		return PatchAdminTournament404JSONResponse{notFound404(msg)}, nil
	case http.StatusBadRequest:
		return PatchAdminTournament400JSONResponse{badRequest400(msg)}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.settings", req.Tournament, b); err != nil {
		return nil, err
	}
	return PatchAdminTournament204Response{}, nil
}

// seasonChange returns the store change a season change asks for, or a message when it is not one the API takes.
func seasonChange(season string, b *SeasonChange) (store.SeasonChange, string) {
	switch {
	case len(season) > 64:
		return store.SeasonChange{}, "a season's label is at most 64 characters"
	case b.Podium != nil && b.ClearPodium != nil && *b.ClearPodium:
		return store.SeasonChange{}, "set a podium or clear it, not both"
	case b.Podium != nil && (len(b.Podium.Podium) == 0 || len(b.Podium.Podium) > 3 || len(b.Podium.Note) > 500):
		return store.SeasonChange{}, "a podium has one to three places and a note of at most 500 characters"
	case b.Note != nil && len(*b.Note) > 2000:
		return store.SeasonChange{}, "a note is at most 2,000 characters"
	}
	c := store.SeasonChange{Rated: b.Rated, InChampions: b.InChampions, InCommunity: b.InCommunity,
		TimeoutPenalty: b.TimeoutPenalty, Note: b.Note, ClearPodium: b.ClearPodium != nil && *b.ClearPodium}
	if p := b.Podium; p != nil {
		podium := make([]store.PodiumEntry, len(p.Podium))
		for i, e := range p.Podium {
			if len(e.Name) > 64 {
				return store.SeasonChange{}, "a player's name is at most 64 characters"
			}
			podium[i] = store.PodiumEntry{Place: e.Place, Name: e.Name}
		}
		c.Podium, c.PodiumNote = &podium, strings.TrimSpace(p.Note)
	}
	if b.StageNotes != nil {
		for stage, note := range *b.StageNotes {
			if stage == "" || len(stage) > 64 || len(note) > 2000 {
				return store.SeasonChange{}, "a stage's label is 1 to 64 characters and its note at most 2,000"
			}
		}
		c.StageNotes = *b.StageNotes
	}
	if b.MoveToTournament != nil && strings.TrimSpace(*b.MoveToTournament) != "" {
		m := strings.TrimSpace(*b.MoveToTournament)
		c.MoveToTournament = &m
		if b.NewSeason != nil && strings.TrimSpace(*b.NewSeason) != "" {
			ns := strings.TrimSpace(*b.NewSeason)
			c.NewSeason = &ns
		}
	}
	if c.Podium == nil && !c.ClearPodium && c.Rated == nil && c.InChampions == nil && c.InCommunity == nil &&
		c.TimeoutPenalty == nil && c.Note == nil && len(c.StageNotes) == 0 && c.MoveToTournament == nil {
		return store.SeasonChange{}, "nothing to change"
	}
	return c, ""
}

func (s *server) PatchAdminSeason(ctx context.Context, req PatchAdminSeasonRequestObject) (PatchAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if req.Body == nil {
		return PatchAdminSeason400JSONResponse{badRequest400("nothing to change")}, nil
	}
	change, msg := seasonChange(req.Season, req.Body)
	if msg != "" {
		return PatchAdminSeason400JSONResponse{badRequest400(msg)}, nil
	}
	err := s.admins.ChangeSeason(ctx, req.Tournament, req.Season, change)
	switch code, msg := refusal(err); code {
	case http.StatusNotFound:
		return PatchAdminSeason404JSONResponse{notFound404(msg)}, nil
	case http.StatusBadRequest:
		return PatchAdminSeason400JSONResponse{badRequest400(msg)}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season", req.Tournament, map[string]any{"season": req.Season, "change": req.Body}); err != nil {
		return nil, err
	}
	return PatchAdminSeason204Response{}, nil
}

func (s *server) DeleteAdminSeason(ctx context.Context, req DeleteAdminSeasonRequestObject) (DeleteAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	switch err := s.admins.RemoveSeason(ctx, req.Tournament, req.Season); {
	case errors.Is(err, store.ErrNoTournament):
		return DeleteAdminSeason404JSONResponse{notFound404("no such season")}, nil
	case errors.Is(err, store.ErrSeasonHasGames):
		return DeleteAdminSeason400JSONResponse{badRequest400("the season has games, so it stays")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.remove", req.Tournament, map[string]string{"label": req.Season}); err != nil {
		return nil, err
	}
	return DeleteAdminSeason204Response{}, nil
}

func (s *server) PostAdminSeason(ctx context.Context, req PostAdminSeasonRequestObject) (PostAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if req.Body == nil || strings.TrimSpace(req.Body.Label) == "" || len(req.Body.Label) > 64 {
		return PostAdminSeason400JSONResponse{badRequest400("a season needs a label")}, nil
	}
	label := strings.TrimSpace(req.Body.Label)
	switch err := s.admins.AddSeason(ctx, req.Tournament, label); {
	case errors.Is(err, store.ErrNoTournament):
		return PostAdminSeason404JSONResponse{notFound404("no such tournament")}, nil
	case errors.Is(err, store.ErrSeasonExists):
		return PostAdminSeason400JSONResponse{badRequest400("the tournament already has that season")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.season.add", req.Tournament, map[string]string{"label": label}); err != nil {
		return nil, err
	}
	return PostAdminSeason204Response{}, nil
}

// tournamentCard turns a tournament's card into the API's.
func tournamentCard(c store.Card) TournamentCard {
	return TournamentCard{Icon: c.Icon, Tagline: c.Tagline, Summary: c.Summary, Subtitle: c.Subtitle, Cadence: c.Cadence}
}

// storyList turns a tournament's stories into the API's.
func storyList(list []store.Story) []Story {
	out := make([]Story, len(list))
	for i, st := range list {
		out[i] = Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
	}
	return out
}

// dateOf returns t as an API date, or nil for none.
func dateOf(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

// podiumEntries returns a podium as the API's, empty for none.
func podiumEntries(p []store.PodiumEntry) []PodiumEntry {
	out := make([]PodiumEntry, len(p))
	for i, e := range p {
		out[i] = PodiumEntry{Place: e.Place, Name: e.Name}
	}
	return out
}
