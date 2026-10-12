// Admin's blitz tournaments: the editions of every series marked Blitz, and an edition's format and entrants.
// Organisers may do all of it.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// blitzModel is the rating model shown beside an entrant, in the 2-player format.
const blitzModel = "glicko2_daneo"

// zoneProblem says what is wrong with a time zone, or "" when it is an IANA name.
func zoneProblem(zone string) string {
	if blitz.ValidZone(zone) {
		return ""
	}
	return fmt.Sprintf("not a time zone: %q (use an IANA name such as Europe/Berlin)", zone)
}

// blitzEditionKey names an edition in the audit.
func blitzEditionKey(slug, edition string) string {
	return slug + "/" + edition
}

// blitzRatings finds the 2-player ratings of the entrants the ratings know, in the current run; none when no run
// is published.
func (s *server) blitzRatings(ctx context.Context, entrants []store.BlitzEntrant) (map[string]store.Rated, error) {
	run, err := s.ratings.CurrentRun(ctx)
	if errors.Is(err, store.ErrNoPublishedRun) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entrants {
		if e.Player != "" {
			names = append(names, e.Player)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	return s.ratings.RatingsOf(ctx, run.ID, blitzModel, 2, names)
}

func (s *server) adminBlitzEntrant(e store.BlitzEntrant, rated map[string]store.Rated) AdminBlitzEntrant {
	// A stored zone was checked on the way in, so an error here is an unknown later zone: shown with no offset.
	offset, _ := blitz.Offset(e.Zone, s.now())
	out := AdminBlitzEntrant{Id: e.ID, Name: e.Name, Player: e.Player, DiscordName: e.DiscordName, Zone: e.Zone,
		UtcOffset: offset, Status: AdminBlitzEntrantStatus(e.Status)}
	if r, ok := rated[e.Player]; ok {
		out.Rating = &r.Rating
	}
	return out
}

func adminBlitzEditionRow(r store.BlitzEditionRow) AdminBlitzEditionRow {
	return AdminBlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants, Pending: r.Pending, Dropped: r.Dropped,
		Format: BlitzFormat{SwissRounds: r.Format.SwissRounds, KnockoutSize: r.Format.KnockoutSize}}
}

func (s *server) GetAdminBlitzEditions(ctx context.Context, _ GetAdminBlitzEditionsRequestObject) (GetAdminBlitzEditionsResponseObject, error) {
	rows, err := s.admins.BlitzEditions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AdminBlitzEditionRow, len(rows))
	for i, r := range rows {
		out[i] = adminBlitzEditionRow(r)
	}
	return GetAdminBlitzEditions200JSONResponse{Body: out, Headers: GetAdminBlitzEditions200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminBlitzEdition(ctx context.Context, req GetAdminBlitzEditionRequestObject) (GetAdminBlitzEditionResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetAdminBlitzEdition400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetAdminBlitzEdition404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	row := adminBlitzEditionRow(ed.BlitzEditionRow)
	body := AdminBlitzEdition{Tournament: row.Tournament, Name: row.Name, Edition: row.Edition, Format: row.Format,
		Entrants: make([]AdminBlitzEntrant, len(ed.Entrants)), Rounds: make([]AdminBlitzRound, len(ed.Rounds)), Standings: blitzStandings(ed)}
	for i, e := range ed.Entrants {
		body.Entrants[i] = s.adminBlitzEntrant(e, rated)
	}
	now := s.now()
	for i, r := range ed.Rounds {
		body.Rounds[i] = s.adminBlitzRound(r, ed, rated, now)
	}
	return GetAdminBlitzEdition200JSONResponse{Body: body, Headers: GetAdminBlitzEdition200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminBlitzEdition(ctx context.Context, req PatchAdminBlitzEditionRequestObject) (PatchAdminBlitzEditionResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PatchAdminBlitzEdition400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PatchAdminBlitzEdition400JSONResponse{badRequest400("a format: swiss_rounds and knockout_size")}, nil
	}
	f := store.BlitzFormat{SwissRounds: req.Body.SwissRounds, KnockoutSize: req.Body.KnockoutSize}
	switch err := s.admins.SetBlitzFormat(ctx, req.Tournament, req.Edition, f); {
	case errors.Is(err, store.ErrBadBlitzFormat):
		return PatchAdminBlitzEdition400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PatchAdminBlitzEdition404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.format", blitzEditionKey(req.Tournament, req.Edition), req.Body); err != nil {
		return nil, err
	}
	return PatchAdminBlitzEdition204Response{}, nil
}

func (s *server) PostAdminBlitzEntrants(ctx context.Context, req PostAdminBlitzEntrantsRequestObject) (PostAdminBlitzEntrantsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzEntrants400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil || len(req.Body.Entrants) == 0 {
		return PostAdminBlitzEntrants400JSONResponse{badRequest400("at least one entrant")}, nil
	}
	entrants := make([]store.BlitzEntrant, len(req.Body.Entrants))
	for i, e := range req.Body.Entrants {
		if msg := zoneProblem(strings.TrimSpace(e.Zone)); msg != "" {
			return PostAdminBlitzEntrants400JSONResponse{badRequest400(fmt.Sprintf("row %d: %s", i+1, msg))}, nil
		}
		entrants[i] = store.BlitzEntrant{Name: e.Name, Zone: strings.TrimSpace(e.Zone)}
		if e.DiscordName != nil {
			entrants[i].DiscordName = *e.DiscordName
		}
		if e.Status != nil {
			entrants[i].Status = string(*e.Status)
		}
	}
	ids, err := s.admins.AddBlitzEntrants(ctx, req.Tournament, req.Edition, entrants)
	var exists *store.BlitzEntrantExistsError
	switch {
	case errors.As(err, &exists):
		return PostAdminBlitzEntrants409JSONResponse{conflict409(exists.Error())}, nil
	case errors.Is(err, store.ErrBadBlitzEntrant):
		return PostAdminBlitzEntrants400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzEntrants404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.entrants.add", blitzEditionKey(req.Tournament, req.Edition), req.Body); err != nil {
		return nil, err
	}
	return PostAdminBlitzEntrants201JSONResponse{Body: BlitzEntrantIds{Ids: ids}, Headers: PostAdminBlitzEntrants201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminBlitzEntrant(ctx context.Context, req PatchAdminBlitzEntrantRequestObject) (PatchAdminBlitzEntrantResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil || (req.Body.DiscordName == nil && req.Body.Zone == nil && req.Body.Status == nil) {
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400("nothing to change")}, nil
	}
	c := store.BlitzEntrantChange{DiscordName: req.Body.DiscordName}
	if req.Body.Zone != nil {
		zone := strings.TrimSpace(*req.Body.Zone)
		if msg := zoneProblem(zone); msg != "" {
			return PatchAdminBlitzEntrant400JSONResponse{badRequest400(msg)}, nil
		}
		c.Zone = &zone
	}
	if req.Body.Status != nil {
		status := string(*req.Body.Status)
		c.Status = &status
	}
	switch err := s.admins.SetBlitzEntrant(ctx, req.Tournament, req.Edition, req.Entrant, c); {
	case errors.Is(err, store.ErrBadBlitzEntrant), errors.Is(err, store.ErrBadBlitzStatus):
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PatchAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return PatchAdminBlitzEntrant404JSONResponse{notFound404("no such entrant")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.entrant.set", fmt.Sprint(req.Entrant), req.Body); err != nil {
		return nil, err
	}
	return PatchAdminBlitzEntrant204Response{}, nil
}

func (s *server) DeleteAdminBlitzEntrant(ctx context.Context, req DeleteAdminBlitzEntrantRequestObject) (DeleteAdminBlitzEntrantResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	switch err := s.admins.RemoveBlitzEntrant(ctx, req.Tournament, req.Edition, req.Entrant); {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such entrant")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.entrant.remove", fmt.Sprint(req.Entrant), nil); err != nil {
		return nil, err
	}
	return DeleteAdminBlitzEntrant204Response{}, nil
}
