// Nostradamus, the prediction game: the editions, an edition's standings, and a participant's answers in its closed
// rounds. Admin's routes (an edition's questions, an outcome set, a participant linked) are audited.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ttacommunity/backend/internal/store"
)

// NostradamusReads are the site's Nostradamus reads: the editions, one's board, and a participant's answers.
type NostradamusReads interface {
	NostradamusEditions(ctx context.Context) ([]store.NostradamusEdition, error)
	NostradamusBoard(ctx context.Context, slug string) (store.NostradamusBoard, error)
	NostradamusParticipant(ctx context.Context, slug, name string, now time.Time) (store.NostradamusParticipant, error)
}

// NostradamusAdmin is an edition's questions and participants, an outcome set, and a participant linked to a member.
type NostradamusAdmin interface {
	NostradamusForAdmin(ctx context.Context, slug string) (store.AdminNostradamus, error)
	SetNostradamusOutcome(ctx context.Context, slug string, question int64, outcome []string) error
	LinkNostradamusParticipant(ctx context.Context, slug string, participant int64, member string) error
}

func (s *server) GetNostradamusEditions(ctx context.Context, _ GetNostradamusEditionsRequestObject) (GetNostradamusEditionsResponseObject, error) {
	editions, err := s.ratings.NostradamusEditions(ctx)
	if err != nil {
		return nil, err
	}
	body := NostradamusEditions{Editions: make([]NostradamusEdition, len(editions))}
	for i, e := range editions {
		body.Editions[i] = NostradamusEdition{Slug: e.Slug, Name: e.Name, Series: e.Series, Tournament: e.Tournament}
	}
	return GetNostradamusEditions200JSONResponse{Body: body, Headers: GetNostradamusEditions200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetNostradamusBoard(ctx context.Context, req GetNostradamusBoardRequestObject) (GetNostradamusBoardResponseObject, error) {
	board, err := s.ratings.NostradamusBoard(ctx, req.Edition)
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetNostradamusBoard404JSONResponse{notFound404("no such Nostradamus edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := NostradamusBoard{Slug: board.Slug, Name: board.Name, Rounds: append([]string{}, board.Rounds...),
		Standings: make([]NostradamusStanding, len(board.Standings))}
	for i, st := range board.Standings {
		rounds := make([]NostradamusRoundScore, len(st.Rounds))
		for j, r := range st.Rounds {
			rounds[j] = NostradamusRoundScore{Round: r.Label, Points: r.Points, Missed: r.Missed}
		}
		body.Standings[i] = NostradamusStanding{Name: st.Name, Member: nullableText(st.Member), Bot: st.Bot, Total: st.Total, Rounds: rounds}
	}
	return GetNostradamusBoard200JSONResponse{Body: body, Headers: GetNostradamusBoard200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

// nostradamusQuestion is a question as the API sends it: a null outcome while it is unknown.
func nostradamusQuestion(q store.NostradamusQuestion) NostradamusQuestion {
	out := NostradamusQuestion{Label: q.Label, Prompt: q.Prompt, Kind: NostradamusQuestionKind(q.Kind), Points: q.Points, NearPoints: q.NearPoints}
	if len(q.Outcome) > 0 {
		outcome := append([]string{}, q.Outcome...)
		out.Outcome = &outcome
	}
	return out
}

func (s *server) GetNostradamusParticipant(ctx context.Context, req GetNostradamusParticipantRequestObject) (GetNostradamusParticipantResponseObject, error) {
	p, err := s.ratings.NostradamusParticipant(ctx, req.Edition, req.Name, s.now())
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetNostradamusParticipant404JSONResponse{notFound404("no such participant in that Nostradamus edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := NostradamusParticipant{Name: p.Name, Member: nullableText(p.Member), Bot: p.Bot, Rounds: make([]NostradamusParticipantRound, len(p.Rounds))}
	for i, r := range p.Rounds {
		answers := make([]NostradamusAnswer, len(r.Answers))
		for j, a := range r.Answers {
			answers[j] = NostradamusAnswer{Question: nostradamusQuestion(a.Question), Answer: append([]string{}, a.Answer...), Token: a.Token, Points: a.Points}
		}
		body.Rounds[i] = NostradamusParticipantRound{Round: r.Label, Name: r.Name, ClosesAt: r.ClosesAt.UTC(), Points: r.Points, Missed: r.Missed, Answers: answers}
	}
	return GetNostradamusParticipant200JSONResponse{Body: body, Headers: GetNostradamusParticipant200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetAdminNostradamusEditions(ctx context.Context, _ GetAdminNostradamusEditionsRequestObject) (GetAdminNostradamusEditionsResponseObject, error) {
	editions, err := s.ratings.NostradamusEditions(ctx)
	if err != nil {
		return nil, err
	}
	body := NostradamusEditions{Editions: make([]NostradamusEdition, len(editions))}
	for i, e := range editions {
		body.Editions[i] = NostradamusEdition{Slug: e.Slug, Name: e.Name, Series: e.Series, Tournament: e.Tournament}
	}
	return GetAdminNostradamusEditions200JSONResponse(body), nil
}

func (s *server) GetAdminNostradamus(ctx context.Context, req GetAdminNostradamusRequestObject) (GetAdminNostradamusResponseObject, error) {
	e, err := s.admins.NostradamusForAdmin(ctx, req.Edition)
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetAdminNostradamus404JSONResponse{notFound404("no such Nostradamus edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := AdminNostradamus{Slug: e.Slug, Name: e.Name, Rounds: make([]AdminNostradamusRound, len(e.Rounds)),
		Participants: make([]AdminNostradamusParticipant, len(e.Participants))}
	for i, r := range e.Rounds {
		questions := make([]AdminNostradamusQuestion, len(r.Questions))
		for j, q := range r.Questions {
			questions[j] = AdminNostradamusQuestion{Id: q.ID, Question: nostradamusQuestion(q)}
		}
		body.Rounds[i] = AdminNostradamusRound{Round: r.Label, Name: r.Name, ClosesAt: r.ClosesAt.UTC(), Questions: questions}
	}
	for i, p := range e.Participants {
		body.Participants[i] = AdminNostradamusParticipant{Id: p.ID, Name: p.Name, Member: nullableText(p.Member), Bot: p.Bot}
	}
	return GetAdminNostradamus200JSONResponse(body), nil
}

func (s *server) PutAdminNostradamusOutcome(ctx context.Context, req PutAdminNostradamusOutcomeRequestObject) (PutAdminNostradamusOutcomeResponseObject, error) {
	me, _ := adminFrom(ctx)
	if req.Body == nil || req.Body.Outcome == nil {
		return PutAdminNostradamusOutcome400JSONResponse{badRequest400("the outcome: a list, empty to clear it")}, nil
	}
	outcome := make([]string, len(req.Body.Outcome))
	for i, v := range req.Body.Outcome {
		if outcome[i] = strings.TrimSpace(v); outcome[i] == "" {
			return PutAdminNostradamusOutcome400JSONResponse{badRequest400("an outcome names no one or nothing")}, nil
		}
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetNostradamusOutcome(ctx, req.Edition, req.Question, outcome); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "nostradamus.outcome.set", fmt.Sprint(req.Question), map[string]any{"edition": req.Edition, "outcome": outcome})
	})
	switch {
	case errors.Is(err, store.ErrBadNostradamus):
		return PutAdminNostradamusOutcome400JSONResponse{badRequest400("not an outcome of the question's kind")}, nil
	case errors.Is(err, store.ErrNoNostradamus):
		return PutAdminNostradamusOutcome404JSONResponse{notFound404("no such question in that Nostradamus edition")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminNostradamusOutcome204Response{}, nil
}

func (s *server) PutAdminNostradamusMember(ctx context.Context, req PutAdminNostradamusMemberRequestObject) (PutAdminNostradamusMemberResponseObject, error) {
	me, _ := adminFrom(ctx)
	if req.Body == nil {
		return PutAdminNostradamusMember400JSONResponse{badRequest400("the member: a TTA username, empty to unlink")}, nil
	}
	member := strings.TrimSpace(req.Body.Member)
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.LinkNostradamusParticipant(ctx, req.Edition, req.Participant, member); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "nostradamus.member.link", fmt.Sprint(req.Participant), map[string]any{"edition": req.Edition, "member": member})
	})
	switch {
	case errors.Is(err, store.ErrNoMember):
		return PutAdminNostradamusMember404JSONResponse{notFound404("no member has that TTA username")}, nil
	case errors.Is(err, store.ErrNoNostradamus):
		return PutAdminNostradamusMember404JSONResponse{notFound404("no such participant in that Nostradamus edition")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminNostradamusMember204Response{}, nil
}
