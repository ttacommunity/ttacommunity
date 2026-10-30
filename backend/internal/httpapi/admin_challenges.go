// Admin's challenges: every entry by view, with what looks wrong about it; an entry changed; a name merged into
// another or into a member. Changes are audited.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

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
	view := AdminChallengeViewAll
	if req.Params.View != nil {
		view = *req.Params.View
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
	body := AdminChallengeEntries{Entries: []AdminChallengeEntry{}}
	for _, v := range challengeViews {
		n := 0
		for _, e := range all {
			if !v.in(e, len(flags[e.ID]) > 0) {
				continue
			}
			n++
			if v.view == view {
				body.Entries = append(body.Entries, adminChallengeEntry(e, flags[e.ID]))
			}
		}
		body.Counts = append(body.Counts, AdminChallengeViewCount{View: v.view, Entries: n})
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
		return tx.Audit(ctx, me.DiscordID, "challenge.name.merge", req.Body.From, map[string]any{"to": req.Body.To, "moved": got.Moved})
	})
	switch {
	case errors.Is(err, store.ErrBadChallengeEntry):
		return PostAdminChallengeName400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoMember):
		return PostAdminChallengeName404JSONResponse{notFound404("no member has the TTA username to")}, nil
	case errors.Is(err, store.ErrNoChallengeEntry):
		return PostAdminChallengeName404JSONResponse{notFound404("no entry or weekly placing is under the name from")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminChallengeName200JSONResponse{Moved: got.Moved}, nil
}
