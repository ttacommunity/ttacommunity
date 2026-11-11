// Nostradamus, the prediction game, on the site: the games, one game's published rounds and standings, and a signed-in
// player's place in it: asking to join, reading their own answers, answering an open round.
package httpapi

import (
	"context"
	"errors"
	"sort"
	"strings"

	"ttacommunity/backend/internal/store"
)

// NostradamusReads are the site's Nostradamus reads: the games, and one as the site shows it.
type NostradamusReads interface {
	NostradamusEditions(ctx context.Context) ([]store.NostradamusEdition, error)
	NostradamusGame(ctx context.Context, slug string) (store.NostradamusGame, error)
}

// nostradamusEditions are games as the API sends them.
func nostradamusEditions(editions []store.NostradamusEdition) NostradamusEditions {
	body := NostradamusEditions{Editions: make([]NostradamusEdition, len(editions))}
	for i, e := range editions {
		body.Editions[i] = NostradamusEdition{Slug: e.Slug, Name: e.Name, Series: e.Series, Tournament: e.Tournament}
	}
	return body
}

// nostradamusRounds are rounds as the API sends them: a null deadline for a draft, a null outcome while it is unknown.
func nostradamusRounds(rounds []store.NostradamusRound) []NostradamusRound {
	out := make([]NostradamusRound, len(rounds))
	for i, r := range rounds {
		questions := make([]NostradamusQuestion, len(r.Questions))
		for j, q := range r.Questions {
			questions[j] = NostradamusQuestion{Id: q.ID, Label: q.Label, Prompt: q.Prompt, Kind: NostradamusQuestionKind(q.Kind), Points: q.Points,
				NearPoints: q.NearPoints, Picks: q.Picks, AsksLast: q.AsksLast, Choices: []NostradamusChoice{}, Counts: []NostradamusCount{}}
			for _, c := range q.Choices {
				questions[j].Choices = append(questions[j].Choices, NostradamusChoice{Label: c.Label, Seed: c.Seed, LeaderboardRank: c.LeaderboardRank})
			}
			for _, c := range q.Counts {
				questions[j].Counts = append(questions[j].Counts, NostradamusCount{Value: c.Value, Answers: c.Answers})
			}
			if len(q.Outcome) > 0 {
				outcome := append([]string{}, q.Outcome...)
				questions[j].Outcome = &outcome
			}
		}
		out[i] = NostradamusRound{Id: r.ID, Label: r.Label, Name: r.Name, Tokens: r.Tokens, TokenPoints: r.TokenPoints, Zone: r.Zone,
			Published: r.PublishedAt != nil, Closed: r.ClosedAt != nil, Questions: questions}
		if r.ClosesAt != nil {
			closes := r.ClosesAt.UTC()
			out[i].ClosesAt = &closes
		}
	}
	return out
}

// nostradamusStandings are standings as the API sends them.
func nostradamusStandings(standings []store.NostradamusStanding) []NostradamusStanding {
	out := make([]NostradamusStanding, len(standings))
	for i, st := range standings {
		rounds := make([]NostradamusRoundScore, len(st.Rounds))
		for j, r := range st.Rounds {
			rounds[j] = NostradamusRoundScore{Round: r.Label, Points: r.Points, Missed: r.Missed}
		}
		out[i] = NostradamusStanding{Name: st.Name, Member: nullableText(st.Member), Bot: st.Bot, Total: st.Total, Rounds: rounds}
	}
	return out
}

