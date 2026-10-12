// The site's blitz tournaments: the shown editions, and an edition's entrants still in, never a Discord name.
package httpapi

import (
	"context"
	"errors"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

func blitzEditionRow(r store.BlitzEditionRow) BlitzEditionRow {
	return BlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants,
		Format: BlitzFormat{SwissRounds: r.Format.SwissRounds, KnockoutSize: r.Format.KnockoutSize}}
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
		Entrants: make([]BlitzEntrant, len(ed.Entrants))}
	for i, e := range ed.Entrants {
		offset, _ := blitz.Offset(e.Zone, s.now())
		body.Entrants[i] = BlitzEntrant{Name: e.Name, Zone: e.Zone, UtcOffset: offset}
		if r, ok := rated[e.Player]; ok {
			body.Entrants[i].Rating = &r.Rating
		}
	}
	return GetBlitzEdition200JSONResponse{Body: body, Headers: GetBlitzEdition200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}
