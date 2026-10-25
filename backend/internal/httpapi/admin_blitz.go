// Admin's blitz tournaments: the editions of every series marked Blitz, and an edition's format and entrants.
// Organisers may do all of it.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// blitzModel is the rating model shown beside an entrant, in the 2-player format.
const blitzModel = "glicko2_daneo"

// peopleFound is how many people a search for whom to add or enter finds.
const peopleFound = 5

// zoneProblem says what is wrong with a time zone, or "" when it is an IANA name, a region or Unknown.
func zoneProblem(zone string) string {
	if blitz.ValidZone(zone) {
		return ""
	}
	return fmt.Sprintf("not a time zone: %q (use an IANA name such as Europe/Berlin, a region such as US East, or Unknown)", zone)
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
	out := AdminBlitzEntrant{Id: e.ID, Member: e.MemberID, Name: e.Name, Player: e.Player, DiscordName: e.DiscordName, Email: e.Email,
		Country: nullableText(e.Country), Zone: e.Zone, UtcOffset: offset, Status: AdminBlitzEntrantStatus(e.Status), Skips: []int{}}
	if e.Skips != nil {
		out.Skips = e.Skips
	}
	if r, ok := rated[e.Player]; ok {
		out.Rating = &r.Rating
	}
	return out
}

func adminBlitzEditionRow(r store.BlitzEditionRow) AdminBlitzEditionRow {
	return AdminBlitzEditionRow{Tournament: r.Slug, Name: r.Name, Edition: r.Edition, Entrants: r.Entrants, Dropped: r.Dropped, Flagged: r.Flagged,
		Round: r.Round, Champion: r.Champion, Format: BlitzFormat{SwissRounds: r.Format.SwissRounds, KnockoutSize: r.Format.KnockoutSize}}
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
	body := AdminBlitzEdition{Tournament: row.Tournament, Name: row.Name, Edition: row.Edition, Format: row.Format, Champion: row.Champion,
		Points: blitzPoints(ed.Points), FidePoints: ed.Points == blitz.FIDEScoring, Pairing: blitzPairing(ed.Pairing),
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
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzFormat(ctx, req.Tournament, req.Edition, f); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.format", blitzEditionKey(req.Tournament, req.Edition), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzFormat):
		return PatchAdminBlitzEdition400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrBlitzFormatLocked):
		return PatchAdminBlitzEdition409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PatchAdminBlitzEdition404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
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
		in := store.BlitzEntrant{}
		if e.Member != nil {
			in.MemberID = *e.Member
		}
		if e.Name != nil {
			in.Name = strings.TrimSpace(*e.Name)
		}
		if in.MemberID == 0 && in.Name == "" {
			return PostAdminBlitzEntrants400JSONResponse{badRequest400(fmt.Sprintf("row %d: a member or a name", i+1))}, nil
		}
		// A member entered by id keeps their zone unless one is given; a new one is Unknown until known.
		if e.Zone != nil || in.MemberID == 0 {
			zone, msg := memberZone(e.Zone)
			if msg != "" {
				return PostAdminBlitzEntrants400JSONResponse{badRequest400(fmt.Sprintf("row %d: %s", i+1, msg))}, nil
			}
			in.Zone = zone
		}
		for field, given := range map[*string]*string{&in.DiscordName: e.DiscordName, &in.Email: e.Email, &in.Country: e.Country} {
			if given != nil {
				*field = *given
			}
		}
		entrants[i] = in
	}
	bodies := make([]any, len(req.Body.Entrants))
	for i, e := range req.Body.Entrants {
		bodies[i] = e
	}
	set, err := fieldsSet(bodies...)
	if err != nil {
		return nil, err
	}
	var ids []int64
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if ids, err = tx.AddBlitzEntrants(ctx, req.Tournament, req.Edition, entrants); err != nil {
			return err
		}
		detail := map[string]any{"entrants": len(bodies), "set": set}
		return tx.Audit(ctx, me.DiscordID, "blitz.entrants.add", blitzEditionKey(req.Tournament, req.Edition), detail)
	})
	var exists *store.BlitzEntrantExistsError
	var member *store.MemberExistsError
	switch {
	case errors.As(err, &exists), errors.As(err, &member), errors.Is(err, store.ErrMemberBanned):
		return PostAdminBlitzEntrants409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrBadBlitzEntrant), errors.Is(err, store.ErrBadMember):
		return PostAdminBlitzEntrants400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzEntrants404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoMember):
		return PostAdminBlitzEntrants404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBlitzEntrants201JSONResponse{Body: BlitzEntrantIds{Ids: ids}, Headers: PostAdminBlitzEntrants201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminBlitzEntrant(ctx context.Context, req PatchAdminBlitzEntrantRequestObject) (PatchAdminBlitzEntrantResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	b := req.Body
	if b == nil || (b.DiscordName == nil && b.Email == nil && b.Country == nil && b.Zone == nil && b.Status == nil) {
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400("nothing to change")}, nil
	}
	c := store.BlitzEntrantChange{DiscordName: b.DiscordName, Email: b.Email, Country: b.Country}
	if b.Zone != nil {
		zone, msg := memberZone(b.Zone)
		if msg != "" {
			return PatchAdminBlitzEntrant400JSONResponse{badRequest400(msg)}, nil
		}
		c.Zone = &zone
	}
	if b.Status != nil {
		status := string(*b.Status)
		c.Status = &status
	}
	set, err := fieldsSet(req.Body)
	if err != nil {
		return nil, err
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzEntrant(ctx, req.Tournament, req.Edition, req.Entrant, c); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.entrant.set", fmt.Sprint(req.Entrant), map[string][]string{"set": set})
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzEntrant), errors.Is(err, store.ErrBadBlitzStatus), errors.Is(err, store.ErrBadMember):
		return PatchAdminBlitzEntrant400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PatchAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return PatchAdminBlitzEntrant404JSONResponse{notFound404("no such entrant")}, nil
	case errors.Is(err, store.ErrBlitzEntrantNotPlayed), errors.Is(err, store.ErrBlitzEntrantSkipping):
		return PatchAdminBlitzEntrant409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PatchAdminBlitzEntrant204Response{}, nil
}

