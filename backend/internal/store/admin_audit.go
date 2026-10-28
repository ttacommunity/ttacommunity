// Reading the audit log: who changed what and when, newest first, a page at a time.

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// auditPerPage is how many audit entries a page holds.
const auditPerPage = 50

// AuditFilter narrows the audit log: Action to one action, Text to entries naming it anywhere, any case, and From
// (inclusive) and To (exclusive) to a span of time. Empty or nil narrows nothing.
type AuditFilter struct {
	Action, Text string
	From, To     *time.Time
}

// AuditEntry is one change in the audit log. Admin is the name of the admin who made it, empty for a player
// (ByPlayer) or for someone no longer in admin.
type AuditEntry struct {
	ID       int64
	At       time.Time
	Admin    string
	ByPlayer bool
	Action   string
	Target   string
	Detail   json.RawMessage
}

// AuditPage is one page of the audit log, with every action ever taken, for choosing one.
type AuditPage struct {
	Page, Pages, Total int
	Entries            []AuditEntry
	Actions            []string
}

// AuditLog returns page (from 1; past the last gives the last) of the entries filter allows, newest first.
func (a *AdminStore) AuditLog(ctx context.Context, filter AuditFilter, page int) (AuditPage, error) {
	const matching = `
		FROM admin_audit u LEFT JOIN admins ad ON ad.discord_id = u.discord_id AND u.discord_id <> ''
		WHERE ($1 = '' OR u.action = $1)
		  AND ($2 = '' OR strpos(lower(concat_ws(' ', u.action, u.target, u.detail::text, ad.name)), lower($2)) > 0)
		  AND ($3::timestamptz IS NULL OR u.at >= $3) AND ($4::timestamptz IS NULL OR u.at < $4)`
	args := []any{filter.Action, filter.Text, filter.From, filter.To}
	var p AuditPage
	if err := a.pool.QueryRow(ctx, "SELECT count(*)"+matching, args...).Scan(&p.Total); err != nil {
		return AuditPage{}, fmt.Errorf("counting the audit log: %w", err)
	}
	p.Pages = max(1, (p.Total+auditPerPage-1)/auditPerPage)
	p.Page = min(max(page, 1), p.Pages)
	rows, err := a.pool.Query(ctx, `
		SELECT u.audit_id, u.at, coalesce(ad.name, ''), u.discord_id = '', u.action, u.target, u.detail`+matching+`
		ORDER BY u.at DESC, u.audit_id DESC LIMIT $5 OFFSET $6`, append(args, auditPerPage, (p.Page-1)*auditPerPage)...)
	if err != nil {
		return AuditPage{}, fmt.Errorf("reading the audit log: %w", err)
	}
	if p.Entries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AuditEntry]); err != nil {
		return AuditPage{}, fmt.Errorf("reading the audit log: %w", err)
	}
	rows, err = a.pool.Query(ctx, "SELECT DISTINCT action FROM admin_audit ORDER BY action")
	if err != nil {
		return AuditPage{}, fmt.Errorf("listing the audit's actions: %w", err)
	}
	if p.Actions, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return AuditPage{}, fmt.Errorf("listing the audit's actions: %w", err)
	}
	return p, nil
}
