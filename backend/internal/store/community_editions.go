// Community leaderboard editions: metadata, dynamic categories and event columns derived from the run's data,
// and admin edition configuration.

package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ErrNoCommunityEdition means no edition with that slug exists.
var ErrNoCommunityEdition = errors.New("no such community edition")

// CommunityEdition is one community leaderboard edition's metadata as configured in admin.
type CommunityEdition struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	Title          string `json:"title"`
	ShortName      string `json:"short_name"`
	Badge          string `json:"badge"`
	Maintainer     string `json:"maintainer"`
	MaintainerNote string `json:"maintainer_note"`
	Year           int    `json:"year"`
	CountingEvents int    `json:"counting_events"`
	Visible        bool   `json:"visible"`
	SortOrder      int    `json:"sort_order"`
}

// CommunityEditionChange is an admin edit to an edition.
type CommunityEditionChange struct {
	Name           *string
	Title          *string
	ShortName      *string
	Badge          *string
	Maintainer     *string
	MaintainerNote *string
	Year           *int
	CountingEvents *int
	Visible        *bool
	SortOrder      *int
}

// CommunityEventColumn is one event column in an edition: ID, label, and maximum possible points.
type CommunityEventColumn struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Short  string `json:"short"`
	MaxPts *int   `json:"max_pts"`
	Desc   string `json:"desc,omitempty"`
}

// CommunityCategory is a group of event columns in an edition (e.g. World Championship, International, etc.).
type CommunityCategory struct {
	Category   string                 `json:"category"`
	BadgeClass string                 `json:"badge_class"`
	Columns    []CommunityEventColumn `json:"columns"`
}

// CommunityTournamentSummary is a tournament's aggregated summary column for the standard view.
type CommunityTournamentSummary struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Short      string   `json:"short"`
	Cols       []string `json:"cols"`
	MaxPts     *int     `json:"max_pts"`
	BadgeClass string   `json:"badge_class"`
	Desc       string   `json:"desc"`
}

// CommunityEditionInfo is the header info and champion for the hero block.
type CommunityEditionInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Title          string `json:"title"`
	Short          string `json:"short"`
	Badge          string `json:"badge"`
	Year           int    `json:"year"`
	WC             string `json:"wc"`
	WCDesc         string `json:"wc_desc"`
	Maintainer     string `json:"maintainer"`
	MaintainerNote string `json:"maintainer_note"`
}

// CommunityEditionLayout is the complete dynamic layout for an edition.
type CommunityEditionLayout struct {
	Info           CommunityEditionInfo         `json:"info"`
	Categories     []CommunityCategory          `json:"categories"`
	Summary        []CommunityTournamentSummary `json:"summary"`
	CountingEvents int                          `json:"counting_events"`
	SheetOrder     []string                     `json:"sheet_order"`
}

