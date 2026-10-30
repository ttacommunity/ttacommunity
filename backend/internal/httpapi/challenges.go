// The site's challenges: the highscores, each challenge's board, the players and each player's page, all worked
// out from every entry not hidden, those entries to download, and the weekly challenge's winners.
package httpapi

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"ttacommunity/backend/internal/challenges"
	"ttacommunity/backend/internal/store"
)

var challengeIDPattern = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// challengeSets are the game's sets in its order.
var challengeSets = []string{"original", "expansion", "mix"}

// challengeData is what every challenge route reads: the list, its ids and sets, and every entry not hidden.
type challengeData struct {
	list    []store.ChallengeRow
	listed  []challenges.Listed
	entries []challenges.Entry
}

func (s *server) challengeData(ctx context.Context) (challengeData, error) {
	list, err := s.ratings.Challenges(ctx)
	if err != nil {
		return challengeData{}, err
	}
	stored, err := s.ratings.ChallengeEntries(ctx)
	if err != nil {
		return challengeData{}, err
	}
	d := challengeData{list: list, listed: make([]challenges.Listed, len(list)), entries: make([]challenges.Entry, len(stored))}
	for i, c := range list {
		d.listed[i] = challenges.Listed{ID: c.ID, Set: c.Set}
	}
	for i, e := range stored {
		d.entries[i] = challengeEntry(e)
	}
	return d, nil
}

// challengeEntry is a stored entry as the challenges package sees it.
func challengeEntry(e store.ChallengeEntry) challenges.Entry {
	return challenges.Entry{ID: e.ID, Challenge: e.Challenge, Player: challenges.Player{Member: e.Member, Name: e.Player}.Key(),
		Name: e.Player, Domination: e.Domination, ProofURL: e.ProofURL, Verified: e.ProofURL != "", Bugged: e.Bugged, At: e.At}
}

// newestFirst sorts entries by when they were entered, newest first, then by id.
func newestFirst(a, b challenges.Entry) int {
	return cmp.Or(b.At.Compare(a.At), cmp.Compare(b.ID, a.ID))
}

func challengeBest(b challenges.Best) ChallengeBest {
	return ChallengeBest{Player: b.Name, Domination: b.Domination, ProofUrl: nullableText(b.ProofURL), EnteredAt: b.At.UTC(), Rank: b.Rank}
}

func scopePlayers(ps []challenges.ScopePlayer) []ChallengeScopePlayer {
	got := make([]ChallengeScopePlayer, 0, 4)
	for _, p := range ps[:min(4, len(ps))] {
		got = append(got, ChallengeScopePlayer{Player: p.Name, Total: p.Total, Played: p.Played, Rating: float32(p.Rating)})
	}
	return got
}

