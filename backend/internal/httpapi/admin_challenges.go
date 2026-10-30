// Admin's challenges: every entry by view, a page at a time, with what looks wrong about it; an entry changed; a
// name merged into another or into a member. Changes are audited.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

// challengeEntriesPage is how many entries a page of admin's list holds.
const challengeEntriesPage = 100

// challengeViews are admin's views in the order its tabs show them, each with the entries it holds.
var challengeViews = []struct {
	view AdminChallengeView
	in   func(e store.AdminChallengeEntry, flagged bool) bool
}{
	{AdminChallengeViewAll, func(store.AdminChallengeEntry, bool) bool { return true }},
	{AdminChallengeViewUnchecked, func(e store.AdminChallengeEntry, _ bool) bool { return !e.Hidden && e.ProofURL != "" && !e.Checked }},
	{AdminChallengeViewChecked, func(e store.AdminChallengeEntry, _ bool) bool { return !e.Hidden && e.ProofURL != "" && e.Checked }},
	{AdminChallengeViewUnverified, func(e store.AdminChallengeEntry, _ bool) bool { return !e.Hidden && e.ProofURL == "" }},
	{AdminChallengeViewFlagged, func(e store.AdminChallengeEntry, flagged bool) bool { return !e.Hidden && flagged }},
	{AdminChallengeViewBugged, func(e store.AdminChallengeEntry, _ bool) bool { return !e.Hidden && e.Bugged }},
	{AdminChallengeViewHidden, func(e store.AdminChallengeEntry, _ bool) bool { return e.Hidden }},
}

func adminChallengeEntry(e store.AdminChallengeEntry, flags []string) AdminChallengeEntry {
	return AdminChallengeEntry{Id: e.ID, Challenge: e.Challenge, ChallengeName: e.ChallengeName, Player: e.Player, Member: e.Member,
		EnteredAs: e.EnteredAs, Account: e.Account, Domination: e.Domination, ProofUrl: nullableText(e.ProofURL),
		EnteredAt: e.At.UTC(), Source: AdminChallengeEntrySource(e.Source), Hidden: e.Hidden, HiddenReason: e.HiddenReason,
		Bugged: e.Bugged, Checked: e.Checked, Flags: append([]string{}, flags...)}
}

func (s *server) GetAdminChallengeEntries(ctx context.Context, req GetAdminChallengeEntriesRequestObject) (GetAdminChallengeEntriesResponseObject, error) {
	page, view, q := 1, AdminChallengeViewAll, ""
	if req.Params.Page != nil {
		page = *req.Params.Page
	}
	if req.Params.View != nil {
		view = *req.Params.View
	}
	if req.Params.Q != nil {
		q = strings.ToLower(strings.TrimSpace(*req.Params.Q))
	}
	if page < 1 {
		return GetAdminChallengeEntries400JSONResponse{badRequest400("page is a whole number from 1")}, nil
	}
	if !view.Valid() {
		return GetAdminChallengeEntries400JSONResponse{badRequest400("view is all, unchecked, checked, unverified, flagged, bugged or hidden")}, nil
	}
	all, err := s.admins.ChallengeEntriesForAdmin(ctx)
	if err != nil {
		return nil, err
	}
	var shown []challenges.Entry
	for _, e := range all {
		if !e.Hidden {
			shown = append(shown, challengeEntry(e.ChallengeEntry))
		}
	}
	flags := challenges.Flags(shown)
	body := AdminChallengeEntries{Page: page, PageSize: challengeEntriesPage, Entries: []AdminChallengeEntry{}}
	var in []store.AdminChallengeEntry
	for _, v := range challengeViews {
		n := 0
		for _, e := range all {
			if !v.in(e, len(flags[e.ID]) > 0) {
				continue
			}
			n++
			if v.view == view && (q == "" || strings.Contains(strings.ToLower(e.Player), q) || strings.Contains(e.Challenge, q) ||
				strings.Contains(strings.ToLower(e.ChallengeName), q)) {
				in = append(in, e)
			}
		}
		body.Counts = append(body.Counts, AdminChallengeViewCount{View: v.view, Entries: n})
	}
	body.Total = len(in)
	for _, e := range in[min((page-1)*challengeEntriesPage, len(in)):min(page*challengeEntriesPage, len(in))] {
		body.Entries = append(body.Entries, adminChallengeEntry(e, flags[e.ID]))
	}
	return GetAdminChallengeEntries200JSONResponse{Body: body, Headers: GetAdminChallengeEntries200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PatchAdminChallengeEntry(ctx context.Context, req PatchAdminChallengeEntryRequestObject) (PatchAdminChallengeEntryResponseObject, error) {
	me, _ := adminFrom(ctx)
	b := req.Body
	if b == nil || (b.Domination == nil && b.ProofUrl == nil && b.Checked == nil && b.Bugged == nil && b.Hidden == nil && b.HiddenReason == nil) {
		return PatchAdminChallengeEntry400JSONResponse{badRequest400("nothing to change")}, nil
	}
	if b.Hidden != nil && *b.Hidden && (b.HiddenReason == nil || strings.TrimSpace(*b.HiddenReason) == "") {
		return PatchAdminChallengeEntry400JSONResponse{badRequest400("hiding an entry needs a reason")}, nil
	}
	c := store.ChallengeEntryChange{Domination: b.Domination, Checked: b.Checked, Bugged: b.Bugged, Hidden: b.Hidden, HiddenReason: b.HiddenReason}
	if b.ProofUrl != nil {
		proof := strings.TrimSpace(*b.ProofUrl)
		c.ProofURL = &proof
	}
	set, err := fieldsSet(b)
	if err != nil {
		return nil, err
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetChallengeEntry(ctx, req.Entry, c); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "challenge.entry.set", fmt.Sprint(req.Entry), map[string]any{"set": set, "change": b})
	})
	switch {
	case errors.Is(err, store.ErrBadChallengeEntry):
		return PatchAdminChallengeEntry400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoChallengeEntry):
		return PatchAdminChallengeEntry404JSONResponse{notFound404("no such challenge entry")}, nil
	case err != nil:
		return nil, err
	}
	return PatchAdminChallengeEntry204Response{}, nil
}

func (s *server) PostAdminChallengeName(ctx context.Context, req PostAdminChallengeNameRequestObject) (PostAdminChallengeNameResponseObject, error) {
	me, _ := adminFrom(ctx)
	if req.Body == nil || strings.TrimSpace(req.Body.From) == "" || strings.TrimSpace(req.Body.To) == "" {
		return PostAdminChallengeName400JSONResponse{badRequest400("the names: from and to")}, nil
	}
	var got store.ChallengeNameMerged
	err := s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if got, err = tx.MergeChallengeName(ctx, req.Body.From, req.Body.To); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "challenge.name.merge", req.Body.From, map[string]any{"to": req.Body.To, "moved": got.Moved, "member": got.Member})
	})
	switch {
	case errors.Is(err, store.ErrBadChallengeEntry):
		return PostAdminChallengeName400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoChallengeEntry):
		return PostAdminChallengeName404JSONResponse{notFound404("no entry not linked to a member has that name")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminChallengeName200JSONResponse{Moved: got.Moved, Member: got.Member}, nil
}
