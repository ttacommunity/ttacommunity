// The site's blitz tournaments: the shown editions, and an edition's entrants still in, its published rounds
// with their results and who reported them, and its standings; never a Discord name, a ruling's reason or a flag.
package httpapi

import (
	"context"
	"errors"
	"slices"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/blitzview"
	"ttacommunity/backend/internal/store"
)

func blitzEditionRow(r store.BlitzEditionRow) BlitzEditionRow {
	return BlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants, Round: r.Round, Champion: r.Champion, Knockout: r.Knockout, Bracket: r.Bracket,
		Format: BlitzFormat{SwissRounds: r.Format.SwissRounds, KnockoutSize: r.Format.KnockoutSize}}
}

// blitzPoints is an edition's points as the API sends them.
func blitzPoints(p blitz.Scoring) BlitzPoints {
	return BlitzPoints{Win: p.Win, Draw: p.Draw, Loss: p.Loss, ForfeitWin: p.ForfeitWin, ForfeitLoss: p.ForfeitLoss, Bye: p.Bye, Skip: p.Skip}
}

// blitzResult is a match's result as the API sends it, nil until decided.
func blitzResult(m store.BlitzMatch) *BlitzResult {
	if m.Result == "" {
		return nil
	}
	r := BlitzResult(m.Result)
	return &r
}

// playedGame is a match's first game, nil for none recorded.
func playedGame(m store.BlitzMatch) *BlitzPlayedGame {
	if m.Game == nil {
		return nil
	}
	return new(apiGame(*m.Game))
}

// apiGame is a played game as the API sends it.
func apiGame(g store.BlitzGame) BlitzPlayedGame {
	return BlitzPlayedGame{ShareCode: g.ShareCode, FinishedAt: g.Finished.UTC(),
		ACulture: g.A.Culture, AOutcome: SeatOutcome(g.A.Outcome), ATurn: turnOf(g.A),
		BCulture: g.B.Culture, BOutcome: SeatOutcome(g.B.Outcome), BTurn: turnOf(g.B)}
}

// publicGames are a match's games as the site shows them.
func publicGames(m store.BlitzMatch) []BlitzPublicGame {
	out := make([]BlitzPublicGame, len(m.Games))
	for i, g := range m.Games {
		out[i] = BlitzPublicGame{Number: g.Number, Result: BlitzGameResult(g.Result), Game: apiGame(g.Game)}
	}
	return out
}

// turnOf is a side's turn, nil when it was not recorded.
func turnOf(s store.BlitzSide) *int {
	if s.Turn == 0 {
		return nil
	}
	return &s.Turn
}

// skippers are the entrants who skip round number, in sign-up order.
func skippers(entrants []store.BlitzEntrant, number int) []store.BlitzEntrant {
	var out []store.BlitzEntrant
	for _, e := range entrants {
		if slices.Contains(e.Skips, number) {
			out = append(out, e)
		}
	}
	return out
}

// blitzPairing is an edition's pairing methods as the API sends them.
func blitzPairing(p store.BlitzPairing) BlitzPairingMethods {
	return BlitzPairingMethods{First: BlitzPairingMethod(p.First), Later: BlitzPairingMethod(p.Later), FirstOrder: BlitzPairingOrder(p.FirstOrder), LaterOrder: BlitzPairingOrder(p.LaterOrder)}
}

// roundOrder is the order a round was paired by as the API sends it, nil for a knockout round.
func roundOrder(r store.BlitzRound) *BlitzPairingOrder {
	if r.Order == "" {
		return nil
	}
	o := BlitzPairingOrder(r.Order)
	return &o
}

// ranksFixedByOf is what fixed an edition's starting ranks as the API sends it, nil before round 1.
func ranksFixedByOf[T ~string](fixedBy string) *T {
	if fixedBy == "" {
		return nil
	}
	v := T(fixedBy)
	return &v
}

// blitzStandings is the edition's standings as the API sends them.
func blitzStandings(ed store.BlitzEdition) []BlitzStanding {
	names, countries, starts := map[int64]string{}, map[int64]string{}, map[int64]int{}
	for _, e := range ed.Entrants {
		names[e.ID], countries[e.ID], starts[e.ID] = e.Name, e.Country, e.StartingRank
	}
	standings := blitzview.SwissStandings(ed)
	out := make([]BlitzStanding, 0, len(standings))
	for _, s := range standings {
		status := "active"
		if s.Dropped {
			status = "dropped"
		}
		var start *int
		if starts[s.ID] > 0 {
			start = new(starts[s.ID])
		}
		out = append(out, BlitzStanding{Rank: s.Rank, PlaceFrom: s.PlaceFrom, PlaceTo: s.PlaceTo, Name: names[s.ID], Country: nullableText(countries[s.ID]), StartingRank: start, Points: s.Points, Games: s.Games,
			Wins: s.Wins, Losses: s.Losses, Draws: s.Draws, ForfeitWins: s.ForfeitWins, ForfeitLosses: s.ForfeitLosses, Byes: s.Byes, ForfeitPoints: s.ForfeitPoints, ByePoints: s.ByePoints, BuchholzCut1: s.BuchholzCut1, Buchholz: s.Buchholz, SonnebornBerger: s.SonnebornBerger, OpponentsBuchholz: s.OpponentsBuchholz,
			OpponentsSonnebornBerger: s.OpponentsSonnebornBerger, Status: EntrantStatus(status)})
	}
	return out
}

