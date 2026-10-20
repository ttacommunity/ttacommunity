// Player sign-in by an emailed link, and a signed-in player adding or removing a way in. The link goes to a site
// page, which posts its token here when the player asks, so a mail scanner opening the link spends nothing.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/turnstile"
)

// emailPage is the site page an emailed link opens; the token follows in the fragment, which no server sees.
const emailPage = "/account/email/#"

// linkSent is how sending a link ended, for each route to answer in its own types.
type linkSent int

const (
	linkOK linkSent = iota
	linkTooMany
	linkTaken
	linkHasKind
	linkNotSent
)

// sendLink records a link to address (to add it to account, when set) coming back to back, and emails it.
func (s *server) sendLink(ctx context.Context, address string, account *int64, back string) (linkSent, error) {
	token := newSecret()
	err := s.signIns.NewEmailLink(ctx, address, account, back, hashToken(token), s.now())
	switch {
	case errors.Is(err, store.ErrTooManyLinks):
		return linkTooMany, nil
	case errors.Is(err, store.ErrLoginTaken):
		return linkTaken, nil
	case errors.Is(err, store.ErrHasLoginOfKind):
		return linkHasKind, nil
	case err != nil:
		return 0, err
	}
	if err := s.signIns.SendSignInLink(ctx, address, s.signIns.SiteOrigin()+emailPage+token); err != nil {
		s.log.Error("sending a sign-in link failed", "request_id", requestID(ctx), "err", err)
		return linkNotSent, nil
	}
	return linkOK, nil
}

func notSetUp503(msg string) NotSetUpJSONResponse {
	return NotSetUpJSONResponse{Body: Error{Error: msg}, Headers: NotSetUpResponseHeaders{CacheControl: cacheNone}}
}

func tooMany429() TooManyLinksJSONResponse {
	return TooManyLinksJSONResponse{Body: Error{Error: store.ErrTooManyLinks.Error()}, Headers: TooManyLinksResponseHeaders{CacheControl: cacheNone}}
}

const (
	emailNotSetUp = "signing in by email is not set up here"
	emailNotSent  = "the email could not be sent: try again later"
	takenMessage  = "that email address has an account of its own here: sign in with it and delete that account, then add it"
	hasKindEmail  = "your account has an email address already: remove it first"
)

func (s *server) SendEmailSignIn(ctx context.Context, req SendEmailSignInRequestObject) (SendEmailSignInResponseObject, error) {
	if !s.signIns.EmailReady() {
		return SendEmailSignIn503JSONResponse{notSetUp503(emailNotSetUp)}, nil
	}
	if req.Body == nil {
		return SendEmailSignIn400JSONResponse{badRequest400("an email address and the Turnstile token: email, turnstile")}, nil
	}
	address, err := store.EmailAddress(req.Body.Email)
	if err != nil {
		return SendEmailSignIn400JSONResponse{badRequest400(err.Error())}, nil
	}
	if err := s.signIns.VerifyTurnstile(ctx, req.Body.Turnstile); errors.Is(err, turnstile.ErrFailed) {
		return SendEmailSignIn400JSONResponse{badRequest400("the check that you are a person did not pass: try again")}, nil
	} else if err != nil {
		s.log.Error("checking a Turnstile token failed", "request_id", requestID(ctx), "err", err)
		return SendEmailSignIn503JSONResponse{notSetUp503("the check that you are a person could not be made: try again later")}, nil
	}
	sent, err := s.sendLink(ctx, address, nil, returnPath(req.Body.Return))
	switch {
	case err != nil:
		return nil, err
	case sent == linkTooMany:
		return SendEmailSignIn429JSONResponse{tooMany429()}, nil
	case sent == linkNotSent:
		return SendEmailSignIn503JSONResponse{notSetUp503(emailNotSent)}, nil
	}
	return SendEmailSignIn204Response{}, nil
}

// emailSignedIn answers an opened link: the session cookie, and where to go.
type emailSignedIn struct {
	body   EmailSignedIn
	cookie *http.Cookie
}

