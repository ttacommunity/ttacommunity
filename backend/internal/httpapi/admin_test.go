package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/access"
	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/enginejob"
	"ttacommunity/backend/internal/store"
)

// fakeAdmins accepts the token "good-<id>" for Discord id <id>, and knows one admin and one organiser.
// It records each tournament change and audit, and knows one tournament, worlds.
type fakeAdmins struct {
	settings []store.TournamentSettings
	notes    []string
	audits   []string
	// details is each audit's detail, as JSON.
	details  []string
	startErr error
	stopErr  error
	// engine is what EngineGoing answers: "" for a run going, "ended" for none, "unset" for not set up,
	// "unreachable" for an error.
	engine string
	// deleted receives each run DeleteRun removes, from the handler's background delete.
	deleted chan int64
	// prewarmed receives each run PrewarmPlayerPages loads, from the handler's background prewarm.
	prewarmed chan int64
	// entrants, format, points (FIDE's while zero) and rounds are the blitz edition us_open 2026's
	// (admin_blitz_test.go, admin_blitz_rounds_test.go).
	entrants []store.BlitzEntrant
	format   store.BlitzFormat
	points   blitz.Scoring
	pairing  store.BlitzPairing
	// ranksFixedBy is what fixed the blitz edition's starting ranks, "" before round 1.
	ranksFixedBy string
	rounds       []store.BlitzRound
	// members are the community's (admin_members_test.go).
	members []store.Member
	// added is every entrant AddBlitzEntrants was given; candidates is what BlitzCandidates finds, and searched
	// what it was asked (admin_blitz_test.go).
	added      []store.BlitzEntrant
	candidates []store.Candidate
	searched   string
	// inChange is set while Change runs fn; auditErr is what Audit answers there.
	inChange bool
	auditErr error
	// logAsked is the filter and page AuditLog was last asked for (admin_audit_test.go).
	logAsked *store.AuditFilter
	logPage  int
}

// fakeRuns are runs 4 (complete, published third, after a roll back), 5 (complete, published first),
// 6 (complete, never published), 7 (complete, published second, current), 8 (complete) and 9 (running).
var fakeRuns = []store.AdminRun{
	{ID: 9, Status: "running", Progress: "[3/10] models 2p"},
	{ID: 8, Status: "complete", Games: 100, Bytes: 3_500_000_000, DiskBytes: 9_000_000_000},
	{ID: 7, Status: "complete", Games: 90, Current: true, PublishedAt: at("2026-10-02T10:00:00Z"), Bytes: 4_000_000_000},
	{ID: 6, Status: "complete"},
	{ID: 5, Status: "complete", PublishedAt: at("2026-10-01T10:00:00Z")},
	{ID: 4, Status: "complete", PublishedAt: at("2026-10-03T10:00:00Z")},
}

