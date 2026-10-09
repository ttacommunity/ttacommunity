// Admin's tournaments: the list, one tournament, and changes to a tournament or one of its seasons.
package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"ttacommunity/backend/internal/store"
)

// refused are the store's answers to a change it will not make, each a request the admin can correct.
var refused = []error{store.ErrBadScoring, store.ErrBadLevelName, store.ErrBadPlayerCounts, store.ErrBadKind, store.ErrGroupCycle,
	store.ErrBadGroup, store.ErrNotAGroup, store.ErrBadPenalty, store.ErrBadPodium, store.ErrBadCard, store.ErrBadStory,
	store.ErrSeasonHasGames}

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
			SuggestedFrom: AdminSeasonSuggestedFrom(se.SuggestedFrom), Games: se.Games, Players: se.Players}
	}
	out := AdminTournament{Slug: t.Slug, Name: t.Name, OriginalName: t.OriginalName, Parent: t.Parent, Description: t.Description,
		IsGroup: t.IsGroup, Games: t.Games, FirstGame: dateOf(t.FirstGame), LastGame: dateOf(t.LastGame), Kind: TournamentKind(t.Kind),
		TopDivisions: append([]string{}, t.TopDivisions...), Visible: t.Visible, Rated: t.Rated, Community: t.Community,
		Points2p: append(PlacePoints{}, t.Points[2]...), Points3p: append(PlacePoints{}, t.Points[3]...),
		Points4p: append(PlacePoints{}, t.Points[4]...), SeasonName: t.SeasonName, StageName: t.StageName, Card: tournamentCard(t.Card),
		Stories: storyList(t.Stories), RatedPlayerCounts: append([]int{}, t.RatedPlayerCounts...), Regulations: t.Regulations,
		CgeRegulations: t.CGERegulations, Seasons: seasons}
	return GetAdminTournament200JSONResponse{Body: out, Headers: GetAdminTournament200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// tournamentSettings returns the store's settings from a patch, and whether it asks for any of them.
func tournamentSettings(b *TournamentSettings) (store.TournamentSettings, bool) {
	settings := store.TournamentSettings{Visible: b.Visible, Rated: b.Rated, Community: b.Community, SeasonName: b.SeasonName,
		StageName: b.StageName, Parent: b.Parent, Description: b.Description, Name: b.Name}
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
	asked := b.Visible != nil || b.Rated != nil || b.Community != nil || b.RatedPlayerCounts != nil || b.Kind != nil ||
		b.TopDivisions != nil || settings.Points != nil || b.SeasonName != nil || b.StageName != nil || b.Parent != nil ||
		b.Description != nil || b.Name != nil
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
	var steps []func() error
	if anySetting {
		steps = append(steps, func() error { return s.admins.SetTournament(ctx, req.Tournament, settings) })
	}
	if c := b.Card; c != nil {
		steps = append(steps, func() error {
			return s.admins.SetCard(ctx, req.Tournament, store.Card{Icon: c.Icon, Tagline: c.Tagline, Summary: c.Summary, Subtitle: c.Subtitle, Cadence: c.Cadence})
		})
	}
	if b.Regulations != nil {
		steps = append(steps, func() error { return s.admins.SetRegulations(ctx, req.Tournament, *b.Regulations) })
	}
	if b.Stories != nil {
		list := make([]store.Story, len(*b.Stories))
		for i, st := range *b.Stories {
			list[i] = store.Story{Player: st.Player, Badge: st.Badge, Title: st.Title, Text: st.Text}
		}
		steps = append(steps, func() error { return s.admins.SetStories(ctx, req.Tournament, list) })
	}
	if b.Members != nil {
		steps = append(steps, func() error { return s.admins.SetMembers(ctx, req.Tournament, *b.Members) })
	}
	for _, step := range steps {
		err := step()
		switch code, msg := refusal(err); code {
		case http.StatusNotFound:
			return PatchAdminTournament404JSONResponse{notFound404(msg)}, nil
		case http.StatusBadRequest:
			return PatchAdminTournament400JSONResponse{badRequest400(msg)}, nil
		}
		if err != nil {
			return nil, err
		}
	}
	if err := s.admins.Audit(ctx, a.DiscordID, "tournament.settings", req.Tournament, b); err != nil {
		return nil, err
	}
	return PatchAdminTournament204Response{}, nil
}

// seasonSteps returns the store calls a season change makes, the podium first as it is the likeliest to be refused,
// or a message when it is not one the API takes.
func (s *server) seasonSteps(ctx context.Context, slug, season string, b *SeasonChange) ([]func() error, string) {
	var steps []func() error
	switch {
	case len(season) > 64:
		return nil, "a season's label is at most 64 characters"
	case b.Podium != nil && b.ClearPodium != nil && *b.ClearPodium:
		return nil, "set a podium or clear it, not both"
	case b.Podium != nil && (len(b.Podium.Podium) == 0 || len(b.Podium.Podium) > 3 || len(b.Podium.Note) > 500):
		return nil, "a podium has one to three places and a note of at most 500 characters"
	case b.Note != nil && len(*b.Note) > 2000:
		return nil, "a note is at most 2,000 characters"
	}
	if p := b.Podium; p != nil {
		podium := make([]store.PodiumEntry, len(p.Podium))
		for i, e := range p.Podium {
			if len(e.Name) > 64 {
				return nil, "a player's name is at most 64 characters"
			}
			podium[i] = store.PodiumEntry{Place: e.Place, Name: e.Name}
		}
		steps = append(steps, func() error { return s.admins.SetPodium(ctx, slug, season, podium, strings.TrimSpace(p.Note)) })
	}
	if b.ClearPodium != nil && *b.ClearPodium {
		steps = append(steps, func() error { return s.admins.ClearPodium(ctx, slug, season) })
	}
	if b.Rated != nil {
		steps = append(steps, func() error { return s.admins.SetSeasonRated(ctx, slug, season, *b.Rated) })
	}
	if b.InChampions != nil {
		steps = append(steps, func() error { return s.admins.SetSeasonInChampions(ctx, slug, season, *b.InChampions) })
	}
	if b.InCommunity != nil {
		steps = append(steps, func() error { return s.admins.SetSeasonInCommunity(ctx, slug, season, *b.InCommunity) })
	}
	if b.TimeoutPenalty != nil {
		steps = append(steps, func() error { return s.admins.SetSeasonPenalty(ctx, slug, season, *b.TimeoutPenalty) })
	}
	if b.Note != nil {
		steps = append(steps, func() error { return s.admins.SetNote(ctx, slug, season, "", *b.Note) })
	}
	if b.StageNotes != nil {
		for stage, note := range *b.StageNotes {
			if stage == "" || len(stage) > 64 || len(note) > 2000 {
				return nil, "a stage's label is 1 to 64 characters and its note at most 2,000"
			}
			steps = append(steps, func() error { return s.admins.SetNote(ctx, slug, season, stage, note) })
		}
	}
	if len(steps) == 0 {
		return nil, "nothing to change"
	}
	return steps, ""
}

func (s *server) PatchAdminSeason(ctx context.Context, req PatchAdminSeasonRequestObject) (PatchAdminSeasonResponseObject, error) {
	a, ok := adminFrom(ctx)
	if !ok {
		return nil, errors.New("admin route reached without the gate")
	}
	if req.Body == nil {
		return PatchAdminSeason400JSONResponse{badRequest400("nothing to change")}, nil
	}
	steps, msg := s.seasonSteps(ctx, req.Tournament, req.Season, req.Body)
	if msg != "" {
		return PatchAdminSeason400JSONResponse{badRequest400(msg)}, nil
	}
	for _, step := range steps {
		err := step()
		switch code, msg := refusal(err); code {
		case http.StatusNotFound:
			return PatchAdminSeason404JSONResponse{notFound404(msg)}, nil
		case http.StatusBadRequest:
			return PatchAdminSeason400JSONResponse{badRequest400(msg)}, nil
		}
		if err != nil {
			return nil, err
		}
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
