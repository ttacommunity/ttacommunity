// Admin's blitz rounds: pairing the next round as a draft, changing its matches by hand, publishing and deleting.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// metBefore lists, for each entrant, who they met in any round but the one given (0 for none).
func metBefore(rounds []store.BlitzRound, except int64) map[int64][]int64 {
	met := map[int64][]int64{}
	for _, r := range rounds {
		if r.ID == except {
			continue
		}
		for _, m := range r.Matches {
			if m.B != nil {
				met[m.A] = append(met[m.A], *m.B)
				met[*m.B] = append(met[*m.B], m.A)
			}
		}
	}
	return met
}

// hadBye lists who has had a bye in any round.
func hadBye(rounds []store.BlitzRound) map[int64]bool {
	byes := map[int64]bool{}
	for _, r := range rounds {
		for _, m := range r.Matches {
			if m.B == nil {
				byes[m.A] = true
			}
		}
	}
	return byes
}

// adminBlitzRound shows a round with each match's gaps as the entrants are now, and whether a pair has met in
// another round.
func (s *server) adminBlitzRound(r store.BlitzRound, ed store.BlitzEdition, rated map[string]store.Rated, now time.Time) AdminBlitzRound {
	byID := map[int64]store.BlitzEntrant{}
	for _, e := range ed.Entrants {
		byID[e.ID] = e
	}
	met := metBefore(ed.Rounds, r.ID)
	out := AdminBlitzRound{Id: r.ID, Number: r.Number, Kind: AdminBlitzRoundKind(r.Kind), Timezone: r.Timezone, Rating: r.Rating,
		PublishedAt: r.PublishedAt, Matches: make([]AdminBlitzMatch, len(r.Matches))}
	for i, m := range r.Matches {
		am := AdminBlitzMatch{Id: m.ID, A: m.A, B: m.B, Result: blitzResult(m), Game: playedGame(m), Ruling: m.Ruling, DecidedAt: m.DecidedAt}
		if m.B != nil {
			a, b := byID[m.A], byID[*m.B]
			oa, _ := blitz.OffsetMinutes(a.Zone, now)
			ob, _ := blitz.OffsetMinutes(b.Zone, now)
			am.ZoneGapMinutes = blitz.ZoneGap(oa, ob)
			if ra, ok := rated[a.Player]; ok {
				if rb, ok := rated[b.Player]; ok {
					gap := ra.Rating - rb.Rating
					if gap < 0 {
						gap = -gap
					}
					am.RatingGap = &gap
				}
			}
			for _, o := range met[m.A] {
				if o == *m.B {
					am.Repeat = true
				}
			}
		}
		out.Matches[i] = am
	}
	return out
}

// pairingPlayers turns the edition's active entrants into what the pairing needs, as of now, leaving out the
// round named by except (0 for none: a new round) when counting points, places, opponents and byes, and the
// entrants in busy.
func pairingPlayers(ed store.BlitzEdition, rated map[string]store.Rated, now time.Time, except int64, busy map[int64]bool) []blitz.Player {
	var others []store.BlitzRound
	for _, r := range ed.Rounds {
		if r.ID != except {
			others = append(others, r)
		}
	}
	met, byes := metBefore(others, 0), hadBye(others)
	points, places := map[string]float64{}, map[string]int{}
	for _, s := range blitzStandings(store.BlitzEdition{BlitzEditionRow: ed.BlitzEditionRow, Entrants: ed.Entrants, Rounds: others}) {
		points[s.Name], places[s.Name] = s.Points, s.Rank
	}
	var players []blitz.Player
	for _, e := range ed.Entrants {
		if e.Status != "active" || busy[e.ID] {
			continue
		}
		offset, _ := blitz.OffsetMinutes(e.Zone, now)
		p := blitz.Player{ID: e.ID, Points: points[e.Name], Place: places[e.Name], Offset: offset, Opponents: met[e.ID], HadBye: byes[e.ID]}
		if r, ok := rated[e.Player]; ok {
			p.Rating, p.Rated = r.Rating, true
		}
		players = append(players, p)
	}
	return players
}