func at(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func (*fakeAdmins) Runs(context.Context) ([]store.AdminRun, error) { return fakeRuns, nil }

func (f *fakeAdmins) Publish(_ context.Context, run int64) error {
	switch run {
	case 7, 8:
		f.notes = append(f.notes, fmt.Sprintf("publish %d", run))
		return nil
	case 9:
		return store.ErrRunNotComplete
	}
	return store.ErrRunNotFound
}

func (*fakeAdmins) RunChanges(_ context.Context, run, before int64) (store.RunChanges, error) {
	if run < 4 || run > 9 || before < 4 || before > 9 {
		return store.RunChanges{}, store.ErrRunNotFound
	}
	return store.RunChanges{Ratings: int(run), RatingsBefore: int(before), Biggest: []store.RunMove{{Name: "dan", Before: 1500, After: 1380, Change: -120}}}, nil
}

func (f *fakeAdmins) StopRun(context.Context) ([]string, error) {
	if f.stopErr != nil {
		return nil, f.stopErr
	}
	f.notes = append(f.notes, "stop")
	return []string{"engine-dev-admin-x7k2p"}, nil
}

func (f *fakeAdmins) EngineGoing(context.Context) (bool, error) {
	switch f.engine {
	case "ended":
		return false, nil
	case "unset":
		return false, enginejob.ErrNotSetUp
	case "unreachable":
		return false, errors.New("GET /apis/batch/v1/namespaces/ttac/jobs: dial tcp: i/o timeout")
	}
	return true, nil
}

func (f *fakeAdmins) StartRun(context.Context) (string, error) {
	if f.startErr != nil {
		return "", f.startErr
	}
	f.notes = append(f.notes, "start")
	return "engine-dev-admin-x7k2p", nil
}

func (*fakeAdmins) AllowedOrigin() string { return "https://ttac-admin.example" }

// In the fake, run 4 is protected, 7 current and 9 running; 99 does not exist.
func (f *fakeAdmins) SetRunProtected(_ context.Context, run int64, protected bool) error {
	if run == 99 {
		return store.ErrRunNotFound
	}
	f.notes = append(f.notes, fmt.Sprintf("protect %d %t", run, protected))
	return nil
}

func (f *fakeAdmins) StartDeleteRun(_ context.Context, run int64, engineGoing bool) error {
	switch {
	case run == 99:
		return store.ErrRunNotFound
	case run == 4:
		return store.ErrRunProtected
	case run == 7:
		return store.ErrRunCurrent
	case run == 9 && engineGoing:
		return store.ErrRunRunning
	}
	f.notes = append(f.notes, fmt.Sprintf("delete %d", run))
	return nil
}

func (f *fakeAdmins) PrewarmPlayerPages(_ context.Context, run int64) (int64, error) {
	if f.prewarmed != nil {
		f.prewarmed <- run
	}
	return 1, nil
}

func (f *fakeAdmins) DeleteRun(_ context.Context, run int64) error {
	if f.deleted != nil {
		f.deleted <- run
	}
	return nil
}

func (*fakeAdmins) Tournament(_ context.Context, slug string) (store.AdminTournament, error) {
	if slug != "worlds" {
		return store.AdminTournament{}, store.ErrNoTournament
	}
	return store.AdminTournament{Slug: "worlds", Name: "World Championship", Visible: true, Rated: true, Community: true, Blitz: true,
		Regulations: "Mine.", CGERegulations: "CGE's.",
		Card: store.Card{Icon: "🏆", Tagline: "Annual World Championship", Summary: "The pinnacle.", Subtitle: "4-Player", Cadence: "Annual"},
		Kind: "cup", OriginalName: "World Championships", Points: map[int][]float64{2: nil, 3: {5, 2, 0}, 4: {6, 3, 1, 0}}, StageName: "Round",
		Stories:           []store.Story{{Player: "alder440", Badge: "Reigning World Champion", Title: "2025", Text: "Won."}},
		RatedPlayerCounts: []int{3, 4},
		Divisions:         []store.Division{{Name: "Final", Tier: 1}, {Name: "Semi-final", Tier: 2}}, SuperGM: true, RecordMinGames: 3,
		Seasons: []store.AdminSeason{{Label: "2025", OriginalName: "2025 Edition", Rated: true, InChampions: true, InCommunity: true, RunHere: true, TimeoutPenalty: -1, Games: 3, Finished: 2, Players: 5, Stages: []store.AdminStage{{Label: "Stage 1", OriginalName: "Groups", Note: "n", Number: 1, Games: 2, Finished: 1, Players: 5}},
			Podium: []store.PodiumEntry{{Place: 1, Name: "alder440"}}, PodiumNote: "CGE",
			Suggested: []store.PodiumEntry{{Place: 1, Name: "birch550"}}, SuggestedFrom: "final_stage"}}}, nil
}

func (*fakeAdmins) TournamentRows(context.Context) ([]store.AdminTournamentRow, error) {
	first := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	return []store.AdminTournamentRow{{Slug: "worlds", Name: "World Championship", Kind: "cup", Rated: true, Games: 3, FirstGame: &first,
		LastGame: &first, CGEAbout: "CGE's."}}, nil
}

func (f *fakeAdmins) SetTournament(_ context.Context, slug string, s store.TournamentSettings) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if slices.Contains(s.RatedPlayerCounts, 5) {
		return store.ErrBadPlayerCounts
	}
	if s.Kind == "tour" {
		return store.ErrBadKind
	}
	if slices.ContainsFunc(s.Divisions, func(d store.Division) bool { return d.Tier > 7 }) {
		return store.ErrBadLeagueRules
	}
	if len(s.Points[3]) == 2 {
		return store.ErrBadScoring
	}
	if s.SeasonName != nil && len(*s.SeasonName) > 32 {
		return store.ErrBadLevelName
	}
	if s.Parent != nil && *s.Parent == "worlds" {
		return store.ErrGroupCycle
	}
	f.settings = append(f.settings, s)
	return nil
}

