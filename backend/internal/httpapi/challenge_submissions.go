// A player's own challenge entries: a signed-in player linked to a member submits their dominations.
package httpapi

import (
	"context"
	"errors"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

func (s *server) PostMyChallengeEntries(ctx context.Context, req PostMyChallengeEntriesRequestObject) (PostMyChallengeEntriesResponseObject, error) {
	if req.Body == nil {
		return PostMyChallengeEntries400JSONResponse{badRequest400("the entries: challenge, domination and proof_url")}, nil
	}
	asked := make([]challenges.Submission, len(req.Body.Entries))
	for i, e := range req.Body.Entries {
		asked[i] = challenges.Submission{Challenge: e.Challenge, Domination: e.Domination}
		if e.ProofUrl != nil {
			asked[i].ProofURL = *e.ProofUrl
		}
	}
	entries, msg := challenges.CheckSubmissions(asked)
	if msg != "" {
		return PostMyChallengeEntries400JSONResponse{badRequest400(msg)}, nil
	}
	hash, _ := ctx.Value(sessionKey{}).([]byte)
	saved, err := s.signIns.SubmitChallengeEntries(ctx, hash, entries, s.now())
	switch {
	case err == nil:
		return PostMyChallengeEntries201JSONResponse{Saved: saved}, nil
	case errors.Is(err, store.ErrBadChallengeEntries):
		return PostMyChallengeEntries400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoSession):
		return PostMyChallengeEntries401JSONResponse{signedOut401("not signed in")}, nil
	case errors.Is(err, store.ErrNotLinked), errors.Is(err, store.ErrMemberBanned):
		return PostMyChallengeEntries403JSONResponse{NotLinkedJSONResponse{Error: err.Error()}}, nil
	}
	return nil, err
}
