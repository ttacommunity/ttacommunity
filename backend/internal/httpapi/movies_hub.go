// YouTube's hub's callback: it checks the subscriptions we ask for, and pushes linked channels' new, changed and
// deleted videos, signed with our hub secret.
package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ttacommunity/backend/internal/movies"
	"ttacommunity/backend/internal/store"
)

// MovieHub is what the hub's callback needs: the secret pushes are signed with, our answer to the hub's check
// of a request, a pushed video's verdict, and storing or removing what it pushes.
type MovieHub interface {
	Secret() []byte
	ConfirmMovieHub(ctx context.Context, channelID, mode string, leaseSeconds int, at time.Time) (bool, error)
	ClassifyPushed(ctx context.Context, e movies.PushedEntry) (store.PushedVideo, error)
	StorePushedVideo(ctx context.Context, v store.PushedVideo, at time.Time) error
	RemovePushedVideo(ctx context.Context, videoID, channelID string, at time.Time) error
}

const movieHubPath = "/v1/movies/hub"

// movieHubGate lets through the hub's checks, and only pushes signed with the hub's secret, putting the body
// back for the handler. With hub nil, no secret is set up here and the callback is a 404.
func movieHubGate(next http.Handler, hub MovieHub, log *slog.Logger) http.Handler {
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
			if !movies.VerifySignature(hub.Secret(), r.Header.Get("X-Hub-Signature"), body) {
				log.Info("hub signature refused")
				writeError(w, http.StatusUnauthorized, "invalid request signature")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}

// routeMovieHub sends the hub's callback through hub, without the public routes' CORS, and the rest to rest.
func routeMovieHub(rest, hub http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == movieHubPath {
			hub.ServeHTTP(w, r)
			return
		}
		rest.ServeHTTP(w, r)
	})
}

// ConfirmMovieHub echoes the hub's challenge for a request we made, and is a 404 for any other.
func (s *server) ConfirmMovieHub(ctx context.Context, req ConfirmMovieHubRequestObject) (ConfirmMovieHubResponseObject, error) {
	channel, ok := movies.TopicChannel(req.Params.HubTopic)
	if !ok {
		return ConfirmMovieHub404JSONResponse{notFound404("not found")}, nil
	}
	lease := 0
	if req.Params.HubLeaseSeconds != nil {
		lease = *req.Params.HubLeaseSeconds
	}
	confirmed, err := s.movieHub.ConfirmMovieHub(ctx, channel, req.Params.HubMode, lease, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if !confirmed {
		return ConfirmMovieHub404JSONResponse{notFound404("not found")}, nil
	}
	return ConfirmMovieHub200TextResponse{Body: req.Params.HubChallenge, Headers: ConfirmMovieHub200ResponseHeaders{CacheControl: cacheNone}}, nil
}

// ReceiveMoviePush stores each video the hub pushed for a linked channel, with its verdict, and removes each it
// says was deleted. A failure to store answers 500, so the hub sends the push again.
func (s *server) ReceiveMoviePush(ctx context.Context, req ReceiveMoviePushRequestObject) (ReceiveMoviePushResponseObject, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	push, err := movies.ParsePush(body)
	if err != nil {
		return ReceiveMoviePush400JSONResponse{badRequest400("not a push notification")}, nil
	}
	now := s.now().UTC()
	for _, e := range push.Entries {
		v, err := s.movieHub.ClassifyPushed(ctx, e)
		if err != nil {
			return nil, err
		}
		if err := s.movieHub.StorePushedVideo(ctx, v, now); err != nil && !errors.Is(err, store.ErrNoChannel) {
			return nil, err
		}
	}
	for _, d := range push.Deleted {
		if err := s.movieHub.RemovePushedVideo(ctx, d.VideoID, d.ChannelID, now); err != nil {
			return nil, err
		}
	}
	return ReceiveMoviePush204Response{}, nil
}
