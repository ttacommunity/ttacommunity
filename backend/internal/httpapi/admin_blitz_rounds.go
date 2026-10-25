// Admin's blitz rounds: pairing the next round as a draft, changing its matches by hand, publishing and deleting.
package httpapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/pairing"
	"ttacommunity/backend/internal/store"
)

// metBefore lists, for each entrant, who they met in any round but the one given (0 for none).
func metBefore(rounds []store.BlitzRound, except int64) map[int64][]int64 {
	met := map[int64][]int64{}
	for _, r := range rounds {
		if r.ID == except {
			continue
		}
		for _, m := range r.Matches {
			if m.B != nil {
				met[m.A] = append(met[m.A], *m.B)
				met[*m.B] = append(met[*m.B], m.A)
			}
		}
	}
	return met
}

// adminBlitzRound shows a round with each match's gaps as the entrants are now and, in a Swiss round, whether a
// pair has met in another Swiss round (the knockout may repeat a pairing).
func (s *server) adminBlitzRound(r store.BlitzRound, ed store.BlitzEdition, rated map[string]store.Rated, now time.Time) AdminBlitzRound {
	byID := map[int64]store.BlitzEntrant{}
	for _, e := range ed.Entrants {
		byID[e.ID] = e
	}
	var swiss []store.BlitzRound
	for _, x := range ed.Rounds {
		if x.Kind != "knockout" {
			swiss = append(swiss, x)
		}
	}
	met := metBefore(swiss, r.ID)
	out := AdminBlitzRound{Id: r.ID, Number: r.Number, Kind: AdminBlitzRoundKind(r.Kind), Method: roundMethod(r),
		PublishedAt: r.PublishedAt, Matches: make([]AdminBlitzMatch, len(r.Matches))}
	if r.Deadline != nil {
		out.Deadline = &BlitzDeadline{At: *r.Deadline, Zone: r.DeadlineZone}
	}
	for i, m := range r.Matches {
		am := AdminBlitzMatch{Id: m.ID, A: m.A, B: m.B, Slot: m.Slot, ASeed: m.SeedA, BSeed: m.SeedB, Result: blitzResult(m), Game: playedGame(m),
			Ruling: m.Ruling, DecidedAt: m.DecidedAt, ReportedBy: m.ReportedBy, Report: blitzReport(m), FlagReason: m.FlagReason}
		if m.B != nil {
			a, b := byID[m.A], byID[*m.B]
			oa, _ := blitz.OffsetMinutes(a.Zone, now)
			ob, _ := blitz.OffsetMinutes(b.Zone, now)
			am.ZoneGapMinutes = pairing.ZoneGap(oa, ob)
			if ra, ok := rated[a.Player]; ok {
				if rb, ok := rated[b.Player]; ok {
					gap := ra.Rating - rb.Rating
					if gap < 0 {
						gap = -gap
					}
					am.RatingGap = &gap
				}
			}
			am.Repeat = r.Kind != "knockout" && slices.Contains(met[m.A], *m.B)
		}
		out.Matches[i] = am
	}
	if r.Kind == "knockout" {
		out.Reserve = knockoutReserve(r, ed)
	}
	return out
}

// knockoutReserve is who replaces a knockout player who dropped: the best Swiss place among the active entrants not
// in the round, or nil for nobody.
func knockoutReserve(r store.BlitzRound, ed store.BlitzEdition) *int64 {
	in := map[int64]bool{}
	for _, m := range r.Matches {
		in[m.A] = true
		if m.B != nil {
			in[*m.B] = true
		}
	}
	active := map[int64]bool{}
	for _, e := range ed.Entrants {
		active[e.ID] = e.Status == "active"
	}
	for _, st := range swissStandings(ed) {
		if active[st.ID] && !in[st.ID] {
			return new(st.ID)
		}
	}
	return nil
}

// roundMethod is the method a Swiss round was paired with, nil for a knockout round.
func roundMethod(r store.BlitzRound) *BlitzPairingMethod {
	if r.Method == "" {
		return nil
	}
	return new(BlitzPairingMethod(r.Method))
}

// pairingError is the message for a round that cannot be paired, with what admin can do about it.
func pairingError(err error) string {
	if errors.Is(err, pairing.ErrNoPairing) {
		return err.Error() + ": lower the Swiss rounds in Format to go on to the knockout"
	}
	return err.Error()
}