func (s *server) GetBlitzEditions(ctx context.Context, _ GetBlitzEditionsRequestObject) (GetBlitzEditionsResponseObject, error) {
	rows, err := s.ratings.BlitzEditions(ctx)
	if err != nil {
		return nil, err
	}
	body := BlitzEditions{Editions: make([]BlitzEditionRow, len(rows))}
	for i, r := range rows {
		body.Editions[i] = blitzEditionRow(r)
	}
	return GetBlitzEditions200JSONResponse{Body: body, Headers: GetBlitzEditions200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetBlitzEdition(ctx context.Context, req GetBlitzEditionRequestObject) (GetBlitzEditionResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetBlitzEdition400JSONResponse{badRequestBody("bad parameter: tournament")}, nil
	}
	ed, err := s.ratings.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetBlitzEdition404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	row := blitzEditionRow(ed.BlitzEditionRow)
	body := BlitzEdition{Tournament: row.Tournament, Name: row.Name, Edition: row.Edition, Format: row.Format,
		Points: blitzPoints(ed.Points), FidePoints: ed.Points == blitz.FIDEScoring, Pairing: blitzPairing(ed.Pairing),
		RanksFixedBy: ranksFixedByOf[RanksFixedBy](ed.RanksFixedBy),
		Entrants:     []BlitzEntrant{}, Rounds: make([]BlitzRound, len(ed.Rounds)), Standings: blitzStandings(ed), Champion: row.Champion, Bracket: row.Bracket,
		SignUps: signUpsOf(ed.SignUps)}
	for _, e := range ed.Entrants {
		if e.Status != "active" {
			continue
		}
		offset, _ := blitz.Offset(e.Zone, s.now())
		entrant := BlitzEntrant{Name: e.Name, Country: nullableText(e.Country), Zone: e.Zone, UtcOffset: offset}
		if e.StartingRank > 0 {
			entrant.StartingRank = new(e.StartingRank)
		}
		if r, ok := rated[e.Player]; ok {
			entrant.Rating = &r.Rating
		}
		body.Entrants = append(body.Entrants, entrant)
	}
	// A round may name an entrant dropped since: the names come from every entrant, not the active ones shown.
	named := map[int64]store.BlitzEntrant{}
	for _, e := range ed.Entrants {
		named[e.ID] = e
	}
	rating := func(e store.BlitzEntrant) *float64 {
		if r, ok := rated[e.Player]; ok {
			return &r.Rating
		}
		return nil
	}
	for i, r := range ed.Rounds {
		pr := BlitzRound{Number: r.Number, Kind: RoundKind(r.Kind), Method: roundMethod(r), PairingOrder: roundOrder(r), Matches: make([]BlitzPublicMatch, len(r.Matches)), Skipped: []string{}, BestOf: r.BestOf}
		for _, e := range skippers(ed.Entrants, r.Number) {
			pr.Skipped = append(pr.Skipped, e.Name)
		}
		if r.PublishedAt != nil {
			pr.PublishedAt = *r.PublishedAt
		}
		for j, m := range r.Matches {
			a := named[m.A]
			pm := BlitzPublicMatch{A: a.Name, ARating: rating(a), Result: blitzResult(m), Game: playedGame(m), Ruled: m.Ruling != "",
				BestOf: m.BestOf, Games: publicGames(m)}
			if m.DecidedAt != nil && (body.ResultsUpdatedAt == nil || m.DecidedAt.After(*body.ResultsUpdatedAt)) {
				body.ResultsUpdatedAt = new(m.DecidedAt.UTC())
			}
			pm.ASeed, pm.BSeed = m.SeedA, m.SeedB
			if m.B != nil {
				b := named[*m.B]
				pm.B, pm.BRating = &b.Name, rating(b)
			}
			pr.Matches[j] = pm
		}
		body.Rounds[i] = pr
	}
	return GetBlitzEdition200JSONResponse{Body: body, Headers: GetBlitzEdition200ResponseHeaders{CacheControl: cacheNone}}, nil
}