func (f *fakeAdmins) SetMembers(_ context.Context, group string, members []string) error {
	if group == "worlds" {
		return store.ErrNotAGroup
	}
	if slices.Contains(members, "no_such") {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("members %s %v", group, members))
	return nil
}

func (f *fakeAdmins) ChangeTournament(ctx context.Context, slug string, c store.TournamentChange) error {
	if c.Settings != nil {
		if err := f.SetTournament(ctx, slug, *c.Settings); err != nil {
			return err
		}
	}
	if c.Card != nil {
		if err := f.SetCard(ctx, slug, *c.Card); err != nil {
			return err
		}
	}
	if c.Regulations != nil {
		if err := f.SetRegulations(ctx, slug, *c.Regulations); err != nil {
			return err
		}
	}
	if c.Stories != nil {
		if err := f.SetStories(ctx, slug, *c.Stories); err != nil {
			return err
		}
	}
	if c.Members != nil {
		return f.SetMembers(ctx, slug, *c.Members)
	}
	return nil
}

func (f *fakeAdmins) ChangeSeason(ctx context.Context, slug, season string, c store.SeasonChange) error {
	steps := []struct {
		asked bool
		do    func() error
	}{
		{c.Podium != nil, func() error { return f.SetPodium(ctx, slug, season, *c.Podium, c.PodiumNote) }},
		{c.ClearPodium, func() error { return f.ClearPodium(ctx, slug, season) }},
		{c.Rated != nil, func() error { return f.SetSeasonRated(ctx, slug, season, *c.Rated) }},
		{c.InChampions != nil, func() error { return f.SetSeasonInChampions(ctx, slug, season, *c.InChampions) }},
		{c.InCommunity != nil, func() error { return f.SetSeasonInCommunity(ctx, slug, season, *c.InCommunity) }},
		{c.RunHere != nil, func() error {
			f.notes = append(f.notes, fmt.Sprintf("season %s run here %v", season, *c.RunHere))
			return nil
		}},
		{c.TimeoutPenalty != nil, func() error { return f.SetSeasonPenalty(ctx, slug, season, *c.TimeoutPenalty) }},
		{c.Note != nil, func() error { return f.SetNote(ctx, slug, season, "", *c.Note) }},
	}
	for _, st := range steps {
		if st.asked {
			if err := st.do(); err != nil {
				return err
			}
		}
	}
	for stage, note := range c.StageNotes {
		if err := f.SetNote(ctx, slug, season, stage, note); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeAdmins) CreateGroup(_ context.Context, g store.Group) (string, error) {
	if g.Name == "!" {
		return "", store.ErrBadGroup
	}
	if g.Kind == "bowl" {
		return "", store.ErrBadKind
	}
	if slices.Contains(g.Members, "no_such") {
		return "", store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("group %s %s %v", g.Name, g.Kind, g.Members))
	return "big_cups", nil
}

func (f *fakeAdmins) SetSeasonPenalty(_ context.Context, slug, season string, penalty float64) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	if penalty > 0 {
		return store.ErrBadPenalty
	}
	f.notes = append(f.notes, fmt.Sprintf("season %s penalty %g", season, penalty))
	return nil
}

