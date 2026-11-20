// Players' blitz reports: a signed-in player reads their own match in a shown edition's latest round, reports its
// game (or corrects their report), and confirms or flags the opponent's report. Admin keeps a flagged report.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// blitzReport is where a match's first game's report stands as the API sends it, nil when no player reported it.
func blitzReport(m store.BlitzMatch) *BlitzReport { return reportOf(m.Report) }

// reportOf is where a report stands as the API sends it, nil for none.
func reportOf(report string) *BlitzReport {
	if report == "" {
		return nil
	}
	return new(BlitzReport(report))
}

// sideOf is which side of match m an entrant is, "" for neither or none.
func sideOf(m store.BlitzMatch, entry *int64) string {
	switch {
	case entry == nil:
		return ""
	case *entry == m.A:
		return "a"
	case m.B != nil && *entry == *m.B:
		return "b"
	}
	return ""
}

// myBlitzMatch finds the match of member in the edition's latest published round, as that player sees it; nil when
// they have none there.
func myBlitzMatch(ed store.BlitzEdition, member int64) *MyBlitzMatch {
	if len(ed.Rounds) == 0 {
		return nil
	}
	names, entry := map[int64]string{}, int64(0)
	for _, e := range ed.Entrants {
		names[e.ID] = e.Name
		if e.MemberID == member {
			entry = e.ID
		}
	}
	r := ed.Rounds[len(ed.Rounds)-1]
	for _, m := range r.Matches {
		me := ""
		switch {
		case entry != 0 && m.A == entry:
			me = "a"
		case entry != 0 && m.B != nil && *m.B == entry:
			me = "b"
		default:
			continue
		}
		reportedBy := sideOf(m, m.ReportedBy)
		out := &MyBlitzMatch{Id: m.ID, Round: r.Number, A: names[m.A], Me: MatchSide(me), Result: blitzResult(m), Game: playedGame(m),
			Ruled: m.Ruling != "", Report: blitzReport(m), FlagReason: m.FlagReason, Actions: []MyBlitzMatchActions{},
			BestOf: m.BestOf, BestOfOffered: r.BestOf == 3, BestOfActions: []BlitzBestOfAction{}, Games: make([]MyBlitzGame, len(m.Games))}
		if m.B != nil {
			out.B = new(names[*m.B])
		}
		if reportedBy != "" {
			out.ReportedBy = new(MatchSide(reportedBy))
		}
		closed := ed.PairedRound != nil && *ed.PairedRound > r.Number
		for _, a := range blitz.PlayerActions(blitz.PlayerMatch{Me: me, ReportedBy: reportedBy, Report: m.Report, Decided: m.Result != "",
			Bye: m.B == nil, Closed: closed}) {
			out.Actions = append(out.Actions, MyBlitzMatchActions(a))
		}
		for i, g := range m.Games {
			by := sideOf(m, g.ReportedBy)
			mg := MyBlitzGame{Number: g.Number, Result: BlitzGameResult(g.Result), Game: apiGame(g.Game), Report: reportOf(g.Report),
				FlagReason: g.FlagReason, Actions: []MyBlitzGameActions{}}
			if by != "" {
				mg.ReportedBy = new(MatchSide(by))
			}
			for _, a := range blitz.PlayerActions(blitz.PlayerMatch{Me: me, ReportedBy: by, Report: g.Report, Decided: true, Bye: m.B == nil, Closed: closed}) {
				mg.Actions = append(mg.Actions, MyBlitzGameActions(a))
			}
			out.Games[i] = mg
		}
		state := blitz.BestOfMatch{Me: me, ProposedBy: sideOf(m, m.ProposedBy), Offered: r.BestOf == 3, BestOf: m.BestOf, Played: len(m.Games),
			Decided: m.Result != "", Bye: m.B == nil, Closed: closed}
		if p := state.ProposedBy; p != "" {
			out.ProposedBy = new(MatchSide(p))
		}
		for _, a := range blitz.BestOfActions(state) {
			out.BestOfActions = append(out.BestOfActions, BlitzBestOfAction(a))
		}
		if n := blitz.NextGame(state); n > 0 {
			out.NextGame = &n
		}
		return out
	}
	return nil
}

