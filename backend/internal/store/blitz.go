// Blitz tournaments: which series are blitz, each edition's format (Swiss rounds, then a knockout), points and
// entrants, read for admin with Discord names and for the site without them.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"ttacommunity/backend/internal/blitz"
)

// BlitzFormat is how an edition runs: Swiss rounds, then a knockout of that many players (0 for none).
type BlitzFormat struct {
	SwissRounds, KnockoutSize int
}

// DefaultBlitzFormat is an edition's format until admin sets one: the US Open's.
var DefaultBlitzFormat = BlitzFormat{SwissRounds: 8, KnockoutSize: 8}

// BlitzEditionRow is one edition of a blitz series as the lists show it: the series' slug and name, the
// edition's label and format, how many entrants are in and dropped, and the seed that breaks what the
// standings' tiebreaks leave (0 until the edition has a row of its own).
type BlitzEditionRow struct {
	Slug, Name, Edition string
	Format              BlitzFormat
	Entrants, Dropped   int
	TiebreakSeed        int64
	// Points are what each outcome is worth, FIDE's until admin sets them.
	Points blitz.Scoring
	// Round is the latest published round's number, nil before the first is published.
	Round *int
	// Champion is the winner of the published final, by username: nil until the final has a winner.
	Champion *string
}

// BlitzEntrant is one sign-up: a member in an edition. Name, Player (the ratings' spelling, "" for a player
// they do not know), DiscordName, Email, Zone and Country are the member's; DiscordName and Email are "" when
// read for the site. Status is active (in) or dropped; an entrant is added active. Skips are the rounds they
// skip or skipped, in order (for the site, the published ones). To add one, give MemberID (with any details to
// save on the member), or a Name with the details a new member needs.
type BlitzEntrant struct {
	ID, MemberID                                            int64
	Name, Player, DiscordName, Email, Zone, Country, Status string
	Skips                                                   []int
}

// BlitzEdition is an edition with its entrants, in sign-up order, its rounds (the published ones, for the site)
// and, for admin, its organisers in the order added and its own thread text (nil until set).
type BlitzEdition struct {
	BlitzEditionRow
	Entrants   []BlitzEntrant
	Rounds     []BlitzRound
	Organisers []BlitzOrganiser
	ThreadText *BlitzThreadText
}

// BlitzEntrantChange is what admin may change on an entrant: its status, or the member's Discord name, email,
// zone and country; a nil field is left as it is.
type BlitzEntrantChange struct {
	DiscordName, Email, Zone, Country, Status *string
}

// ErrNoBlitzEdition means no edition of a blitz series has that slug and label.
var ErrNoBlitzEdition = errors.New("no such edition of a blitz tournament")

// ErrNoBlitzEntrant means no entrant of the edition has that id.
var ErrNoBlitzEntrant = errors.New("no such entrant")

// ErrBadBlitzFormat means Swiss rounds outside 1 to 30, or a knockout not of 0, 2, 4, 8, 16 or 32 players.
var ErrBadBlitzFormat = errors.New("a blitz format is 1 to 30 Swiss rounds and a knockout of 0, 2, 4, 8, 16 or 32 players")

// ErrBlitzFormatLocked means the format cannot change once the knockout has a round.
var ErrBlitzFormatLocked = errors.New("the format cannot change once the knockout has started")

// ErrBlitzPointsLocked means the points cannot change once the knockout has a round.
var ErrBlitzPointsLocked = errors.New("the points cannot change once the knockout has started")

// ErrBadBlitzEntrant means an entrant without a name or a time zone, or a name or Discord name over 100 characters.
var ErrBadBlitzEntrant = errors.New("an entrant has a name and a Discord name of at most 100 characters, and a time zone")

// ErrBadBlitzStatus means a status other than active or dropped.
var ErrBadBlitzStatus = errors.New("an entrant is active or dropped")

// BlitzEntrantExistsError means names already in the edition, in any case: these.
type BlitzEntrantExistsError struct{ Names []string }

func (e *BlitzEntrantExistsError) Error() string {
	return "already entered: " + strings.Join(e.Names, ", ")
}

var blitzStatuses = []string{"active", "dropped"}

