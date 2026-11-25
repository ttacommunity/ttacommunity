// Tournament sign-ups: a signed-in player signs up, withdraws and reads where their sign-up stands; admin opens and
// closes a season's sign-ups and enters or removes the ones waiting for an account to be linked.
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

// signUpsOf is a tournament's sign-ups as the API sends them.
func signUpsOf(s store.SignUps) SignUps {
	return SignUps{Open: s.Open, ClosesAt: s.ClosesAt, Taking: s.Taking, Waiting: s.Waiting}
}

// signUpRefusal is the status and message for a player's refused sign-up or withdrawal; 0 for an error that is not
// a refusal.
func signUpRefusal(err error) (int, string) {
	switch {
	case errors.Is(err, store.ErrNoSession):
		return http.StatusUnauthorized, "not signed in"
	case errors.Is(err, store.ErrMemberBanned):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, store.ErrNoBlitzEdition):
		return http.StatusNotFound, "no such tournament"
	case errors.Is(err, store.ErrSignUpsClosed), errors.Is(err, store.ErrNameLinked):
		return http.StatusConflict, err.Error()
	case errors.Is(err, store.ErrSignUpNameNeeded), errors.Is(err, store.ErrBadLinkName):
		return http.StatusBadRequest, store.ErrSignUpNameNeeded.Error()
	}
	return 0, ""
}