func (f *fakeAdmins) SetSeasonRated(_ context.Context, slug, season string, rated bool) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("season %s rated %v", season, rated))
	return nil
}

func (f *fakeAdmins) SetSeasonInChampions(_ context.Context, slug, season string, in bool) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("season %s champions %v", season, in))
	return nil
}

func (f *fakeAdmins) SetSeasonInCommunity(_ context.Context, slug, season string, in bool) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, fmt.Sprintf("season %s community %v", season, in))
	return nil
}

func (f *fakeAdmins) AuditLog(_ context.Context, filter store.AuditFilter, page int) (store.AuditPage, error) {
	f.logAsked, f.logPage = &filter, page
	return store.AuditPage{Page: page, Pages: 3, Total: 101, Actions: []string{"badge.add"}, Entries: []store.AuditEntry{
		{ID: 7, At: time.Date(2026, 10, 28, 12, 13, 0, 0, time.FixedZone("AEDT", 11*60*60)), Admin: "Owner", Action: "badge.add", Target: "Gold", Detail: []byte(`{"label": "Gold"}`)},
	}}, nil
}

func (f *fakeAdmins) Admins(context.Context) ([]store.Admin, error) {
	return []store.Admin{{DiscordID: "111111111111111111", Name: "Owner", Role: store.RoleAdmin}}, nil
}

func (f *fakeAdmins) SetAdmin(_ context.Context, a store.Admin) error {
	if a.DiscordID == "111111111111111111" && a.Role != store.RoleAdmin {
		return store.ErrLastAdmin
	}
	f.notes = append(f.notes, "set "+a.DiscordID+" "+a.Name+" "+a.Role)
	return nil
}

func (f *fakeAdmins) RemoveAdmin(_ context.Context, id string) error {
	if id == "999999999999999999" {
		return store.ErrNotAdmin
	}
	if id == "111111111111111111" {
		return store.ErrLastAdmin
	}
	f.notes = append(f.notes, "remove "+id)
	return nil
}

func (f *fakeAdmins) AddSeason(_ context.Context, slug, label string) error {
	switch {
	case slug != "worlds":
		return store.ErrNoTournament
	case label == "2025":
		return store.ErrSeasonExists
	}
	f.notes = append(f.notes, "add season "+label)
	return nil
}

func (f *fakeAdmins) RemoveSeason(_ context.Context, slug, label string) error {
	switch {
	case slug != "worlds" || label == "1999":
		return store.ErrNoTournament
	case label == "2025":
		return store.ErrSeasonHasGames
	}
	f.notes = append(f.notes, "remove season "+label)
	return nil
}

func (f *fakeAdmins) SetPodium(_ context.Context, slug, season string, podium []store.PodiumEntry, note string) error {
	switch {
	case slug != "worlds" || season == "1999":
		return store.ErrNoTournament
	case len(podium) > 0 && podium[0].Name == "twice":
		return store.ErrBadPodium
	}
	f.notes = append(f.notes, fmt.Sprintf("podium %s %v %s", season, podium, note))
	return nil
}

func (*fakeAdmins) ExportZip(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, "PK zip bytes")
	return err
}

func (*fakeAdmins) DataSummary(context.Context) (store.DataSummary, error) {
	latest := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	return store.DataSummary{Games: 144691, NeverPlayed: 36, Unplaced: 0, Players: 5210, LatestGame: &latest, DatabaseBytes: 2_019_000_000, RunsDiskBytes: 1_765_000_000}, nil
}