// blitzChampionOf is the username of the winner of edition t's published final, the knockout round with one
// match, or null while it has none.
const blitzChampionOf = `(SELECT mm.tta_name FROM blitz_rounds r JOIN blitz_matches m ON m.round_id = r.round_id
	JOIN blitz_entries x ON x.entry_id = CASE m.result WHEN 'a' THEN m.entry_a ELSE m.entry_b END
	JOIN members mm ON mm.member_id = x.member_id
	WHERE r.tournament_id = t.tournament_id AND r.kind = 'knockout' AND r.published_at IS NOT NULL AND m.result IN ('a', 'b')
	  AND NOT EXISTS (SELECT 1 FROM blitz_matches o WHERE o.round_id = r.round_id AND o.match_id <> m.match_id))`

// blitzChampion returns the edition's champion, nil while its final has no winner.
func blitzChampion(ctx context.Context, tx pgx.Tx, edID int64) (*string, error) {
	var champion *string
	if err := tx.QueryRow(ctx, "SELECT "+blitzChampionOf+" FROM tournaments t WHERE t.tournament_id = $1", edID).Scan(&champion); err != nil {
		return nil, fmt.Errorf("finding the champion: %w", err)
	}
	return champion, nil
}

// blitzEditionRows lists blitz editions, the series' hidden ones too unless public, narrowed to one edition when
// slug is given. Newest edition first within a series.
func blitzEditionRows(ctx context.Context, db conn, public bool, slug, edition string) ([]BlitzEditionRow, error) {
	fide := blitz.FIDEScoring
	rows, err := db.Query(ctx, `
		SELECT s.slug, s.name, t.label, coalesce(e.swiss_rounds, $4), coalesce(e.knockout_size, $5),
		       coalesce(e.points_win, $6), coalesce(e.points_draw, $7), coalesce(e.points_loss, $8), coalesce(e.points_forfeit_win, $9),
		       coalesce(e.points_forfeit_loss, $10), coalesce(e.points_bye, $11), coalesce(e.points_skip, $12),
		       (SELECT count(*) FROM blitz_entries x WHERE x.tournament_id = t.tournament_id AND x.status = 'active')::int,
		       (SELECT count(*) FROM blitz_entries x WHERE x.tournament_id = t.tournament_id AND x.status = 'dropped')::int,
		       coalesce(e.tiebreak_seed, 0),
		       (SELECT max(r.number) FROM blitz_rounds r WHERE r.tournament_id = t.tournament_id AND r.published_at IS NOT NULL),
		       `+blitzChampionOf+`
		FROM series s JOIN tournaments t USING (series_id)
		LEFT JOIN blitz_editions e ON e.tournament_id = t.tournament_id
		WHERE s.blitz AND (s.visible OR NOT $1) AND ($2 = '' OR (s.slug = $2 AND t.label = $3))
		ORDER BY s.name, coalesce(substring(t.label FROM '^[0-9]+')::int, -1) DESC, t.label DESC`,
		public, slug, edition, DefaultBlitzFormat.SwissRounds, DefaultBlitzFormat.KnockoutSize, fide.Win, fide.Draw, fide.Loss, fide.ForfeitWin,
		fide.ForfeitLoss, fide.Bye, fide.Skip)
	if err != nil {
		return nil, fmt.Errorf("listing blitz editions: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzEditionRow, error) {
		var r BlitzEditionRow
		p := &r.Points
		return r, row.Scan(&r.Slug, &r.Name, &r.Edition, &r.Format.SwissRounds, &r.Format.KnockoutSize, &p.Win, &p.Draw, &p.Loss, &p.ForfeitWin,
			&p.ForfeitLoss, &p.Bye, &p.Skip, &r.Entrants, &r.Dropped, &r.TiebreakSeed, &r.Round, &r.Champion)
	})
	if err != nil {
		return nil, fmt.Errorf("listing blitz editions: %w", err)
	}
	return out, nil
}

