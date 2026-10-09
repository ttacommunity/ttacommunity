// Tournament settings in admin: shown, rated, counted in the community leaderboard, and season and stage notes.

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// ErrNoTournament means no tournament, or no season of it, has that name.
var ErrNoTournament = errors.New("no such tournament or season")

// AdminTournament is one tournament as admin edits it: Regulations is admin's format and regulations text,
// shown instead of CGERegulations, the latest edition's CGE description; either is empty for none.
type AdminTournament struct {
	Slug, Name        string
	Visible           bool
	Rated             bool
	Community         bool
	Regulations       string
	CGERegulations    string
	Card              Card
	Stories           []Story
	RatedPlayerCounts []int
	Kind              string
	TopDivisions      []string
	// Points are the points per place, by player count (2, 3 and 4), empty for a count it does not score.
	Points map[int][]float64
	// SeasonName and StageName are what the tournament calls its seasons and stages, empty for the default.
	SeasonName, StageName string
	// OriginalName is the name the tournament came in with, kept when it is renamed.
	OriginalName string
	// Parent is the slug of the group it is in, "" for none; Description is admin's text about it.
	Parent, Description string
	// IsGroup is whether it is a group made in admin, holding tournaments and groups and no games of its own.
	IsGroup bool
	// Games counts its played and unplayed games; FirstGame and LastGame are their dates, nil for none.
	Games               int
	FirstGame, LastGame *time.Time
	Seasons             []AdminSeason
}

// AdminSeason is a season with its own note, its stages, and the podium admin set for it, if any.
type AdminSeason struct {
	Label, OriginalName string
	Note                string
	HasGames            bool
	Rated               bool
	// InChampions is whether its winner is listed among the tournament's champions.
	InChampions bool
	// TimeoutPenalty is the points a player loses for each game they time out of, 0 or below.
	TimeoutPenalty float64
	// InCommunity is whether the season counts in the community leaderboard.
	InCommunity bool
	Stages      []AdminStage
	Podium      []PodiumEntry
	PodiumNote  string
	// Suggested is the podium to start from when admin has set none: the organisers' recorded one (SuggestedFrom
	// "recorded"), else the final stage's games when it is one group ("final_stage"), else none ("").
	Suggested     []PodiumEntry
	SuggestedFrom string
	// Games and Players count its games, across its stages, and the players in them.
	Games, Players int
}

// PodiumEntry is one place on a podium set in admin.
type PodiumEntry struct {
	Place int
	Name  string
}

// AdminStage is a stage of a season (an empty label is a season without stages) with its note, its number (CGE's,
// else the last number in its label, else 0), and how many games it holds, were played, and players are in them.
type AdminStage struct {
	Label, OriginalName      string
	Note                     string
	Number                   int
	Games, Finished, Players int
}

// TournamentSettings are the settings to change; a nil field is left as it is. RatedPlayerCounts are the player
// counts whose games count towards ratings (2, 3, 4).
type TournamentSettings struct {
	Visible, Rated, Community *bool
	RatedPlayerCounts         []int
	// Kind is league, cup or ladder ("" unchanged); TopDivisions give a league's podium (nil unchanged).
	Kind         string
	TopDivisions []string
	// Points replace the points per place for each player count given; an empty list clears one.
	Points map[int][]float64
	// SeasonName and StageName name its seasons and stages (nil unchanged, "" the default).
	SeasonName, StageName *string
	// Parent is the slug of the group to put it in, "" for none; Description replaces its description, and Name a
	// group's name (nil unchanged).
	Parent, Description, Name *string
}

// ErrBadLevelName means a season or stage name longer than 32 characters.
var ErrBadLevelName = errors.New("a season or stage name is at most 32 characters")

// ErrBadScoring means points for a player count other than 2, 3 or 4, not one per place, negative,
// or higher for a worse place.
var ErrBadScoring = errors.New("points are one per place, never negative, and never higher for a worse place")

// ErrBadKind means a kind other than league, cup or ladder.
var ErrBadKind = errors.New("a tournament is a league, a cup or a ladder; a group may also be a series or a group of groups")

// ErrBadPlayerCounts means rated player counts that are empty or not among 2, 3 and 4.
var ErrBadPlayerCounts = errors.New("rated player counts are one or more of 2, 3 and 4")

