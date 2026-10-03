// Admins and organisers, and the audit of what they change, through the API's admin login.

package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The roles a person can hold in admin.
const (
	RoleAdmin     = "admin"
	RoleOrganiser = "organiser"
)

// ErrNotAdmin means the Discord id is in no admin role.
var ErrNotAdmin = errors.New("not an admin or organiser")

// Admin is one person allowed into admin, by Discord id, with their role.
type Admin struct {
	DiscordID string
	Name      string
	Role      string
}

// AdminStore reads admins and writes the audit, as the ratings_admin login.
type AdminStore struct {
	pool *pgxpool.Pool
}

// NewAdminStore returns a store on the admin login's pool.
func NewAdminStore(pool *pgxpool.Pool) *AdminStore {
	return &AdminStore{pool: pool}
}

// Admin returns the admin with this Discord id, or ErrNotAdmin.
func (a *AdminStore) Admin(ctx context.Context, discordID string) (Admin, error) {
	got := Admin{DiscordID: discordID}
	err := a.pool.QueryRow(ctx, "SELECT name, role FROM admins WHERE discord_id = $1", discordID).Scan(&got.Name, &got.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrNotAdmin
	}
	if err != nil {
		return Admin{}, fmt.Errorf("reading an admin: %w", err)
	}
	return got, nil
}

// Audit records that discordID did action to target, with detail as JSON.
func (a *AdminStore) Audit(ctx context.Context, discordID, action, target string, detail any) error {
	b, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("auditing %s: %w", action, err)
	}
	if _, err := a.pool.Exec(ctx, "INSERT INTO admin_audit (discord_id, action, target, detail) VALUES ($1, $2, $3, $4)",
		discordID, action, target, b); err != nil {
		return fmt.Errorf("auditing %s: %w", action, err)
	}
	return nil
}

// AddAdmin adds or updates a person's admin role, as the database owner (the load job). The table's checks refuse a bad row.
func AddAdmin(ctx context.Context, pool *pgxpool.Pool, a Admin) error {
	if _, err := pool.Exec(ctx, `
		INSERT INTO admins (discord_id, name, role) VALUES ($1, $2, $3)
		ON CONFLICT (discord_id) DO UPDATE SET name = excluded.name, role = excluded.role`,
		a.DiscordID, a.Name, a.Role); err != nil {
		return fmt.Errorf("adding an admin: %w", err)
	}
	return nil
}

// ErrAdminTooStrong means the admin login could change games, players or runs, or create tables.
var ErrAdminTooStrong = errors.New("the admin login can change more than admin edits")

// CheckLimits returns ErrAdminTooStrong if the pool's role is a superuser, can create tables, or can
// delete games, players or runs. The API calls it at start, so a misconfigured login refuses to serve.
func (a *AdminStore) CheckLimits(ctx context.Context) error {
	var tooStrong bool
	err := a.pool.QueryRow(ctx, `
		SELECT (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)
			OR has_schema_privilege('public', 'CREATE')
			OR has_table_privilege('matches', 'INSERT, DELETE, TRUNCATE')
			OR has_table_privilege('players', 'UPDATE, DELETE, TRUNCATE')
			OR has_table_privilege('rating_runs', 'DELETE, TRUNCATE')`).Scan(&tooStrong)
	if err != nil {
		return fmt.Errorf("checking the admin login's privileges: %w", err)
	}
	if tooStrong {
		return ErrAdminTooStrong
	}
	return nil
}

// Admins returns everyone in admin, by name.
func (a *AdminStore) Admins(ctx context.Context) ([]Admin, error) {
	rows, err := a.pool.Query(ctx, "SELECT discord_id, name, role FROM admins ORDER BY name, discord_id")
	if err != nil {
		return nil, fmt.Errorf("listing admins: %w", err)
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Admin])
	if err != nil {
		return nil, fmt.Errorf("listing admins: %w", err)
	}
	return got, nil
}

// ErrLastAdmin means the change would leave admin without anyone in the admin role.
var ErrLastAdmin = errors.New("admin must keep at least one admin")

// keepingAnAdmin runs change in a transaction that holds the admin rows, and undoes it with ErrLastAdmin
// if it would take admin from at least one admin to none.
func (a *AdminStore) keepingAnAdmin(ctx context.Context, change func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT discord_id FROM admins WHERE role = 'admin' FOR UPDATE")
		if err != nil {
			return fmt.Errorf("holding the admins: %w", err)
		}
		held, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("holding the admins: %w", err)
		}
		if err := change(tx); err != nil {
			return err
		}
		var left int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM admins WHERE role = 'admin'").Scan(&left); err != nil {
			return fmt.Errorf("counting admins: %w", err)
		}
		if len(held) > 0 && left == 0 {
			return ErrLastAdmin
		}
		return nil
	})
}

// SetAdmin adds a person to admin or changes their name and role, as the admin login. The table's checks refuse
// a bad row; demoting the last admin is ErrLastAdmin.
func (a *AdminStore) SetAdmin(ctx context.Context, x Admin) error {
	return a.keepingAnAdmin(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO admins (discord_id, name, role) VALUES ($1, $2, $3)
			ON CONFLICT (discord_id) DO UPDATE SET name = excluded.name, role = excluded.role`,
			x.DiscordID, x.Name, x.Role); err != nil {
			return fmt.Errorf("setting an admin: %w", err)
		}
		return nil
	})
}

// RemoveAdmin takes a person out of admin. It returns ErrNotAdmin if they were not in it, and ErrLastAdmin
// for the last admin.
func (a *AdminStore) RemoveAdmin(ctx context.Context, discordID string) error {
	return a.keepingAnAdmin(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM admins WHERE discord_id = $1", discordID)
		if err != nil {
			return fmt.Errorf("removing an admin: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotAdmin
		}
		return nil
	})
}