func (e emailSignedIn) VisitFinishEmailSignInResponse(w http.ResponseWriter) error {
	http.SetCookie(w, e.cookie)
	w.Header().Set("Cache-Control", cacheNone)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(e.body)
}

func (s *server) FinishEmailSignIn(ctx context.Context, req FinishEmailSignInRequestObject) (FinishEmailSignInResponseObject, error) {
	if !s.signIns.EmailReady() {
		return FinishEmailSignIn503JSONResponse{notSetUp503(emailNotSetUp)}, nil
	}
	if req.Body == nil {
		return FinishEmailSignIn400JSONResponse{badRequest400("the link's token: token")}, nil
	}
	token := newSecret()
	got, err := s.signIns.UseEmailLink(ctx, hashToken(req.Body.Token), hashToken(token), s.now())
	switch {
	case errors.Is(err, store.ErrLinkNotValid):
		return FinishEmailSignIn410JSONResponse{Body: Error{Error: err.Error()}, Headers: FinishEmailSignIn410ResponseHeaders{CacheControl: cacheNone}}, nil
	case errors.Is(err, store.ErrLoginTaken):
		return FinishEmailSignIn409JSONResponse{conflict409(takenMessage)}, nil
	case errors.Is(err, store.ErrHasLoginOfKind):
		return FinishEmailSignIn409JSONResponse{conflict409(hasKindEmail)}, nil
	case err != nil:
		return nil, err
	}
	outcome := Ok
	switch {
	case got.Added:
		outcome = Added
	case got.First:
		outcome = First
	}
	return emailSignedIn{body: EmailSignedIn{Outcome: outcome, Back: returnPath(&got.Back)}, cookie: sessionCookieWith(token, int(cookieLife.Seconds()))}, nil
}

func (s *server) PostMyEmailLogin(ctx context.Context, req PostMyEmailLoginRequestObject) (PostMyEmailLoginResponseObject, error) {
	a, err := accountFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !s.signIns.EmailReady() {
		return PostMyEmailLogin503JSONResponse{notSetUp503(emailNotSetUp)}, nil
	}
	if req.Body == nil {
		return PostMyEmailLogin400JSONResponse{badRequest400("an email address: email")}, nil
	}
	address, err := store.EmailAddress(req.Body.Email)
	if err != nil {
		return PostMyEmailLogin400JSONResponse{badRequest400(err.Error())}, nil
	}
	sent, err := s.sendLink(ctx, address, &a.ID, accountPath)
	switch {
	case err != nil:
		return nil, err
	case sent == linkTooMany:
		return PostMyEmailLogin429JSONResponse{tooMany429()}, nil
	case sent == linkTaken:
		return PostMyEmailLogin409JSONResponse{conflict409(takenMessage)}, nil
	case sent == linkHasKind:
		return PostMyEmailLogin409JSONResponse{conflict409(hasKindEmail)}, nil
	case sent == linkNotSent:
		return PostMyEmailLogin503JSONResponse{notSetUp503(emailNotSent)}, nil
	}
	return PostMyEmailLogin204Response{}, nil
}

func (s *server) DeleteMyLogin(ctx context.Context, req DeleteMyLoginRequestObject) (DeleteMyLoginResponseObject, error) {
	if !req.Provider.Valid() {
		return DeleteMyLogin404JSONResponse{notFound404("no such way in: discord or email")}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	err := s.signIns.RemoveOwnLogin(ctx, hash, string(req.Provider), s.now())
	switch {
	case errors.Is(err, store.ErrNoSession):
		return DeleteMyLogin401JSONResponse{SignedOutJSONResponse{Body: Error{Error: "not signed in"}, Headers: SignedOutResponseHeaders{CacheControl: cacheNone}}}, nil
	case errors.Is(err, store.ErrNoLogin):
		return DeleteMyLogin404JSONResponse{notFound404("your account has no such way in")}, nil
	case errors.Is(err, store.ErrLastLogin):
		return DeleteMyLogin409JSONResponse{conflict409("your only way in cannot be removed: Delete account removes it with the account")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteMyLogin204Response{}, nil
}
