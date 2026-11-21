// A blitz edition's news as it is sent: a round's games or the podium, read from the edition as the site shows
// it, with the link to its page.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"ttacommunity/backend/internal/blitzview"
	"ttacommunity/backend/internal/store"
)

// blitzEditions reads an edition as the site shows it.
type blitzEditions interface {
	BlitzEdition(ctx context.Context, slug, edition string) (store.BlitzEdition, error)
}

// withBlitzNews returns a blitz_round, blitz_reminder (with its games by match, for threads), blitz_finished, blitz_sign_ups, blitz_thread or blitz_deadline n
// with its round, its games still to play, its podium, its format and closing time, or its game's thread read from full, admin's view of the edition (its players' Discord names and ids), and its link at site,
// and whether to post it: an edition shown, the site's view, does not show, a round no longer published or with
// every game played, a game played or gone, an edition no longer finished, or sign-ups no longer taking entries is not. Any other notification is returned as it is, to post.
func withBlitzNews(ctx context.Context, shown, full blitzEditions, site string, n store.Notification) (store.Notification, bool, error) {
	if n.Kind != "blitz_round" && n.Kind != "blitz_reminder" && n.Kind != "blitz_finished" && n.Kind != "blitz_sign_ups" && n.Kind != "blitz_thread" &&
		n.Kind != "blitz_deadline" && n.Kind != "blitz_replaced" {
		return n, true, nil
	}
	_, err := shown.BlitzEdition(ctx, n.Slug, n.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return n, false, nil
	}
	if err != nil {
		return n, false, fmt.Errorf("reading a blitz edition: %w", err)
	}
	ed, err := full.BlitzEdition(ctx, n.Slug, n.Edition)
	if err != nil {
		return n, false, fmt.Errorf("reading a blitz edition for admin: %w", err)
	}
	if n.Kind == "blitz_replaced" {
		return n, true, nil
	}
	if n.Kind == "blitz_thread" || n.Kind == "blitz_deadline" {
		threads, game, ok := blitzview.GameThread(ed, n.Round, n.MatchID)
		n.Thread = store.BlitzThreadNews{Threads: threads, Game: game}
		return n, ok, nil
	}
	link := site + "/blitz/?t=" + url.QueryEscape(n.Slug) + "&e=" + url.QueryEscape(n.Edition)
	if n.Kind == "blitz_sign_ups" {
		n.Blitz = store.BlitzNews{Format: ed.Format, Closes: ed.SignUps.ClosesAt, Link: link}
		return n, ed.SignUps.Taking, nil
	}
	if n.Kind == "blitz_finished" {
		podium, ok := blitzview.Podium(ed)
		n.Blitz = store.BlitzNews{Podium: podium, Link: link}
		return n, ok, nil
	}
	read := blitzview.Round
	if n.Kind == "blitz_reminder" {
		read = blitzview.Reminder
	}
	news, ok := read(ed, n.Round)
	if n.Kind == "blitz_reminder" {
		threads, games := blitzview.ThreadsByMatch(ed, n.Round)
		n.Thread = store.BlitzThreadNews{Threads: threads, ByMatch: games}
	}
	news.Again, news.Link = n.Blitz.Again, link
	n.Blitz = news
	return n, ok, nil
}

// editionFinished says whether an edition has its podium.
func editionFinished(ed store.BlitzEdition) bool {
	_, ok := blitzview.Podium(ed)
	return ok
}
