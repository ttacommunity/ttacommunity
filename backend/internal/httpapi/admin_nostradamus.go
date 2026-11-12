// Admin's Nostradamus: the games and one game whole; a game made; rounds drafted with their questions, published,
// answered for bots and closed; outcomes set; participants added, accepted or refused, and linked. Changes are audited.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// NostradamusAdmin is admin's Nostradamus: each read and change of a game's rounds, questions and participants.
type NostradamusAdmin interface {
	NostradamusEditions(ctx context.Context, now time.Time) ([]store.NostradamusEdition, error)
	NostradamusGame(ctx context.Context, slug string) (store.NostradamusGame, error)
	AddNostradamusEdition(ctx context.Context, e store.NostradamusEdition) error
	AddNostradamusRound(ctx context.Context, slug string, d store.NostradamusRoundDraft) (int64, error)
	SetNostradamusRound(ctx context.Context, slug string, round int64, d store.NostradamusRoundDraft) error
	RemoveNostradamusRound(ctx context.Context, slug string, round int64) error
	AddNostradamusQuestion(ctx context.Context, slug string, round int64, q store.NostradamusQuestion) (int64, error)
	SetNostradamusQuestion(ctx context.Context, slug string, question int64, q store.NostradamusQuestion) error
	RemoveNostradamusQuestion(ctx context.Context, slug string, question int64) error
	PublishNostradamusRound(ctx context.Context, slug string, round int64, closes time.Time, zone string, now time.Time) error
	CloseNostradamusRound(ctx context.Context, slug string, round int64, now time.Time) error
	UnpublishNostradamusRound(ctx context.Context, slug string, round int64) error
	SetNostradamusDeadline(ctx context.Context, slug string, round int64, closes time.Time, zone string, now time.Time) error
	SetNostradamusOutcome(ctx context.Context, slug string, question int64, outcome []string) error
	NostradamusRoundAnswers(ctx context.Context, slug string, round int64) ([]store.NostradamusAnswer, error)
	SetNostradamusBotAnswers(ctx context.Context, slug string, round, participant int64, answers []store.NostradamusGiven, now time.Time) error
	AddNostradamusParticipant(ctx context.Context, slug string, e store.NostradamusEntrant) (int64, error)
	SetNostradamusStatus(ctx context.Context, slug string, participant int64, status string) error
	LinkNostradamusParticipant(ctx context.Context, slug string, participant int64, member string) error
}

// nostradamusChange runs change in admin's transaction with its audit row, and answers how the change went: 0 when
// done, else the status to refuse it with and what to say.
func (s *server) nostradamusChange(ctx context.Context, action, target string, detail any, change func(tx Admins) error) (int, string, error) {
	me, _ := adminFrom(ctx)
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := change(tx); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, action, target, detail)
	})
	switch {
	case err == nil:
		return 0, "", nil
	case errors.Is(err, store.ErrBadNostradamus), errors.Is(err, store.ErrMemberBanned), errors.Is(err, store.ErrBadMember):
		return 400, err.Error(), nil
	case errors.Is(err, store.ErrNoNostradamus):
		return 404, "no such game, round, question or participant", nil
	case errors.Is(err, store.ErrNoTournament):
		return 404, "no such tournament", nil
	case errors.Is(err, store.ErrNoMember):
		return 404, "no member has that TTA username", nil
	case errors.Is(err, store.ErrNostradamusRoundState):
		return 409, err.Error(), nil
	}
	return 0, "", err
}

// questionDraft is a question as the store takes it.
func questionDraft(b AdminNostradamusQuestionDraft) store.NostradamusQuestion {
	q := store.NostradamusQuestion{Label: b.Label, Prompt: b.Prompt, Kind: string(b.Kind), Points: b.Points, NearPoints: b.NearPoints,
		Picks: b.Picks, AsksLast: b.AsksLast}
	for _, c := range b.Choices {
		q.Choices = append(q.Choices, store.NostradamusChoice{Label: strings.TrimSpace(c.Label), Seed: c.Seed, LeaderboardRank: c.LeaderboardRank})
	}
	return q
}

