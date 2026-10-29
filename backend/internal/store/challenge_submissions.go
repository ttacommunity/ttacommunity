// Players' challenge entries: a signed-in player linked to a member submits their own dominations, through the
// database's submit_challenge_entries, which checks the session, the link and the challenges.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ttacommunity/backend/internal/challenges"
)

// ErrBadChallengeEntries means a submission the database refused: a challenge not in the list, one challenge twice,
// or a proof that is not a message in the TtA Discord.
var ErrBadChallengeEntries = errors.New("an entry names a challenge not in the list, or one twice, or links a proof from elsewhere")

// SubmitChallengeEntries saves entries, checked by challenges.CheckSubmissions, as the member the session hashing to
// tokenHash is linked to, at now, and returns how many it saved. It returns ErrNoSession, ErrNotLinked,
// ErrMemberBanned or ErrBadChallengeEntries, saving none.
func (s *PlayerStore) SubmitChallengeEntries(ctx context.Context, tokenHash []byte, entries []challenges.Submission, now time.Time) (int, error) {
	type entry struct {
		Challenge  string  `json:"challenge"`
		Domination int     `json:"domination"`
		ProofURL   *string `json:"proof_url"`
	}
	list := make([]entry, len(entries))
	for i, e := range entries {
		list[i] = entry{Challenge: e.Challenge, Domination: e.Domination}
		if e.ProofURL != "" {
			list[i].ProofURL = &e.ProofURL
		}
	}
	body, err := json.Marshal(list)
	if err != nil {
		return 0, fmt.Errorf("submitting challenge entries: %w", err)
	}
	var got string
	if err := s.pool.QueryRow(ctx, "SELECT submit_challenge_entries($1, $2, $3)", tokenHash, now, body).Scan(&got); err != nil {
		return 0, fmt.Errorf("submitting challenge entries: %w", err)
	}
	if n, ok := strings.CutPrefix(got, "saved:"); ok {
		return strconv.Atoi(n)
	}
	switch got {
	case "signed-out":
		return 0, ErrNoSession
	case "not-linked":
		return 0, ErrNotLinked
	case "banned":
		return 0, ErrMemberBanned
	case "bad":
		return 0, ErrBadChallengeEntries
	}
	return 0, fmt.Errorf("submitting challenge entries: unexpected answer %q", got)
}
