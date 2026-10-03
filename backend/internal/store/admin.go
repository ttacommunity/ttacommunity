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