// blitzReports lists who reported each game of the edition, round by round, when member is entered in it; empty
// otherwise.
func blitzReports(ed store.BlitzEdition, member int64) []BlitzMatchReport {
	out := []BlitzMatchReport{}
	names, entered := map[int64]string{}, false
	for _, e := range ed.Entrants {
		names[e.ID] = e.Name
		entered = entered || e.MemberID == member
	}
	if !entered {
		return out
	}
	for _, r := range ed.Rounds {
		for _, m := range r.Matches {
			for _, g := range m.Games {
				if g.ReportedBy != nil {
					out = append(out, BlitzMatchReport{Round: r.Number, Game: g.Number, A: names[m.A], ReportedBy: names[*g.ReportedBy], Confirmed: g.Report == "confirmed"})
				}
			}
		}
	}
	return out
}

func (s *server) GetMyBlitzMatch(ctx context.Context, req GetMyBlitzMatchRequestObject) (GetMyBlitzMatchResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetMyBlitzMatch400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	ed, err := s.ratings.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetMyBlitzMatch404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := MyBlitz{Reports: []BlitzMatchReport{}}
	if l.Member != nil {
		body.Match = myBlitzMatch(ed, l.Member.ID)
		body.Reports = blitzReports(ed, l.Member.ID)
	}
	return GetMyBlitzMatch200JSONResponse{Body: body, Headers: GetMyBlitzMatch200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// reportRefusal is the status and message for a player's refused report, confirmation or flag; 0 for an error that is
// not a refusal.
func reportRefusal(err error) (int, string) {
	var taken *store.BlitzShareCodeError
	switch {
	case errors.As(err, &taken):
		return http.StatusConflict, "that share code is already on another game"
	case errors.Is(err, store.ErrBlitzShareCodeNeeded), errors.Is(err, store.ErrBadBlitzGame), errors.Is(err, store.ErrBadBlitzFlag),
		errors.Is(err, store.ErrBlitzBye), errors.Is(err, store.ErrBlitzFinishedAhead):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrNoSession):
		return http.StatusUnauthorized, "not signed in"
	case errors.Is(err, store.ErrNotLinked), errors.Is(err, store.ErrMemberBanned), errors.Is(err, store.ErrNotYourMatch):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, store.ErrNoBlitzEdition):
		return http.StatusNotFound, "no such blitz edition"
	case errors.Is(err, store.ErrNoBlitzMatch):
		return http.StatusNotFound, "no such match"
	case errors.Is(err, store.ErrNoBlitzGame):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, store.ErrBadBlitzBestOf):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrBlitzRoundClosed), errors.Is(err, store.ErrBlitzResultLocked), errors.Is(err, store.ErrBlitzGameNotNeeded),
		errors.Is(err, store.ErrBlitzBestOfNotOffered), errors.Is(err, store.ErrBlitzBestOfClosed):
		return http.StatusConflict, err.Error()
	}
	return 0, ""
}

// blitzGame is the game a player reports or an admin records, or the message refusing it: no game, or a finish
// that cannot be read or is after now.
func blitzGame(b *BlitzGameRecord, now time.Time) (store.BlitzGame, string) {
	if b == nil {
		return store.BlitzGame{}, "the game: share_code, a, b, finished_on, finished_time and zone"
	}
	finished, err := blitz.FinishedAt(b.FinishedOn.Format(time.DateOnly), b.FinishedTime, strings.TrimSpace(b.Zone))
	if err == nil {
		err = blitz.FinishedBy(finished, now)
	}
	if err != nil {
		return store.BlitzGame{}, err.Error()
	}
	return store.BlitzGame{ShareCode: b.ShareCode, Finished: finished,
		A: store.BlitzSide{Culture: b.A.Culture, Outcome: string(b.A.Outcome), Turn: b.A.Turn},
		B: store.BlitzSide{Culture: b.B.Culture, Outcome: string(b.B.Outcome), Turn: b.B.Turn}}, ""
}

func signedOut401(msg string) SignedOutJSONResponse {
	return SignedOutJSONResponse{Body: Error{Error: msg}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}
}

