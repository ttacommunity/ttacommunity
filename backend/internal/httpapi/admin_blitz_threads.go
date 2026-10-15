// Admin's blitz threads: an edition's organisers, each round's deadline, and a round's Discord threads as text for
// an organiser to paste: each game's title and welcome message.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// errNoThreads means a round's threads cannot be written yet: the reason says what is missing.
var errNoThreads = errors.New("cannot write the threads")

// fileNameUnsafe is what a download's file name leaves out of an edition's label.
var fileNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// roundThreads gathers what round r's threads say: every game with two players, numbered by its place in the round
// as the site numbers it, the deadline and the edition's organisers. It refuses a round with no deadline, an
// edition with no organiser and a round with no game, wrapping errNoThreads.
func roundThreads(ed store.BlitzEdition, r store.BlitzRound) (blitz.Threads, error) {
	switch {
	case r.Deadline == nil:
		return blitz.Threads{}, fmt.Errorf("%w: set the round's deadline first", errNoThreads)
	case len(ed.Organisers) == 0:
		return blitz.Threads{}, fmt.Errorf("%w: add the edition's organisers first", errNoThreads)
	}
	people := map[int64]blitz.Person{}
	for _, e := range ed.Entrants {
		people[e.ID] = blitz.Person{Name: e.Name, Discord: e.DiscordName}
	}
	t := blitz.Threads{Round: blitz.RoundTitle(r.Number, ed.Format.SwissRounds, ed.Format.KnockoutSize), Deadline: *r.Deadline}
	for _, o := range ed.Organisers {
		t.Organisers = append(t.Organisers, blitz.Person{Name: o.Name, Discord: o.DiscordName})
	}
	for i, m := range r.Matches {
		if m.B != nil {
			t.Games = append(t.Games, blitz.ThreadGame{Number: i + 1, A: people[m.A], B: people[*m.B]})
		}
	}
	if len(t.Games) == 0 {
		return blitz.Threads{}, fmt.Errorf("%w: the round has no game to make a thread for", errNoThreads)
	}
	return t, nil
}

func (s *server) GetAdminBlitzRoundThreads(ctx context.Context, req GetAdminBlitzRoundThreadsRequestObject) (GetAdminBlitzRoundThreadsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetAdminBlitzRoundThreads404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetAdminBlitzRoundThreads404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(ed.Rounds, func(r store.BlitzRound) bool { return r.ID == req.Round })
	if i < 0 {
		return GetAdminBlitzRoundThreads404JSONResponse{notFound404("no such round")}, nil
	}
	threads, err := roundThreads(ed, ed.Rounds[i])
	if errors.Is(err, errNoThreads) {
		return GetAdminBlitzRoundThreads409JSONResponse{conflict409(err.Error())}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.threads", fmt.Sprint(req.Round), nil); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s-round-%d-threads.txt", req.Tournament, fileNameUnsafe.ReplaceAllString(req.Edition, "-"), ed.Rounds[i].Number)
	return GetAdminBlitzRoundThreads200TextResponse{Body: blitz.ThreadsText(threads),
		Headers: GetAdminBlitzRoundThreads200ResponseHeaders{CacheControl: cacheNone, ContentDisposition: `attachment; filename="` + name + `"`}}, nil
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
	switch err := s.admins.SetBlitzRoundDeadline(ctx, req.Tournament, req.Edition, req.Round, at.UTC(), zone); {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzRoundDeadline404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzRoundDeadline404JSONResponse{notFound404("no such round")}, nil
	case errors.Is(err, store.ErrBlitzRoundFinished):
		return PutAdminBlitzRoundDeadline409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.round.deadline", fmt.Sprint(req.Round), req.Body); err != nil {
		return nil, err
	}
	return PutAdminBlitzRoundDeadline204Response{}, nil
}

func (s *server) PostAdminBlitzOrganiser(ctx context.Context, req PostAdminBlitzOrganiserRequestObject) (PostAdminBlitzOrganiserResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzOrganiser404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if req.Body == nil {
		return PostAdminBlitzOrganiser400JSONResponse{badRequest400("the member to add")}, nil
	}
	switch err := s.admins.AddBlitzOrganiser(ctx, req.Tournament, req.Edition, req.Body.Member); {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzOrganiser404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoMember):
		return PostAdminBlitzOrganiser404JSONResponse{notFound404("no such member")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.organiser.add", blitzEditionKey(req.Tournament, req.Edition), req.Body); err != nil {
		return nil, err
	}
	return PostAdminBlitzOrganiser204Response{}, nil
}

func (s *server) DeleteAdminBlitzOrganiser(ctx context.Context, req DeleteAdminBlitzOrganiserRequestObject) (DeleteAdminBlitzOrganiserResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzOrganiser404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	switch err := s.admins.RemoveBlitzOrganiser(ctx, req.Tournament, req.Edition, req.Member); {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzOrganiser404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzOrganiser):
		return DeleteAdminBlitzOrganiser404JSONResponse{notFound404(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.organiser.remove", blitzEditionKey(req.Tournament, req.Edition), map[string]int64{"member": req.Member}); err != nil {
		return nil, err
	}
	return DeleteAdminBlitzOrganiser204Response{}, nil
}