func (s *server) GetChallenges(ctx context.Context, req GetChallengesRequestObject) (GetChallengesResponseObject, error) {
	d, err := s.challengeData(ctx)
	if err != nil {
		return nil, err
	}
	stats := challenges.Compute(d.listed, d.entries, req.Params.Verified == nil || *req.Params.Verified, "")
	body := ChallengeHighscores{Challenges: make([]Challenge, len(d.list)), Scopes: []ChallengeScope{}, RecordHolders: []ChallengeRecordHolder{}}
	for i, c := range d.list {
		board := stats.Boards[c.ID]
		body.Challenges[i] = Challenge{Id: c.ID, Set: ChallengeSet(c.Set), Group: c.Group, Slot: c.Slot, Difficulty: c.Difficulty,
			Players: c.Players, Name: c.Name, Entrants: len(board)}
		if len(board) > 0 {
			record := challengeBest(board[0])
			body.Challenges[i].Record = &record
		}
	}
	scope := func(set, group string, ids []string) {
		sc := stats.Scope(ids)
		body.Scopes = append(body.Scopes, ChallengeScope{Set: ChallengeScopeSet(set), Group: group, Pooled: sc.Pooled, Covered: sc.Covered,
			Size: sc.Size, ByTotal: scopePlayers(sc.ByTotal), ByRating: scopePlayers(sc.ByRating)})
	}
	var all []string
	for _, set := range challengeSets {
		var ids, groups []string
		byGroup := map[string][]string{}
		for _, c := range d.list {
			if c.Set != set {
				continue
			}
			ids = append(ids, c.ID)
			if _, ok := byGroup[c.Group]; !ok {
				groups = append(groups, c.Group)
			}
			byGroup[c.Group] = append(byGroup[c.Group], c.ID)
		}
		if len(ids) == 0 {
			continue
		}
		scope(set, "", ids)
		for _, g := range groups {
			scope(set, g, byGroup[g])
		}
		all = append(all, ids...)
	}
	scope("all", "", all)

	var holders []*challenges.Summary
	for _, p := range stats.Players {
		if p.Records > 0 {
			holders = append(holders, p)
		}
	}
	slices.SortFunc(holders, func(a, b *challenges.Summary) int {
		return cmp.Or(cmp.Compare(b.Records, a.Records), strings.Compare(a.Name, b.Name))
	})
	for _, p := range holders[:min(10, len(holders))] {
		body.RecordHolders = append(body.RecordHolders, ChallengeRecordHolder{Player: p.Name, Records: p.Records})
	}
	return GetChallenges200JSONResponse{Body: body, Headers: GetChallenges200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetChallengeBoard(ctx context.Context, req GetChallengeBoardRequestObject) (GetChallengeBoardResponseObject, error) {
	if !challengeIDPattern.MatchString(req.Challenge) {
		return GetChallengeBoard400JSONResponse{badRequest400("a challenge id is 1 to 40 lowercase letters, digits and underscores")}, nil
	}
	d, err := s.challengeData(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(d.list, func(c store.ChallengeRow) bool { return c.ID == req.Challenge }) {
		return GetChallengeBoard404JSONResponse{notFound404("no such challenge")}, nil
	}
	verified := req.Params.Verified == nil || *req.Params.Verified
	stats := challenges.Compute(d.listed, d.entries, verified, "")
	mine := slices.DeleteFunc(slices.Clone(d.entries), func(e challenges.Entry) bool { return e.Challenge != req.Challenge })
	body := ChallengeBoard{Ranking: []ChallengeBest{}, History: []ChallengeRecordStep{}, Entries: make([]ChallengeSubmission, 0, len(mine))}
	for _, b := range stats.Boards[req.Challenge] {
		body.Ranking = append(body.Ranking, challengeBest(b))
	}
	for _, st := range challenges.Steps(mine, verified) {
		body.History = append(body.History, ChallengeRecordStep{Player: st.Name, Domination: st.Domination, ProofUrl: nullableText(st.ProofURL),
			EnteredAt: st.At.UTC(), Gain: st.Gain})
	}
	slices.SortFunc(mine, newestFirst)
	for _, e := range mine {
		body.Entries = append(body.Entries, ChallengeSubmission{Player: e.Name, Domination: e.Domination, ProofUrl: nullableText(e.ProofURL),
			EnteredAt: e.At.UTC(), Bugged: e.Bugged})
	}
	return GetChallengeBoard200JSONResponse{Body: body, Headers: GetChallengeBoard200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func figures[T int | float64](byset map[string]T) ChallengeSetFigures {
	return ChallengeSetFigures{Original: float32(byset["original"]), Expansion: float32(byset["expansion"]), Mix: float32(byset["mix"])}
}

func challengeSummary(p *challenges.Summary) ChallengePlayerSummary {
	return ChallengePlayerSummary{Player: p.Name, Played: p.Played, Verified: p.Verified, Records: p.Records, Total: p.Total,
		Totals: figures(p.Totals), Rating: float32(p.Rating), Ratings: figures(p.Ratings), LastEntry: p.LastEntry.UTC()}
}

func (s *server) GetChallengePlayers(ctx context.Context, req GetChallengePlayersRequestObject) (GetChallengePlayersResponseObject, error) {
	d, err := s.challengeData(ctx)
	if err != nil {
		return nil, err
	}
	stats := challenges.Compute(d.listed, d.entries, req.Params.Verified == nil || *req.Params.Verified, "")
	players := make([]*challenges.Summary, 0, len(stats.Players))
	for _, p := range stats.Players {
		players = append(players, p)
	}
	slices.SortFunc(players, func(a, b *challenges.Summary) int {
		return cmp.Or(cmp.Compare(b.Total, a.Total), strings.Compare(a.Name, b.Name))
	})
	body := ChallengePlayers{Players: make([]ChallengePlayerSummary, len(players))}
	for i, p := range players {
		body.Players[i] = challengeSummary(p)
	}
	return GetChallengePlayers200JSONResponse{Body: body, Headers: GetChallengePlayers200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func (s *server) GetChallengePlayer(ctx context.Context, req GetChallengePlayerRequestObject) (GetChallengePlayerResponseObject, error) {
	d, err := s.challengeData(ctx)
	if err != nil {
		return nil, err
	}
	key, name := "", ""
	var theirs []challenges.Entry
	for _, e := range d.entries {
		if strings.EqualFold(e.Name, req.Name) {
			key = e.Player
		}
	}
	for _, e := range d.entries {
		if e.Player == key {
			theirs, name = append(theirs, e), e.Name
		}
	}
	if key == "" {
		return GetChallengePlayer404JSONResponse{notFound404("no challenge entries under this name")}, nil
	}
	verified := req.Params.Verified == nil || *req.Params.Verified
	trust := ""
	if verified && req.Params.AsIfVerified != nil && *req.Params.AsIfVerified {
		trust = key
	}
	stats := challenges.Compute(d.listed, d.entries, verified, trust)
	body := ChallengePlayer{Player: name, Bests: []ChallengePlayerBest{}, Recent: []ChallengePlayerEntry{}}
	for _, e := range theirs {
		if !e.Verified && !e.Bugged {
			body.Unproven++
		}
	}
	if p := stats.Players[key]; p != nil {
		summary := challengeSummary(p)
		body.Summary = &summary
		for _, c := range d.list {
			b, ok := p.Bests[c.ID]
			if !ok {
				continue
			}
			board := stats.Boards[c.ID]
			best := ChallengePlayerBest{Challenge: c.ID, Domination: b.Domination, ProofUrl: nullableText(b.ProofURL), Rank: b.Rank,
				Entrants: len(board), Share: float32(b.Share)}
			if !slices.ContainsFunc(board, func(o challenges.Best) bool { return o.Player == key }) {
				best.Entrants++
			}
			if len(board) > 0 {
				best.Record, best.RecordHolder = board[0].Domination, board[0].Name
			}
			body.Bests = append(body.Bests, best)
		}
	}
	slices.SortFunc(theirs, newestFirst)
	for _, e := range theirs[:min(10, len(theirs))] {
		body.Recent = append(body.Recent, ChallengePlayerEntry{Challenge: e.Challenge, Domination: e.Domination, ProofUrl: nullableText(e.ProofURL),
			EnteredAt: e.At.UTC(), Bugged: e.Bugged})
	}
	return GetChallengePlayer200JSONResponse{Body: body, Headers: GetChallengePlayer200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}

func challengePlacings(ps []challenges.Placing) []ChallengePlacing {
	got := make([]ChallengePlacing, len(ps))
	for i, p := range ps {
		got[i] = ChallengePlacing{Player: p.Name, Domination: p.Domination}
	}
	return got
}

func (s *server) GetChallengeWeeks(ctx context.Context, _ GetChallengeWeeksRequestObject) (GetChallengeWeeksResponseObject, error) {
	weeks, err := s.ratings.ChallengeWeeks(ctx)
	if err != nil {
		return nil, err
	}
	body := ChallengeWeeks{Weeks: make([]ChallengeWeek, len(weeks))}
	for i, w := range weeks {
		body.Weeks[i] = ChallengeWeek{Week: w.Number, WeekOf: openapi_types.Date{Time: w.Of}, Challenge: w.Challenge,
			Winners: challengePlacings(w.Winners), RunnersUp: challengePlacings(w.RunnersUp)}
	}
	return GetChallengeWeeks200JSONResponse{Body: body, Headers: GetChallengeWeeks200ResponseHeaders{CacheControl: cacheDay}}, nil
}

func (s *server) GetChallengeEntries(ctx context.Context, _ GetChallengeEntriesRequestObject) (GetChallengeEntriesResponseObject, error) {
	stored, err := s.ratings.ChallengeEntries(ctx)
	if err != nil {
		return nil, err
	}
	body := ChallengeEntries{Entries: make([]ChallengeEntry, len(stored))}
	for i, e := range stored {
		body.Entries[i] = ChallengeEntry{Id: e.ID, Challenge: e.Challenge, Player: e.Player, Domination: e.Domination,
			ProofUrl: nullableText(e.ProofURL), EnteredAt: e.At.UTC(), Bugged: e.Bugged}
	}
	return GetChallengeEntries200JSONResponse{Body: body, Headers: GetChallengeEntries200ResponseHeaders{CacheControl: cacheCurrent}}, nil
}
