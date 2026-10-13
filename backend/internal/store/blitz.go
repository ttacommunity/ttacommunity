// Blitz tournaments: which series are blitz, each edition's format (Swiss rounds, then a knockout) and its
// entrants, read for admin with Discord names and for the site without them.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// BlitzFormat is how an edition runs: Swiss rounds, then a knockout of that many players (0 for none).
type BlitzFormat struct {
	SwissRounds, KnockoutSize int
}

// DefaultBlitzFormat is an edition's format until admin sets one: the US Open's.
var DefaultBlitzFormat = BlitzFormat{SwissRounds: 8, KnockoutSize: 8}

// BlitzEditionRow is one edition of a blitz series as the lists show it: the series' slug and name, the
// edition's label and format, how many entrants are in, pending and dropped, and the seed that breaks what
// the standings' tiebreaks leave (0 until the edition has a row of its own).
type BlitzEditionRow struct {
	Slug, Name, Edition        string
	Format                     BlitzFormat
	Entrants, Pending, Dropped int
	TiebreakSeed               int64
	// Round is the latest published round's number, nil before the first is published.
	Round *int
}

// BlitzEntrant is one sign-up: a member in an edition. Name, Player (the ratings' spelling, "" for a player
// they do not know), DiscordName, Email, Zone and Country are the member's; DiscordName and Email are "" when
// read for the site. Status is pending (not yet confirmed), active or dropped. To add one, give MemberID, or a
// Name with the details a new member needs.
type BlitzEntrant struct {
	ID, MemberID                                            int64
	Name, Player, DiscordName, Email, Zone, Country, Status string
}