func (*fakeAdmins) CheckStandings(context.Context) (store.StandingsCheck, error) {
	return store.StandingsCheck{StagesOnlyOnCGE: 18, Stages: []store.StageCheck{{
		CGETournament: 19, CGESeason: 4, Name: "Intermezzo Championship", Players: 520, Agree: 519,
		Differences: []store.StandingDifference{{
			CGEUser: 80023, Player: "Heron_Hill", Kind: store.DiffPlacesTimeout, CGEGames: 6, HeldGames: 6, CGEPoints: 2,
			CGEPlaces: store.Places{P3: []int{0, 1, 5}}, HeldPlaces: store.Places{P3: []int{0, 4, 2}},
		}},
	}}}, nil
}

func (*fakeAdmins) Badges(context.Context) ([]store.Badge, error) {
	return []store.Badge{{ID: 3, Label: "Organiser", Description: "Runs a league", Players: []string{"Ann"}}}, nil
}

func (f *fakeAdmins) AddBadge(_ context.Context, b store.Badge) (int64, error) {
	if b.Label == "Organiser" {
		return 0, store.ErrBadgeExists
	}
	f.notes = append(f.notes, "add badge "+b.Label)
	return 4, nil
}

func (f *fakeAdmins) SetBadge(_ context.Context, b store.Badge) error {
	switch {
	case b.ID != 3:
		return store.ErrNoBadge
	case slices.Contains(b.Players, "Nobody"):
		return store.ErrNoPlayer
	}
	f.notes = append(f.notes, fmt.Sprintf("set badge %d %s %v", b.ID, b.Label, b.Players))
	return nil
}

func (f *fakeAdmins) RemoveBadge(_ context.Context, id int64) error {
	if id != 3 {
		return store.ErrNoBadge
	}
	f.notes = append(f.notes, fmt.Sprintf("remove badge %d", id))
	return nil
}

func (*fakeAdmins) TitleOverrides(context.Context) ([]store.TitleOverride, error) {
	return []store.TitleOverride{{Name: "Ann", Title: "SGM", Reason: "won 2019"}}, nil
}

func (f *fakeAdmins) SetTitle(_ context.Context, o store.TitleOverride) error {
	if o.Name == "Nobody" {
		return store.ErrNoPlayer
	}
	if o.Title == "WC" {
		return store.ErrBadTitle
	}
	f.notes = append(f.notes, "title "+o.Name+" "+o.Title+" "+o.Reason)
	return nil
}

func (f *fakeAdmins) ClearTitle(_ context.Context, name string) error {
	if name != "Ann" {
		return store.ErrNoTitle
	}
	f.notes = append(f.notes, "clear title "+name)
	return nil
}

func (f *fakeAdmins) SetStories(_ context.Context, slug string, list []store.Story) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if len(list) > 0 && list[0].Title == "" {
		return store.ErrBadStory
	}
	for _, st := range list {
		f.notes = append(f.notes, "story "+st.Player+"|"+st.Title)
	}
	return nil
}

func (f *fakeAdmins) SetCard(_ context.Context, slug string, c store.Card) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	if c.Icon == "toolong" {
		return store.ErrBadCard
	}
	f.notes = append(f.notes, "card "+c.Icon+"|"+c.Tagline+"|"+c.Summary+"|"+c.Subtitle+"|"+c.Cadence)
	return nil
}

func (f *fakeAdmins) SetRegulations(_ context.Context, slug, text string) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, "regulations "+text)
	return nil
}

func (f *fakeAdmins) ClearPodium(_ context.Context, slug, season string) error {
	if slug != "worlds" || season == "1999" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, "clear podium "+season)
	return nil
}

func (f *fakeAdmins) SetNote(_ context.Context, slug, season, stage, note string) error {
	if slug != "worlds" {
		return store.ErrNoTournament
	}
	f.notes = append(f.notes, season+"|"+stage+"|"+note)
	return nil
}

func (f *fakeAdmins) AdminCommunityEditions(_ context.Context) ([]store.CommunityEdition, error) {
	return []store.CommunityEdition{{
		Slug: "race26", Name: "Race to 2026", Title: "Race to 2026 Title", ShortName: "Race26",
		Badge: "⭐ Community", Maintainer: "dee", MaintainerNote: "Note", Year: 2026,
		CountingEvents: 8, Visible: true, SortOrder: 1,
	}}, nil
}