// Tournament returns one tournament with its settings, seasons (latest first), stages and notes, each season's and
// stage's games and players, its podiums and its stories; ErrNoTournament when there is none.
func (a *AdminStore) Tournament(ctx context.Context, slug string) (AdminTournament, error) {
	out, err := a.tournament(ctx, slug)
	if err != nil {
		return AdminTournament{}, err
	}
	if len(out) == 0 {
		return AdminTournament{}, ErrNoTournament
	}
	t := out[0]
	counts, err := a.SeasonCounts(ctx, slug)
	if err != nil {
		return AdminTournament{}, err
	}
	for i := range t.Seasons {
		se := &t.Seasons[i]
		for _, c := range counts {
			if c.Season != se.Label {
				continue
			}
			se.Games, se.Players = c.Games, c.Players
			for j := range se.Stages {
				for _, sc := range c.Stages {
					if sc.Stage == se.Stages[j].Label {
						st := &se.Stages[j]
						st.Number, st.Games, st.Finished, st.Players = sc.Number, sc.Games, sc.Finished, sc.Players
					}
				}
			}
		}
	}
	return t, nil
}

// tournament reads the tournament slug's rows: none when there is no such tournament, else one.
func (a *AdminStore) tournament(ctx context.Context, slug string) ([]AdminTournament, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT t.slug, t.name, t.visible, t.in_community_leaderboard,
		       EXISTS (SELECT 1 FROM ranking_series rt JOIN rankings r USING (ranking_id)
		               WHERE r.slug = 'community' AND rt.series_id = t.series_id),
		       coalesce((SELECT text FROM tournament_regulations g WHERE g.tournament = t.slug), ''),
		       coalesce(k.icon, ''), coalesce(k.tagline, ''), coalesce(k.summary, ''), coalesce(k.subtitle, ''), coalesce(k.cadence, ''),
		       coalesce((SELECT c.description FROM cge_tournaments c WHERE c.series_id = t.series_id AND c.description <> ''
		                 ORDER BY coalesce(c.started, c.created, c.finished) DESC NULLS LAST, c.cge_tournament DESC LIMIT 1), ''),
		       se.label, coalesce(st.label, ''), coalesce(se.original_name, ''), coalesce(st.original_name, ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = ''), ''),
		       coalesce((SELECT note FROM tournament_season_notes n WHERE n.tournament = t.slug AND n.season = se.label AND n.stage = st.label AND st.label <> ''), ''),
		       d.last IS NOT NULL, coalesce(se.rated, true), coalesce(se.in_champions, true), coalesce(se.in_community, true),
		       t.rated_player_counts, t.kind, t.top_divisions, t.original_name,
		       coalesce(sp.points_2p, '{}'), coalesce(sp.points_3p, '{}'), coalesce(sp.points_4p, '{}'), t.tournament_noun, t.stage_noun,
		       coalesce((SELECT r.timeout_penalty FROM settings r WHERE r.tournament_id = se.tournament_id), 0),
		       coalesce((SELECT p.slug FROM series p WHERE p.series_id = t.parent_id), ''), t.description, t.is_group
		FROM series t
		LEFT JOIN settings sp ON sp.series_id = t.series_id
		LEFT JOIN tournament_cards k ON k.tournament = t.slug
		LEFT JOIN tournaments se ON se.series_id = t.series_id
		LEFT JOIN stages st ON st.tournament_id = se.tournament_id
		LEFT JOIN (SELECT s2.tournament_id, max(m.date) AS last FROM series x JOIN tournaments t2 USING (series_id)
		           JOIN stages s2 USING (tournament_id) JOIN groups g2 USING (stage_id) JOIN games m USING (group_id)
		           WHERE x.slug = $1 GROUP BY s2.tournament_id) d ON d.tournament_id = se.tournament_id
		WHERE t.slug = $1
		ORDER BY coalesce(substring(se.label FROM '^[0-9]+')::int, -1) DESC, d.last DESC NULLS LAST, se.label DESC, st.stage_id`, slug)
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	var out []AdminTournament
	var rowSlug, name string
	var visible, community, rated bool
	var regulations, cgeRegulations string
	var card Card
	var season, stage, seasonNote, stageNote *string
	var seasonOriginal, stageOriginal string
	var hasGames, seasonRated, inChampions, inCommunity bool
	var counts []int
	var kind, original string
	var top []string
	var p2, p3, p4 []float64
	var seasonName, stageName string
	var penalty float64
	var parent, description string
	var isGroup bool
	_, err = pgx.ForEachRow(rows, []any{&rowSlug, &name, &visible, &community, &rated, &regulations,
		&card.Icon, &card.Tagline, &card.Summary, &card.Subtitle, &card.Cadence, &cgeRegulations, &season, &stage, &seasonOriginal, &stageOriginal, &seasonNote, &stageNote, &hasGames,
		&seasonRated, &inChampions, &inCommunity, &counts, &kind, &top, &original, &p2, &p3, &p4, &seasonName, &stageName, &penalty, &parent, &description, &isGroup}, func() error {
		if len(out) == 0 || out[len(out)-1].Slug != rowSlug {
			out = append(out, AdminTournament{Slug: rowSlug, Name: name, Visible: visible, Rated: rated, Community: community,
				Regulations: regulations, CGERegulations: cgeRegulations, Card: card, RatedPlayerCounts: counts,
				Kind: kind, TopDivisions: top, Points: map[int][]float64{2: p2, 3: p3, 4: p4},
				SeasonName: seasonName, StageName: stageName, OriginalName: original, Parent: parent, Description: description,
				IsGroup: isGroup, Seasons: []AdminSeason{}})
		}
		t := &out[len(out)-1]
		if season == nil {
			return nil
		}
		if len(t.Seasons) == 0 || t.Seasons[len(t.Seasons)-1].Label != *season {
			t.Seasons = append(t.Seasons, AdminSeason{Label: *season, OriginalName: seasonOriginal, Note: *seasonNote, HasGames: hasGames, Rated: seasonRated, InChampions: inChampions, InCommunity: inCommunity, TimeoutPenalty: penalty, Stages: []AdminStage{}})
		}
		s := &t.Seasons[len(t.Seasons)-1]
		if *stage != "" {
			s.Stages = append(s.Stages, AdminStage{Label: *stage, OriginalName: stageOriginal, Note: *stageNote})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading tournaments for admin: %w", err)
	}
	if err := a.addPodiums(ctx, out); err != nil {
		return nil, err
	}
	if err := a.addGames(ctx, out, slug); err != nil {
		return nil, err
	}
	if err := a.addSuggestedPodiums(ctx, out, slug); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Stories, err = stories(ctx, a.pool, out[i].Slug); err != nil {
			return nil, fmt.Errorf("reading %s's stories for admin: %w", out[i].Slug, err)
		}
	}
	return out, nil
}

// addSuggestedPodiums puts on each season the podium admin would start from: its recorded placements, else, once it
// has finished, its final stage's games when that stage is one group, ranked by game points, then culture (equal on
// both share a place).
func (a *AdminStore) addSuggestedPodiums(ctx context.Context, ts []AdminTournament, slug string) error {
	rows, err := a.pool.Query(ctx, `
		WITH s AS (SELECT st.tournament_id, st.stage_id, max(g.date) AS last, count(DISTINCT g.group_id) AS groups
		           FROM games g JOIN groups gr USING (group_id) JOIN stages st USING (stage_id)
		           JOIN tournaments tt USING (tournament_id) JOIN series ss USING (series_id)
		           WHERE g.state = 'played' AND ss.slug = $1 GROUP BY 1, 2),
		final AS (SELECT DISTINCT ON (tournament_id) tournament_id, stage_id, groups FROM s
		          -- Only a finished tournament has a final: its CGE tournament finished, or a league past its season.
		          WHERE NOT EXISTS (SELECT FROM stages x JOIN cge_tournaments c ON c.cge_tournament = x.cge_tournament
		                            WHERE x.tournament_id = s.tournament_id AND c.status <> 'finished'
		                              AND NOT (c.status ~ '^after season [0-9]+$'
		                                       AND x.cge_stage_number <= substring(c.status FROM '[0-9]+$')::int))
		          ORDER BY tournament_id, last DESC, stage_id DESC),
		-- game_lines only for the one-group finals: reading it whole takes seconds.
		seat AS (SELECT f.tournament_id, p.player, p.pts, p.culture FROM final f
		         JOIN game_lines l ON l.stage_id = f.stage_id AND l.state = 'played',
		         unnest(l.players, l.game_points, l.cultures) AS p (player, pts, culture) WHERE f.groups = 1),
		total AS (SELECT tournament_id, player, sum(pts) AS pts, sum(culture) AS culture FROM seat GROUP BY 1, 2),
		derived AS (SELECT tournament_id, player, rank() OVER (PARTITION BY tournament_id ORDER BY pts DESC NULLS LAST,
		                                                         culture DESC NULLS LAST)::int AS place FROM total),
		recorded AS (SELECT t.tournament_id, o.player_name AS player, o.place FROM tournament_official_placements o
		             JOIN series se ON se.slug = o.series JOIN tournaments t ON t.series_id = se.series_id AND t.label = o.season::text
		             WHERE se.slug = $1)
		SELECT se.slug, t.label, x.source, x.place, x.player FROM (
		    SELECT tournament_id, 'recorded' AS source, place, player FROM recorded
		    UNION ALL SELECT d.tournament_id, 'final_stage', d.place, d.player FROM derived d
		    WHERE NOT EXISTS (SELECT FROM recorded r WHERE r.tournament_id = d.tournament_id)) x
		JOIN tournaments t USING (tournament_id) JOIN series se USING (series_id)
		WHERE x.place <= 3 ORDER BY 1, 2, x.place, x.player`, slug)
	if err != nil {
		return fmt.Errorf("suggesting podiums for admin: %w", err)
	}
	type key struct{ slug, season string }
	type suggestion struct {
		from   string
		places []PodiumEntry
	}
	by := map[key]*suggestion{}
	var rowSlug, season, from, player string
	var place int
	if _, err := pgx.ForEachRow(rows, []any{&rowSlug, &season, &from, &place, &player}, func() error {
		k := key{rowSlug, season}
		if by[k] == nil {
			by[k] = &suggestion{from: from}
		}
		by[k].places = append(by[k].places, PodiumEntry{Place: place, Name: player})
		return nil
	}); err != nil {
		return fmt.Errorf("suggesting podiums for admin: %w", err)
	}
	for i := range ts {
		for j := range ts[i].Seasons {
			if s := by[key{ts[i].Slug, ts[i].Seasons[j].Label}]; s != nil {
				ts[i].Seasons[j].Suggested, ts[i].Seasons[j].SuggestedFrom = s.places, s.from
			}
		}
	}
	return nil
}

// addGames puts each series' game count and first and last game dates on it.
func (a *AdminStore) addGames(ctx context.Context, ts []AdminTournament, slug string) error {
	rows, err := a.pool.Query(ctx, `SELECT se.slug, count(*), min(g.date), max(g.date)
		FROM games g JOIN groups gr USING (group_id) JOIN stages st USING (stage_id)
		JOIN tournaments t USING (tournament_id) JOIN series se USING (series_id) WHERE se.slug = $1 GROUP BY se.slug`, slug)
	if err != nil {
		return fmt.Errorf("reading each tournament's games for admin: %w", err)
	}
	type games struct {
		n           int
		first, last time.Time
	}
	by := map[string]games{}
	var rowSlug string
	var g games
	if _, err := pgx.ForEachRow(rows, []any{&rowSlug, &g.n, &g.first, &g.last}, func() error {
		by[rowSlug] = g
		return nil
	}); err != nil {
		return fmt.Errorf("reading each tournament's games for admin: %w", err)
	}
	for i := range ts {
		if g, ok := by[ts[i].Slug]; ok {
			ts[i].Games, ts[i].FirstGame, ts[i].LastGame = g.n, &g.first, &g.last
		}
	}
	return nil
}

// addPodiums puts each podium set in admin on its season.
func (a *AdminStore) addPodiums(ctx context.Context, ts []AdminTournament) error {
	rows, err := a.pool.Query(ctx, "SELECT tournament, season, place, player_name, note FROM tournament_podium_overrides ORDER BY place")
	if err != nil {
		return fmt.Errorf("reading podiums for admin: %w", err)
	}
	seasons := map[[2]string]*AdminSeason{}
	for i := range ts {
		for j := range ts[i].Seasons {
			seasons[[2]string{ts[i].Slug, ts[i].Seasons[j].Label}] = &ts[i].Seasons[j]
		}
	}
	var slug, season, name, note string
	var place int
	_, err = pgx.ForEachRow(rows, []any{&slug, &season, &place, &name, &note}, func() error {
		if se, ok := seasons[[2]string{slug, season}]; ok {
			se.Podium = append(se.Podium, PodiumEntry{Place: place, Name: name})
			if note != "" {
				se.PodiumNote = note
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("reading podiums for admin: %w", err)
	}
	return nil
}

// ErrBadPodium means a podium has nobody, a place outside 1-3 or twice, a player twice, or a blank name.
var ErrBadPodium = errors.New("a podium is 1st to 3rd, each place and player once")

// SetPodium replaces a season's podium with the one given, which the site shows over a recorded or derived
// one, with the note on its first place. It returns ErrNoTournament or ErrBadPodium.
func (a *AdminStore) SetPodium(ctx context.Context, slug, season string, podium []PodiumEntry, note string) error {
	places, names := map[int]bool{}, map[string]bool{}
	for _, e := range podium {
		name := strings.TrimSpace(e.Name)
		if e.Place < 1 || e.Place > 3 || places[e.Place] || name == "" || names[name] {
			return ErrBadPodium
		}
		places[e.Place], names[name] = true, true
	}
	if len(podium) == 0 {
		return ErrBadPodium
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := seasonExists(ctx, tx, slug, season); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_podium_overrides WHERE tournament = $1 AND season = $2", slug, season); err != nil {
			return fmt.Errorf("setting the podium of %s %s: %w", slug, season, err)
		}
		for _, e := range podium {
			n := ""
			if e.Place == podium[0].Place {
				n = note
			}
			if _, err := tx.Exec(ctx, `INSERT INTO tournament_podium_overrides (tournament, season, place, player_name, note)
				VALUES ($1, $2, $3, $4, $5)`, slug, season, e.Place, strings.TrimSpace(e.Name), n); err != nil {
				return fmt.Errorf("setting the podium of %s %s: %w", slug, season, err)
			}
		}
		return nil
	})
}

// ClearPodium removes the podium admin set for a season, so the recorded or derived one shows again.
// It returns ErrNoTournament for a season that does not exist.
func (a *AdminStore) ClearPodium(ctx context.Context, slug, season string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := seasonExists(ctx, tx, slug, season); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_podium_overrides WHERE tournament = $1 AND season = $2", slug, season); err != nil {
			return fmt.Errorf("clearing the podium of %s %s: %w", slug, season, err)
		}
		return nil
	})
}

// seasonExists returns ErrNoTournament unless the tournament has that season.
func seasonExists(ctx context.Context, tx pgx.Tx, slug, season string) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM series t JOIN tournaments se USING (series_id)
		WHERE t.slug = $1 AND se.label = $2)`, slug, season).Scan(&exists)
	if err != nil {
		return fmt.Errorf("finding %s %s: %w", slug, season, err)
	}
	if !exists {
		return ErrNoTournament
	}
	return nil
}