func (s *server) PostAdminBlitzRound(ctx context.Context, req PostAdminBlitzRoundRequestObject) (PostAdminBlitzRoundResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRound400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	criteria := blitz.Criteria{Timezone: true}
	if req.Body != nil && req.Body.Timezone != nil {
		criteria.Timezone = *req.Body.Timezone
	}
	if req.Body != nil && req.Body.Rating != nil {
		criteria.Rating = *req.Body.Rating
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return PostAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	now := s.now()
	seed := uint64(now.UnixNano())
	paired, err := blitz.Pair(pairingPlayers(ed, rated, now, 0, nil), criteria, seed)
	if errors.Is(err, blitz.ErrTooFew) || errors.Is(err, blitz.ErrNoPairing) {
		return PostAdminBlitzRound400JSONResponse{badRequest400(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	round := store.BlitzRound{Timezone: criteria.Timezone, Rating: criteria.Rating, Seed: int64(seed)}
	for _, m := range paired.Matches {
		b := m.B
		round.Matches = append(round.Matches, store.BlitzMatch{A: m.A, B: &b})
	}
	if paired.Bye != nil {
		round.Matches = append(round.Matches, store.BlitzMatch{A: *paired.Bye})
	}
	made, err := s.admins.AddBlitzRound(ctx, req.Tournament, req.Edition, round)
	switch {
	case errors.Is(err, store.ErrBlitzDraftExists), errors.Is(err, store.ErrBlitzRoundUndecided):
		return PostAdminBlitzRound409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrBadBlitzMatch), errors.Is(err, store.ErrBlitzRepeat):
		return PostAdminBlitzRound400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.add", blitzEditionKey(req.Tournament, req.Edition), map[string]any{"round": made.Number, "timezone": criteria.Timezone, "rating": criteria.Rating, "seed": seed}); err != nil {
		return nil, err
	}
	ed.Rounds = append(ed.Rounds, made)
	return PostAdminBlitzRound201JSONResponse{Body: s.adminBlitzRound(made, ed, rated, now), Headers: PostAdminBlitzRound201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminBlitzRoundMatches(ctx context.Context, req PutAdminBlitzRoundMatchesRequestObject) (PutAdminBlitzRoundMatchesResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400("the matches")}, nil
	}
	matches := make([]store.BlitzMatch, len(req.Body.Matches))
	for i, m := range req.Body.Matches {
		matches[i] = store.BlitzMatch{A: m.A, B: m.B}
	}
	switch err := s.admins.SetBlitzRoundMatches(ctx, req.Tournament, req.Edition, req.Round, matches); {
	case errors.Is(err, store.ErrBadBlitzMatch), errors.Is(err, store.ErrBlitzRepeat), errors.Is(err, store.ErrBlitzMatchDecided):
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzRoundMatches404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzRoundMatches404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.matches", fmt.Sprint(req.Round), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBlitzRoundMatches204Response{}, nil
}

func (s *server) PostAdminBlitzRoundPublish(ctx context.Context, req PostAdminBlitzRoundPublishRequestObject) (PostAdminBlitzRoundPublishResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	switch err := s.admins.PublishBlitzRound(ctx, req.Tournament, req.Edition, req.Round); {
	case errors.Is(err, store.ErrBlitzRoundIncomplete), errors.Is(err, store.ErrBlitzRepeat), errors.Is(err, store.ErrBadBlitzMatch):
		return PostAdminBlitzRoundPublish409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.publish", fmt.Sprint(req.Round), nil); err != nil {
		return nil, err
	}
	return PostAdminBlitzRoundPublish204Response{}, nil
}

func (s *server) DeleteAdminBlitzRound(ctx context.Context, req DeleteAdminBlitzRoundRequestObject) (DeleteAdminBlitzRoundResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	switch err := s.admins.DeleteBlitzRound(ctx, req.Tournament, req.Edition, req.Round); {
	case errors.Is(err, store.ErrBlitzRoundNotLast), errors.Is(err, store.ErrBlitzRoundPlayed):
		return DeleteAdminBlitzRound409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.remove", fmt.Sprint(req.Round), nil); err != nil {
		return nil, err
	}
	return DeleteAdminBlitzRound204Response{}, nil
}

// PostAdminBlitzRoundPairing proposes a fresh pairing of a round: its decided matches kept, everyone else active
// paired again as for a new round. Nothing is written and nothing audited.
func (s *server) PostAdminBlitzRoundPairing(ctx context.Context, req PostAdminBlitzRoundPairingRequestObject) (PostAdminBlitzRoundPairingResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRoundPairing400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	criteria := blitz.Criteria{Timezone: true}
	if req.Body != nil && req.Body.Timezone != nil {
		criteria.Timezone = *req.Body.Timezone
	}
	if req.Body != nil && req.Body.Rating != nil {
		criteria.Rating = *req.Body.Rating
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return PostAdminBlitzRoundPairing404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	var round *store.BlitzRound
	for i := range ed.Rounds {
		if ed.Rounds[i].ID == req.Round {
			round = &ed.Rounds[i]
		}
	}
	if round == nil {
		return PostAdminBlitzRoundPairing404JSONResponse{notFound404("no such round")}, nil
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	proposal := store.BlitzRound{ID: round.ID, Number: round.Number, Kind: round.Kind, Timezone: criteria.Timezone, Rating: criteria.Rating, PublishedAt: round.PublishedAt}
	busy := map[int64]bool{}
	for _, m := range round.Matches {
		if m.Result != "" {
			proposal.Matches = append(proposal.Matches, m)
			busy[m.A] = true
			if m.B != nil {
				busy[*m.B] = true
			}
		}
	}
	now := s.now()
	seed := uint64(now.UnixNano())
	proposal.Seed = int64(seed)
	paired, err := blitz.Pair(pairingPlayers(ed, rated, now, round.ID, busy), criteria, seed)
	if errors.Is(err, blitz.ErrTooFew) || errors.Is(err, blitz.ErrNoPairing) {
		return PostAdminBlitzRoundPairing400JSONResponse{badRequest400(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, m := range paired.Matches {
		b := m.B
		proposal.Matches = append(proposal.Matches, store.BlitzMatch{A: m.A, B: &b})
	}
	if paired.Bye != nil {
		proposal.Matches = append(proposal.Matches, store.BlitzMatch{A: *paired.Bye})
	}
	return PostAdminBlitzRoundPairing200JSONResponse{Body: s.adminBlitzRound(proposal, ed, rated, now), Headers: PostAdminBlitzRoundPairing200ResponseHeaders{CacheControl: cacheNone}}, nil
}