func (f *fakeAdmins) SetCommunityEdition(_ context.Context, slug string, c store.CommunityEditionChange) error {
	if slug != "race26" {
		return store.ErrNoCommunityEdition
	}
	f.notes = append(f.notes, "community_edition "+slug)
	return nil
}

func (f *fakeAdmins) CreateCommunityEdition(_ context.Context, e store.CommunityEdition) error {
	f.notes = append(f.notes, "create_community_edition "+e.Slug)
	return nil
}

func (*fakeAdmins) Verify(_ context.Context, token string) (access.Identity, error) {
	if len(token) > 5 && token[:5] == "good-" {
		return access.Identity{DiscordID: token[5:]}, nil
	}
	return access.Identity{}, errors.New("bad token")
}

func (*fakeAdmins) Admin(_ context.Context, id string) (store.Admin, error) {
	switch id {
	case "111111111111111111":
		return store.Admin{DiscordID: id, Name: "Owner", Role: store.RoleAdmin}, nil
	case "222222222222222222":
		return store.Admin{DiscordID: id, Name: "Org", Role: store.RoleOrganiser}, nil
	}
	return store.Admin{}, store.ErrNotAdmin
}

func (f *fakeAdmins) Change(_ context.Context, fn func(tx Admins) error) error {
	f.inChange = true
	defer func() { f.inChange = false }()
	return fn(f)
}

func (f *fakeAdmins) Audit(_ context.Context, id, action, target string, detail any) error {
	if !f.inChange {
		return errors.New("audited outside a Change: " + action)
	}
	if f.auditErr != nil {
		return f.auditErr
	}
	b, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	f.audits = append(f.audits, id+" "+action+" "+target)
	f.details = append(f.details, string(b))
	return nil
}

func adminGet(t *testing.T, admins Admins, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/me", nil)
	if token != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", token)
	}
	rec := httptest.NewRecorder()
	newMux(&fakeRatings{}, admins, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	return rec
}

func TestAdminMeNamesTheSignedInAdminAndTheirRole(t *testing.T) {
	for _, tt := range []struct{ id, name, role string }{
		{"111111111111111111", "Owner", "admin"},
		{"222222222222222222", "Org", "organiser"},
	} {
		rec := adminGet(t, &fakeAdmins{}, "good-"+tt.id)
		var got AdminMe
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("GET /v1/admin/me as %s = %d %s", tt.name, rec.Code, rec.Body)
		}
		if got.Name != tt.name || string(got.Role) != tt.role {
			t.Errorf("me = %+v, want %s as %s", got, tt.name, tt.role)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("headers = %v, want no-store and no CORS", rec.Header())
		}
	}
}

func TestAdminRoutesRefuseWithoutAVerifiedAdmin(t *testing.T) {
	tests := []struct {
		name   string
		admins Admins
		token  string
		want   int
	}{
		{"no token", &fakeAdmins{}, "", http.StatusUnauthorized},
		{"a token that does not verify", &fakeAdmins{}, "forged", http.StatusUnauthorized},
		{"a valid token for someone not in admin", &fakeAdmins{}, "good-333333333333333333", http.StatusForbidden},
		{"admin not configured", nil, "good-111111111111111111", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminGet(t, tt.admins, tt.token)
			if rec.Code != tt.want {
				t.Errorf("GET /v1/admin/me with %s = %d, want %d", tt.name, rec.Code, tt.want)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
			}
		})
	}
}