func (s *server) GetNostradamusEditions(ctx context.Context, _ GetNostradamusEditionsRequestObject) (GetNostradamusEditionsResponseObject, error) {
	editions, err := s.ratings.NostradamusEditions(ctx)
	if err != nil {
		return nil, err
	}
	return GetNostradamusEditions200JSONResponse{Body: nostradamusEditions(editions), Headers: GetNostradamusEditions200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetNostradamusGame(ctx context.Context, req GetNostradamusGameRequestObject) (GetNostradamusGameResponseObject, error) {
	g, err := s.ratings.NostradamusGame(ctx, req.Edition)
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetNostradamusGame404JSONResponse{notFound404("no such Nostradamus game")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := NostradamusGame{Slug: g.Slug, Name: g.Name, Series: g.Series, Tournament: g.Tournament, Rounds: nostradamusRounds(g.Rounds),
		Scored: append([]string{}, g.Scored...), Standings: nostradamusStandings(g.Standings)}
	return GetNostradamusGame200JSONResponse{Body: body, Headers: GetNostradamusGame200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

// nostradamusRefusal is the response a player's Nostradamus request gets for err: its status and what to say, or ok false
// for an error that is not a refusal.
func nostradamusRefusal(err error) (status int, msg string, ok bool) {
	switch {
	case errors.Is(err, store.ErrNoSession):
		return 401, "not signed in", true
	case errors.Is(err, store.ErrNotLinked), errors.Is(err, store.ErrMemberBanned), errors.Is(err, store.ErrNotEntered):
		return 403, err.Error(), true
	case errors.Is(err, store.ErrNoNostradamus):
		return 404, "no such Nostradamus game", true
	case errors.Is(err, store.ErrRoundNotOpen), errors.Is(err, store.ErrPastDeadline):
		return 409, err.Error(), true
	case errors.Is(err, store.ErrBadNostradamus):
		return 400, "an answer the rules refuse: a name not among the choices, too many tokens, or one question twice", true
	}
	return 0, "", false
}

func (s *server) GetMyNostradamus(ctx context.Context, req GetMyNostradamusRequestObject) (GetMyNostradamusResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	mine, err := s.signIns.MyNostradamus(ctx, hash, req.Edition, s.now())
	if status, msg, ok := nostradamusRefusal(err); ok {
		if status == 401 {
			return GetMyNostradamus401JSONResponse{signedOut401(msg)}, nil
		}
		return GetMyNostradamus403JSONResponse{NotLinkedJSONResponse{Error: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	body := MyNostradamus{Status: MyNostradamusStatus(mine.Status), Answers: []NostradamusGiven{}}
	for _, a := range mine.Answers {
		body.Answers = append(body.Answers, NostradamusGiven{Question: a.Question, Answer: append([]string{}, a.Answer...), Token: a.Token})
	}
	sort.Slice(body.Answers, func(i, j int) bool { return body.Answers[i].Question < body.Answers[j].Question })
	return GetMyNostradamus200JSONResponse{Body: body, Headers: GetMyNostradamus200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostMyNostradamusEntry(ctx context.Context, req PostMyNostradamusEntryRequestObject) (PostMyNostradamusEntryResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	got, err := s.signIns.RequestNostradamusEntry(ctx, hash, req.Edition, s.now())
	if status, msg, ok := nostradamusRefusal(err); ok {
		switch status {
		case 401:
			return PostMyNostradamusEntry401JSONResponse{signedOut401(msg)}, nil
		case 404:
			return PostMyNostradamusEntry404JSONResponse{notFound404(msg)}, nil
		}
		return PostMyNostradamusEntry403JSONResponse{NotLinkedJSONResponse{Error: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	return PostMyNostradamusEntry200JSONResponse{Status: MyNostradamusEntryStatus(got)}, nil
}

// givenAnswers are answers as the store takes them, each name trimmed; an empty name is refused.
func givenAnswers(answers []NostradamusGiven) ([]store.NostradamusGiven, bool) {
	out := make([]store.NostradamusGiven, len(answers))
	for i, a := range answers {
		out[i] = store.NostradamusGiven{Question: a.Question, Token: a.Token}
		for _, v := range a.Answer {
			if v = strings.TrimSpace(v); v == "" {
				return nil, false
			}
			out[i].Answer = append(out[i].Answer, v)
		}
	}
	return out, true
}

func (s *server) PutMyNostradamusAnswers(ctx context.Context, req PutMyNostradamusAnswersRequestObject) (PutMyNostradamusAnswersResponseObject, error) {
	if req.Body == nil {
		return PutMyNostradamusAnswers400JSONResponse{badRequest400("the answers: question, answer and token")}, nil
	}
	answers, ok := givenAnswers(req.Body.Answers)
	if !ok {
		return PutMyNostradamusAnswers400JSONResponse{badRequest400("an answer names no one or nothing")}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	saved, err := s.signIns.SubmitNostradamusAnswers(ctx, hash, req.Round, answers, s.now())
	if status, msg, ok := nostradamusRefusal(err); ok {
		switch status {
		case 400:
			return PutMyNostradamusAnswers400JSONResponse{badRequest400(msg)}, nil
		case 401:
			return PutMyNostradamusAnswers401JSONResponse{signedOut401(msg)}, nil
		case 409:
			return PutMyNostradamusAnswers409JSONResponse{conflict409(msg)}, nil
		}
		return PutMyNostradamusAnswers403JSONResponse{NotLinkedJSONResponse{Error: msg}}, nil
	}
	if err != nil {
		return nil, err
	}
	return PutMyNostradamusAnswers200JSONResponse{Saved: saved}, nil
}