// blitzEdition reads one edition and its entrants, with Discord names and emails for admin only.
func blitzEdition(ctx context.Context, db conn, public bool, slug, edition string) (BlitzEdition, error) {
	rows, err := blitzEditionRows(ctx, db, public, slug, edition)
	if err != nil {
		return BlitzEdition{}, err
	}
	if len(rows) == 0 {
		return BlitzEdition{}, ErrNoBlitzEdition
	}
	got := BlitzEdition{BlitzEditionRow: rows[0], Entrants: []BlitzEntrant{}}
	discord, email := "m.discord_name", "m.email"
	if public {
		discord, email = "''", "''"
	}
	// The public role may not read a member's Discord name or email, so those columns are named only for admin.
	entrants, err := db.Query(ctx, `
		SELECT x.entry_id, x.member_id, m.tta_name, coalesce(p.name, ''), `+discord+`, `+email+`, m.zone, coalesce(m.country_code, ''), x.status
		FROM blitz_entries x JOIN members m USING (member_id) LEFT JOIN players p ON p.player_id = m.player_id
		JOIN tournaments t ON t.tournament_id = x.tournament_id JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2
		ORDER BY x.joined_at, x.entry_id`, slug, edition)
	if err != nil {
		return BlitzEdition{}, fmt.Errorf("reading %s %s's entrants: %w", slug, edition, err)
	}
	got.Entrants, err = pgx.CollectRows(entrants, func(row pgx.CollectableRow) (BlitzEntrant, error) {
		var e BlitzEntrant
		return e, row.Scan(&e.ID, &e.MemberID, &e.Name, &e.Player, &e.DiscordName, &e.Email, &e.Zone, &e.Country, &e.Status)
	})
	if err != nil {
		return BlitzEdition{}, fmt.Errorf("reading %s %s's entrants: %w", slug, edition, err)
	}
	skips, err := blitzSkips(ctx, db, public, slug, edition)
	if err != nil {
		return BlitzEdition{}, err
	}
	for i := range got.Entrants {
		got.Entrants[i].Skips = skips[got.Entrants[i].ID]
	}
	if got.Rounds, err = blitzRounds(ctx, db, public, slug, edition); err != nil {
		return BlitzEdition{}, err
	}
	if public {
		return got, nil
	}
	if got.Organisers, err = blitzOrganisers(ctx, db, slug, edition); err != nil {
		return BlitzEdition{}, err
	}
	if got.ThreadText, err = blitzThreadText(ctx, db, slug, edition); err != nil {
		return BlitzEdition{}, err
	}
	return got, nil
}

// BlitzEditions lists every edition of every blitz series for admin.
func (a *AdminStore) BlitzEditions(ctx context.Context) ([]BlitzEditionRow, error) {
	return blitzEditionRows(ctx, a.pool, false, "", "")
}

// BlitzEdition reads an edition and all its entrants for admin, or ErrNoBlitzEdition.
func (a *AdminStore) BlitzEdition(ctx context.Context, slug, edition string) (BlitzEdition, error) {
	return blitzEdition(ctx, a.pool, false, slug, edition)
}

// BlitzEditions lists every edition of every shown blitz series for the site.
func (r *RatingsReader) BlitzEditions(ctx context.Context) ([]BlitzEditionRow, error) {
	return blitzEditionRows(ctx, r.pool, true, "", "")
}

// BlitzEdition reads a shown edition for the site, with its entrants (no Discord names or emails), its published
// rounds and their results, or ErrNoBlitzEdition.
func (r *RatingsReader) BlitzEdition(ctx context.Context, slug, edition string) (BlitzEdition, error) {
	return blitzEdition(ctx, r.pool, true, slug, edition)
}

// blitzEditionID finds the edition, and gives it its row when it has none yet, or returns ErrNoBlitzEdition.
func blitzEditionID(ctx context.Context, tx pgx.Tx, slug, edition string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		SELECT t.tournament_id FROM tournaments t JOIN series s USING (series_id)
		WHERE s.slug = $1 AND t.label = $2 AND s.blitz`, slug, edition).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoBlitzEdition
	}
	if err != nil {
		return 0, fmt.Errorf("finding blitz edition %s %s: %w", slug, edition, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO blitz_editions (tournament_id, swiss_rounds, knockout_size) VALUES ($1, $2, $3)
		ON CONFLICT (tournament_id) DO NOTHING`, id, DefaultBlitzFormat.SwissRounds, DefaultBlitzFormat.KnockoutSize); err != nil {
		return 0, fmt.Errorf("setting up blitz edition %s %s: %w", slug, edition, err)
	}
	return id, nil
}

