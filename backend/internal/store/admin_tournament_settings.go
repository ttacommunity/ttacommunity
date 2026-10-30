// Changes to a tournament in admin: its settings, card, regulations and stories, each alone or all in one transaction.

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// TournamentSettings are the settings to change; a nil field is left as it is. RatedPlayerCounts are the player
// counts whose games count towards ratings (2, 3, 4).
type TournamentSettings struct {
	Visible, Rated, Community *bool
	// Blitz marks a series run from admin as a blitz tournament: entrants, weekly pairings and results.
	Blitz             *bool
	RatedPlayerCounts []int
	// Kind is league, cup or ladder ("" unchanged); TopDivisions give a league's podium (nil unchanged).
	Kind         string
	TopDivisions []string
	// Points replace the points per place for each player count given; an empty list clears one.
	Points map[int][]float64
	// SeasonName and StageName name its seasons and stages (nil unchanged, "" the default).
	SeasonName, StageName *string
	// Parent is the slug of the group to put it in, "" for none; Description replaces its description, and Name
	// renames the tournament or group (nil unchanged).
	Parent, Description, Name *string
	// FormatSummary replaces its format in brief, trimmed (nil unchanged, "" none).
	FormatSummary *string
	// PodiumSource is how its podium is derived: top_divisions, final_stage, recorded, or none (nil unchanged).
	PodiumSource *string
	// Divisions replace a league's divisions and their title tiers, highest first (nil unchanged, empty none);
	// SuperGM and RecordMinGames set whether a top-division win makes a Super GM and the fewest games a season
	// record needs (nil unchanged).
	Divisions      []Division
	SuperGM        *bool
	RecordMinGames *int
}

// Division is one of a league's divisions and the title tier its players earn, 1 (GM) to 7 (W).
type Division struct {
	Name string
	Tier int
}

// divisions pairs a league's division names with their tiers, as the series table holds them.
func divisions(names []string, tiers []int16) []Division {
	out := make([]Division, len(names))
	for i, n := range names {
		out[i] = Division{Name: n, Tier: int(tiers[i])}
	}
	return out
}

// ErrBadLeagueRules means a division without a name, listed twice or with a tier outside 1 to 7, or a negative
// number of games for a season record.
var ErrBadLeagueRules = errors.New("each division has a name, appears once and has a tier from 1 to 7; a season record needs zero or more games")

// ErrBadPodiumSource means a podium source other than top_divisions, final_stage, recorded, or none.
var ErrBadPodiumSource = errors.New("podium source is top_divisions, final_stage, recorded, or none")

// ErrBadLevelName means a season or stage name longer than 32 characters.
var ErrBadLevelName = errors.New("a season or stage name is at most 32 characters")

// ErrBadScoring means points for a player count other than 2, 3 or 4, not one per place, negative,
// or higher for a worse place.
var ErrBadScoring = errors.New("points are one per place, never negative, and never higher for a worse place")

// ErrBadKind means a kind other than league, cup or ladder.
var ErrBadKind = errors.New("a tournament is a league, a cup or a ladder; a group may also be a series or a group of groups")

// ErrBadPlayerCounts means rated player counts that are empty or not among 2, 3 and 4.
var ErrBadPlayerCounts = errors.New("rated player counts are one or more of 2, 3 and 4")

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
	if s.Divisions != nil {
		clean := make([]Division, 0, len(s.Divisions))
		for _, d := range s.Divisions {
			d.Name = strings.TrimSpace(d.Name)
			if d.Name == "" || d.Tier < 1 || d.Tier > 7 || slices.ContainsFunc(clean, func(c Division) bool { return c.Name == d.Name }) {
				return ErrBadLeagueRules
			}
			clean = append(clean, d)
		}
		s.Divisions = clean
	}
	if s.RecordMinGames != nil && (*s.RecordMinGames < 0 || *s.RecordMinGames > 100) {
		return ErrBadLeagueRules
	}
	if s.PodiumSource != nil && *s.PodiumSource != "" && !slices.Contains([]string{"top_divisions", "final_stage", "recorded", "none"}, *s.PodiumSource) {
		return ErrBadPodiumSource
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
			if _, err := tx.Exec(ctx, "UPDATE series SET name = $2 WHERE series_id = $1", id, *s.Name); err != nil {
				return fmt.Errorf("renaming %s: %w", slug, err)
			}
		}
		if s.PodiumSource != nil {
			var ps *string
			if *s.PodiumSource != "" {
				ps = s.PodiumSource
			}
			if _, err := tx.Exec(ctx, "UPDATE series SET podium_source = $2 WHERE series_id = $1", id, ps); err != nil {
				return fmt.Errorf("setting podium source of %s: %w", slug, err)
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
		if s.FormatSummary != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET format_summary = $2 WHERE series_id = $1", id, strings.TrimSpace(*s.FormatSummary)); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Visible != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET visible = $2 WHERE series_id = $1", id, *s.Visible); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.Blitz != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET blitz = $2 WHERE series_id = $1", id, *s.Blitz); err != nil {
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
		if s.Divisions != nil {
			names, tiers := make([]string, len(s.Divisions)), make([]int16, len(s.Divisions))
			for i, d := range s.Divisions {
				names[i], tiers[i] = d.Name, int16(d.Tier)
			}
			if _, err := tx.Exec(ctx, "UPDATE series SET divisions = $2, division_tiers = $3 WHERE series_id = $1", id, names, tiers); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.SuperGM != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET super_gm = $2 WHERE series_id = $1", id, *s.SuperGM); err != nil {
				return fmt.Errorf("setting %s: %w", slug, err)
			}
		}
		if s.RecordMinGames != nil {
			if _, err := tx.Exec(ctx, "UPDATE series SET record_min_games = $2 WHERE series_id = $1", id, *s.RecordMinGames); err != nil {
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

// TournamentChange is a change to a tournament: each part that is not nil is set.
type TournamentChange struct {
	Settings    *TournamentSettings
	Card        *Card
	Regulations *string
	Stories     *[]Story
	Members     *[]string
}

// ChangeTournament sets every part of c on tournament slug in one transaction, so a refused part changes
// nothing. It returns what the refused part's Set method would.
func (a *AdminStore) ChangeTournament(ctx context.Context, slug string, c TournamentChange) error {
	return a.InTx(ctx, func(tx *AdminStore) error {
		if c.Settings != nil {
			if err := tx.SetTournament(ctx, slug, *c.Settings); err != nil {
				return err
			}
		}
		if c.Card != nil {
			if err := tx.SetCard(ctx, slug, *c.Card); err != nil {
				return err
			}
		}
		if c.Regulations != nil {
			if err := tx.SetRegulations(ctx, slug, *c.Regulations); err != nil {
				return err
			}
		}
		if c.Stories != nil {
			if err := tx.SetStories(ctx, slug, *c.Stories); err != nil {
				return err
			}
		}
		if c.Members != nil {
			return tx.SetMembers(ctx, slug, *c.Members)
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

// SetStories replaces a tournament's stories with these, in this order. It returns ErrNoTournament, ErrBadStory, or
// ErrNoPlayer for a story whose player is no player's name.
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
			tag, err := tx.Exec(ctx, `INSERT INTO tournament_stories (tournament, position, player_id, badge, title, text)
				SELECT $1, $2, player_id, $4, $5, $6 FROM players WHERE name = $3`, slug, i+1, s.Player, s.Badge, s.Title, s.Text)
			if err != nil {
				return fmt.Errorf("setting the stories of %s: %w", slug, err)
			}
			if tag.RowsAffected() == 0 {
				return ErrNoPlayer
			}
		}
		return nil
	})
}