// pairingPlayers turns the edition's active entrants into what the pairing needs, as of now, leaving out the
// round named by except (0 for none: a new round) when counting points, places, opponents, byes and floats, and the
// entrants in busy (already paired, or skipping the round). Starting ranks follow the ratings (startingRanks).
func pairingPlayers(ed store.BlitzEdition, rated map[string]store.Rated, now time.Time, except int64, busy map[int64]bool) []pairing.Player {
	var others []store.BlitzRound
	for _, r := range ed.Rounds {
		if r.ID != except {
			others = append(others, r)
		}
	}
	without := store.BlitzEdition{BlitzEditionRow: ed.BlitzEditionRow, Entrants: ed.Entrants, Rounds: others}
	met := metBefore(others, 0)
	points, places := map[string]float64{}, map[string]int{}
	for _, s := range blitzStandings(without) {
		points[s.Name], places[s.Name] = s.Points, s.Rank
	}
	entrants, matches, numbers := swissPlayed(without)
	history := blitz.Histories(entrants, matches, numbers, ed.Points)
	ranks := startingRanks(ed.Entrants, rated)
	var players []pairing.Player
	for _, e := range ed.Entrants {
		if e.Status != "active" || busy[e.ID] {
			continue
		}
		offset, _ := blitz.OffsetMinutes(e.Zone, now)
		h := history[e.ID]
		p := pairing.Player{ID: e.ID, Points: points[e.Name], TPN: ranks[e.ID], Place: places[e.Name], Offset: offset, Opponents: met[e.ID],
			HadBye: h.HadBye, Unplayed: h.Unplayed, Floats: h.Floats}
		if r, ok := rated[e.Player]; ok {
			p.Rating, p.Rated = r.Rating, true
		}
		players = append(players, p)
	}
	return players
}