// SetBlitzFormat sets an edition's format until its knockout has a round, or returns ErrBadBlitzFormat,
// ErrBlitzFormatLocked or ErrNoBlitzEdition.
func (a *AdminStore) SetBlitzFormat(ctx context.Context, slug, edition string, f BlitzFormat) error {
	if f.SwissRounds < 1 || f.SwissRounds > 30 || !slices.Contains([]int{0, 2, 4, 8, 16, 32}, f.KnockoutSize) {
		return ErrBadBlitzFormat
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		switch started, err := knockoutStarted(ctx, tx, id); {
		case err != nil:
			return fmt.Errorf("setting %s %s's format: %w", slug, edition, err)
		case started:
			return ErrBlitzFormatLocked
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_editions SET swiss_rounds = $2, knockout_size = $3 WHERE tournament_id = $1",
			id, f.SwissRounds, f.KnockoutSize); err != nil {
			return fmt.Errorf("setting %s %s's format: %w", slug, edition, err)
		}
		return nil
	})
}

// knockoutStarted says whether edition id's knockout has a round, draft or published.
func knockoutStarted(ctx context.Context, tx pgx.Tx, id int64) (bool, error) {
	var started bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_rounds WHERE tournament_id = $1 AND kind = 'knockout')", id).Scan(&started)
	return started, err
}

// SetBlitzPoints sets what each outcome of an edition is worth (checked by the caller) until its knockout has a
// round, or returns ErrBlitzPointsLocked or ErrNoBlitzEdition.
func (a *AdminStore) SetBlitzPoints(ctx context.Context, slug, edition string, s blitz.Scoring) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		switch started, err := knockoutStarted(ctx, tx, id); {
		case err != nil:
			return fmt.Errorf("setting %s %s's points: %w", slug, edition, err)
		case started:
			return ErrBlitzPointsLocked
		}
		if _, err := tx.Exec(ctx, `
			UPDATE blitz_editions SET points_win = $2, points_draw = $3, points_loss = $4, points_forfeit_win = $5,
			       points_forfeit_loss = $6, points_bye = $7, points_skip = $8
			WHERE tournament_id = $1`, id, s.Win, s.Draw, s.Loss, s.ForfeitWin, s.ForfeitLoss, s.Bye, s.Skip); err != nil {
			return fmt.Errorf("setting %s %s's points: %w", slug, edition, err)
		}
		return nil
	})
}

// given is a detail an entrant gives for its member, or nil for one left empty.
func given(detail string) *string {
	if detail == "" {
		return nil
	}
	return &detail
}

// entrantMember finds the member an entrant names: by MemberID, saving the details the entrant gives, else by
// Name without case, else a new member with the entrant's details. A banned member is ErrMemberBanned.
func entrantMember(ctx context.Context, tx pgx.Tx, e BlitzEntrant) (id int64, name string, err error) {
	var banned bool
	switch {
	case e.MemberID > 0:
		err = tx.QueryRow(ctx, "SELECT member_id, tta_name, banned FROM members WHERE member_id = $1", e.MemberID).Scan(&id, &name, &banned)
	default:
		err = tx.QueryRow(ctx, "SELECT member_id, tta_name, banned FROM members WHERE lower(tta_name) = lower($1)", e.Name).Scan(&id, &name, &banned)
	}
	switch {
	case errors.Is(err, pgx.ErrNoRows) && e.MemberID > 0:
		return 0, "", ErrNoMember
	case errors.Is(err, pgx.ErrNoRows):
		m := Member{Name: e.Name, DiscordName: e.DiscordName, Email: e.Email, Zone: e.Zone, Country: e.Country}
		if !validMember(&m) {
			return 0, "", ErrBadMember
		}
		id, err = addMember(ctx, tx, m)
		if err != nil {
			return 0, "", err
		}
		return id, m.Name, nil
	case err != nil:
		return 0, "", fmt.Errorf("finding a member: %w", err)
	case banned:
		return 0, "", fmt.Errorf("%w: %s", ErrMemberBanned, name)
	}
	c := MemberChange{DiscordName: given(e.DiscordName), Email: given(e.Email), Country: given(e.Country), Zone: given(e.Zone)}
	if e.MemberID > 0 && (c.DiscordName != nil || c.Email != nil || c.Country != nil || c.Zone != nil) {
		if err := setMember(ctx, tx, id, c); err != nil {
			return 0, "", err
		}
	}
	return id, name, nil
}

