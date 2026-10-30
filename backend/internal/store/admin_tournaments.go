// A tournament as admin edits it: its settings, seasons, stages, notes and podiums, read in one go. Changes to it
// are in admin_tournament_settings.go, and to its seasons in admin_tournament_seasons.go.

package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrNoTournament means no tournament, or no season of it, has that name.
var ErrNoTournament = errors.New("no such tournament or season")

// AdminTournament is one tournament as admin edits it: Regulations is admin's format and regulations text,
// shown instead of CGERegulations, the latest edition's CGE description; either is empty for none.
type AdminTournament struct {
	Slug, Name     string
	Visible        bool
	Rated          bool
	Community      bool
	Blitz          bool
	Regulations    string
	CGERegulations string
	// FormatSummary is its format in brief, which its group's page lists; "" for none.
	FormatSummary     string
	Card              Card
	Stories           []Story
	RatedPlayerCounts []int
	Kind              string
	PodiumSource      string
	TopDivisions      []string
	// Divisions are a league's divisions, highest first, with their title tiers; SuperGM is whether a top-division
	// win makes a Super GM, and RecordMinGames the fewest games a season record needs.
	Divisions      []Division
	SuperGM        bool
	RecordMinGames int
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
	// RunHere is whether it is run on this site: its CGE tournament gives its page, never its stages or games.
	RunHere bool
	// Organiser is who organised it, as admin set it; CGEOrganiser is its latest CGE tournament's, shown when
	// admin has set none.
	Organiser, CGEOrganiser string
	Stages                  []AdminStage
	Podium                  []PodiumEntry
	PodiumNote              string
	// Suggested is the podium to start from when admin has set none: the organisers' recorded one (SuggestedFrom
	// "recorded"), else the final stage's games when it is one group ("final_stage"), else none ("").
	Suggested     []PodiumEntry
	SuggestedFrom string
	// Games, Finished and Players count its games, those played, and the players in them, across its stages: a
	// season without stages keeps its games in one, unnamed, which Stages leaves out.
	Games, Finished, Players int
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
			se.Games, se.Finished, se.Players = c.Games, c.Finished, c.Players
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
		SELECT t.slug, t.name, t.visible, t.in_community_leaderboard, t.blitz,
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
		       coalesce(se.run_here, false), coalesce(se.organiser, ''), `+seasonCGEOrganiser(cgeOrganiser)+`,
		       t.rated_player_counts, t.kind, coalesce(t.podium_source, ''), t.top_divisions, t.original_name,
		       coalesce(sp.points_2p, '{}'), coalesce(sp.points_3p, '{}'), coalesce(sp.points_4p, '{}'), t.tournament_noun, t.stage_noun,
		       coalesce((SELECT r.timeout_penalty FROM settings r WHERE r.tournament_id = se.tournament_id), 0),
		       coalesce((SELECT p.slug FROM series p WHERE p.series_id = t.parent_id), ''), t.description, t.is_group,
		       t.format_summary, t.divisions, t.division_tiers, t.super_gm, t.record_min_games
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
	var visible, community, blitz, rated bool
	var regulations, cgeRegulations string
	var card Card
	var season, stage, seasonNote, stageNote *string
	var seasonOriginal, stageOriginal string
	var hasGames, seasonRated, inChampions, inCommunity, runHere bool
	var organiser, cgeOrganiserName string
	var counts []int
	var kind, podiumSource, original string
	var top []string
	var p2, p3, p4 []float64
	var seasonName, stageName string
	var penalty float64
	var parent, description, formatSummary string
	var isGroup, superGM bool
	var divisionNames []string
	var divisionTiers []int16
	var recordMinGames int16
	_, err = pgx.ForEachRow(rows, []any{&rowSlug, &name, &visible, &community, &blitz, &rated, &regulations,
		&card.Icon, &card.Tagline, &card.Summary, &card.Subtitle, &card.Cadence, &cgeRegulations, &season, &stage, &seasonOriginal, &stageOriginal, &seasonNote, &stageNote, &hasGames,
		&seasonRated, &inChampions, &inCommunity, &runHere, &organiser, &cgeOrganiserName, &counts, &kind, &podiumSource, &top, &original, &p2, &p3, &p4, &seasonName, &stageName, &penalty, &parent, &description, &isGroup,
		&formatSummary, &divisionNames, &divisionTiers, &superGM, &recordMinGames}, func() error {
		if len(out) == 0 || out[len(out)-1].Slug != rowSlug {
			out = append(out, AdminTournament{Slug: rowSlug, Name: name, Visible: visible, Rated: rated, Community: community, Blitz: blitz,
				Regulations: regulations, CGERegulations: cgeRegulations, FormatSummary: formatSummary, Card: card, RatedPlayerCounts: counts,
				Kind: kind, PodiumSource: podiumSource, TopDivisions: top, Points: map[int][]float64{2: p2, 3: p3, 4: p4},
				SeasonName: seasonName, StageName: stageName, OriginalName: original, Parent: parent, Description: description,
				IsGroup: isGroup, Divisions: divisions(divisionNames, divisionTiers), SuperGM: superGM,
				RecordMinGames: int(recordMinGames), Seasons: []AdminSeason{}})
		}
		t := &out[len(out)-1]
		if season == nil {
			return nil
		}
		if len(t.Seasons) == 0 || t.Seasons[len(t.Seasons)-1].Label != *season {
			t.Seasons = append(t.Seasons, AdminSeason{Label: *season, OriginalName: seasonOriginal, Note: *seasonNote, HasGames: hasGames, Rated: seasonRated, InChampions: inChampions, InCommunity: inCommunity, RunHere: runHere, Organiser: organiser, CGEOrganiser: cgeOrganiserName, TimeoutPenalty: penalty, Stages: []AdminStage{}})
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
		recorded AS (SELECT t.tournament_id, p.name AS player, o.place FROM tournament_official_placements o JOIN players p USING (player_id)
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
	rows, err := a.pool.Query(ctx, `SELECT o.tournament, o.season, o.place, p.name, o.note FROM tournament_podium_overrides o
		JOIN players p USING (player_id) ORDER BY o.place`)
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
