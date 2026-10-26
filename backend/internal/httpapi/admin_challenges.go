// Admin's challenges: every entry, a page at a time, for admins and organisers.
package httpapi

import (
	"context"

	"ttacommunity/backend/internal/store"
)

// challengeEntriesPage is how many entries a page of admin's list holds.
const challengeEntriesPage = 100

func adminChallengeEntry(e store.AdminChallengeEntry) AdminChallengeEntry {
	return AdminChallengeEntry{Id: e.ID, Challenge: e.Challenge, ChallengeName: e.ChallengeName, Player: e.Player, Member: e.Member,
		EnteredAs: e.EnteredAs, Account: e.Account, Domination: e.Domination, ProofUrl: nullableText(e.ProofURL),
		EnteredAt: e.At.UTC(), Source: AdminChallengeEntrySource(e.Source), Hidden: e.Hidden, HiddenReason: e.HiddenReason,
		Bugged: e.Bugged, Checked: e.Checked}
}

func (s *server) GetAdminChallengeEntries(ctx context.Context, req GetAdminChallengeEntriesRequestObject) (GetAdminChallengeEntriesResponseObject, error) {
	page := 1
	if req.Params.Page != nil {
		page = *req.Params.Page
	}
	if page < 1 {
		return GetAdminChallengeEntries400JSONResponse{badRequest400("page is a whole number from 1")}, nil
	}
	got, err := s.admins.ChallengeEntryPage(ctx, page, challengeEntriesPage)
	if err != nil {
		return nil, err
	}
	body := AdminChallengeEntries{Total: got.Total, Page: page, PageSize: challengeEntriesPage, Entries: make([]AdminChallengeEntry, len(got.Entries))}
	for i, e := range got.Entries {
		body.Entries[i] = adminChallengeEntry(e)
	}
	return GetAdminChallengeEntries200JSONResponse{Body: body, Headers: GetAdminChallengeEntries200ResponseHeaders{CacheControl: cacheNone}}, nil
}