// AddBlitzEntrants enters members in an edition, all or none, and returns the entries' ids in order. An entrant
// names a member by id or by name; an unknown name makes a member with the entrant's details. A member already in
// the edition, or twice in the list, is a *BlitzEntrantExistsError naming each; a banned one is ErrMemberBanned.
func (a *AdminStore) AddBlitzEntrants(ctx context.Context, slug, edition string, entrants []BlitzEntrant) ([]int64, error) {
	for i := range entrants {
		entrants[i].Name = strings.TrimSpace(entrants[i].Name)
		if entrants[i].MemberID <= 0 && entrants[i].Name == "" {
			return nil, ErrBadBlitzEntrant
		}
	}
	ids := make([]int64, 0, len(entrants))
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		seen := map[int64]bool{}
		var twice []string
		for _, e := range entrants {
			memberID, name, err := entrantMember(ctx, tx, e)
			if err != nil {
				return err
			}
			var in bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_entries WHERE tournament_id = $1 AND member_id = $2)", id, memberID).Scan(&in); err != nil {
				return fmt.Errorf("checking %s %s's entrants: %w", slug, edition, err)
			}
			if in || seen[memberID] {
				twice = append(twice, name)
				continue
			}
			seen[memberID] = true
			var got int64
			if err := tx.QueryRow(ctx, "INSERT INTO blitz_entries (tournament_id, member_id) VALUES ($1, $2) RETURNING entry_id",
				id, memberID).Scan(&got); err != nil {
				return fmt.Errorf("adding an entrant to %s %s: %w", slug, edition, err)
			}
			ids = append(ids, got)
		}
		if len(twice) > 0 {
			slices.Sort(twice)
			return &BlitzEntrantExistsError{Names: twice}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// Candidate is someone admin may add or enter: a member, with their details, or a player the ratings know who is
// no member yet (MemberID 0, Name the ratings' spelling, their country where known, no zone).
type Candidate struct {
	MemberID                                        int64
	Name, Player, DiscordName, Email, Country, Zone string
	Banned                                          bool
}

// candidates finds up to limit members and players the ratings know who are no member, whose name, or ratings
// spelling, holds q in any case; members of the edition editionID are left out (none for 0). The name itself comes
// first, then names starting with q, then the rest; members before players, then by name.
func candidates(ctx context.Context, db conn, editionID int64, q string, limit int) ([]Candidate, error) {
	rows, err := db.Query(ctx, `
		WITH people AS (
			SELECT m.member_id, m.tta_name AS name, coalesce(p.name, '') AS player, m.discord_name, m.email,
			       coalesce(m.country_code, '') AS country, m.zone, m.banned
			FROM members m LEFT JOIN players p ON p.player_id = m.player_id
			WHERE NOT EXISTS (SELECT 1 FROM blitz_entries e WHERE e.tournament_id = $1 AND e.member_id = m.member_id)
			UNION ALL
			SELECT 0, p.name, p.name, '', '', coalesce(p.country_code, ''), '', false
			FROM players p
			WHERE NOT EXISTS (SELECT 1 FROM members m WHERE m.player_id = p.player_id OR lower(m.tta_name) = lower(p.name))
		)
		SELECT member_id, name, player, discord_name, email, country, zone, banned FROM people
		WHERE strpos(lower(name), lower($2)) > 0 OR strpos(lower(player), lower($2)) > 0
		ORDER BY CASE WHEN lower($2) IN (lower(name), lower(player)) THEN 0
		              WHEN starts_with(lower(name), lower($2)) OR starts_with(lower(player), lower($2)) THEN 1
		              ELSE 2 END,
		         member_id = 0, lower(name), member_id
		LIMIT $3`, editionID, q, limit)
	if err != nil {
		return nil, fmt.Errorf("finding people: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Candidate, error) {
		var c Candidate
		return c, row.Scan(&c.MemberID, &c.Name, &c.Player, &c.DiscordName, &c.Email, &c.Country, &c.Zone, &c.Banned)
	})
	if err != nil {
		return nil, fmt.Errorf("finding people: %w", err)
	}
	return found, nil
}

// People finds up to limit members and players the ratings know who are no member, whose name holds q; see
// candidates for the order.
func (a *AdminStore) People(ctx context.Context, q string, limit int) ([]Candidate, error) {
	return candidates(ctx, a.pool, 0, q, limit)
}

// BlitzCandidates is People for an edition: the members already in it are left out. It returns ErrNoBlitzEdition
// for no such edition.
func (a *AdminStore) BlitzCandidates(ctx context.Context, slug, edition, q string, limit int) ([]Candidate, error) {
	var found []Candidate
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		found, err = candidates(ctx, tx, id, q, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

// SetBlitzEntrant changes an entrant's status, or the member's Discord name, email, zone or country, or returns
// ErrNoBlitzEdition, ErrNoBlitzEntrant, ErrBadBlitzStatus, ErrBadMember, or, for dropping, ErrBlitzEntrantNotPlayed
// for someone with no result and no bye in a published round and ErrBlitzEntrantSkipping for someone skipping the
// next round.
func (a *AdminStore) SetBlitzEntrant(ctx context.Context, slug, edition string, id int64, c BlitzEntrantChange) error {
	if c.Status != nil && !slices.Contains(blitzStatuses, *c.Status) {
		return ErrBadBlitzStatus
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if c.Status != nil && *c.Status == "dropped" {
			played, err := blitzEntrantPlayed(ctx, tx, edID, id)
			if err != nil {
				return err
			}
			if !played {
				var known bool
				if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blitz_entries WHERE entry_id = $1 AND tournament_id = $2)", id, edID).Scan(&known); err != nil {
					return fmt.Errorf("checking an entrant of %s %s: %w", slug, edition, err)
				}
				if known {
					return ErrBlitzEntrantNotPlayed
				}
			}
			switch skipping, err := skippingNext(ctx, tx, edID, id); {
			case err != nil:
				return err
			case skipping:
				return ErrBlitzEntrantSkipping
			}
		}
		var memberID int64
		err = tx.QueryRow(ctx, "UPDATE blitz_entries SET status = coalesce($3, status) WHERE entry_id = $1 AND tournament_id = $2 RETURNING member_id",
			id, edID, c.Status).Scan(&memberID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNoBlitzEntrant
		}
		if err != nil {
			return fmt.Errorf("changing an entrant of %s %s: %w", slug, edition, err)
		}
		if c.DiscordName == nil && c.Email == nil && c.Zone == nil && c.Country == nil {
			return nil
		}
		return setMember(ctx, tx, memberID, MemberChange{DiscordName: c.DiscordName, Email: c.Email, Zone: c.Zone, Country: c.Country})
	})
}