// startingRanks numbers the active entrants 1, 2, 3… by rating, highest first, the unrated last, then by name
// without case, for the Dutch system's order within a score group.
func startingRanks(entrants []store.BlitzEntrant, rated map[string]store.Rated) map[int64]int {
	var active []store.BlitzEntrant
	for _, e := range entrants {
		if e.Status == "active" {
			active = append(active, e)
		}
	}
	slices.SortStableFunc(active, func(a, b store.BlitzEntrant) int {
		ra, okA := rated[a.Player]
		rb, okB := rated[b.Player]
		switch {
		case okA != okB:
			if okA {
				return -1
			}
			return 1
		case okA && ra.Rating != rb.Rating:
			return cmp.Compare(rb.Rating, ra.Rating)
		}
		return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	ranks := make(map[int64]int, len(active))
	for i, e := range active {
		ranks[e.ID] = i + 1
	}
	return ranks
}

// nextMethod is how the edition's next Swiss round is paired: round 1's method for its first, else the later one.
func nextMethod(ed store.BlitzEdition, swissMade int) string {
	if swissMade == 0 {
		return ed.Pairing.First
	}
	return ed.Pairing.Later
}

// errNoKnockout means the knockout's next round cannot be paired yet, or at all.
var errNoKnockout = errors.New("cannot pair the knockout")

// nextKnockout pairs the knockout's next round: the top of the Swiss standings, best seed against worst, when it
// has none yet; else the winners of its latest round, neighbouring slots meeting. Each match carries its slot and
// its players' seeds, their places in the Swiss standings when the knockout began.
func nextKnockout(ed store.BlitzEdition) ([]store.BlitzMatch, error) {
	var last *store.BlitzRound
	for i := range ed.Rounds {
		if ed.Rounds[i].Kind == "knockout" && (last == nil || ed.Rounds[i].Number > last.Number) {
			last = &ed.Rounds[i]
		}
	}
	var pairs [][2]int64
	var err error
	seedOf := map[int64]int{}
	if last == nil {
		size := ed.Format.KnockoutSize
		var seeded []int64
		for _, s := range swissStandings(ed) {
			if !s.Dropped && len(seeded) < size {
				seeded = append(seeded, s.ID)
				seedOf[s.ID] = len(seeded)
			}
		}
		if len(seeded) < size {
			return nil, fmt.Errorf("%w: it takes the top %d, and %d players are still in", errNoKnockout, size, len(seeded))
		}
		pairs, err = blitz.FirstKnockout(seeded)
	} else {
		names := map[int64]string{}
		for _, e := range ed.Entrants {
			names[e.ID] = e.Name
		}
		matches := slices.SortedFunc(slices.Values(last.Matches), func(a, b store.BlitzMatch) int { return cmp.Compare(*a.Slot, *b.Slot) })
		var winners []int64
		var open []string
		for _, m := range matches {
			if w, ok := blitz.Winner(played(last.Number, m)); ok {
				winners = append(winners, w)
				seedOf[m.A], seedOf[*m.B] = *m.SeedA, *m.SeedB
			} else {
				open = append(open, names[m.A]+" v "+names[*m.B])
			}
		}
		if len(open) > 0 {
			return nil, fmt.Errorf("%w: no winner yet in %s", store.ErrBlitzRoundUndecided, strings.Join(open, ", "))
		}
		if len(winners) == 1 {
			return nil, store.ErrBlitzFinished
		}
		pairs, err = blitz.NextKnockout(winners)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNoKnockout, err)
	}
	out := make([]store.BlitzMatch, len(pairs))
	for i, p := range pairs {
		out[i] = store.BlitzMatch{A: p[0], B: new(p[1]), Slot: new(i), SeedA: new(seedOf[p[0]]), SeedB: new(seedOf[p[1]])}
	}
	return out, nil
}

// PostAdminBlitzRound pairs the edition's next round as a draft: a Swiss round while the format has some left,
// then the knockout's rounds.
func (s *server) PostAdminBlitzRound(ctx context.Context, req PostAdminBlitzRoundRequestObject) (PostAdminBlitzRoundResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRound400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return PostAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(ed.Rounds, func(r store.BlitzRound) bool { return r.PublishedAt == nil }) {
		return PostAdminBlitzRound409JSONResponse{conflict409(store.ErrBlitzDraftExists.Error())}, nil
	}
	swissMade, next := 0, 1
	for _, r := range ed.Rounds {
		if r.Kind != "knockout" {
			swissMade++
		}
		next = max(next, r.Number+1)
	}
	now := s.now()
	seed := uint64(now.UnixNano())
	var round store.BlitzRound
	switch {
	case swissMade < ed.Format.SwissRounds:
		busy := map[int64]bool{}
		for _, e := range skippers(ed.Entrants, next) {
			busy[e.ID] = true
		}
		method := nextMethod(ed, swissMade)
		paired, err := pairing.Pair(pairingPlayers(ed, rated, now, 0, busy), pairing.Method(method), seed)
		if errors.Is(err, pairing.ErrTooFew) || errors.Is(err, pairing.ErrNoPairing) {
			return PostAdminBlitzRound400JSONResponse{badRequest400(pairingError(err))}, nil
		}
		if err != nil {
			return nil, err
		}
		round = store.BlitzRound{Kind: "swiss", Method: method, Seed: int64(seed)}
		for _, m := range paired.Matches {
			round.Matches = append(round.Matches, store.BlitzMatch{A: m.A, B: new(m.B)})
		}
		if paired.Bye != nil {
			round.Matches = append(round.Matches, store.BlitzMatch{A: *paired.Bye})
		}
	case ed.Format.KnockoutSize == 0:
		return PostAdminBlitzRound409JSONResponse{conflict409(store.ErrBlitzFinished.Error())}, nil
	default:
		matches, err := nextKnockout(ed)
		switch {
		case errors.Is(err, errNoKnockout), errors.Is(err, store.ErrBlitzRoundUndecided), errors.Is(err, store.ErrBlitzFinished):
			return PostAdminBlitzRound409JSONResponse{conflict409(err.Error())}, nil
		case err != nil:
			return nil, err
		}
		round = store.BlitzRound{Kind: "knockout", Seed: int64(seed), Matches: matches}
	}
	var made store.BlitzRound
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if made, err = tx.AddBlitzRound(ctx, req.Tournament, req.Edition, round); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.add", blitzEditionKey(req.Tournament, req.Edition), map[string]any{"round": made.Number, "kind": made.Kind, "method": made.Method, "seed": seed})
	})
	switch {
	case errors.Is(err, store.ErrBlitzDraftExists), errors.Is(err, store.ErrBlitzRoundUndecided), errors.Is(err, store.ErrBlitzFinished):
		return PostAdminBlitzRound409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrBadBlitzMatch), errors.Is(err, store.ErrBlitzRepeat):
		return PostAdminBlitzRound400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	case err != nil:
		return nil, err
	}
	ed.Rounds = append(ed.Rounds, made)
	return PostAdminBlitzRound201JSONResponse{Body: s.adminBlitzRound(made, ed, rated, now), Headers: PostAdminBlitzRound201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminBlitzRoundMatches(ctx context.Context, req PutAdminBlitzRoundMatchesRequestObject) (PutAdminBlitzRoundMatchesResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	if req.Body == nil {
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400("the matches")}, nil
	}
	matches := make([]store.BlitzMatch, len(req.Body.Matches))
	for i, m := range req.Body.Matches {
		matches[i] = store.BlitzMatch{A: m.A, B: m.B}
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetBlitzRoundMatches(ctx, req.Tournament, req.Edition, req.Round, matches); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.matches", fmt.Sprint(req.Round), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadBlitzMatch), errors.Is(err, store.ErrBlitzRepeat), errors.Is(err, store.ErrBlitzMatchDecided):
		return PutAdminBlitzRoundMatches400JSONResponse{badRequest400(err.Error())}, nil
	case errors.Is(err, store.ErrBlitzKnockoutFixed):
		return PutAdminBlitzRoundMatches409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PutAdminBlitzRoundMatches404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PutAdminBlitzRoundMatches404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminBlitzRoundMatches204Response{}, nil
}

func (s *server) PostAdminBlitzRoundPublish(ctx context.Context, req PostAdminBlitzRoundPublishRequestObject) (PostAdminBlitzRoundPublishResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.PublishBlitzRound(ctx, req.Tournament, req.Edition, req.Round); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.publish", fmt.Sprint(req.Round), nil)
	})
	switch {
	case errors.Is(err, store.ErrBlitzRoundIncomplete), errors.Is(err, store.ErrBlitzRepeat), errors.Is(err, store.ErrBadBlitzMatch):
		return PostAdminBlitzRoundPublish409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return PostAdminBlitzRoundPublish404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminBlitzRoundPublish204Response{}, nil
}