func (s *server) GetAdminNostradamusEditions(ctx context.Context, _ GetAdminNostradamusEditionsRequestObject) (GetAdminNostradamusEditionsResponseObject, error) {
	editions, err := s.admins.NostradamusEditions(ctx, s.now())
	if err != nil {
		return nil, err
	}
	return GetAdminNostradamusEditions200JSONResponse(nostradamusEditions(editions)), nil
}

func (s *server) GetAdminNostradamusGame(ctx context.Context, req GetAdminNostradamusGameRequestObject) (GetAdminNostradamusGameResponseObject, error) {
	g, err := s.admins.NostradamusGame(ctx, req.Edition)
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetAdminNostradamusGame404JSONResponse{notFound404("no such Nostradamus game")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := AdminNostradamusGame{Slug: g.Slug, Name: g.Name, Series: g.Series, Tournament: g.Tournament, Rounds: nostradamusRounds(g.Rounds),
		Scored: append([]string{}, g.Scored...), Standings: nostradamusStandings(g.Standings), Participants: make([]AdminNostradamusParticipant, len(g.Participants))}
	for i, p := range g.Participants {
		body.Participants[i] = AdminNostradamusParticipant{Id: p.ID, Name: p.Name, Member: nullableText(p.Member), DiscordName: p.DiscordName, Bot: p.Bot,
			BotRule: AdminNostradamusParticipantBotRule(p.BotRule), Status: AdminNostradamusParticipantStatus(p.Status), RequestedAt: p.RequestedAt}
	}
	return GetAdminNostradamusGame200JSONResponse(body), nil
}

func (s *server) GetAdminNostradamusAnswers(ctx context.Context, req GetAdminNostradamusAnswersRequestObject) (GetAdminNostradamusAnswersResponseObject, error) {
	answers, err := s.admins.NostradamusRoundAnswers(ctx, req.Edition, req.Round)
	if errors.Is(err, store.ErrNoNostradamus) {
		return GetAdminNostradamusAnswers404JSONResponse{notFound404("no such round in that Nostradamus game")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := AdminNostradamusAnswers{Answers: make([]AdminNostradamusAnswer, len(answers))}
	for i, a := range answers {
		body.Answers[i] = AdminNostradamusAnswer{Participant: a.Participant, Question: a.Question, Answer: append([]string{}, a.Answer...), Token: a.Token, Points: a.Points}
	}
	return GetAdminNostradamusAnswers200JSONResponse(body), nil
}

func (s *server) PostAdminNostradamusEdition(ctx context.Context, req PostAdminNostradamusEditionRequestObject) (PostAdminNostradamusEditionResponseObject, error) {
	if req.Body == nil {
		return PostAdminNostradamusEdition400JSONResponse{badRequest400("the game: slug, name, series and tournament")}, nil
	}
	e := store.NostradamusEdition{Slug: req.Body.Slug, Name: req.Body.Name, Series: req.Body.Series, Tournament: req.Body.Tournament}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.edition.add", e.Slug, req.Body, func(tx Admins) error { return tx.AddNostradamusEdition(ctx, e) })
	switch status {
	case 0:
		return PostAdminNostradamusEdition204Response{}, err
	case 404:
		return PostAdminNostradamusEdition404JSONResponse{notFound404(msg)}, nil
	}
	return PostAdminNostradamusEdition400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PostAdminNostradamusRound(ctx context.Context, req PostAdminNostradamusRoundRequestObject) (PostAdminNostradamusRoundResponseObject, error) {
	if req.Body == nil {
		return PostAdminNostradamusRound400JSONResponse{badRequest400("the round: label, name, tokens and token_points")}, nil
	}
	d := store.NostradamusRoundDraft{Label: req.Body.Label, Name: req.Body.Name, Tokens: req.Body.Tokens, TokenPoints: req.Body.TokenPoints}
	var id int64
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.add", req.Edition, req.Body, func(tx Admins) error {
		var err error
		id, err = tx.AddNostradamusRound(ctx, req.Edition, d)
		return err
	})
	switch status {
	case 0:
		return PostAdminNostradamusRound201JSONResponse{Id: id}, err
	case 404:
		return PostAdminNostradamusRound404JSONResponse{notFound404(msg)}, nil
	}
	return PostAdminNostradamusRound400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PutAdminNostradamusRound(ctx context.Context, req PutAdminNostradamusRoundRequestObject) (PutAdminNostradamusRoundResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusRound400JSONResponse{badRequest400("the round: label, name, tokens and token_points")}, nil
	}
	d := store.NostradamusRoundDraft{Label: req.Body.Label, Name: req.Body.Name, Tokens: req.Body.Tokens, TokenPoints: req.Body.TokenPoints}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.set", fmt.Sprint(req.Round), req.Body, func(tx Admins) error {
		return tx.SetNostradamusRound(ctx, req.Edition, req.Round, d)
	})
	switch status {
	case 0:
		return PutAdminNostradamusRound204Response{}, err
	case 404:
		return PutAdminNostradamusRound404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PutAdminNostradamusRound409JSONResponse{conflict409(msg)}, nil
	}
	return PutAdminNostradamusRound400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) DeleteAdminNostradamusRound(ctx context.Context, req DeleteAdminNostradamusRoundRequestObject) (DeleteAdminNostradamusRoundResponseObject, error) {
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.remove", fmt.Sprint(req.Round), map[string]any{"edition": req.Edition}, func(tx Admins) error {
		return tx.RemoveNostradamusRound(ctx, req.Edition, req.Round)
	})
	switch status {
	case 0:
		return DeleteAdminNostradamusRound204Response{}, err
	case 404:
		return DeleteAdminNostradamusRound404JSONResponse{notFound404(msg)}, nil
	}
	return DeleteAdminNostradamusRound409JSONResponse{conflict409(msg)}, nil
}