// validScoring reports whether points suit a game of that many players: none, or one per place,
// never negative and never higher for a worse place.
func validScoring(players int, points []float64) bool {
	if players < 2 || players > 4 || (len(points) != 0 && len(points) != players) {
		return false
	}
	for i, p := range points {
		if p < 0 || (i > 0 && p > points[i-1]) {
			return false
		}
	}
	return true
}

// SetTournament changes a tournament's settings in one transaction, or returns ErrNoTournament.
func (a *AdminStore) SetTournament(ctx context.Context, slug string, s TournamentSettings) error {
	if s.Kind != "" && !slices.Contains(kinds, s.Kind) && !slices.Contains(groupKinds, s.Kind) {
		return ErrBadKind
	}
	if s.Name != nil {
		if *s.Name = strings.TrimSpace(*s.Name); *s.Name == "" || utf8.RuneCountInString(*s.Name) > 100 {
			return ErrBadGroup
		}
	}
	if s.TopDivisions != nil {
		top := make([]string, 0, len(s.TopDivisions))
		for _, d := range s.TopDivisions {
			if d = strings.TrimSpace(d); d != "" && !slices.Contains(top, d) {
				top = append(top, d)
			}
		}
		s.TopDivisions = top
	}
	if s.RatedPlayerCounts != nil {
		counts := slices.Sorted(slices.Values(s.RatedPlayerCounts))
		counts = slices.Compact(counts)
		if len(counts) == 0 || counts[0] < 2 || counts[len(counts)-1] > 4 {
			return ErrBadPlayerCounts
		}
		s.RatedPlayerCounts = counts
	}
	for players, points := range s.Points {
		if !validScoring(players, points) {
			return ErrBadScoring
		}
	}
	levels := map[string]*string{"tournament_noun": s.SeasonName, "stage_noun": s.StageName}
	for _, name := range levels {
		if name != nil {
			if *name = strings.TrimSpace(*name); utf8.RuneCountInString(*name) > 32 {
				return ErrBadLevelName
			}
		}
	}
	if s.Description != nil {
		if *s.Description = strings.TrimSpace(*s.Description); utf8.RuneCountInString(*s.Description) > 2000 {
			return ErrBadGroup
		}
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		if err := tx.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", slug).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		} else if err != nil {
			return fmt.Errorf("setting %s: %w", slug, err)
		}
		var isGroup bool
		if err := tx.QueryRow(ctx, "SELECT is_group FROM series WHERE series_id = $1", id).Scan(&isGroup); err != nil {
			return fmt.Errorf("setting %s: %w", slug, err)
		}
		if slices.Contains(groupKinds, s.Kind) && !isGroup {
			return ErrBadKind
		}
		if s.Name != nil {
			if !isGroup {
				return ErrNotAGroup
			}
			if _, err := tx.Exec(ctx, "UPDATE series SET name = $2 WHERE series_id = $1", id, *s.Name); err != nil {
				return fmt.Errorf("renaming %s: %w", slug, err)
			}
		}
		if s.Parent != nil {
			var parent *int64
			if *s.Parent != "" {
				parent = new(int64)
				if err := tx.QueryRow(ctx, "SELECT series_id FROM series WHERE slug = $1", *s.Parent).Scan(parent); errors.Is(err, pgx.ErrNoRows) {
					return ErrNoTournament
				} else if err != nil {
					return fmt.Errorf("setting %s's group: %w", slug, err)
				}
			}
			if err := setParent(ctx, tx, slug, parent); err != nil {
				return err
			}
		}
		if s.Description != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET description = $2 WHERE series_id = $1", id, *s.Description); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Visible != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET visible = $2 WHERE series_id = $1", id, *s.Visible); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Kind != "" {
			if _, err := tx.Exec(ctx, "UPDATE series SET kind = $2 WHERE series_id = $1", id, s.Kind); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.TopDivisions != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET top_divisions = $2 WHERE series_id = $1", id, s.TopDivisions); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.RatedPlayerCounts != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET rated_player_counts = $2 WHERE series_id = $1", id, s.RatedPlayerCounts); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		for players, points := range s.Points {
			var set *[]float64 // none set: nothing to score with
			if len(points) > 0 {
				set = &points
			}
			column := pgx.Identifier{fmt.Sprintf("points_%dp", players)}.Sanitize()
			if _, err := tx.Exec(ctx, "INSERT INTO settings (series_id, "+column+", source) VALUES ($1, $2, 'admin') "+
				"ON CONFLICT (series_id) DO UPDATE SET "+column+" = excluded."+column, id, set); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		for column, name := range levels {
			if name != nil {
				if _, err := tx.Exec(ctx, "UPDATE series SET "+pgx.Identifier{column}.Sanitize()+" = $2 WHERE series_id = $1", id, *name); err != nil {
					return fmt.Errorf("setting %s: %w", slug, err)
				}
			}
		}
		if s.Community != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET in_community_leaderboard = $2 WHERE series_id = $1", id, *s.Community); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Rated != nil {
			q := `DELETE FROM ranking_series WHERE series_id = $1 AND ranking_id = community_ranking()`
			if *s.Rated {
				q = `INSERT INTO ranking_series (ranking_id, series_id) VALUES (community_ranking(), $1) ON CONFLICT DO NOTHING`
			}
			if _, err := tx.Exec(ctx, q, id); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		return nil
	})
}

