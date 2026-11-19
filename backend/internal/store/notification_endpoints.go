// Admin's notification endpoints: the webhooks notifications are posted to, each with its format, the events it
// takes and text put before its messages. The database checks each field.
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// NotificationEndpoint is a webhook notifications are posted to: its URL, its format (discord so far), the events
// it takes (sign_up, link_request, blitz_report, blitz_flag, feed_failing, ratings_run, ratings_top10, blitz_round, blitz_finished,
// blitz_reminder), the feeds it takes, by id, with each
// one's options, and the prefix put before each message. Paused, it is sent nothing. Username and AvatarURL ("" for the webhook's own) are the name and
// picture its messages show; Forum posts each as a new thread of a forum channel; RoleID ("" for none) is pinged with
// each; ThreadID ("" for none) is an existing thread of its channel, such as a forum post, that every message goes
// into; BlitzReminderHours is how long before a blitz round's deadline it is reminded; Look is how a feed's item
// looks in it. LastSentAt, LastError and LastErrorAt say how its posts went.
type NotificationEndpoint struct {
	ID                  int64
	Name                string
	URL                 string
	Format              string
	Events              []string
	Feeds               []int64
	FeedOptions         []FeedOptions
	Prefix              string
	Paused, Forum       bool
	Username, AvatarURL string
	RoleID              string
	ThreadID            string
	BlitzReminderHours  int
	Look                EmbedLook
	LastSentAt          *time.Time
	LastError           string
	LastErrorAt         *time.Time
}

// FeedOptions is how an endpoint takes a feed: a Twitch stream only in one of Categories and with one of Keywords
// in its title, or either when Match is "any" ("all", the default, else); an empty list does not narrow. WasLive
// edits its post when the stream ends. A YouTube stream only with one of Keywords in its title, WasLive as Twitch's.
// An RSS item only with one of Keywords in its title or summary.
type FeedOptions struct {
	Feed       int64    `json:"feed"`
	Categories []string `json:"categories"`
	Keywords   []string `json:"keywords"`
	Match      string   `json:"match"`
	WasLive    bool     `json:"was_live"`
}

// options returns e's options for feed, and whether e takes it.
func (e NotificationEndpoint) options(feed int64) (FeedOptions, bool) {
	if !slices.Contains(e.Feeds, feed) {
		return FeedOptions{}, false
	}
	for _, o := range e.FeedOptions {
		if o.Feed == feed {
			return o, true
		}
	}
	return FeedOptions{Feed: feed}, true
}

// TakesWasLive says whether e edits its post of feed's stream when the stream ends.
func (e NotificationEndpoint) TakesWasLive(feed int64) bool {
	o, ok := e.options(feed)
	return ok && o.WasLive
}

// matches says whether stream t passes o, in any letters: in one of its categories and with one of its keywords in
// the title, or either for Match "any"; a list o leaves empty does not narrow, so o with neither takes any stream.
func (o FeedOptions) matches(t TwitchNotice) bool {
	if len(o.Categories) == 0 && len(o.Keywords) == 0 {
		return true
	}
	inCategory := slices.ContainsFunc(o.Categories, func(c string) bool { return strings.EqualFold(c, t.Category) })
	title := strings.ToLower(t.Title)
	withKeyword := slices.ContainsFunc(o.Keywords, func(k string) bool { return strings.Contains(title, strings.ToLower(k)) })
	if o.Match == "any" {
		return inCategory || withKeyword
	}
	return (len(o.Categories) == 0 || inCategory) && (len(o.Keywords) == 0 || withKeyword)
}

// matchesTitle says whether a YouTube stream titled title passes o: with one of its keywords in the title, in any
// letters; o without keywords takes every stream.
func (o FeedOptions) matchesTitle(title string) bool {
	lower := strings.ToLower(title)
	return len(o.Keywords) == 0 || slices.ContainsFunc(o.Keywords, func(k string) bool { return strings.Contains(lower, strings.ToLower(k)) })
}

// matchesItem says whether RSS item i passes o: with one of its keywords in its title or summary, in any letters;
// o without keywords takes every item.
func (o FeedOptions) matchesItem(i RSSItem) bool {
	text := strings.ToLower(i.Title + "\n" + i.Summary)
	return len(o.Keywords) == 0 || slices.ContainsFunc(o.Keywords, func(k string) bool { return strings.Contains(text, strings.ToLower(k)) })
}

// EmbedLook is how a feed's item looks in an endpoint's channel: the embed's colour (#rrggbb), how much of its
// text it shows (full, short or none), and whether it shows its image.
type EmbedLook struct {
	Colour, Text string
	Image        bool
}

// DefaultBlitzReminderHours is how long before a round's deadline an endpoint is reminded when none is given.
const DefaultBlitzReminderHours = 24

// DefaultLook is an endpoint's look when none is given, the database's defaults: orange, all of it.
var DefaultLook = EmbedLook{Colour: "#ff4500", Text: "full", Image: true}

