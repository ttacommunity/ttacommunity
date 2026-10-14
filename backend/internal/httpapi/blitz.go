// The site's blitz tournaments: the shown editions, and an edition's entrants still in, its published rounds
// with their results, and its standings; never a Discord name or a ruling's reason.
package httpapi

import (
	"context"
	"errors"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func blitzEditionRow(r store.BlitzEditionRow) BlitzEditionRow {
	return BlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants, Round: r.Round, Champion: r.Champion,
		Format: BlitzFormat{SwissRounds: r.Format.SwissRounds, KnockoutSize: r.Format.KnockoutSize}}
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
		ACulture: m.Game.A.Culture, AOutcome: BlitzOutcome(m.Game.A.Outcome),
		BCulture: m.Game.B.Culture, BOutcome: BlitzOutcome(m.Game.B.Outcome)}
}

// played is a match as the blitz package sees it; B is 0 for a bye.
func played(m store.BlitzMatch) blitz.Played {
	p := blitz.Played{A: m.A, Result: blitz.Result(m.Result)}
	if m.B != nil {
		p.B = *m.B
	}
	return p
}

// swissStandings ranks an edition's entrants, pending ones aside, by its Swiss rounds' results and its tiebreak
// seed. The knockout's results never count: the standings after the last Swiss round are its seeds.
func swissStandings(ed store.BlitzEdition) []blitz.Standing {
	var entrants []blitz.Entrant
	for _, e := range ed.Entrants {
		if e.Status != "pending" {
			entrants = append(entrants, blitz.Entrant{ID: e.ID, Dropped: e.Status == "dropped"})
		}
	}
	var matches []blitz.Played
	for _, r := range ed.Rounds {
		if r.Kind == "knockout" {
			continue
		}
		for _, m := range r.Matches {
			matches = append(matches, played(m))
		}
	}
	return blitz.Standings(entrants, matches, uint64(ed.TiebreakSeed))
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
			Byes: s.Byes, OpponentsPoints: s.OpponentsPoints, Status: BlitzStandingStatus(status)})
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
		pr := BlitzRound{Number: r.Number, Kind: BlitzRoundKind(r.Kind), Matches: make([]BlitzPublicMatch, len(r.Matches))}
		if r.PublishedAt != nil {
			pr.PublishedAt = *r.PublishedAt
		}
		for j, m := range r.Matches {
			a := named[m.A]
			pm := BlitzPublicMatch{A: a.Name, ARating: rating(a), Result: blitzResult(m), Game: playedGame(m), Ruled: m.Ruling != ""}
			pm.ASeed, pm.BSeed = m.SeedA, m.SeedB
			if m.B != nil {
				b := named[*m.B]
				pm.B, pm.BRating = &b.Name, rating(b)
			}
			pr.Matches[j] = pm
		}
		body.Rounds[i] = pr
	}
	return GetBlitzEdition200JSONResponse{Body: body, Headers: GetBlitzEdition200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}