func (s *server) DeleteAdminBlitzRound(ctx context.Context, req DeleteAdminBlitzRoundRequestObject) (DeleteAdminBlitzRoundResponseObject, error) {
	me, _ := adminFrom(ctx)
	if !tournamentSlug.MatchString(req.Tournament) {
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	err := s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.DeleteBlitzRound(ctx, req.Tournament, req.Edition, req.Round); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "blitz.round.remove", fmt.Sprint(req.Round), nil)
	})
	switch {
	case errors.Is(err, store.ErrBlitzRoundNotLast), errors.Is(err, store.ErrBlitzRoundPlayed):
		return DeleteAdminBlitzRound409JSONResponse{conflict409(err.Error())}, nil
	case errors.Is(err, store.ErrNoBlitzEdition):
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such blitz edition")}, nil
	case errors.Is(err, store.ErrNoBlitzRound):
		return DeleteAdminBlitzRound404JSONResponse{notFound404("no such round")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminBlitzRound204Response{}, nil
}

// keptMatches is what a proposed pairing keeps of a round: every match with a result, then each match in keep (the
// saved one when the round has that pair), and who is in them. A kept match naming someone not active, skipping the
// round, or anyone twice, is refused with ErrBadBlitzMatch.
func keptMatches(entrants []store.BlitzEntrant, round store.BlitzRound, keep []BlitzMatchChange) ([]store.BlitzMatch, map[int64]bool, error) {
	key := func(a int64, b *int64) [2]int64 {
		if b == nil {
			return [2]int64{a, 0}
		}
		return [2]int64{a, *b}
	}
	saved := map[[2]int64]store.BlitzMatch{}
	var kept []store.BlitzMatch
	busy := map[int64]bool{}
	for _, m := range round.Matches {
		saved[key(m.A, m.B)] = m
		if m.Result != "" {
			kept = append(kept, m)
			busy[m.A] = true
			if m.B != nil {
				busy[*m.B] = true
			}
		}
	}
	active, names := map[int64]bool{}, map[int64]string{}
	for _, e := range entrants {
		active[e.ID], names[e.ID] = e.Status == "active" && !slices.Contains(e.Skips, round.Number), e.Name
	}
	for _, k := range keep {
		m, ok := saved[key(k.A, k.B)]
		if ok && m.Result != "" {
			continue
		}
		if !ok {
			m = store.BlitzMatch{A: k.A, B: k.B}
		}
		for _, id := range []*int64{&k.A, k.B} {
			if id == nil {
				continue
			}
			if !active[*id] || busy[*id] {
				return nil, nil, fmt.Errorf("%w: %s", store.ErrBadBlitzMatch, cmp.Or(names[*id], fmt.Sprint("entrant ", *id)))
			}
			busy[*id] = true
		}
		kept = append(kept, m)
	}
	return kept, busy, nil
}

// PostAdminBlitzRoundPairing proposes a pairing of a round: its decided matches and the matches in keep stay, and
// everyone else active is paired as for a new round. Nothing is written and nothing audited.
func (s *server) PostAdminBlitzRoundPairing(ctx context.Context, req PostAdminBlitzRoundPairingRequestObject) (PostAdminBlitzRoundPairingResponseObject, error) {
	if !tournamentSlug.MatchString(req.Tournament) {
		return PostAdminBlitzRoundPairing400JSONResponse{badRequest400("bad parameter: tournament")}, nil
	}
	ed, err := s.admins.BlitzEdition(ctx, req.Tournament, req.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return PostAdminBlitzRoundPairing404JSONResponse{notFound404("no such blitz edition")}, nil
	}
	if err != nil {
		return nil, err
	}
	var round *store.BlitzRound
	for i := range ed.Rounds {
		if ed.Rounds[i].ID == req.Round {
			round = &ed.Rounds[i]
		}
	}
	if round == nil {
		return PostAdminBlitzRoundPairing404JSONResponse{notFound404("no such round")}, nil
	}
	if round.Kind == "knockout" {
		return PostAdminBlitzRoundPairing409JSONResponse{conflict409(store.ErrBlitzKnockoutFixed.Error())}, nil
	}
	rated, err := s.blitzRatings(ctx, ed.Entrants)
	if err != nil {
		return nil, err
	}
	var keep []BlitzMatchChange
	if req.Body != nil && req.Body.Keep != nil {
		keep = *req.Body.Keep
	}
	kept, busy, err := keptMatches(ed.Entrants, *round, keep)
	if err != nil {
		return PostAdminBlitzRoundPairing400JSONResponse{badRequest400(err.Error())}, nil
	}
	for _, e := range skippers(ed.Entrants, round.Number) {
		busy[e.ID] = true
	}
	proposal := store.BlitzRound{ID: round.ID, Number: round.Number, Kind: round.Kind, Method: round.Method, PublishedAt: round.PublishedAt, Matches: kept}
	now := s.now()
	free := pairingPlayers(ed, rated, now, round.ID, busy)
	if i := slices.IndexFunc(proposal.Matches, func(m store.BlitzMatch) bool { return m.B == nil }); i >= 0 && len(free)%2 == 1 {
		delete(busy, proposal.Matches[i].A)
		proposal.Matches = slices.Delete(proposal.Matches, i, i+1)
		free = pairingPlayers(ed, rated, now, round.ID, busy)
	}
	seed := uint64(now.UnixNano())
	proposal.Seed = int64(seed)
	switch len(free) {
	case 0:
	case 1:
		proposal.Matches = append(proposal.Matches, store.BlitzMatch{A: free[0].ID})
	default:
		paired, err := pairing.Pair(free, pairing.Method(round.Method), seed)
		if errors.Is(err, pairing.ErrNoPairing) {
			return PostAdminBlitzRoundPairing400JSONResponse{badRequest400(pairingError(err))}, nil
		}
		if err != nil {
			return nil, err
		}
		for _, m := range paired.Matches {
			b := m.B
			proposal.Matches = append(proposal.Matches, store.BlitzMatch{A: m.A, B: &b})
		}
		if paired.Bye != nil {
			proposal.Matches = append(proposal.Matches, store.BlitzMatch{A: *paired.Bye})
		}
	}
	return PostAdminBlitzRoundPairing200JSONResponse{Body: s.adminBlitzRound(proposal, ed, rated, now), Headers: PostAdminBlitzRoundPairing200ResponseHeaders{CacheControl: cacheNone}}, nil
}