// BlitzEdition is an edition with its entrants, in sign-up order, and its rounds (the published ones, for the site).
type BlitzEdition struct {
	BlitzEditionRow
	Entrants []BlitzEntrant
	Rounds   []BlitzRound
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

// ErrBadBlitzEntrant means an entrant without a name or a time zone, or a name or Discord name over 100 characters.
var ErrBadBlitzEntrant = errors.New("an entrant has a name and a Discord name of at most 100 characters, and a time zone")

// ErrBadBlitzStatus means a status other than pending, active or dropped.
var ErrBadBlitzStatus = errors.New("an entrant is pending, active or dropped")

// BlitzEntrantExistsError means names already in the edition, in any case: these.
type BlitzEntrantExistsError struct{ Names []string }

func (e *BlitzEntrantExistsError) Error() string {
	return "already entered: " + strings.Join(e.Names, ", ")
}

var blitzStatuses = []string{"pending", "active", "dropped"}

// blitzEditionRows lists blitz editions, the series' hidden ones too unless public, narrowed to one edition when
// slug is given. Newest edition first within a series.
func blitzEditionRows(ctx context.Context, db conn, public bool, slug, edition string) ([]BlitzEditionRow, error) {
	rows, err := db.Query(ctx, `
		SELECT s.slug, s.name, t.label, coalesce(e.swiss_rounds, $4), coalesce(e.knockout_size, $5),
		       (SELECT count(*) FROM blitz_entries x WHERE x.tournament_id = t.tournament_id AND x.status = 'active')::int,
		       (SELECT count(*) FROM blitz_entries x WHERE x.tournament_id = t.tournament_id AND x.status = 'pending')::int,
		       (SELECT count(*) FROM blitz_entries x WHERE x.tournament_id = t.tournament_id AND x.status = 'dropped')::int,
		       coalesce(e.tiebreak_seed, 0),
		       (SELECT max(r.number) FROM blitz_rounds r WHERE r.tournament_id = t.tournament_id AND r.published_at IS NOT NULL)
		FROM series s JOIN tournaments t USING (series_id)
		LEFT JOIN blitz_editions e ON e.tournament_id = t.tournament_id
		WHERE s.blitz AND (s.visible OR NOT $1) AND ($2 = '' OR (s.slug = $2 AND t.label = $3))
		ORDER BY s.name, coalesce(substring(t.label FROM '^[0-9]+')::int, -1) DESC, t.label DESC`,
		public, slug, edition, DefaultBlitzFormat.SwissRounds, DefaultBlitzFormat.KnockoutSize)
	if err != nil {
		return nil, fmt.Errorf("listing blitz editions: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (BlitzEditionRow, error) {
		var r BlitzEditionRow
		return r, row.Scan(&r.Slug, &r.Name, &r.Edition, &r.Format.SwissRounds, &r.Format.KnockoutSize, &r.Entrants, &r.Pending, &r.Dropped, &r.TiebreakSeed, &r.Round)
	})
	if err != nil {
		return nil, fmt.Errorf("listing blitz editions: %w", err)
	}
	return out, nil
}

// blitzEdition reads one edition and its entrants: every one with Discord names for admin; for the site the
// active and dropped ones (the standings mark the dropped) without Discord names, never a pending one.
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
		WHERE s.slug = $1 AND t.label = $2 AND (x.status <> 'pending' OR NOT $3)
		ORDER BY x.joined_at, x.entry_id`, slug, edition, public)
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
	if got.Rounds, err = blitzRounds(ctx, db, public, slug, edition); err != nil {
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

// BlitzEdition reads a shown edition for the site, with its active and dropped entrants (no Discord names, no
// pending ones), its published rounds and their results, or ErrNoBlitzEdition.
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

// SetBlitzFormat sets an edition's format, or returns ErrBadBlitzFormat or ErrNoBlitzEdition.
func (a *AdminStore) SetBlitzFormat(ctx context.Context, slug, edition string, f BlitzFormat) error {
	if f.SwissRounds < 1 || f.SwissRounds > 30 || !slices.Contains([]int{0, 2, 4, 8, 16, 32}, f.KnockoutSize) {
		return ErrBadBlitzFormat
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		id, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE blitz_editions SET swiss_rounds = $2, knockout_size = $3 WHERE tournament_id = $1",
			id, f.SwissRounds, f.KnockoutSize); err != nil {
			return fmt.Errorf("setting %s %s's format: %w", slug, edition, err)
		}
		return nil
	})
}

// entrantMember finds the member an entrant names: by MemberID, else by Name without case, else a new member
// with the entrant's details. A banned member is ErrMemberBanned.
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
	return id, name, nil
}

// AddBlitzEntrants enters members in an edition, all or none, and returns the entries' ids in order. An entrant
// names a member by id or by name; an unknown name makes a member with the entrant's details. An entrant with
// no Status is active. A member already in the edition, or twice in the list, is a *BlitzEntrantExistsError
// naming each; a banned one is ErrMemberBanned.
func (a *AdminStore) AddBlitzEntrants(ctx context.Context, slug, edition string, entrants []BlitzEntrant) ([]int64, error) {
	for i := range entrants {
		entrants[i].Name = strings.TrimSpace(entrants[i].Name)
		if entrants[i].MemberID <= 0 && entrants[i].Name == "" {
			return nil, ErrBadBlitzEntrant
		}
		if entrants[i].Status == "" {
			entrants[i].Status = "active"
		}
		if !slices.Contains(blitzStatuses, entrants[i].Status) {
			return nil, ErrBadBlitzStatus
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
			if err := tx.QueryRow(ctx, "INSERT INTO blitz_entries (tournament_id, member_id, status) VALUES ($1, $2, $3) RETURNING entry_id",
				id, memberID, e.Status).Scan(&got); err != nil {
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

// SetBlitzEntrant changes an entrant's status, or the member's Discord name, email, zone or country, or returns
// ErrNoBlitzEdition, ErrNoBlitzEntrant, ErrBadBlitzStatus or ErrBadMember.
func (a *AdminStore) SetBlitzEntrant(ctx context.Context, slug, edition string, id int64, c BlitzEntrantChange) error {
	if c.Status != nil && !slices.Contains(blitzStatuses, *c.Status) {
		return ErrBadBlitzStatus
	}
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
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

// RemoveBlitzEntrant removes an entrant, or returns ErrNoBlitzEdition, ErrNoBlitzEntrant, or
// ErrBlitzEntrantInRound for one a round names.
func (a *AdminStore) RemoveBlitzEntrant(ctx context.Context, slug, edition string, id int64) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		edID, err := blitzEditionID(ctx, tx, slug, edition)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM blitz_entries WHERE entry_id = $1 AND tournament_id = $2", id, edID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation: a match names them
			return ErrBlitzEntrantInRound
		}
		if err != nil {
			return fmt.Errorf("removing an entrant of %s %s: %w", slug, edition, err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNoBlitzEntrant
		}
		return nil
	})
}