func (s *server) DeleteAdminBlitzEntrant(ctx context.Context, req DeleteAdminBlitzEntrantRequestObject) (DeleteAdminBlitzEntrantResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveBlitzEntrant(ctx, req.Tournament, req.Edition, req.Entrant); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.entrant.remove", fmt.Sprint(req.Entrant), nil)
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzEntrant):
		return DeleteAdminBlitzEntrant404JSONResponse{notFound404("no such entrant")}, nil
	case errors.Is(err, store.ErrBlitzEntrantPlayed), errors.Is(err, store.ErrBlitzEntrantInKnockout):
		return DeleteAdminBlitzEntrant409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzEntrant204Response{}, nil
}

func (s *server) GetAdminBlitzCandidates(ctx context.Context, req GetAdminBlitzCandidatesRequestObject) (GetAdminBlitzCandidatesResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetAdminBlitzCandidates400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	q, ok := searchText(req.Params.Q)
	if !ok {
		return GetAdminBlitzCandidates400JSONResponse{badRequest400("a search of 2 to 100 characters")}, nil
	}
	found, err := s.admins.BlitzCandidates(ctx, req.Tournament, req.Edition, q, peopleFound)
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return GetAdminBlitzCandidates404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	return GetAdminBlitzCandidates200JSONResponse{Body: candidatesOut(found), Headers: GetAdminBlitzCandidates200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminRegions(context.Context, GetAdminRegionsRequestObject) (GetAdminRegionsResponseObject, error) {
	out := []Region{{Name: blitz.Unknown}}
	for _, r := range blitz.Regions {
		offset, err := blitz.Offset(r.Name, s.now())
		if err != nil {
			return nil, err
		}
		out = append(out, Region{Name: r.Name, Zone: r.Zone, UtcOffset: offset})
	}
	return GetAdminRegions200JSONResponse{Body: out, Headers: GetAdminRegions200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminCountries(context.Context, GetAdminCountriesRequestObject) (GetAdminCountriesResponseObject, error) {
	return GetAdminCountries200JSONResponse{Body: store.CountryCodes(), Headers: GetAdminCountries200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) GetAdminPeople(ctx context.Context, req GetAdminPeopleRequestObject) (GetAdminPeopleResponseObject, error) {
	q, ok := searchText(req.Params.Q)
	if !ok {
		return GetAdminPeople400JSONResponse{badRequest400("a search of 2 to 100 characters")}, nil
	}
	found, err := s.admins.People(ctx, q, peopleFound)
	if err != nil {
		return nil, err
	}
	return GetAdminPeople200JSONResponse{Body: candidatesOut(found), Headers: GetAdminPeople200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// searchText is a search's text, trimmed, and whether it is 2 to 100 characters long.
func searchText(q string) (string, bool) {
	q = strings.TrimSpace(q)
	n := utf8.RuneCountInString(q)
	return q, n >= 2 && n <= 100
}

// candidatesOut is the people found as the API sends them.
func candidatesOut(found []store.Candidate) []Candidate {
	out := make([]Candidate, len(found))
	for i, c := range found {
		out[i] = Candidate{Name: c.Name, Player: c.Player, DiscordName: c.DiscordName, Email: c.Email,
			Country: nullableText(c.Country), Zone: nullableText(c.Zone), Banned: c.Banned}
		if c.MemberID > 0 {
			out[i].Member = &c.MemberID
		}
	}
	return out
}

func (s *server) PutAdminBlitzPoints(ctx context.Context, req PutAdminBlitzPointsRequestObject) (PutAdminBlitzPointsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzPoints404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzPoints400JSONResponse{badRequest400("the points: win, draw, loss, forfeit_win, forfeit_loss, bye and skip")}, nil
	}
	b := req.Body
	points := blitz.Scoring{Win: b.Win, Draw: b.Draw, Loss: b.Loss, ForfeitWin: b.ForfeitWin, ForfeitLoss: b.ForfeitLoss, Bye: b.Bye, Skip: b.Skip}
	if err := blitz.CheckScoring(points); err != nil {
		return PutAdminBlitzPoints400JSONResponse{badRequest400(err.Error())}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzPoints(ctx, req.Tournament, req.Edition, points); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.points", blitzEditionKey(req.Tournament, req.Edition), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBlitzPointsLocked):
		return PutAdminBlitzPoints409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzPoints404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzPoints204Response{}, nil
}

func (s *server) PutAdminBlitzPairing(ctx context.Context, req PutAdminBlitzPairingRequestObject) (PutAdminBlitzPairingResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzPairing404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzPairing400JSONResponse{badRequest400("the pairing: first and later")}, nil
	}
	p := store.BlitzPairing{First: string(req.Body.First), Later: string(req.Body.Later)}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzPairing(ctx, req.Tournament, req.Edition, p); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.pairing", blitzEditionKey(req.Tournament, req.Edition), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzPairing):
		return PutAdminBlitzPairing400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzPairing404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzPairing204Response{}, nil
}

func (s *server) DeleteAdminBlitzPoints(ctx context.Context, req DeleteAdminBlitzPointsRequestObject) (DeleteAdminBlitzPointsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzPoints404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzPoints(ctx, req.Tournament, req.Edition, blitz.FIDEScoring); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.points.reset", blitzEditionKey(req.Tournament, req.Edition), nil)
	})
	switch {
	case errors.Is(err, store.ErrBlitzPointsLocked):
		return DeleteAdminBlitzPoints409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzPoints404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzPoints204Response{}, nil
}
