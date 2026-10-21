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

// blitzReport is where a match's report stands as the API sends it, nil when no player reported it.
func blitzReport(m store.BlitzMatch) *BlitzReport {
	if m.Report == "" {
		return nil
	}
	r := BlitzReport(m.Report)
	return &r
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
		reportedBy := ""
		if m.ReportedBy != nil {
			reportedBy = "b"
			if *m.ReportedBy == m.A {
				reportedBy = "a"
			}
		}
		out := &MyBlitzMatch{Id: m.ID, Round: r.Number, A: names[m.A], Me: MyBlitzMatchMe(me), Result: blitzResult(m), Game: playedGame(m),
			Ruled: m.Ruling != "", Report: blitzReport(m), FlagReason: m.FlagReason, Actions: []MyBlitzMatchActions{}}
		if m.B != nil {
			out.B = new(names[*m.B])
		}
		if reportedBy != "" {
			out.ReportedBy = new(MyBlitzMatchReportedBy(reportedBy))
		}
		closed := ed.PairedRound != nil && *ed.PairedRound > r.Number
		for _, a := range blitz.PlayerActions(blitz.PlayerMatch{Me: me, ReportedBy: reportedBy, Report: m.Report, Decided: m.Result != "",
			Bye: m.B == nil, Closed: closed}) {
			out.Actions = append(out.Actions, MyBlitzMatchActions(a))
		}
		return out
	}
	return nil
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
	var body MyBlitz
	if l.Member != nil {
		body.Match = myBlitzMatch(ed, l.Member.ID)
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
		errors.Is(err, store.ErrBlitzBye):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, store.ErrNoSession):
		return http.StatusUnauthorized, "not signed in"
	case errors.Is(err, store.ErrNotLinked), errors.Is(err, store.ErrMemberBanned), errors.Is(err, store.ErrNotYourMatch):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, store.ErrNoBlitzEdition):
		return http.StatusNotFound, "no such blitz edition"
	case errors.Is(err, store.ErrNoBlitzMatch):
		return http.StatusNotFound, "no such match"
	case errors.Is(err, store.ErrBlitzRoundClosed), errors.Is(err, store.ErrBlitzResultLocked):
		return http.StatusConflict, err.Error()
	}
	return 0, ""
}

func signedOut401(msg string) SignedOutJSONResponse {
	return SignedOutJSONResponse{Body: Error{Error: msg}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}
}

func (s *server) PutMyBlitzGame(ctx context.Context, req PutMyBlitzGameRequestObject) (PutMyBlitzGameResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutMyBlitzGame400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutMyBlitzGame400JSONResponse{badRequest400("the game: share_code, a, b, finished_on, finished_time and zone")}, nil
	}
	finished, err := blitz.FinishedAt(req.Body.FinishedOn.Format(time.DateOnly), req.Body.FinishedTime, strings.TrimSpace(req.Body.Zone))
	if err != nil {
		return PutMyBlitzGame400JSONResponse{badRequest400(err.Error())}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	g := store.BlitzGame{ShareCode: req.Body.ShareCode, Finished: finished,
		A: store.BlitzSide{Culture: req.Body.A.Culture, Outcome: string(req.Body.A.Outcome)},
		B: store.BlitzSide{Culture: req.Body.B.Culture, Outcome: string(req.Body.B.Outcome)}}
	err = s.signIns.ReportBlitzGame(ctx, hash, req.Tournament, req.Edition, req.Match, g, s.now())
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
	err := s.signIns.ConfirmBlitzResult(ctx, hash, req.Tournament, req.Edition, req.Match, s.now())
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
	err := s.signIns.FlagBlitzResult(ctx, hash, req.Tournament, req.Edition, req.Match, req.Body.Reason, s.now())
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

func (s *server) PostAdminBlitzMatchKeep(ctx context.Context, req PostAdminBlitzMatchKeepRequestObject) (PostAdminBlitzMatchKeepResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	switch err := s.admins.KeepBlitzReport(ctx, req.Tournament, req.Edition, req.Match); {
	case errors.Is(err, store.ErrBlitzNotFlagged):
		return PostAdminBlitzMatchKeep409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzMatch):
		return PostAdminBlitzMatchKeep404JSONResponse{notFound404("no such match")}, nil
	case err != nil:
		return nil, err
	}
	if err := s.admins.Audit(ctx, me.DiscordID, "blitz.match.keep", fmt.Sprint(req.Match), map[string]any{}); err != nil {
		return nil, err
	}
	return PostAdminBlitzMatchKeep204Response{}, nil
}