var (
	// ErrNoNotificationEndpoint means no endpoint has that id.
	ErrNoNotificationEndpoint = errors.New("no such notification endpoint")
	// ErrBadNotificationEndpoint means an endpoint the database refuses.
	ErrBadNotificationEndpoint = errors.New("an endpoint needs a name of 1 to 100 characters, a Discord webhook URL " +
		"(https://discord.com/api/webhooks/…), known events and feeds, a prefix of at most 20 characters, a display name of at " +
		"most 80 without \"discord\" or \"clyde\", an https picture, a role's and a thread's id of 17 to 20 digits, reminders 1 to 168 hours ahead, a colour #rrggbb in lower " +
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
	coalesce((SELECT json_agg(json_build_object('feed', s.feed_id, 'categories', s.categories, 'keywords', s.keywords, 'match', s.match, 'was_live', s.was_live)
		ORDER BY s.feed_id) FROM notification_feed_subscriptions s WHERE s.endpoint_id = notification_endpoints.endpoint_id), '[]'),
	prefix, paused, forum, username, avatar_url, role_id, thread_id, blitz_reminder_hours, colour, show_text, show_image, last_sent_at,
	coalesce(last_error, ''), last_error_at`

func scanEndpoint(row pgx.CollectableRow) (NotificationEndpoint, error) {
	var e NotificationEndpoint
	err := row.Scan(&e.ID, &e.Name, &e.URL, &e.Format, &e.Events, &e.Feeds, &e.FeedOptions, &e.Prefix, &e.Paused, &e.Forum, &e.Username, &e.AvatarURL,
		&e.RoleID, &e.ThreadID, &e.BlitzReminderHours, &e.Look.Colour, &e.Look.Text, &e.Look.Image, &e.LastSentAt, &e.LastError, &e.LastErrorAt)
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
	e.ThreadID = strings.TrimSpace(e.ThreadID)
	if e.BlitzReminderHours == 0 {
		e.BlitzReminderHours = DefaultBlitzReminderHours
	}
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
				paused, forum, username, avatar_url, role_id, colour, show_text, show_image, thread_id, blitz_reminder_hours)
			VALUES ($1, $2, $3, ARRAY(SELECT DISTINCT unnest($4::text[]) ORDER BY 1), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			RETURNING endpoint_id`,
			e.Name, e.URL, e.Format, e.Events, e.Prefix, now, e.Paused, e.Forum, e.Username, e.AvatarURL, e.RoleID,
			e.Look.Colour, e.Look.Text, e.Look.Image, e.ThreadID, e.BlitzReminderHours).Scan(&id); err != nil {
			return err
		}
		return setFeeds(ctx, tx, id, e.Feeds, e.FeedOptions)
	})
	if err != nil {
		return 0, fmt.Errorf("adding a notification endpoint: %w", badEndpoint(err))
	}
	return id, nil
}

// setFeeds makes feeds the feeds endpoint takes, each with its options from opts (none when it has no entry).
func setFeeds(ctx context.Context, tx pgx.Tx, endpoint int64, feeds []int64, opts []FeedOptions) error {
	if _, err := tx.Exec(ctx, "DELETE FROM notification_feed_subscriptions WHERE endpoint_id = $1", endpoint); err != nil {
		return err
	}
	e := NotificationEndpoint{Feeds: feeds, FeedOptions: opts}
	for _, feed := range slices.Compact(slices.Sorted(slices.Values(feeds))) {
		o, _ := e.options(feed)
		match := o.Match
		if match == "" {
			match = "all"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO notification_feed_subscriptions (endpoint_id, feed_id, categories, keywords, match, was_live)
			VALUES ($1, $2, $3, $4, $5, $6)`, endpoint, feed, cleanWords(o.Categories), cleanWords(o.Keywords), match, o.WasLive); err != nil {
			return err
		}
	}
	return nil
}

// cleanWords is words trimmed, without empty ones or repeats.
func cleanWords(words []string) []string {
	out := []string{}
	for _, w := range words {
		if w = strings.TrimSpace(w); w != "" && !slices.Contains(out, w) {
			out = append(out, w)
		}
	}
	return out
}

// SetNotificationEndpoint changes an endpoint, keeping its URL when e's is empty, or returns ErrNoNotificationEndpoint
// or ErrBadNotificationEndpoint.
func (a *AdminStore) SetNotificationEndpoint(ctx context.Context, e NotificationEndpoint) error {
	e = trimEndpoint(e)
	err := pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE notification_endpoints SET name = $2, url = coalesce(nullif($3, ''), url), format = $4,
			events = ARRAY(SELECT DISTINCT unnest($5::text[]) ORDER BY 1), prefix = $6, paused = $7, forum = $8, username = $9,
			avatar_url = $10, role_id = $11, colour = $12, show_text = $13, show_image = $14, thread_id = $15,
			blitz_reminder_hours = $16 WHERE endpoint_id = $1`,
			e.ID, e.Name, e.URL, e.Format, e.Events, e.Prefix, e.Paused, e.Forum, e.Username, e.AvatarURL, e.RoleID,
			e.Look.Colour, e.Look.Text, e.Look.Image, e.ThreadID, e.BlitzReminderHours)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNoNotificationEndpoint
		}
		return setFeeds(ctx, tx, e.ID, e.Feeds, e.FeedOptions)
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
