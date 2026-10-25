// Admin's blitz rounds for Discord: each round's deadline, and a round's games as text an organiser pastes into
// Discord so everyone playing is notified.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// roundGames lists round r's games with two players, in the order the site numbers them, as Discord text names
// their players.
func roundGames(ed store.BlitzEdition, r store.BlitzRound) [][2]blitz.Person {
	people := map[int64]blitz.Person{}
	for _, e := range ed.Entrants {
		people[e.ID] = blitz.Person{Name: e.Name, Discord: e.DiscordName}
	}
	var games [][2]blitz.Person
	for _, m := range r.Matches {
		if m.B != nil {
			games = append(games, [2]blitz.Person{people[m.A], people[*m.B]})
		}
	}
	return games
}

func (s *server) GetAdminBlitzRoundDiscord(ctx context.Context, req GetAdminBlitzRoundDiscordRequestObject) (GetAdminBlitzRoundDiscordResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetAdminBlitzRoundDiscord404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetAdminBlitzRoundDiscord404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(ed.Rounds, func(r store.BlitzRound) bool { return r.ID == req.Round })
	if i < 0 {
		return GetAdminBlitzRoundDiscord404JSONResponse{notFound404("no such round")}, nil
	}
	games := roundGames(ed, ed.Rounds[i])
	if len(games) == 0 {
		return GetAdminBlitzRoundDiscord409JSONResponse{conflict409("the round has no game to write")}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		return tx.Audit(ctx, me.DiscordID, "blitz.round.discord", fmt.Sprint(req.Round), nil)
	})
	if err != nil {
		return nil, err
	}
	return GetAdminBlitzRoundDiscord200JSONResponse{Body: BlitzDiscordText{Text: blitz.DiscordText(games)},
		Headers: GetAdminBlitzRoundDiscord200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminBlitzRoundDeadline(ctx context.Context, req PutAdminBlitzRoundDeadlineRequestObject) (PutAdminBlitzRoundDeadlineResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzRoundDeadline404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzRoundDeadline400JSONResponse{badRequest400("the deadline: day, time and zone")}, nil
	}
	zone := strings.TrimSpace(req.Body.Zone)
	at, err := blitz.FinishedAt(req.Body.Day.Format(time.DateOnly), req.Body.Time, zone)
	if err != nil {
		return PutAdminBlitzRoundDeadline400JSONResponse{badRequest400(err.Error())}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzRoundDeadline(ctx, req.Tournament, req.Edition, req.Round, at.UTC(), zone); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.deadline", fmt.Sprint(req.Round), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzRoundDeadline404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzRoundDeadline404JSONResponse{notFound404("no such round")}, nil
	case errors.Is(err, store.ErrBlitzRoundFinished):
		return PutAdminBlitzRoundDeadline409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzRoundDeadline204Response{}, nil
}