func (s *server) PostAdminNostradamusQuestion(ctx context.Context, req PostAdminNostradamusQuestionRequestObject) (PostAdminNostradamusQuestionResponseObject, error) {
	if req.Body == nil {
		return PostAdminNostradamusQuestion400JSONResponse{badRequest400("the question")}, nil
	}
	var id int64
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.question.add", fmt.Sprint(req.Round), req.Body, func(tx Admins) error {
		var err error
		id, err = tx.AddNostradamusQuestion(ctx, req.Edition, req.Round, questionDraft(*req.Body))
		return err
	})
	switch status {
	case 0:
		return PostAdminNostradamusQuestion201JSONResponse{Id: id}, err
	case 404:
		return PostAdminNostradamusQuestion404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PostAdminNostradamusQuestion409JSONResponse{conflict409(msg)}, nil
	}
	return PostAdminNostradamusQuestion400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PutAdminNostradamusQuestion(ctx context.Context, req PutAdminNostradamusQuestionRequestObject) (PutAdminNostradamusQuestionResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusQuestion400JSONResponse{badRequest400("the question")}, nil
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.question.set", fmt.Sprint(req.Question), req.Body, func(tx Admins) error {
		return tx.SetNostradamusQuestion(ctx, req.Edition, req.Question, questionDraft(*req.Body))
	})
	switch status {
	case 0:
		return PutAdminNostradamusQuestion204Response{}, err
	case 404:
		return PutAdminNostradamusQuestion404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PutAdminNostradamusQuestion409JSONResponse{conflict409(msg)}, nil
	}
	return PutAdminNostradamusQuestion400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) DeleteAdminNostradamusQuestion(ctx context.Context, req DeleteAdminNostradamusQuestionRequestObject) (DeleteAdminNostradamusQuestionResponseObject, error) {
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.question.remove", fmt.Sprint(req.Question), map[string]any{"edition": req.Edition}, func(tx Admins) error {
		return tx.RemoveNostradamusQuestion(ctx, req.Edition, req.Question)
	})
	switch status {
	case 0:
		return DeleteAdminNostradamusQuestion204Response{}, err
	case 404:
		return DeleteAdminNostradamusQuestion404JSONResponse{notFound404(msg)}, nil
	}
	return DeleteAdminNostradamusQuestion409JSONResponse{conflict409(msg)}, nil
}

