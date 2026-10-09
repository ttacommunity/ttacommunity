// Groups in admin: a series holding related tournaments and groups, made, renamed and given its members.
package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrGroupCycle means a series would end up in itself, directly or through its members.
var ErrGroupCycle = errors.New("a tournament cannot be in itself or in one of its own members")

// ErrNotAGroup means a series that is not a group was asked to hold members or be renamed.
var ErrNotAGroup = errors.New("only a group holds other tournaments or is renamed")

// ErrBadGroup means a group's name has no letter or digit, or its description is over 2000 characters.
var ErrBadGroup = errors.New("a group needs a name with a letter, and a description of at most 2000 characters")

// Group is a new group: its name, description, kind (league, cup, ladder; series for editions of one event, group
// for a group of groups) and the slugs of its members.
type Group struct {
	Name, Description, Kind string
	Members                 []string
}

// kinds are what a tournament is; groupKinds what only a group can be too.
var kinds, groupKinds = []string{"league", "cup", "ladder"}, []string{"series", "group"}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// groupSlug returns a slug from a name ("The Big Leagues!" gives the_big_leagues), or "" when it has no letter.
func groupSlug(name string) string {
	s := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return ""
	}
	return s
}

// CreateGroup makes a group holding its members and returns its slug, taken from its name (with _2, _3 … when
// taken). It returns ErrBadGroup, ErrBadKind or ErrNoTournament for a member that does not exist, making nothing.
func (a *AdminStore) CreateGroup(ctx context.Context, g Group) (string, error) {
	g.Name, g.Description = strings.TrimSpace(g.Name), strings.TrimSpace(g.Description)
	base := groupSlug(g.Name)
	if base == "" || utf8.RuneCountInString(g.Description) > 2000 {
		return "", ErrBadGroup
	}
	if !slices.Contains(kinds, g.Kind) && !slices.Contains(groupKinds, g.Kind) {
		return "", ErrBadKind
	}
	var slug string
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var taken []string
		rows, err := tx.Query(ctx, "SELECT slug FROM series WHERE slug = $1 OR slug ~ ('^' || $1 || '_[0-9]+$')", base)
		if err != nil {
			return err
		}
		if taken, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		slug = base
		for n := 2; slices.Contains(taken, slug); n++ {
			slug = base + "_" + strconv.Itoa(n)
		}
		var id int64
		if err := tx.QueryRow(ctx, `INSERT INTO series (slug, name, original_name, kind, description, is_group)
			VALUES ($1, $2, $2, $3, $4, true) RETURNING series_id`, slug, g.Name, g.Kind, g.Description).Scan(&id); err != nil {
			return err
		}
		for _, m := range g.Members {
			if err := setParent(ctx, tx, m, &id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNoTournament) || errors.Is(err, ErrGroupCycle) {
			return "", err
		}
		return "", fmt.Errorf("making group %s: %w", g.Name, err)
	}
	return slug, nil
}

// SetMembers makes members exactly what the group holds: each is put in it, and any other it held taken out to the
// top. It returns ErrNotAGroup, ErrNoTournament or ErrGroupCycle, changing nothing.
func (a *AdminStore) SetMembers(ctx context.Context, group string, members []string) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		var id int64
		var isGroup bool
		if err := tx.QueryRow(ctx, "SELECT series_id, is_group FROM series WHERE slug = $1", group).Scan(&id, &isGroup); errors.Is(err, pgx.ErrNoRows) {
			return ErrNoTournament
		} else if err != nil {
			return fmt.Errorf("setting %s's members: %w", group, err)
		}
		if !isGroup {
			return ErrNotAGroup
		}
		if _, err := tx.Exec(ctx, "UPDATE series SET parent_id = NULL WHERE parent_id = $1 AND NOT slug = ANY($2)", id, members); err != nil {
			return fmt.Errorf("setting %s's members: %w", group, err)
		}
		for _, m := range members {
			if err := setParent(ctx, tx, m, &id); err != nil {
				return err
			}
		}
		return nil
	})
}

// setParent puts the series slug in the group parent, or takes it out of any for nil. It returns ErrNoTournament
// when slug does not exist, ErrNotAGroup when parent is not a group and ErrGroupCycle when it would be in itself.
func setParent(ctx context.Context, tx pgx.Tx, slug string, parent *int64) error {
	tag, err := tx.Exec(ctx, "UPDATE series SET parent_id = $2 WHERE slug = $1", slug, parent)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.ConstraintName == "series_no_cycle" {
		return ErrGroupCycle
	}
	if errors.As(err, &pg) && pg.ConstraintName == "series_parent_group" {
		return ErrNotAGroup
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNoTournament
	}
	return nil
}
