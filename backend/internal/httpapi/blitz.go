// The site's blitz tournaments: the shown editions, and an edition's entrants still in, its published rounds
// with their results and who reported them, and its standings; never a Discord name, a ruling's reason or a flag.
package httpapi

import (
	"context"
	"errors"
	"slices"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func blitzEditionRow(r store.BlitzEditionRow) BlitzEditionRow {
	return BlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants, Round: r.Round, Champion: r.Champion,
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

// playedGame is the game a match was played as, nil for none recorded.
func playedGame(m store.BlitzMatch) *BlitzPlayedGame {
	if m.Game == nil {
		return nil
	}
	return &BlitzPlayedGame{ShareCode: m.Game.ShareCode, FinishedAt: m.Game.Finished.UTC(),
		ACulture: m.Game.A.Culture, AOutcome: BlitzOutcome(m.Game.A.Outcome), ATurn: turnOf(m.Game.A),
		BCulture: m.Game.B.Culture, BOutcome: BlitzOutcome(m.Game.B.Outcome), BTurn: turnOf(m.Game.B)}
}

// turnOf is a side's turn, nil when it was not recorded.
func turnOf(s store.BlitzSide) *int {
	if s.Turn == 0 {
		return nil
	}
	return &s.Turn
}

// played is a match of round as the blitz package sees it; B is 0 for a bye, and a result with no game is a
// forfeit.
func played(round int, m store.BlitzMatch) blitz.Played {
	p := blitz.Played{Round: round, A: m.A, Result: blitz.Result(m.Result), Forfeit: m.Result != "" && m.GameID == nil}
	if m.B != nil {
		p.B = *m.B
	}
	return p
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

// swissStandings ranks an edition's entrants by its published Swiss rounds' results and skips, as its points score
// them, and its tiebreak seed. The knockout's results never count: the standings after the last Swiss round are its
// seeds.
func swissStandings(ed store.BlitzEdition) []blitz.Standing {
	var matches []blitz.Played
	published := map[int]bool{}
	for _, r := range ed.Rounds {
		if r.Kind == "knockout" || r.PublishedAt == nil {
			continue
		}
		published[r.Number] = true
		for _, m := range r.Matches {
			matches = append(matches, played(r.Number, m))
		}
	}
	var entrants []blitz.Entrant
	for _, e := range ed.Entrants {
		var skipped []int
		for _, n := range e.Skips {
			if published[n] {
				skipped = append(skipped, n)
			}
		}
		entrants = append(entrants, blitz.Entrant{ID: e.ID, Dropped: e.Status == "dropped", Skipped: skipped})
	}
	return blitz.Standings(entrants, matches, ed.Format.SwissRounds, uint64(ed.TiebreakSeed), ed.Points)
}

// blitzStandings is the edition's standings as the API sends them.
func blitzStandings(ed store.BlitzEdition) []BlitzStanding {
	names, countries := map[int64]string{}, map[int64]string{}
	for _, e := range ed.Entrants {
		names[e.ID], countries[e.ID] = e.Name, e.Country
	}
	standings := swissStandings(ed)
	out := make([]BlitzStanding, 0, len(standings))
	for _, s := range standings {
		status := "active"
		if s.Dropped {
			status = "dropped"
		}
		out = append(out, BlitzStanding{Rank: s.Rank, Name: names[s.ID], Country: nullableText(countries[s.ID]), Points: s.Points, Games: s.Games,
			Byes: s.Byes, Buchholz: s.Buchholz, Status: BlitzStandingStatus(status)})
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
		Points: blitzPoints(ed.Points), FidePoints: ed.Points == blitz.FIDEScoring,
		Entrants: []BlitzEntrant{}, Rounds: make([]BlitzRound, len(ed.Rounds)), Standings: blitzStandings(ed), Champion: row.Champion}
	for _, e := range ed.Entrants {
		if e.Status != "active" {
			continue
		}
		offset, _ := blitz.Offset(e.Zone, s.now())
		entrant := BlitzEntrant{Name: e.Name, Country: nullableText(e.Country), Zone: e.Zone, UtcOffset: offset}
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
		pr := BlitzRound{Number: r.Number, Kind: BlitzRoundKind(r.Kind), Matches: make([]BlitzPublicMatch, len(r.Matches)), Skipped: []string{}}
		for _, e := range skippers(ed.Entrants, r.Number) {
			pr.Skipped = append(pr.Skipped, e.Name)
		}
		if r.PublishedAt != nil {
			pr.PublishedAt = *r.PublishedAt
		}
		for j, m := range r.Matches {
			a := named[m.A]
			pm := BlitzPublicMatch{A: a.Name, ARating: rating(a), Result: blitzResult(m), Game: playedGame(m), Ruled: m.Ruling != "",
				Confirmed: m.Report == "confirmed"}
			if m.ReportedBy != nil {
				pm.ReportedBy = new(named[*m.ReportedBy].Name)
			}
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