func TestAnAuditThatFailsAnswers500(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct{ name, method, path, body string }{
		{"adding a badge", http.MethodPost, "/v1/admin/badges", `{"label": "TD", "description": ""}`},
		{"removing a badge", http.MethodDelete, "/v1/admin/badges/3", ""},
		{"publishing a run", http.MethodPost, "/v1/admin/runs/8/publish", ""},
		{"exporting the data", http.MethodGet, "/v1/admin/export", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{auditErr: errors.New("the audit row was refused")}
			if rec := adminDoAs(t, f, owner, tt.method, tt.path, site, tt.body); rec.Code != http.StatusInternalServerError {
				t.Errorf("%s %s with the audit failing = %d %s, want 500", tt.method, tt.path, rec.Code, rec.Body)
			}
		})
	}
}

func TestNoPathReachesAnAdminRouteAroundTheGate(t *testing.T) {
	for _, path := range []string{"/v1/./admin/me", "/v1//admin/me", "/v1/x/../admin/me", "/v1/%61dmin/me", "/v1/admin/me/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		newMux(&fakeRatings{}, &fakeAdmins{}, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s without a token = 200 %s, want a refusal", path, rec.Body)
		}
	}
}

func adminDo(t *testing.T, admins Admins, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	return adminDoAs(t, admins, "222222222222222222", method, path, origin, body)
}

func adminDoAs(t *testing.T, admins Admins, who, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	return adminDoWith(t, &fakeRatings{}, admins, who, method, path, origin, body)
}

// adminDoWith is adminDoAs for a route that also reads ratings.
func adminDoWith(t *testing.T, ratings Ratings, admins Admins, who, method, path, origin, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Cf-Access-Jwt-Assertion", "good-"+who)
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	newMux(ratings, admins, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)
	return rec
}