func (s *server) PostAdminNostradamusPublish(ctx context.Context, req PostAdminNostradamusPublishRequestObject) (PostAdminNostradamusPublishResponseObject, error) {
	if req.Body == nil {
		return PostAdminNostradamusPublish400JSONResponse{badRequest400("the deadline: day, time and zone")}, nil
	}
	closes, err := blitz.FinishedAt(req.Body.Day.String(), req.Body.Time, req.Body.Zone)
	if err != nil {
		return PostAdminNostradamusPublish400JSONResponse{badRequest400(err.Error())}, nil
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.publish", fmt.Sprint(req.Round), req.Body, func(tx Admins) error {
		return tx.PublishNostradamusRound(ctx, req.Edition, req.Round, closes, req.Body.Zone, s.now())
	})
	switch status {
	case 0:
		return PostAdminNostradamusPublish204Response{}, err
	case 404:
		return PostAdminNostradamusPublish404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PostAdminNostradamusPublish409JSONResponse{conflict409(msg)}, nil
	}
	return PostAdminNostradamusPublish400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PostAdminNostradamusUnpublish(ctx context.Context, req PostAdminNostradamusUnpublishRequestObject) (PostAdminNostradamusUnpublishResponseObject, error) {
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.unpublish", fmt.Sprint(req.Round), map[string]any{"edition": req.Edition}, func(tx Admins) error {
		return tx.UnpublishNostradamusRound(ctx, req.Edition, req.Round)
	})
	switch status {
	case 0:
		return PostAdminNostradamusUnpublish204Response{}, err
	case 404:
		return PostAdminNostradamusUnpublish404JSONResponse{notFound404(msg)}, nil
	}
	return PostAdminNostradamusUnpublish409JSONResponse{conflict409(msg)}, nil
}

func (s *server) PutAdminNostradamusDeadline(ctx context.Context, req PutAdminNostradamusDeadlineRequestObject) (PutAdminNostradamusDeadlineResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusDeadline400JSONResponse{badRequest400("the deadline: day, time and zone")}, nil
	}
	closes, err := blitz.FinishedAt(req.Body.Day.String(), req.Body.Time, req.Body.Zone)
	if err != nil {
		return PutAdminNostradamusDeadline400JSONResponse{badRequest400(err.Error())}, nil
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.deadline", fmt.Sprint(req.Round), req.Body, func(tx Admins) error {
		return tx.SetNostradamusDeadline(ctx, req.Edition, req.Round, closes, req.Body.Zone, s.now())
	})
	switch status {
	case 0:
		return PutAdminNostradamusDeadline204Response{}, err
	case 404:
		return PutAdminNostradamusDeadline404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PutAdminNostradamusDeadline409JSONResponse{conflict409(msg)}, nil
	}
	return PutAdminNostradamusDeadline400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PostAdminNostradamusClose(ctx context.Context, req PostAdminNostradamusCloseRequestObject) (PostAdminNostradamusCloseResponseObject, error) {
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.round.close", fmt.Sprint(req.Round), map[string]any{"edition": req.Edition}, func(tx Admins) error {
		return tx.CloseNostradamusRound(ctx, req.Edition, req.Round, s.now())
	})
	switch status {
	case 0:
		return PostAdminNostradamusClose204Response{}, err
	case 404:
		return PostAdminNostradamusClose404JSONResponse{notFound404(msg)}, nil
	}
	return PostAdminNostradamusClose409JSONResponse{conflict409(msg)}, nil
}

