// Admin's notification endpoints: the webhooks notifications are posted to, each with its format, the events it
// takes and text put before its messages. The database checks each field.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// NotificationEndpoint is a webhook notifications are posted to: its URL, its format (discord so far), the events
// it takes (link_request, blitz_report, blitz_flag) and the prefix put before each message.
type NotificationEndpoint struct {
	ID     int64
	Name   string
	URL    string
	Format string
	Events []string
	Prefix string
}

var (
	// ErrNoNotificationEndpoint means no endpoint has that id.
	ErrNoNotificationEndpoint = errors.New("no such notification endpoint")
	// ErrBadNotificationEndpoint means an endpoint the database refuses.
	ErrBadNotificationEndpoint = errors.New("an endpoint needs a name of 1 to 100 characters, a Discord webhook URL " +
		"(https://discord.com/api/webhooks/…), known events and a prefix of at most 20 characters")
)

// badEndpoint turns a check violation into ErrBadNotificationEndpoint.
func badEndpoint(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23514" || pg.Code == "23502") {
		return ErrBadNotificationEndpoint
	}
	return err
}

// endpointColumns are an endpoint's columns as scanEndpoint reads them.
const endpointColumns = "endpoint_id, name, url, format, events, prefix"

func scanEndpoint(row pgx.CollectableRow) (NotificationEndpoint, error) {
	var e NotificationEndpoint
	err := row.Scan(&e.ID, &e.Name, &e.URL, &e.Format, &e.Events, &e.Prefix)
	return e, err
}

// NotificationEndpoints lists the endpoints, by name.
func (a *AdminStore) NotificationEndpoints(ctx context.Context) ([]NotificationEndpoint, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+endpointColumns+" FROM notification_endpoints ORDER BY lower(name), endpoint_id")
	if err != nil {
		return nil, fmt.Errorf("listing notification endpoints: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanEndpoint)
	if err != nil {
		return nil, fmt.Errorf("listing notification endpoints: %w", err)
	}
	return list, nil
}

// NotificationEndpoint returns the endpoint with that id, or ErrNoNotificationEndpoint.
func (a *AdminStore) NotificationEndpoint(ctx context.Context, id int64) (NotificationEndpoint, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+endpointColumns+" FROM notification_endpoints WHERE endpoint_id = $1", id)
	if err != nil {
		return NotificationEndpoint{}, fmt.Errorf("reading a notification endpoint: %w", err)
	}
	e, err := pgx.CollectExactlyOneRow(rows, scanEndpoint)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationEndpoint{}, ErrNoNotificationEndpoint
	}
	if err != nil {
		return NotificationEndpoint{}, fmt.Errorf("reading a notification endpoint: %w", err)
	}
	return e, nil
}

// trimEndpoint trims an endpoint's text fields; nil events are none.
func trimEndpoint(e NotificationEndpoint) NotificationEndpoint {
	e.Name, e.URL, e.Format, e.Prefix = strings.TrimSpace(e.Name), strings.TrimSpace(e.URL), strings.TrimSpace(e.Format), strings.TrimSpace(e.Prefix)
	if e.Events == nil {
		e.Events = []string{}
	}
	return e
}

// AddNotificationEndpoint adds an endpoint at now and returns its id, or ErrBadNotificationEndpoint.
func (a *AdminStore) AddNotificationEndpoint(ctx context.Context, e NotificationEndpoint, now time.Time) (int64, error) {
	e = trimEndpoint(e)
	var id int64
	err := a.pool.QueryRow(ctx, `INSERT INTO notification_endpoints (name, url, format, events, prefix, created_at)
		VALUES ($1, $2, $3, ARRAY(SELECT DISTINCT unnest($4::text[]) ORDER BY 1), $5, $6) RETURNING endpoint_id`,
		e.Name, e.URL, e.Format, e.Events, e.Prefix, now).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("adding a notification endpoint: %w", badEndpoint(err))
	}
	return id, nil
}

// SetNotificationEndpoint changes an endpoint, or returns ErrNoNotificationEndpoint or ErrBadNotificationEndpoint.
func (a *AdminStore) SetNotificationEndpoint(ctx context.Context, e NotificationEndpoint) error {
	e = trimEndpoint(e)
	tag, err := a.pool.Exec(ctx, `UPDATE notification_endpoints SET name = $2, url = $3, format = $4,
		events = ARRAY(SELECT DISTINCT unnest($5::text[]) ORDER BY 1), prefix = $6 WHERE endpoint_id = $1`,
		e.ID, e.Name, e.URL, e.Format, e.Events, e.Prefix)
	if err != nil {
		return fmt.Errorf("changing a notification endpoint: %w", badEndpoint(err))
	}
	if tag.RowsAffected() == 0 {
		return ErrNoNotificationEndpoint
	}
	return nil
}

// RemoveNotificationEndpoint removes an endpoint, or returns ErrNoNotificationEndpoint.
func (a *AdminStore) RemoveNotificationEndpoint(ctx context.Context, id int64) error {
	tag, err := a.pool.Exec(ctx, "DELETE FROM notification_endpoints WHERE endpoint_id = $1", id)
	if err != nil {
		return fmt.Errorf("removing a notification endpoint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoNotificationEndpoint
	}
	return nil
}
