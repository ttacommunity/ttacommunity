// Admin's notification endpoints: the Discord webhooks notifications are posted to, and a test post to one.
// Admins only.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const onlyAdminsNotifications = "only admins manage notifications"

// endpointOut is an endpoint as the API sends it.
func endpointOut(e store.NotificationEndpoint) NotificationEndpoint {
	events := make([]NotificationEvent, len(e.Events))
	for i, ev := range e.Events {
		events[i] = NotificationEvent(ev)
	}
	return NotificationEndpoint{Id: e.ID, Name: e.Name, Url: e.URL, Format: NotificationFormat(e.Format), Events: events, Prefix: e.Prefix}
}

// endpointIn is the endpoint a setting asks for, discord when no format is given, or the refusal of an unknown
// format or event. The database checks the rest.
func endpointIn(b *NotificationEndpointSetting) (store.NotificationEndpoint, string) {
	if b == nil {
		return store.NotificationEndpoint{}, "an endpoint: name, url and events"
	}
	e := store.NotificationEndpoint{Name: b.Name, URL: b.Url, Format: string(NotificationFormatDiscord), Events: []string{}}
	if b.Format != nil {
		if !b.Format.Valid() {
			return store.NotificationEndpoint{}, "the format is discord"
		}
		e.Format = string(*b.Format)
	}
	for _, ev := range b.Events {
		if !ev.Valid() {
			return store.NotificationEndpoint{}, "events are link_request, blitz_report and blitz_flag"
		}
		e.Events = append(e.Events, string(ev))
	}
	if b.Prefix != nil {
		e.Prefix = *b.Prefix
	}
	return e, ""
}

func (s *server) GetAdminNotificationEndpoints(ctx context.Context, _ GetAdminNotificationEndpointsRequestObject) (GetAdminNotificationEndpointsResponseObject, error) {
	_, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return GetAdminNotificationEndpoints403JSONResponse{forbidden(onlyAdminsNotifications)}, nil
	}
	list, err := s.admins.NotificationEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]NotificationEndpoint, len(list))
	for i, e := range list {
		out[i] = endpointOut(e)
	}
	return GetAdminNotificationEndpoints200JSONResponse{Body: out, Headers: GetAdminNotificationEndpoints200ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PostAdminNotificationEndpoint(ctx context.Context, req PostAdminNotificationEndpointRequestObject) (PostAdminNotificationEndpointResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminNotificationEndpoint403JSONResponse{forbidden(onlyAdminsNotifications)}, nil
	}
	e, msg := endpointIn(req.Body)
	if msg != "" {
		return PostAdminNotificationEndpoint400JSONResponse{badRequest400(msg)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		var err error
		if e.ID, err = tx.AddNotificationEndpoint(ctx, e, s.now()); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "notification.endpoint.add", e.Name, req.Body)
	})
	switch {
	case errors.Is(err, store.ErrBadNotificationEndpoint):
		return PostAdminNotificationEndpoint400JSONResponse{badRequest400(store.ErrBadNotificationEndpoint.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PostAdminNotificationEndpoint201JSONResponse{Body: endpointOut(e), Headers: PostAdminNotificationEndpoint201ResponseHeaders{CacheControl: cacheNone}}, nil
}

func (s *server) PutAdminNotificationEndpoint(ctx context.Context, req PutAdminNotificationEndpointRequestObject) (PutAdminNotificationEndpointResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PutAdminNotificationEndpoint403JSONResponse{forbidden(onlyAdminsNotifications)}, nil
	}
	e, msg := endpointIn(req.Body)
	if msg != "" {
		return PutAdminNotificationEndpoint400JSONResponse{badRequest400(msg)}, nil
	}
	e.ID = req.Endpoint
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.SetNotificationEndpoint(ctx, e); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "notification.endpoint.set", fmt.Sprint(e.ID), req.Body)
	})
	switch {
	case errors.Is(err, store.ErrNoNotificationEndpoint):
		return PutAdminNotificationEndpoint404JSONResponse{notFound404("no such endpoint")}, nil
	case errors.Is(err, store.ErrBadNotificationEndpoint):
		return PutAdminNotificationEndpoint400JSONResponse{badRequest400(store.ErrBadNotificationEndpoint.Error())}, nil
	case err != nil:
		return nil, err
	}
	return PutAdminNotificationEndpoint204Response{}, nil
}

func (s *server) DeleteAdminNotificationEndpoint(ctx context.Context, req DeleteAdminNotificationEndpointRequestObject) (DeleteAdminNotificationEndpointResponseObject, error) {
	me, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return DeleteAdminNotificationEndpoint403JSONResponse{forbidden(onlyAdminsNotifications)}, nil
	}
	err = s.admins.Change(ctx, func(tx Admins) error {
		if err := tx.RemoveNotificationEndpoint(ctx, req.Endpoint); err != nil {
			return err
		}
		return tx.Audit(ctx, me.DiscordID, "notification.endpoint.remove", fmt.Sprint(req.Endpoint), map[string]any{})
	})
	switch {
	case errors.Is(err, store.ErrNoNotificationEndpoint):
		return DeleteAdminNotificationEndpoint404JSONResponse{notFound404("no such endpoint")}, nil
	case err != nil:
		return nil, err
	}
	return DeleteAdminNotificationEndpoint204Response{}, nil
}

// testOutcome says what a test post's error means for admin.
func testOutcome(err error) NotificationTest {
	var refused *notify.RefusedError
	switch {
	case err == nil:
		return NotificationTest{Delivered: true, Message: "Posted: look for the test message in the channel"}
	case errors.As(err, &refused) && refused.Status == http.StatusNotFound:
		return NotificationTest{Message: "Discord answered 404: the webhook was deleted, or the URL is wrong"}
	case errors.As(err, &refused):
		return NotificationTest{Message: fmt.Sprintf("Discord answered %d", refused.Status)}
	}
	return NotificationTest{Message: "Discord could not be reached"}
}

func (s *server) PostAdminNotificationEndpointTest(ctx context.Context, req PostAdminNotificationEndpointTestRequestObject) (PostAdminNotificationEndpointTestResponseObject, error) {
	_, ok, err := adminOnly(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return PostAdminNotificationEndpointTest403JSONResponse{forbidden(onlyAdminsNotifications)}, nil
	}
	e, err := s.admins.NotificationEndpoint(ctx, req.Endpoint)
	if errors.Is(err, store.ErrNoNotificationEndpoint) {
		return PostAdminNotificationEndpointTest404JSONResponse{notFound404("no such endpoint")}, nil
	}
	if err != nil {
		return nil, err
	}
	return PostAdminNotificationEndpointTest200JSONResponse{Body: testOutcome(s.admins.PostTestNotification(ctx, e)),
		Headers: PostAdminNotificationEndpointTest200ResponseHeaders{CacheControl: cacheNone}}, nil
}