func (s *server) PutMyBlitzGame(ctx context.Context, req PutMyBlitzGameRequestObject) (PutMyBlitzGameResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutMyBlitzGame400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	g, msg := blitzGame(req.Body, s.now())
	if msg != "" {
		return PutMyBlitzGame400JSONResponse{badRequest400(msg)}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.ReportBlitzGame(ctx, hash, req.Tournament, req.Edition, req.Match, req.Game, g, s.now())
	switch code, msg := reportRefusal(err); code {
	case 0:
		if err != nil {
			return nil, err
		}
		return PutMyBlitzGame204Response{}, nil
	case http.StatusBadRequest:
		return PutMyBlitzGame400JSONResponse{badRequest400(msg)}, nil
	case http.StatusUnauthorized:
		return PutMyBlitzGame401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return PutMyBlitzGame403JSONResponse{NotYourMatchJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return PutMyBlitzGame404JSONResponse{notFound404(msg)}, nil
	default:
		return PutMyBlitzGame409JSONResponse{conflict409(msg)}, nil
	}
}

func (s *server) PostMyBlitzConfirm(ctx context.Context, req PostMyBlitzConfirmRequestObject) (PostMyBlitzConfirmResponseObject, error) {
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.ConfirmBlitzResult(ctx, hash, req.Tournament, req.Edition, req.Match, req.Game, s.now())
	switch code, msg := reportRefusal(err); code {
	case 0:
		if err != nil {
			return nil, err
		}
		return PostMyBlitzConfirm204Response{}, nil
	case http.StatusUnauthorized:
		return PostMyBlitzConfirm401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return PostMyBlitzConfirm403JSONResponse{NotYourMatchJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return PostMyBlitzConfirm404JSONResponse{notFound404(msg)}, nil
	default:
		// A bye has no report to confirm: as good as locked.
		return PostMyBlitzConfirm409JSONResponse{conflict409(msg)}, nil
	}
}

func (s *server) PostMyBlitzFlag(ctx context.Context, req PostMyBlitzFlagRequestObject) (PostMyBlitzFlagResponseObject, error) {
	if req.Body == nil {
		return PostMyBlitzFlag400JSONResponse{badRequest400("what is wrong: reason")}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.FlagBlitzResult(ctx, hash, req.Tournament, req.Edition, req.Match, req.Game, req.Body.Reason, s.now())
	switch code, msg := reportRefusal(err); code {
	case 0:
		if err != nil {
			return nil, err
		}
		return PostMyBlitzFlag204Response{}, nil
	case http.StatusBadRequest:
		return PostMyBlitzFlag400JSONResponse{badRequest400(msg)}, nil
	case http.StatusUnauthorized:
		return PostMyBlitzFlag401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return PostMyBlitzFlag403JSONResponse{NotYourMatchJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return PostMyBlitzFlag404JSONResponse{notFound404(msg)}, nil
	default:
		return PostMyBlitzFlag409JSONResponse{conflict409(msg)}, nil
	}
}

func (s *server) PostMyBlitzBestOf(ctx context.Context, req PostMyBlitzBestOfRequestObject) (PostMyBlitzBestOfResponseObject, error) {
	if req.Body == nil || !req.Body.Answer.Valid() {
		return PostMyBlitzBestOf400JSONResponse{badRequest400("the answer: propose, accept, decline or withdraw")}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.AnswerBlitzBestOf(ctx, hash, req.Tournament, req.Edition, req.Match, string(req.Body.Answer), s.now())
	switch code, msg := reportRefusal(err); code {
	case 0:
		if err != nil {
			return nil, err
		}
		return PostMyBlitzBestOf204Response{}, nil
	case http.StatusBadRequest:
		return PostMyBlitzBestOf400JSONResponse{badRequest400(msg)}, nil
	case http.StatusUnauthorized:
		return PostMyBlitzBestOf401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return PostMyBlitzBestOf403JSONResponse{NotYourMatchJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return PostMyBlitzBestOf404JSONResponse{notFound404(msg)}, nil
	default:
		return PostMyBlitzBestOf409JSONResponse{conflict409(msg)}, nil
	}
}

func (s *server) PostAdminBlitzMatchKeep(ctx context.Context, req PostAdminBlitzMatchKeepRequestObject) (PostAdminBlitzMatchKeepResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.KeepBlitzReport(ctx, req.Tournament, req.Edition, req.Match, req.Game); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.match.keep", fmt.Sprint(req.Match), map[string]any{"game": req.Game})
	})
	switch {
	case errors.Is(err, store.ErrBlitzNotFlagged):
		return PostAdminBlitzMatchKeep409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBlitzMatchKeep204Response{}, nil
}