func (s *server) PutAdminNostradamusBotAnswers(ctx context.Context, req PutAdminNostradamusBotAnswersRequestObject) (PutAdminNostradamusBotAnswersResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusBotAnswers400JSONResponse{badRequest400("the answers: question, answer and token")}, nil
	}
	answers, ok := givenAnswers(req.Body.Answers)
	if !ok {
		return PutAdminNostradamusBotAnswers400JSONResponse{badRequest400("an answer names no one or nothing")}, nil
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.bot.answers", fmt.Sprint(req.Participant), req.Body, func(tx Admins) error {
		return tx.SetNostradamusBotAnswers(ctx, req.Edition, req.Round, req.Participant, answers, s.now())
	})
	switch status {
	case 0:
		return PutAdminNostradamusBotAnswers204Response{}, err
	case 404:
		return PutAdminNostradamusBotAnswers404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PutAdminNostradamusBotAnswers409JSONResponse{conflict409(msg)}, nil
	}
	return PutAdminNostradamusBotAnswers400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PutAdminNostradamusOutcome(ctx context.Context, req PutAdminNostradamusOutcomeRequestObject) (PutAdminNostradamusOutcomeResponseObject, error) {
	if req.Body == nil || req.Body.Outcome == nil {
		return PutAdminNostradamusOutcome400JSONResponse{badRequest400("the outcome: a list, empty to clear it")}, nil
	}
	outcome := make([]string, len(req.Body.Outcome))
	for i, v := range req.Body.Outcome {
		if outcome[i] = strings.TrimSpace(v); outcome[i] == "" {
			return PutAdminNostradamusOutcome400JSONResponse{badRequest400("an outcome names no one or nothing")}, nil
		}
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.outcome.set", fmt.Sprint(req.Question), map[string]any{"edition": req.Edition, "outcome": outcome},
		func(tx Admins) error { return tx.SetNostradamusOutcome(ctx, req.Edition, req.Question, outcome) })
	switch status {
	case 0:
		return PutAdminNostradamusOutcome204Response{}, err
	case 404:
		return PutAdminNostradamusOutcome404JSONResponse{notFound404(msg)}, nil
	case 409:
		return PutAdminNostradamusOutcome409JSONResponse{conflict409(msg)}, nil
	}
	return PutAdminNostradamusOutcome400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PostAdminNostradamusParticipant(ctx context.Context, req PostAdminNostradamusParticipantRequestObject) (PostAdminNostradamusParticipantResponseObject, error) {
	if req.Body == nil {
		return PostAdminNostradamusParticipant400JSONResponse{badRequest400("the participant: a member, a TTA username, or a bot's name and rule")}, nil
	}
	b := req.Body
	var e store.NostradamusEntrant
	if b.BotRule != nil && *b.BotRule != "" {
		if b.Name == nil || strings.TrimSpace(*b.Name) == "" {
			return PostAdminNostradamusParticipant400JSONResponse{badRequest400("a bot's name")}, nil
		}
		e = store.NostradamusEntrant{Entrant: store.BlitzEntrant{Name: strings.TrimSpace(*b.Name)}, BotRule: string(*b.BotRule)}
	} else {
		in, msg := entrantFrom(b.Member, b.Name, b.DiscordName, b.Email, b.Country, b.Zone)
		if msg != "" {
			return PostAdminNostradamusParticipant400JSONResponse{badRequest400(msg)}, nil
		}
		e = store.NostradamusEntrant{Entrant: in}
	}
	var id int64
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.participant.add", req.Edition, b, func(tx Admins) error {
		var err error
		id, err = tx.AddNostradamusParticipant(ctx, req.Edition, e)
		return err
	})
	switch status {
	case 0:
		return PostAdminNostradamusParticipant201JSONResponse{Id: id}, err
	case 404:
		return PostAdminNostradamusParticipant404JSONResponse{notFound404(msg)}, nil
	}
	return PostAdminNostradamusParticipant400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PutAdminNostradamusStatus(ctx context.Context, req PutAdminNostradamusStatusRequestObject) (PutAdminNostradamusStatusResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusStatus400JSONResponse{badRequest400("the status: accepted or refused")}, nil
	}
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.participant.status", fmt.Sprint(req.Participant), req.Body, func(tx Admins) error {
		return tx.SetNostradamusStatus(ctx, req.Edition, req.Participant, string(req.Body.Status))
	})
	switch status {
	case 0:
		return PutAdminNostradamusStatus204Response{}, err
	case 404:
		return PutAdminNostradamusStatus404JSONResponse{notFound404(msg)}, nil
	}
	return PutAdminNostradamusStatus400JSONResponse{badRequest400(msg)}, nil
}

func (s *server) PutAdminNostradamusMember(ctx context.Context, req PutAdminNostradamusMemberRequestObject) (PutAdminNostradamusMemberResponseObject, error) {
	if req.Body == nil {
		return PutAdminNostradamusMember400JSONResponse{badRequest400("the member: a TTA username, empty to unlink")}, nil
	}
	member := strings.TrimSpace(req.Body.Member)
	status, msg, err := s.nostradamusChange(ctx, "nostradamus.member.link", fmt.Sprint(req.Participant), map[string]any{"edition": req.Edition, "member": member},
		func(tx Admins) error { return tx.LinkNostradamusParticipant(ctx, req.Edition, req.Participant, member) })
	switch status {
	case 0:
		return PutAdminNostradamusMember204Response{}, err
	case 404:
		return PutAdminNostradamusMember404JSONResponse{notFound404(msg)}, nil
	}
	return PutAdminNostradamusMember400JSONResponse{badRequest400(msg)}, nil
}
