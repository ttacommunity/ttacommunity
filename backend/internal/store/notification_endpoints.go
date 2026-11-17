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
// it takes (sign_up, link_request, blitz_report, blitz_flag, reddit_failing), the Reddit feeds it takes, by id, and the prefix put
// before each message. Paused, it is sent nothing. Username and AvatarURL ("" for the webhook's own) are the name and
// picture its messages show; Forum posts each as a new thread of a forum channel; RoleID ("" for none) is pinged with
// each; Look is how a Reddit post looks in it. LastSentAt, LastError and LastErrorAt say how its posts went.
type NotificationEndpoint struct {
	ID                  int64
	Name                string
	URL                 string
	Format              string
	Events              []string
	Feeds               []int64
	Prefix              string
	Paused, Forum       bool
	Username, AvatarURL string
	RoleID              string
	Look                EmbedLook
	LastSentAt          *time.Time
	LastError           string
	LastErrorAt         *time.Time
}

// EmbedLook is how a Reddit post looks in an endpoint's channel: the embed's colour (#rrggbb), how much of the
// post's text it shows (full, short or none), and whether it shows the image and the author.
type EmbedLook struct {
	Colour, Text  string
	Image, Author bool
}

// DefaultLook is an endpoint's look when none is given, the database's defaults: Reddit's orange, all of it.
var DefaultLook = EmbedLook{Colour: "#ff4500", Text: "full", Image: true, Author: true}

var (
	// ErrNoNotificationEndpoint means no endpoint has that id.
	ErrNoNotificationEndpoint = errors.New("no such notification endpoint")
	// ErrBadNotificationEndpoint means an endpoint the database refuses.
	ErrBadNotificationEndpoint = errors.New("an endpoint needs a name of 1 to 100 characters, a Discord webhook URL " +
		"(https://discord.com/api/webhooks/…), known events and feeds, a prefix of at most 20 characters, a display name of at " +
		"most 80 without \"discord\" or \"clyde\", an https picture, a role's id of 17 to 20 digits, a colour #rrggbb in lower " +
		"case, and post text full, short or none")
)

// badEndpoint turns a check violation into ErrBadNotificationEndpoint.
func badEndpoint(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23514" || pg.Code == "23502" || pg.Code == "23503") {
		return ErrBadNotificationEndpoint
	}
	return err
}

// endpointColumns are an endpoint's columns as scanEndpoint reads them.
const endpointColumns = `endpoint_id, name, url, format, events,
	ARRAY(SELECT s.feed_id FROM notification_feed_subscriptions s WHERE s.endpoint_id = notification_endpoints.endpoint_id ORDER BY s.feed_id),
	prefix, paused, forum, username, avatar_url, role_id, colour, show_text, show_image, show_author, last_sent_at,
	coalesce(last_error, ''), last_error_at`

func scanEndpoint(row pgx.CollectableRow) (NotificationEndpoint, error) {
	var e NotificationEndpoint
	err := row.Scan(&e.ID, &e.Name, &e.URL, &e.Format, &e.Events, &e.Feeds, &e.Prefix, &e.Paused, &e.Forum, &e.Username, &e.AvatarURL,
		&e.RoleID, &e.Look.Colour, &e.Look.Text, &e.Look.Image, &e.Look.Author, &e.LastSentAt, &e.LastError, &e.LastErrorAt)
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

// trimEndpoint trims an endpoint's text fields; nil events and feeds are none, and no look is DefaultLook.
func trimEndpoint(e NotificationEndpoint) NotificationEndpoint {
	e.Name, e.URL, e.Format, e.Prefix = strings.TrimSpace(e.Name), strings.TrimSpace(e.URL), strings.TrimSpace(e.Format), strings.TrimSpace(e.Prefix)
	e.Username, e.AvatarURL, e.RoleID = strings.TrimSpace(e.Username), strings.TrimSpace(e.AvatarURL), strings.TrimSpace(e.RoleID)
	if e.Events == nil {
		e.Events = []string{}
	}
	if e.Feeds == nil {
		e.Feeds = []int64{}
	}
	if e.Look == (EmbedLook{}) {
		e.Look = DefaultLook
	}
	return e
}

// AddNotificationEndpoint adds an endpoint at now and returns its id, or ErrBadNotificationEndpoint.
func (a *AdminStore) AddNotificationEndpoint(ctx context.Context, e NotificationEndpoint, now time.Time) (int64, error) {
	e = trimEndpoint(e)
	var id int64
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO notification_endpoints (name, url, format, events, prefix, created_at,
				paused, forum, username, avatar_url, role_id, colour, show_text, show_image, show_author)
			VALUES ($1, $2, $3, ARRAY(SELECT DISTINCT unnest($4::text[]) ORDER BY 1), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			RETURNING endpoint_id`,
			e.Name, e.URL, e.Format, e.Events, e.Prefix, now, e.Paused, e.Forum, e.Username, e.AvatarURL, e.RoleID,
			e.Look.Colour, e.Look.Text, e.Look.Image, e.Look.Author).Scan(&id); err != nil {
			return err
		}
		return setFeeds(ctx, tx, id, e.Feeds)
	})
	if err != nil {
		return 0, fmt.Errorf("adding a notification endpoint: %w", badEndpoint(err))
	}
	return id, nil
}

// setFeeds makes feeds the Reddit feeds endpoint takes.
func setFeeds(ctx context.Context, tx pgx.Tx, endpoint int64, feeds []int64) error {
	if _, err := tx.Exec(ctx, "DELETE FROM notification_feed_subscriptions WHERE endpoint_id = $1", endpoint); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO notification_feed_subscriptions (endpoint_id, feed_id)
		SELECT DISTINCT $1::bigint, unnest($2::bigint[])`, endpoint, feeds)
	return err
}

// SetNotificationEndpoint changes an endpoint, keeping its URL when e's is empty, or returns ErrNoNotificationEndpoint
// or ErrBadNotificationEndpoint.
func (a *AdminStore) SetNotificationEndpoint(ctx context.Context, e NotificationEndpoint) error {
	e = trimEndpoint(e)
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE notification_endpoints SET name = $2, url = coalesce(nullif($3, ''), url), format = $4,
			events = ARRAY(SELECT DISTINCT unnest($5::text[]) ORDER BY 1), prefix = $6, paused = $7, forum = $8, username = $9,
			avatar_url = $10, role_id = $11, colour = $12, show_text = $13, show_image = $14, show_author = $15 WHERE endpoint_id = $1`,
			e.ID, e.Name, e.URL, e.Format, e.Events, e.Prefix, e.Paused, e.Forum, e.Username, e.AvatarURL, e.RoleID,
			e.Look.Colour, e.Look.Text, e.Look.Image, e.Look.Author)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNoNotificationEndpoint
		}
		return setFeeds(ctx, tx, e.ID, e.Feeds)
	})
	if errors.Is(err, ErrNoNotificationEndpoint) {
		return err
	}
	if err != nil {
		return fmt.Errorf("changing a notification endpoint: %w", badEndpoint(err))
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

// WebhookID is the id in e's webhook URL, the part of it that is not secret, or "" when the URL holds none.
func (e NotificationEndpoint) WebhookID() string {
	_, rest, ok := strings.Cut(e.URL, "/api/webhooks/")
	if !ok {
		return ""
	}
	id, _, ok := strings.Cut(rest, "/")
	if !ok {
		return ""
	}
	return id
}