func TestAdminChangesRefuseAnotherOriginOrABadRequest(t *testing.T) {
	tests := []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"no Origin", http.MethodPatch, "/v1/admin/tournaments/worlds", "", `{"visible": true}`, http.StatusForbidden},
		{"another site", http.MethodPatch, "/v1/admin/tournaments/worlds", "https://evil.example", `{"visible": true}`, http.StatusForbidden},
		{"an unknown tournament", http.MethodPatch, "/v1/admin/tournaments/nope", "https://ttac-admin.example", `{"visible": true}`, http.StatusNotFound},
		{"an unknown setting", http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{"deleted": true}`, http.StatusBadRequest},
		{"nothing to change", http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{}`, http.StatusBadRequest},
		{"nothing to change in a season", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://ttac-admin.example", `{}`, http.StatusBadRequest},
		{"a stage note without a stage", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://ttac-admin.example", `{"stage_notes": {"": "x"}}`, http.StatusBadRequest},
		{"a long stage note", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://ttac-admin.example", `{"stage_notes": {"Stage 1": "` + strings.Repeat("x", 2001) + `"}}`, http.StatusBadRequest},
		{"a long season note", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://ttac-admin.example", `{"note": "` + strings.Repeat("x", 2001) + `"}`, http.StatusBadRequest},
		{"a long season label", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/" + strings.Repeat("x", 65), "https://ttac-admin.example", `{"note": "x"}`, http.StatusBadRequest},
		{"a season change from another site", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://evil.example", `{"note": "x"}`, http.StatusForbidden},
		{"a deadline from another site", http.MethodPut, "/v1/admin/blitz/us_open/2026/rounds/101/deadline", "https://evil.example", `{"day": "2026-10-19", "time": "12:00", "zone": "UTC"}`, http.StatusForbidden},
		{"an organiser from another site", http.MethodPost, "/v1/admin/blitz/us_open/2026/organisers", "https://evil.example", `{"member": 21}`, http.StatusForbidden},
		{"a thread text from another site", http.MethodPut, "/v1/admin/blitz/us_open/2026/thread-text", "https://evil.example", `{"message": "Hi {players}.", "no_discord": "{name}."}`, http.StatusForbidden},
		{"a thread text reset from another site", http.MethodDelete, "/v1/admin/blitz/us_open/2026/thread-text", "https://evil.example", "", http.StatusForbidden},
		{"points from another site", http.MethodPut, "/v1/admin/blitz/us_open/2026/points", "https://evil.example", `{"win": 3, "draw": 1, "loss": 0, "forfeit_win": 3, "forfeit_loss": 0, "bye": 3, "skip": 0}`, http.StatusForbidden},
		{"a points reset from another site", http.MethodDelete, "/v1/admin/blitz/us_open/2026/points", "https://evil.example", "", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
}

type countingReader struct {
	r    io.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

func TestRequestBodiesAreLimited(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"a body over the limit", `{"name": "` + strings.Repeat("x", 2<<20) + `"}`, http.StatusRequestEntityTooLarge},
		{"bad JSON under the limit", `{"name": `, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &countingReader{r: strings.NewReader(tt.body)}
			req := httptest.NewRequest(http.MethodPost, "/v1/admin/tournaments", body)
			req.Header.Set("Cf-Access-Jwt-Assertion", "good-222222222222222222")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "https://ttac-admin.example")
			rec := httptest.NewRecorder()

			newMux(&fakeRatings{}, &fakeAdmins{}, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("POST %d bytes: code = %d %s, want %d", len(tt.body), rec.Code, rec.Body, tt.want)
			}
			if body.read > 1<<20+1 {
				t.Errorf("POST %d bytes: read %d, want at most the 1 MiB limit", len(tt.body), body.read)
			}
		})
	}
}

func TestAnAdminManagesOthersInAdmin(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, owner, http.MethodGet, "/v1/admin/admins", "", "")
	var got []AdminPerson
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got) != 1 || got[0].Name != "Owner" {
		t.Fatalf("GET admins = %d %s", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPut, "/v1/admin/admins/100000000000000002", site, `{"name": "Dahlia", "role": "organiser"}`); rec.Code != http.StatusNoContent {
		t.Errorf("PUT another = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDoAs(t, f, owner, http.MethodDelete, "/v1/admin/admins/100000000000000002", site, ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE another = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := []string{"set 100000000000000002 Dahlia organiser", "remove 100000000000000002"}; strings.Join(f.notes, "|") != strings.Join(want, "|") || len(f.audits) != 2 {
		t.Errorf("changes = %q, audits %q", f.notes, f.audits)
	}
}

func TestAdminListChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	tests := []struct {
		name, who, method, path, body string
		want                          int
	}{
		{"an organiser lists", org, http.MethodGet, "/v1/admin/admins", "", http.StatusForbidden},
		{"an organiser promotes themselves", org, http.MethodPut, "/v1/admin/admins/" + org, `{"name": "Org", "role": "admin"}`, http.StatusForbidden},
		{"an organiser adds", org, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "X", "role": "admin"}`, http.StatusForbidden},
		{"the last admin demotes themselves", owner, http.MethodPut, "/v1/admin/admins/" + owner, `{"name": "Owner", "role": "organiser"}`, http.StatusForbidden},
		{"the last admin removes themselves", owner, http.MethodDelete, "/v1/admin/admins/" + owner, "", http.StatusForbidden},
		{"no name", owner, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "", "role": "admin"}`, http.StatusBadRequest},
		{"an unknown role", owner, http.MethodPut, "/v1/admin/admins/333333333333333333", `{"name": "X", "role": "root"}`, http.StatusBadRequest},
		{"someone not in admin", owner, http.MethodDelete, "/v1/admin/admins/999999999999999999", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, tt.who, tt.method, tt.path, site, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || (tt.want != http.StatusNotFound && len(f.notes) != 0) {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAnAdminRenamesThemselves(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, "111111111111111111", http.MethodPut, "/v1/admin/admins/111111111111111111", "https://ttac-admin.example",
		`{"name": "Owner, renamed", "role": "admin"}`)
	if rec.Code != http.StatusNoContent || len(f.audits) != 1 {
		t.Errorf("PUT self = %d %s, audits %q; want 204 and audited", rec.Code, rec.Body, f.audits)
	}
}