// SetNote saves the note shown beside a season (stage "") or one of its stages; an empty note removes it.
// It returns ErrNoTournament when the tournament has no such season or stage.
func (a *AdminStore) SetNote(ctx context.Context, slug, season, stage, note string) error {
	var exists bool
	if err := a.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM series t JOIN tournaments se USING (series_id)
		               LEFT JOIN stages st USING (tournament_id)
		               WHERE t.slug = $1 AND se.label = $2 AND ($3 = '' OR st.label = $3))`, slug, season, stage).Scan(&exists); err != nil {
		return fmt.Errorf("noting %s %s: %w", slug, season, err)
	}
	if !exists {
		return ErrNoTournament
	}
	if note == "" {
		_, err := a.pool.Exec(ctx, "DELETE FROM tournament_season_notes WHERE tournament = $1 AND season = $2 AND stage = $3", slug, season, stage)
		return err
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO tournament_season_notes (tournament, season, stage, note, source) VALUES ($1, $2, $3, $4, 'admin')
		ON CONFLICT (tournament, season, stage) DO UPDATE SET note = excluded.note, source = excluded.source`, slug, season, stage, note)
	if err != nil {
		return fmt.Errorf("noting %s %s: %w", slug, season, err)
	}
	return nil
}

// ErrSeasonExists means the tournament already has a season with that label.
var ErrSeasonExists = errors.New("the tournament already has that season")

