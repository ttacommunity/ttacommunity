// Twitch's EventSub callback: it checks our subscriptions, pushes listed streamers going live and offline, and
// revokes subscriptions, each message signed with our secret.
package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ttacommunity/backend/internal/store"
	"ttacommunity/backend/internal/twitch"
)

// TwitchHub is what the callback needs: the secret messages are signed with, a live stream's details from Twitch,
// and storing what a message tells.
type TwitchHub interface {
	Secret() []byte
	Stream(ctx context.Context, userID string) (twitch.Stream, bool, error)
	TwitchStreamOnline(ctx context.Context, message string, s store.TwitchStream, now time.Time) error
	TwitchStreamOffline(ctx context.Context, message, userID string, now time.Time) error
	TwitchSubscriptionStatus(ctx context.Context, id, status string, now time.Time) error
}

// errNotDescribed means Twitch pushed a stream going live that Helix does not show yet.
var errNotDescribed = errors.New("the stream is not described on Twitch yet")

// twitchGate lets through only messages signed with the hub's secret within minutes of now, putting the body back
// for the handler. With hub nil, Twitch is not set up here and the callback is a 404.
func twitchGate(next http.Handler, hub TwitchHub, now func() time.Time, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheNone)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if hub == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				badRequest(w, r, err)
				return
			}
			if !twitch.Verify(hub.Secret(), r.Header.Get(twitch.HeaderID), r.Header.Get(twitch.HeaderTimestamp),
				r.Header.Get(twitch.HeaderSignature), body, now()) {
				log.Info("twitch signature refused")
				writeError(w, http.StatusForbidden, "invalid request signature")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}

// routeTwitch sends Twitch's callback through twitch, without the public routes' CORS, and the rest to rest.
func routeTwitch(rest, twitchHub http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == twitch.CallbackPath {
			twitchHub.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

// ReceiveTwitchEvent echoes a verification's challenge, records a revocation, and stores a stream going live,
// with its details from Twitch, or offline. A stream Twitch cannot describe yet, or a failure to store, answers
// 500, so Twitch sends the message again; the message's id is kept only with what it told.
func (s *server) ReceiveTwitchEvent(ctx context.Context, req ReceiveTwitchEventRequestObject) (ReceiveTwitchEventResponseObject, error) {
	b, now := req.Body, s.now().UTC()
	sub := b.Subscription
	status := ""
	if sub.Status != nil {
		status = *sub.Status
	}
	if req.Params.TwitchEventsubMessageType == nil || !req.Params.TwitchEventsubMessageType.Valid() ||
		req.Params.TwitchEventsubMessageId == nil || *req.Params.TwitchEventsubMessageId == "" {
		return ReceiveTwitchEvent400JSONResponse{badRequest400("a message without a known type, or without its id")}, nil
	}
	switch *req.Params.TwitchEventsubMessageType {
	case WebhookCallbackVerification:
		if b.Challenge == nil || *b.Challenge == "" {
			return ReceiveTwitchEvent400JSONResponse{badRequest400("a verification without its challenge")}, nil
		}
		if err := s.twitch.TwitchSubscriptionStatus(ctx, sub.Id, "enabled", now); err != nil {
			return nil, err
		}
		return ReceiveTwitchEvent200TextResponse{Body: *b.Challenge, Headers: ReceiveTwitchEvent200ResponseHeaders{CacheControl: cacheNone}}, nil
	case Revocation:
		if err := s.twitch.TwitchSubscriptionStatus(ctx, sub.Id, status, now); err != nil {
			return nil, err
		}
		return ReceiveTwitchEvent204Response{}, nil
	}
	if b.Event == nil || b.Event.BroadcasterUserId == nil || sub.Type == nil {
		return ReceiveTwitchEvent204Response{}, nil
	}
	message, user := *req.Params.TwitchEventsubMessageId, *b.Event.BroadcasterUserId
	switch *sub.Type {
	case "stream.online":
		live, ok, err := s.twitch.Stream(ctx, user)
		if err == nil && !ok {
			err = errNotDescribed
		}
		if err != nil {
			return nil, err
		}
		st := store.TwitchStream{StreamID: live.ID, UserID: user, Title: live.Title, Category: live.Category,
			ThumbnailURL: live.ThumbnailURL, StartedAt: live.StartedAt}
		if b.Event.Id != nil && *b.Event.Id != "" {
			st.StreamID = *b.Event.Id
		}
		if b.Event.StartedAt != nil {
			st.StartedAt = *b.Event.StartedAt
		}
		if err := s.twitch.TwitchStreamOnline(ctx, message, st, now); err != nil {
			return nil, err
		}
	case "stream.offline":
		if err := s.twitch.TwitchStreamOffline(ctx, message, user, now); err != nil {
			return nil, err
		}
	}
	return ReceiveTwitchEvent204Response{}, nil
}