func (s *server) GetMySignUp(ctx context.Context, req GetMySignUpRequestObject) (GetMySignUpResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetMySignUp400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	ed, err := s.ratings.BlitzEdition(ctx, req.Tournament, req.Season)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetMySignUp404JSONResponse{notFound404("no such tournament")}, nil
	}
	if err != nil {
		return nil, err
	}
	body := MySignUp{Status: "none", Linked: l.Member != nil}
	if l.Member != nil {
		for _, e := range ed.Entrants {
			if e.MemberID == l.Member.ID {
				body.Status = "entered"
			}
		}
	}
	if body.Status == "none" {
		hash, _ := ctx.Value(sessionKey{}).([]byte)
		p, ok, err := s.signIns.MySignUp(ctx, hash, req.Tournament, req.Season, s.now())
		if err != nil {
			return nil, err
		}
		if ok {
			body.Status, body.Name = "pending", new(p.Name)
		}
	}
	return GetMySignUp200JSONResponse{Body: body, Headers: GetMySignUp200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutMySignUp(ctx context.Context, req PutMySignUpRequestObject) (PutMySignUpResponseObject, error) {
	l, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutMySignUp400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	name := ""
	if req.Body != nil && req.Body.TtaName != nil {
		name = *req.Body.TtaName
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	got, err := s.signIns.SignUp(ctx, hash, req.Tournament, req.Season, name, s.now())
	switch status, msg := signUpRefusal(err); status {
	case http.StatusUnauthorized:
		return PutMySignUp401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return PutMySignUp403JSONResponse{NotLinkedJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return PutMySignUp404JSONResponse{notFound404(msg)}, nil
	case http.StatusConflict:
		return PutMySignUp409JSONResponse{conflict409(msg)}, nil
	case http.StatusBadRequest:
		return PutMySignUp400JSONResponse{badRequest400(msg)}, nil
	}
	if err != nil {
		return nil, err
	}
	body := MySignUp{Status: MySignUpStatus(got), Linked: l.Member != nil}
	if got == "pending" {
		hash, _ := ctx.Value(sessionKey{}).([]byte)
		p, ok, err := s.signIns.MySignUp(ctx, hash, req.Tournament, req.Season, s.now())
		if err != nil {
			return nil, err
		}
		if ok {
			body.Name = new(p.Name)
		}
	}
	return PutMySignUp200JSONResponse{Body: body, Headers: PutMySignUp200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) DeleteMySignUp(ctx context.Context, req DeleteMySignUpRequestObject) (DeleteMySignUpResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteMySignUp400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.WithdrawSignUp(ctx, hash, req.Tournament, req.Season, s.now())
	switch status, msg := signUpRefusal(err); status {
	case http.StatusUnauthorized:
		return DeleteMySignUp401JSONResponse{signedOut401(msg)}, nil
	case http.StatusForbidden:
		return DeleteMySignUp403JSONResponse{NotLinkedJSONResponse{Error: msg}}, nil
	case http.StatusNotFound:
		return DeleteMySignUp404JSONResponse{notFound404(msg)}, nil
	case http.StatusConflict:
		return DeleteMySignUp409JSONResponse{conflict409(msg)}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteMySignUp204Response{}, nil
}

func (s *server) GetAdminSignUps(ctx context.Context, req GetAdminSignUpsRequestObject) (GetAdminSignUpsResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return GetAdminSignUps400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	state, err := s.admins.SignUpsOf(ctx, req.Tournament, req.Season)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return GetAdminSignUps404JSONResponse{notFound404("no such season")}, nil
	}
	if err != nil {
		return nil, err
	}
	pending, err := s.admins.PendingSignUps(ctx, req.Tournament, req.Season)
	if err != nil {
		return nil, err
	}
	body := AdminSignUps{SignUps: signUpsOf(state), Pending: make([]AdminPendingSignUp, len(pending))}
	for i, p := range pending {
		body.Pending[i] = AdminPendingSignUp{Id: p.ID, Account: p.Account, Name: p.Name, SignedUpAt: p.SignedUpAt, Linked: nullableText(p.Linked)}
		if p.Request != 0 {
			body.Pending[i].Request = new(p.Request)
		}
	}
	return GetAdminSignUps200JSONResponse(body), nil
}

func (s *server) PutAdminSignUps(ctx context.Context, req PutAdminSignUpsRequestObject) (PutAdminSignUpsResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminSignUps404JSONResponse{notFound404("no such season")}, nil
	}
	if req.Body == nil {
		return PutAdminSignUps400JSONResponse{badRequest400("open, and closes_at or none")}, nil
	}
	set := store.SignUps{Open: req.Body.Open}
	if c := req.Body.Closes; c != nil {
		at, err := blitz.FinishedAt(c.Day.Format(time.DateOnly), c.Time, strings.TrimSpace(c.Zone))
		if err != nil {
			return PutAdminSignUps400JSONResponse{badRequest400(err.Error())}, nil
		}
		at = at.UTC()
		set.ClosesAt = &at
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetSignUps(ctx, req.Tournament, req.Season, set, s.now()); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "sign_ups.set", blitzEditionKey(req.Tournament, req.Season),
			map[string]any{"open": set.Open, "closes_at": set.ClosesAt})
	})
	switch {
	case errors.Is(err, store.ErrSignUpsNotOffered), errors.Is(err, store.ErrBadSignUps):
		return PutAdminSignUps400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrSignUpsHidden), errors.Is(err, store.ErrSignUpsStarted):
		return PutAdminSignUps409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminSignUps404JSONResponse{notFound404("no such season")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminSignUps204Response{}, nil
}

func (s *server) PostAdminSignUpAccept(ctx context.Context, req PostAdminSignUpAcceptRequestObject) (PostAdminSignUpAcceptResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminSignUpAccept404JSONResponse{notFound404("no such season")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.AcceptSignUp(ctx, req.Tournament, req.Season, req.SignUp, s.now()); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "sign_up.accept", fmt.Sprint(req.SignUp), map[string]any{"season": blitzEditionKey(req.Tournament, req.Season)})
	})
	switch {
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminSignUpAccept404JSONResponse{notFound404("no such season")}, nil
	case errors.Is(err, store.ErrNoSignUp):
		return PostAdminSignUpAccept404JSONResponse{notFound404("no such sign-up")}, nil
	case errors.Is(err, store.ErrNoRequest):
		return PostAdminSignUpAccept409JSONResponse{conflict409("the account has no request to be linked waiting: link it in Accounts first")}, nil
	case errors.Is(err, store.ErrMemberBanned), errors.Is(err, store.ErrMemberHasAccount), errors.Is(err, store.ErrAlreadyLinked):
		return PostAdminSignUpAccept409JSONResponse{conflict409(err.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminSignUpAccept204Response{}, nil
}

func (s *server) DeleteAdminSignUp(ctx context.Context, req DeleteAdminSignUpRequestObject) (DeleteAdminSignUpResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminSignUp404JSONResponse{notFound404("no such season")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveSignUp(ctx, req.Tournament, req.Season, req.SignUp); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "sign_up.remove", fmt.Sprint(req.SignUp), map[string]any{"season": blitzEditionKey(req.Tournament, req.Season)})
	})
	if errors.Is(err, store.ErrNoSignUp) {
		return DeleteAdminSignUp404JSONResponse{notFound404("no such sign-up")}, nil
	}
	if err != nil {
		return nil, err
	}
	return DeleteAdminSignUp204Response{}, nil
}