// ErrSeasonHasGames means the season has stages (and so games), and cannot be removed in admin.
var ErrSeasonHasGames = errors.New("the season has games")

// seasonPrefix is a leading "Season", which the site adds to a label itself.
var seasonPrefix = regexp.MustCompile(`(?i)^season\s+`)

// AddSeason adds a season with no games yet to a tournament, so a note can be shown for it. A leading
// "Season" is dropped from the label ("Season 1" is season "1"), as the site adds it.
func (a *AdminStore) AddSeason(ctx context.Context, slug, label string) error {
	typed := strings.TrimSpace(label)
	label = seasonPrefix.ReplaceAllString(typed, "")
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO tournaments (series_id, label, original_name)
		SELECT series_id, $2, $3 FROM series WHERE slug = $1
		ON CONFLICT (series_id, label) DO NOTHING`, slug, label, typed)
	if err != nil {
		return fmt.Errorf("adding season %s of %s: %w", label, slug, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := a.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM series WHERE slug = $1)", slug).Scan(&exists); err != nil {
		return fmt.Errorf("adding season %s of %s: %w", label, slug, err)
	}
	if !exists {
		return ErrNoTournament
	}
	return ErrSeasonExists
}

// RemoveSeason removes a season that has no stages (no games), and its notes. It returns ErrNoTournament for a
// season that does not exist and ErrSeasonHasGames for one with stages.
func (a *AdminStore) RemoveSeason(ctx context.Context, slug, label string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		var stages bool
		err := tx.QueryRow(ctx, `
			SELECT se.tournament_id, EXISTS (SELECT 1 FROM stages st WHERE st.tournament_id = se.tournament_id)
			FROM series t JOIN tournaments se USING (series_id) WHERE t.slug = $1 AND se.label = $2`,
			slug, label).Scan(&id, &stages)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		}
		if err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		if stages {
			return ErrSeasonHasGames
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_season_notes WHERE tournament = $1 AND season = $2", slug, label); err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournaments WHERE tournament_id = $1", id); err != nil {
			return fmt.Errorf("removing season %s of %s: %w", label, slug, err)
		}
		return nil
	})
}

// SetRegulations saves a tournament's format and regulations text, shown instead of CGE's; an empty text
// removes it. It returns ErrNoTournament for a tournament that does not exist.
func (a *AdminStore) SetRegulations(ctx context.Context, slug, text string) error {
	text = strings.TrimSpace(text)
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM series WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the regulations of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		q := `INSERT INTO tournament_regulations (tournament, text) VALUES ($1, $2)
			ON CONFLICT (tournament) DO UPDATE SET text = excluded.text`
		args := []any{slug, text}
		if text == "" {
			q, args = "DELETE FROM tournament_regulations WHERE tournament = $1", []any{slug}
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("setting the regulations of %s: %w", slug, err)
		}
		return nil
	})
}

// ErrBadCard means a card's icon is over 8 characters, its description over 60 or its information box over 1,000.
var ErrBadCard = errors.New("a card's icon is at most 8 characters, its description, subtitle and cadence 60 and its information box 1,000")

// SetCard saves a tournament's card, trimmed; an empty card removes it. It returns ErrNoTournament or ErrBadCard.
func (a *AdminStore) SetCard(ctx context.Context, slug string, c Card) error {
	c = Card{Icon: strings.TrimSpace(c.Icon), Tagline: strings.TrimSpace(c.Tagline), Summary: strings.TrimSpace(c.Summary),
		Subtitle: strings.TrimSpace(c.Subtitle), Cadence: strings.TrimSpace(c.Cadence)}
	if utf8.RuneCountInString(c.Icon) > 8 || utf8.RuneCountInString(c.Tagline) > 60 || utf8.RuneCountInString(c.Summary) > 1000 ||
		utf8.RuneCountInString(c.Subtitle) > 60 || utf8.RuneCountInString(c.Cadence) > 60 {
		return ErrBadCard
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM series WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the card of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		q := `INSERT INTO tournament_cards (tournament, icon, tagline, summary, subtitle, cadence) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (tournament) DO UPDATE SET icon = excluded.icon, tagline = excluded.tagline, summary = excluded.summary,
				subtitle = excluded.subtitle, cadence = excluded.cadence`
		args := []any{slug, c.Icon, c.Tagline, c.Summary, c.Subtitle, c.Cadence}
		if c == (Card{}) {
			q, args = "DELETE FROM tournament_cards WHERE tournament = $1", []any{slug}
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return fmt.Errorf("setting the card of %s: %w", slug, err)
		}
		return nil
	})
}

// ErrBadStory means more than 12 stories, or one without a player, title or story, or over its lengths.
var ErrBadStory = errors.New("at most 12 stories, each with a player, a title (120) and the story (1,000), a badge of up to 60")

// SetStories replaces a tournament's stories with these, in this order. It returns ErrNoTournament or ErrBadStory.
func (a *AdminStore) SetStories(ctx context.Context, slug string, list []Story) error {
	if len(list) > 12 {
		return ErrBadStory
	}
	clean := make([]Story, len(list))
	for i, s := range list {
		s = Story{Player: strings.TrimSpace(s.Player), Badge: strings.TrimSpace(s.Badge), Title: strings.TrimSpace(s.Title), Text: strings.TrimSpace(s.Text)}
		if s.Player == "" || s.Title == "" || s.Text == "" || utf8.RuneCountInString(s.Player) > 200 ||
			utf8.RuneCountInString(s.Badge) > 60 || utf8.RuneCountInString(s.Title) > 120 || utf8.RuneCountInString(s.Text) > 1000 {
			return ErrBadStory
		}
		clean[i] = s
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM series WHERE slug = $1)", slug).Scan(&exists); err != nil {
			return fmt.Errorf("setting the stories of %s: %w", slug, err)
		}
		if !exists {
			return ErrNoTournament
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tournament_stories WHERE tournament = $1", slug); err != nil {
			return fmt.Errorf("setting the stories of %s: %w", slug, err)
		}
		for i, s := range clean {
			if _, err := tx.Exec(ctx, `INSERT INTO tournament_stories (tournament, position, player_name, badge, title, text)
				VALUES ($1, $2, $3, $4, $5, $6)`, slug, i+1, s.Player, s.Badge, s.Title, s.Text); err != nil {
				return fmt.Errorf("setting the stories of %s: %w", slug, err)
			}
		}
		return nil
	})
}

// SetSeasonInChampions sets whether a season's winner is listed among the tournament's champions, or returns
// ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonInChampions(ctx context.Context, slug, season string, in bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET in_champions = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, in)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is among the champions: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonInCommunity sets whether a season counts in the community leaderboard, from the next run, or
// returns ErrNoTournament for a season that does not exist.
func (a *AdminStore) SetSeasonInCommunity(ctx context.Context, slug, season string, in bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET in_community = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, in)
	if err != nil {
		return fmt.Errorf("setting whether %s %s counts in the community leaderboard: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

// SetSeasonRated sets whether a season's games count towards ratings, from the next run, or returns
// ErrNoTournament for a season that does not exist.
// ErrBadPenalty means a time-out penalty above 0, or for a season not numbered.
var ErrBadPenalty = errors.New("a time-out penalty is 0 or below, for a numbered season")

// SetSeasonPenalty sets the points a player loses in a season's standings for each game they time out of.
func (a *AdminStore) SetSeasonPenalty(ctx context.Context, slug, season string, penalty float64) error {
	number, err := strconv.Atoi(season)
	if penalty > 0 || err != nil || number < 1 {
		return ErrBadPenalty
	}
	tag, err := a.pool.Exec(ctx, `
		INSERT INTO settings (tournament_id, timeout_penalty, source)
		SELECT se.tournament_id, $3, 'admin' FROM series t JOIN tournaments se USING (series_id)
		WHERE t.slug = $1 AND se.label = $2::int::text
		ON CONFLICT (tournament_id) DO UPDATE SET timeout_penalty = excluded.timeout_penalty, source = excluded.source`,
		slug, number, penalty)
	if err != nil {
		return fmt.Errorf("setting %s %s's time-out penalty: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}

func (a *AdminStore) SetSeasonRated(ctx context.Context, slug, season string, rated bool) error {
	tag, err := a.pool.Exec(ctx, `UPDATE tournaments se SET rated = $3 FROM series t
		WHERE t.series_id = se.series_id AND t.slug = $1 AND se.label = $2`, slug, season, rated)
	if err != nil {
		return fmt.Errorf("setting whether %s %s is rated: %w", slug, season, err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}
