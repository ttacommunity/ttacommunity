// The site's challenges: each of the game's challenges with the verified entry holding its record.
package httpapi

import (
	"context"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

// challengeEntry is a stored entry as the challenges package sees it.
func challengeEntry(e store.ChallengeEntry) challenges.Entry {
	return challenges.Entry{ID: e.ID, Player: challenges.Player{Member: e.Member, Name: e.Player}.Key(),
		Domination: e.Domination, Verified: e.ProofURL != "", Bugged: e.Bugged, At: e.At}
}

func (s *server) GetChallenges(ctx context.Context, _ GetChallengesRequestObject) (GetChallengesResponseObject, error) {
	list, err := s.ratings.Challenges(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := s.ratings.ChallengeEntries(ctx)
	if err != nil {
		return nil, err
	}
	byChallenge := map[string][]store.ChallengeEntry{}
	for _, e := range entries {
		byChallenge[e.Challenge] = append(byChallenge[e.Challenge], e)
	}
	body := ChallengeList{Challenges: make([]Challenge, len(list))}
	for i, c := range list {
		body.Challenges[i] = Challenge{Id: c.ID, Set: ChallengeSet(c.Set), Group: c.Group, Slot: c.Slot, Difficulty: c.Difficulty,
			Players: c.Players, Name: c.Name, Record: record(byChallenge[c.ID])}
	}
	return GetChallenges200JSONResponse{Body: body, Headers: GetChallenges200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

// record is the verified entry holding the record among a challenge's entries, nil when none counts.
func record(entries []store.ChallengeEntry) *ChallengeRecord {
	ruled := make([]challenges.Entry, len(entries))
	for i, e := range entries {
		ruled[i] = challengeEntry(e)
	}
	i := challenges.Record(ruled, true)
	if i < 0 {
		return nil
	}
	e := entries[i]
	return &ChallengeRecord{Player: e.Player, Domination: e.Domination, ProofUrl: e.ProofURL, EnteredAt: e.At.UTC()}
}