// blitzEntrantPlayed says whether an entrant has played: a match with a result, or a bye in a published round.
func blitzEntrantPlayed(ctx context.Context, tx pgx.Tx, edID, id int64) (bool, error) {
	var played bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
		               WHERE r.tournament_id = $1 AND (m.entry_a = $2 OR m.entry_b = $2)
		                 AND (m.result IS NOT NULL OR (m.entry_b IS NULL AND r.published_at IS NOT NULL)))`, edID, id).Scan(&played); err != nil {
		return false, fmt.Errorf("checking whether entrant %d played: %w", id, err)
	}
	return played, nil
}

// RemoveBlitzEntrant removes an entrant who has not played, taking them out of the Swiss matches they are paired
// in (their opponent is left unpaired), or returns ErrNoBlitzEdition, ErrNoBlitzEntrant, ErrBlitzEntrantPlayed, or
// ErrBlitzEntrantInKnockout.
func (a *AdminStore) RemoveBlitzEntrant(ctx context.Context, slug, edition string, id int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		var known, knockout bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM blitz_entries WHERE entry_id = $2 AND tournament_id = $1),
			       EXISTS (SELECT 1 FROM blitz_matches m JOIN blitz_rounds r USING (round_id)
			               WHERE r.tournament_id = $1 AND r.kind = 'knockout' AND (m.entry_a = $2 OR m.entry_b = $2))`, edID, id).Scan(&known, &knockout); err != nil {
			return fmt.Errorf("removing an entrant of %s %s: %w", slug, edition, err)
		}
		if !known {
			return ErrNoBlitzEntrant
		}
		played, err := blitzEntrantPlayed(ctx, tx, edID, id)
		switch {
		case err != nil:
			return err
		case played:
			return ErrBlitzEntrantPlayed
		case knockout:
			return ErrBlitzEntrantInKnockout
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM blitz_matches m USING blitz_rounds r
			WHERE m.round_id = r.round_id AND r.tournament_id = $1 AND (m.entry_a = $2 OR m.entry_b = $2)`, edID, id); err != nil {
			return fmt.Errorf("unpairing an entrant of %s %s: %w", slug, edition, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM blitz_entries WHERE entry_id = $1", id); err != nil {
			return fmt.Errorf("removing an entrant of %s %s: %w", slug, edition, err)
		}
		return nil
	})
}
