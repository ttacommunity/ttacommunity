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

// withBlitzNews returns a blitz_round or blitz_finished n with its round or podium read from editions and its
// link at site, and whether to post it: an edition the site does not show, a round no longer published or an
// edition no longer finished is not. Any other notification is returned as it is, to post.
func withBlitzNews(ctx context.Context, editions blitzEditions, site string, n store.Notification) (store.Notification, bool, error) {
	if n.Kind != "blitz_round" && n.Kind != "blitz_finished" {
		return n, true, nil
	}
	ed, err := editions.BlitzEdition(ctx, n.Slug, n.Edition)
	if errors.Is(err, store.ErrNoBlitzEdition) {
		return n, false, nil
	}
	if err != nil {
		return n, false, fmt.Errorf("reading a blitz edition: %w", err)
	}
	link := site + "/blitz/?t=" + url.QueryEscape(n.Slug) + "&e=" + url.QueryEscape(n.Edition)
	if n.Kind == "blitz_finished" {
		podium, ok := blitzview.Podium(ed)
		n.Blitz = store.BlitzNews{Podium: podium, Link: link}
		return n, ok, nil
	}
	news, ok := blitzview.Round(ed, n.Round)
	news.Again, news.Link = n.Blitz.Again, link
	n.Blitz = news
	return n, ok, nil
}

// editionFinished says whether an edition has its podium.
func editionFinished(ed store.BlitzEdition) bool {
	_, ok := blitzview.Podium(ed)
	return ok
}