// CommunityEditions returns all visible community editions for the site, ordered by sort_order.
func (r *RatingsReader) CommunityEditions(ctx context.Context) ([]CommunityEdition, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order
		FROM community_editions
		WHERE visible = true
		ORDER BY sort_order, year DESC, slug`)
	if err != nil {
		return nil, fmt.Errorf("reading community editions: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[CommunityEdition])
}

// AdminCommunityEditions returns all community editions for admin, including non-visible ones.
func (a *AdminStore) AdminCommunityEditions(ctx context.Context) ([]CommunityEdition, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order
		FROM community_editions
		ORDER BY sort_order, year DESC, slug`)
	if err != nil {
		return nil, fmt.Errorf("reading admin community editions: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[CommunityEdition])
}

// SetCommunityEdition updates an edition's fields in admin.
func (a *AdminStore) SetCommunityEdition(ctx context.Context, slug string, c CommunityEditionChange) error {
	var current CommunityEdition
	err := a.pool.QueryRow(ctx, `
		SELECT slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order
		FROM community_editions WHERE slug = $1`, slug).Scan(
		&current.Slug, &current.Name, &current.Title, &current.ShortName, &current.Badge,
		&current.Maintainer, &current.MaintainerNote, &current.Year, &current.CountingEvents,
		&current.Visible, &current.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoCommunityEdition
	}
	if err != nil {
		return fmt.Errorf("reading edition %s: %w", slug, err)
	}

	if c.Name != nil {
		current.Name = *c.Name
	}
	if c.Title != nil {
		current.Title = *c.Title
	}
	if c.ShortName != nil {
		current.ShortName = *c.ShortName
	}
	if c.Badge != nil {
		current.Badge = *c.Badge
	}
	if c.Maintainer != nil {
		current.Maintainer = *c.Maintainer
	}
	if c.MaintainerNote != nil {
		current.MaintainerNote = *c.MaintainerNote
	}
	if c.Year != nil {
		current.Year = *c.Year
	}
	if c.CountingEvents != nil {
		current.CountingEvents = *c.CountingEvents
	}
	if c.Visible != nil {
		current.Visible = *c.Visible
	}
	if c.SortOrder != nil {
		current.SortOrder = *c.SortOrder
	}

	_, err = a.pool.Exec(ctx, `
		UPDATE community_editions
		SET name = $2, title = $3, short_name = $4, badge = $5, maintainer = $6, maintainer_note = $7,
		    year = $8, counting_events = $9, visible = $10, sort_order = $11
		WHERE slug = $1`,
		slug, current.Name, current.Title, current.ShortName, current.Badge, current.Maintainer, current.MaintainerNote,
		current.Year, current.CountingEvents, current.Visible, current.SortOrder)
	if err != nil {
		return fmt.Errorf("updating edition %s: %w", slug, err)
	}
	return nil
}

// CreateCommunityEdition creates a new edition in admin.
func (a *AdminStore) CreateCommunityEdition(ctx context.Context, e CommunityEdition) error {
	if strings.TrimSpace(e.Slug) == "" {
		return errors.New("edition slug is required")
	}
	if strings.TrimSpace(e.Name) == "" {
		return errors.New("edition name is required")
	}
	if e.CountingEvents <= 0 {
		e.CountingEvents = 8
	}
	_, err := a.pool.Exec(ctx, `
		INSERT INTO community_editions (slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		e.Slug, e.Name, e.Title, e.ShortName, e.Badge, e.Maintainer, e.MaintainerNote, e.Year, e.CountingEvents, e.Visible, e.SortOrder)
	if err != nil {
		return fmt.Errorf("creating edition %s: %w", e.Slug, err)
	}
	return nil
}

// CommunityLeaderboardLayout builds the dynamic layout (categories, columns, max points, summary, reigning WC)
// from the database for an edition in a published run: a column for each event its rows have points in. An
// unknown edition, or one admin has not shown, is ErrNoCommunityEdition.
func (r *RatingsReader) CommunityLeaderboardLayout(ctx context.Context, run int64, slug string) (CommunityEditionLayout, error) {
	var layout CommunityEditionLayout
	var ed CommunityEdition
	err := r.pool.QueryRow(ctx, `
		SELECT slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order
		FROM community_editions WHERE slug = $1 AND visible`, slug).Scan(
		&ed.Slug, &ed.Name, &ed.Title, &ed.ShortName, &ed.Badge,
		&ed.Maintainer, &ed.MaintainerNote, &ed.Year, &ed.CountingEvents,
		&ed.Visible, &ed.SortOrder)
	if errors.Is(err, pgx.ErrNoRows) {
		return layout, ErrNoCommunityEdition
	}
	if err != nil {
		return layout, fmt.Errorf("reading edition config %s: %w", slug, err)
	}

	layout.CountingEvents = ed.CountingEvents
	layout.Info = CommunityEditionInfo{
		ID:             ed.Slug,
		Name:           ed.Name,
		Title:          ed.Title,
		Short:          ed.ShortName,
		Badge:          ed.Badge,
		Year:           ed.Year,
		Maintainer:     ed.Maintainer,
		MaintainerNote: ed.MaintainerNote,
	}
	if layout.Info.WC, layout.Info.WCDesc, err = r.editionChampion(ctx, run, ed.Year); err != nil {
		return layout, fmt.Errorf("reading edition %s's champion: %w", ed.Slug, err)
	}

	// 1. The events its rows have points in, each with the most its points table gives.
	rows, err := r.pool.Query(ctx, "SELECT DISTINCT event FROM community_scores WHERE edition = $1", ed.Slug)
	if err != nil {
		return layout, fmt.Errorf("reading events for edition %s: %w", ed.Slug, err)
	}
	presentEvents, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return layout, fmt.Errorf("reading events for edition %s: %w", ed.Slug, err)
	}
	rows, err = r.pool.Query(ctx, `
		SELECT s.community_code, p.from_edition, max(p.points)
		FROM community_points p JOIN series s ON s.slug = p.tournament
		WHERE s.community_code IS NOT NULL GROUP BY 1, 2`)
	if err != nil {
		return layout, fmt.Errorf("reading the points tables: %w", err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowToStructByPos[pointsTable])
	if err != nil {
		return layout, fmt.Errorf("reading the points tables: %w", err)
	}
	eventMax := map[string]*int{}
	for _, ev := range presentEvents {
		eventMax[ev] = eventMaximum(ev, ed.Year, tables)
	}

	// 2. Order events by the canonical sheet order:
	columnsOrder := []string{"wrld", "ic", "im", "rl", "wmbl", "aus", "surv", "snail", "sl", "ml", "eif", "qd", "ldrb"}
	sheetColRank := func(code string) (int, int, string) {
		kind, rest, _ := strings.Cut(code, "_")
		idx := slices.Index(columnsOrder, kind)
		if idx == -1 {
			idx = len(columnsOrder)
		}
		season, _ := strconv.Atoi(rest)
		return idx, season, code
	}

	slices.SortFunc(presentEvents, func(a, b string) int {
		ka, sa, ca := sheetColRank(a)
		kb, sb, cb := sheetColRank(b)
		if ka != kb {
			return ka - kb
		}
		if sa != sb {
			return sa - sb
		}
		return strings.Compare(ca, cb)
	})

	layout.SheetOrder = presentEvents

	// 3. Assemble Categories and Columns
	categoryBuilders := []struct {
		category   string
		badgeClass string
		prefix     string
		nameFmt    func(code string) (name, short, desc string)
	}{
		{
			category:   "World Championship",
			badgeClass: "badge-wc",
			prefix:     "wrld",
			nameFmt: func(code string) (string, string, string) {
				return "World Championship", "Worlds", worldsDesc(layout.Info.WC)
			},
		},
		{
			category:   "International Championship (4-Player League)",
			badgeClass: "badge-gm",
			prefix:     "ic",
			nameFmt: func(code string) (string, string, string) {
				_, s, _ := strings.Cut(code, "_")
				return fmt.Sprintf("International Season %s", s), fmt.Sprintf("IC %s", s), ""
			},
		},
		{
			category:   "Intermezzo Championship (3-Player League)",
			badgeClass: "badge-m",
			prefix:     "im",
			nameFmt: func(code string) (string, string, string) {
				_, s, _ := strings.Cut(code, "_")
				return fmt.Sprintf("Intermezzo Season %s", s), fmt.Sprintf("IM %s", s), ""
			},
		},
		{
			category:   "Royal League (2-Player League)",
			badgeClass: "badge-p",
			prefix:     "rl",
			nameFmt: func(code string) (string, string, string) {
				_, s, _ := strings.Cut(code, "_")
				return fmt.Sprintf("Royal League Season %s", s), fmt.Sprintf("RL %s", s), ""
			},
		},
		{
			category:   "Grand Slams & Major Opens",
			badgeClass: "badge-g",
			prefix:     "wmbl|aus|snail",
			nameFmt: func(code string) (string, string, string) {
				switch code {
				case "wmbl":
					return "Wimbledon Open", "Wimbledon", ""
				case "aus":
					return "Australian Open", "Aussie Open", ""
				case "snail":
					return "Snail Cup", "Snail Cup", ""
				default:
					return code, code, ""
				}
			},
		},
		{
			category:   "Competitive Ladders",
			badgeClass: "badge-rank",
			prefix:     "sl|ml",
			nameFmt: func(code string) (string, string, string) {
				switch code {
				case "sl":
					return "Sodium Ladder", "Sodium", ""
				case "ml":
					return "Mercurial Ladder", "Mercurial", ""
				default:
					return code, code, ""
				}
			},
		},
		{
			category:   "Invitational & Rapid Cups",
			badgeClass: "badge-rank",
			prefix:     "eif|qd|ldrb|surv",
			nameFmt: func(code string) (string, string, string) {
				switch code {
				case "eif":
					return "Eiffel Tower Cup", "Eiffel", ""
				case "qd":
					return "Quick & Dirty (Swiss)", "Q&D", ""
				case "ldrb":
					return "Leaderboard Trophy", "Trophy", ""
				case "surv":
					return "Survivors Cup", "Survivors", ""
				default:
					return code, code, ""
				}
			},
		},
	}

	for _, b := range categoryBuilders {
		var catCols []CommunityEventColumn
		patterns := strings.Split(b.prefix, "|")
		for _, ev := range presentEvents {
			matches := false
			for _, pat := range patterns {
				if ev == pat || strings.HasPrefix(ev, pat+"_") {
					matches = true
					break
				}
			}
			if matches {
				name, short, desc := b.nameFmt(ev)
				catCols = append(catCols, CommunityEventColumn{
					ID:     ev,
					Name:   name,
					Short:  short,
					MaxPts: eventMax[ev],
					Desc:   desc,
				})
			}
		}
		if len(catCols) > 0 {
			layout.Categories = append(layout.Categories, CommunityCategory{
				Category:   b.category,
				BadgeClass: b.badgeClass,
				Columns:    catCols,
			})
		}
	}

	// 4. Build Summary list (Standard View)
	summaryGroups := []struct {
		id         string
		name       string
		short      string
		badgeClass string
		descFmt    func(cols []string) string
	}{
		{"wrld", "World Championship", "Worlds", "badge-wc", func(cols []string) string {
			return worldsDesc(layout.Info.WC)
		}},
		{"ic", "International (4P)", "International (4P)", "badge-gm", func(cols []string) string {
			return formatRangeDesc("International", cols)
		}},
		{"im", "Intermezzo (3P)", "Intermezzo (3P)", "badge-m", func(cols []string) string {
			return formatRangeDesc("Intermezzo", cols)
		}},
		{"rl", "Royal League", "Royal League", "badge-p", func(cols []string) string {
			return formatRangeDesc("Royal League", cols)
		}},
		{"wmbl", "Wimbledon Open", "Wimbledon", "badge-g", func(cols []string) string { return "Grand Slam knockout cup" }},
		{"aus", "Australian Open", "Aussie Open", "badge-g", func(cols []string) string { return "Premier Grand Slam open event" }},
		{"snail", "Snail Cup", "Snail Cup", "badge-g", func(cols []string) string { return "Grand Slam event" }},
		{"sl", "Sodium Ladder", "Sodium", "badge-rank", func(cols []string) string { return "Tiered annual competitive ladder" }},
		{"ml", "Mercurial Ladder", "Mercurial", "badge-rank", func(cols []string) string { return "Tiered annual competitive ladder" }},
		{"eif", "Eiffel Tower Cup", "Eiffel", "badge-rank", func(cols []string) string { return "Invitational championship cup" }},
		{"qd", "Quick & Dirty (Swiss)", "Q&D", "badge-rank", func(cols []string) string { return "Fast-paced Swiss trophy tournament" }},
		{"ldrb", "Leaderboard Trophy", "Trophy", "badge-rank", func(cols []string) string { return "Annual leaderboard invitational" }},
		{"surv", "Survivors Cup", "Survivors", "badge-rank", func(cols []string) string { return "Survival cup championship" }},
	}

	for _, sg := range summaryGroups {
		var cols []string
		sumMax := new(int)
		for _, ev := range presentEvents {
			if ev == sg.id || strings.HasPrefix(ev, sg.id+"_") {
				cols = append(cols, ev)
				if eventMax[ev] == nil || sumMax == nil {
					sumMax = nil
				} else {
					*sumMax += *eventMax[ev]
				}
			}
		}
		if len(cols) > 0 {
			layout.Summary = append(layout.Summary, CommunityTournamentSummary{
				ID:         sg.id,
				Name:       sg.name,
				Short:      sg.short,
				Cols:       cols,
				MaxPts:     sumMax,
				BadgeClass: sg.badgeClass,
				Desc:       sg.descFmt(cols),
			})
		}
	}

	return layout, nil
}

// editionChampion returns the World Champion reigning in year and what they are: the run's latest gold up to
// that year, else the latest recorded podium's winner up to it, else "" for none. A hidden one is DeletedPlayer.
func (r *RatingsReader) editionChampion(ctx context.Context, run int64, year int) (string, string, error) {
	var name, season string
	err := r.pool.QueryRow(ctx, `
		SELECT player_name, season FROM tournament_records
		WHERE run_id = $1 AND tournament = 'World Championship' AND medal = 'gold'
		  AND substring(season from '^[0-9]+')::int <= $2
		ORDER BY substring(season from '^[0-9]+')::int DESC LIMIT 1`, run, year).Scan(&name, &season)
	desc := "Reigning World Champion (%s)"
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.pool.QueryRow(ctx, `
			SELECT player_name, substring(season from '^[0-9]+') FROM tournament_cup_podiums
			WHERE cup = 'worlds' AND place = 1 AND substring(season from '^[0-9]+')::int <= $1
			ORDER BY finished DESC LIMIT 1`, year).Scan(&name, &season)
		desc = "World Champion (%s)"
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if err := r.maskNames(ctx, []*string{&name}); err != nil {
		return "", "", err
	}
	return name, fmt.Sprintf(desc, season), nil
}

// worldsDesc describes the World Championship's column, with its champion when there is one.
func worldsDesc(champion string) string {
	if champion == "" {
		return "World Championship"
	}
	return fmt.Sprintf("World Championship (won by %s)", champion)
}

// pointsTable is a series' community points table from an edition on: its column code ("ic_{}" adds the
// season) and the most points it gives.
type pointsTable struct {
	Code string
	From int
	Top  int
}

// eventMaximum returns the most points an event's table gives in year: the latest table from or before it, else
// the earliest, as the engine picks one; nil for an event no table's code matches.
func eventMaximum(event string, year int, tables []pointsTable) *int {
	var picked *pointsTable
	for i, t := range tables {
		if !codeMatches(t.Code, event) {
			continue
		}
		switch {
		case picked == nil,
			t.From <= year && (picked.From > year || t.From > picked.From),
			t.From > year && picked.From > year && t.From < picked.From:
			picked = &tables[i]
		}
	}
	if picked == nil {
		return nil
	}
	return &picked.Top
}

// codeMatches reports whether event is a column of code: code itself, or with a season number for "ic_{}" or
// "rl_{:02d}".
func codeMatches(code, event string) bool {
	prefix, seasonal := strings.CutSuffix(code, "_{}")
	if !seasonal {
		prefix, seasonal = strings.CutSuffix(code, "_{:02d}")
	}
	if !seasonal {
		return event == code
	}
	season, ok := strings.CutPrefix(event, prefix+"_")
	_, err := strconv.Atoi(season)
	return ok && season != "" && err == nil
}

func formatRangeDesc(prefix string, cols []string) string {
	var nums []int
	re := regexp.MustCompile(`_(\d+)$`)
	for _, c := range cols {
		if m := re.FindStringSubmatch(c); len(m) > 1 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				nums = append(nums, n)
			}
		}
	}
	if len(nums) == 0 {
		return prefix
	}
	slices.Sort(nums)
	if len(nums) == 1 {
		return fmt.Sprintf("%s Season %d", prefix, nums[0])
	}
	return fmt.Sprintf("Sum of %d seasons (S%02d–S%02d)", len(nums), nums[0], nums[len(nums)-1])
}
